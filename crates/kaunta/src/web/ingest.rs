use crate::domain::{
    event::{EventInsert, SessionAttributes, SessionUpsert},
    ingest::{
        BatchError, BatchIngestRequest, BatchIngestResponse, IngestPayload, IngestResponse,
        IngestValidationError, validate_batch, validate_ingest,
    },
};
use rama::http::{
    Body, Request, Response, StatusCode, body::util::BodyExt as _, header,
    service::web::extract::State,
};
use serde::de::DeserializeOwned;
use serde_json::Value;
use time::OffsetDateTime;
use uuid::Uuid;

use crate::web::{
    AppState,
    auth::authenticate_api_key,
    http::{client_ip, error_response, header_string, json_response, server_error},
    tracking::{
        DatePeriod, UrlParts, deterministic_uuid, hash_date, parse_referrer, parse_url,
        parse_user_agent, record_goal,
    },
};

pub async fn single(State(state): State<AppState>, request: Request) -> Response {
    let api_key = match authenticate_api_key(&state, request.headers(), Some("ingest")).await {
        Ok(api_key) => api_key,
        Err(response) => return response.into(),
    };
    let (payload, request) = match decode_preserving_request::<IngestPayload>(request).await {
        Ok(result) => result,
        Err(response) => return response.into(),
    };
    let now = OffsetDateTime::now_utc();
    if let Err(error) = validate_ingest(&payload, now) {
        return error_response(StatusCode::BAD_REQUEST, error.to_string());
    }

    if let Some(event_id) = parse_event_id(&payload) {
        match crate::db::api_keys::event_id_exists(&state.pool, event_id, api_key.website_id).await
        {
            Ok(true) => {
                return json_response(
                    StatusCode::ACCEPTED,
                    IngestResponse {
                        status: "accepted".to_owned(),
                        session_id: None,
                        visit_id: None,
                        idempotent: Some(true),
                        bot_detected: None,
                    },
                );
            }
            Ok(false) => {}
            Err(error) => tracing::warn!(?error, "idempotency check failed"),
        }
    }

    match process_event(&state, &request, api_key.website_id, &payload, now).await {
        Ok(response) => json_response(StatusCode::ACCEPTED, response),
        Err(error) => server_error(error, "Failed to process event"),
    }
}

pub async fn batch(State(state): State<AppState>, request: Request) -> Response {
    let api_key = match authenticate_api_key(&state, request.headers(), Some("ingest")).await {
        Ok(api_key) => api_key,
        Err(response) => return response.into(),
    };
    let (batch, request) = match decode_preserving_request::<BatchIngestRequest>(request).await {
        Ok(result) => result,
        Err(response) => return response.into(),
    };
    if let Err(error) = validate_batch(&batch) {
        let message = match error {
            IngestValidationError::EmptyBatch => "Events array is required".to_owned(),
            IngestValidationError::BatchTooLarge => "Maximum 100 events per batch".to_owned(),
            other => other.to_string(),
        };
        return error_response(StatusCode::BAD_REQUEST, message);
    }

    let now = OffsetDateTime::now_utc();
    let mut response = BatchIngestResponse::default();
    for (index, payload) in batch.events.iter().enumerate() {
        if let Err(error) = validate_ingest(payload, now) {
            response.failed += 1;
            response.errors.push(BatchError {
                index,
                error: error.to_string(),
            });
            continue;
        }

        if let Some(event_id) = parse_event_id(payload)
            && crate::db::api_keys::event_id_exists(&state.pool, event_id, api_key.website_id)
                .await
                .unwrap_or(false)
        {
            response.accepted += 1;
            continue;
        }

        match process_event(&state, &request, api_key.website_id, payload, now).await {
            Ok(_) => response.accepted += 1,
            Err(error) => {
                tracing::warn!(?error, index, "batch ingest event failed");
                response.failed += 1;
                response.errors.push(BatchError {
                    index,
                    error: "processing failed".to_owned(),
                });
            }
        }
    }

    json_response(StatusCode::ACCEPTED, response)
}

async fn process_event(
    state: &AppState,
    request: &Request,
    website_id: Uuid,
    payload: &IngestPayload,
    now: OffsetDateTime,
) -> anyhow::Result<IngestResponse> {
    let website = crate::db::websites::get_by_id(&state.pool, website_id).await?;
    let ip_address = client_ip(request, website.proxy_mode, state.config.proxy_mode);
    let user_agent = header_string(request.headers(), header::USER_AGENT);
    if crate::db::events::update_ip_metadata(&state.pool, &ip_address, &user_agent, None)
        .await
        .unwrap_or(false)
    {
        return Ok(IngestResponse {
            status: "accepted".to_owned(),
            session_id: None,
            visit_id: None,
            idempotent: None,
            bot_detected: Some(true),
        });
    }

    let (browser, os, device) = parse_user_agent(&user_agent);
    let (country, city, region) = state.geoip.lookup(&ip_address);
    let created_at = payload
        .timestamp
        .and_then(|timestamp| OffsetDateTime::from_unix_timestamp(timestamp).ok())
        .unwrap_or(now);
    let session_id = resolve_session_id(payload, website_id, created_at);
    let url_parts = if payload.url.is_empty() {
        UrlParts::default()
    } else {
        parse_url(&payload.url)
    };
    let context = payload.context.as_ref();

    crate::db::events::upsert_session(
        &state.pool,
        &SessionUpsert {
            session_id,
            website_id,
            created_at,
            attributes: SessionAttributes {
                browser: Some(browser),
                os: Some(os),
                device: Some(device),
                screen: context
                    .map(|context| context.screen.clone())
                    .filter(|value| !value.is_empty()),
                language: context
                    .map(|context| context.locale.clone())
                    .filter(|value| !value.is_empty()),
                country: Some(country),
                region: Some(region),
                city: Some(city),
                distinct_id: payload.user_id.clone(),
                entry_page: url_parts.path.clone(),
                exit_page: url_parts.path.clone(),
                ..SessionAttributes::default()
            },
        },
    )
    .await?;

    let visit_id = deterministic_uuid(&[
        session_id.to_string(),
        hash_date(created_at, DatePeriod::Hour),
    ]);
    let event_type = if payload.event == "page_view" { 1 } else { 2 };
    let event_name = (event_type == 2).then(|| payload.event.clone());
    let referrer = if payload.referrer.is_empty() {
        UrlParts::default()
    } else {
        parse_referrer(&payload.referrer)
    };
    let event_id = Uuid::new_v4();
    let event = EventInsert {
        event_id,
        website_id,
        session_id,
        visit_id,
        created_at,
        url_path: url_parts.path,
        url_query: url_parts.query,
        referrer_path: referrer.path,
        referrer_query: referrer.query,
        referrer_domain: referrer.hostname,
        page_title: (!payload.title.is_empty()).then(|| payload.title.clone()),
        hostname: (!payload.hostname.is_empty())
            .then(|| payload.hostname.clone())
            .or(url_parts.hostname),
        event_type,
        event_name,
        tag: None,
        scroll_depth: None,
        engagement_time: None,
        props: (!payload.properties.is_empty())
            .then(|| serde_json::to_value(&payload.properties).unwrap_or(Value::Null)),
        utm_source: payload.utm_source.clone(),
        utm_medium: payload.utm_medium.clone(),
        utm_campaign: payload.utm_campaign.clone(),
        utm_term: payload.utm_term.clone(),
        utm_content: payload.utm_content.clone(),
        goal_id: None,
    };
    crate::db::events::insert_event(&state.pool, &event).await?;

    if let Some(idempotency_id) = parse_event_id(payload)
        && let Err(error) =
            crate::db::api_keys::record_event_id(&state.pool, idempotency_id, website_id).await
    {
        tracing::warn!(?error, "failed to record ingest idempotency key");
    }

    record_goal(
        state,
        crate::web::tracking::GoalRecord {
            website_id,
            session_id,
            event_id,
            created_at,
            event_type,
            url_path: event.url_path.as_deref(),
            event_name: event.event_name.as_deref(),
        },
    )
    .await;

    Ok(IngestResponse {
        status: "accepted".to_owned(),
        session_id: Some(session_id),
        visit_id: Some(visit_id),
        idempotent: None,
        bot_detected: None,
    })
}

fn resolve_session_id(
    payload: &IngestPayload,
    website_id: Uuid,
    created_at: OffsetDateTime,
) -> Uuid {
    if let Some(session_id) = payload
        .session_id
        .as_deref()
        .filter(|session_id| !session_id.is_empty())
    {
        return Uuid::parse_str(session_id).unwrap_or_else(|_| {
            deterministic_uuid(&[website_id.to_string(), session_id.to_owned()])
        });
    }
    deterministic_uuid(&[
        website_id.to_string(),
        payload.visitor_id.clone(),
        hash_date(created_at, DatePeriod::Hour),
    ])
}

fn parse_event_id(payload: &IngestPayload) -> Option<Uuid> {
    payload
        .event_id
        .as_deref()
        .and_then(|event_id| Uuid::parse_str(event_id).ok())
}

async fn decode_preserving_request<T: DeserializeOwned>(
    request: Request,
) -> crate::web::http::HttpResult<(T, Request)> {
    let (parts, body) = request.into_parts();
    let bytes = body
        .collect()
        .await
        .map_err(|error| {
            tracing::debug!(?error, "failed to collect ingest request");
            error_response(StatusCode::BAD_REQUEST, "Invalid JSON payload")
        })?
        .to_bytes();
    let payload = serde_json::from_slice(&bytes)
        .map_err(|_| error_response(StatusCode::BAD_REQUEST, "Invalid JSON payload"))?;
    Ok((payload, Request::from_parts(parts, Body::empty())))
}

#[cfg(test)]
mod tests;
