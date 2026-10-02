use crate::domain::{
    SELF_WEBSITE_ID,
    auth::SESSION_COOKIE,
    event::{EventInsert, RealtimeEvent, SessionAttributes, SessionUpsert, TrackingAccepted},
    tracking::{MAX_URL_SIZE, PayloadData, TrackingPayload},
};
use rama::http::{
    Body, Request, Response, StatusCode,
    body::util::BodyExt as _,
    header,
    service::web::extract::{Path, State},
};
use serde::Deserialize;
use serde_json::{Value, json};
use time::OffsetDateTime;
use url::Url;
use uuid::Uuid;

use crate::web::{
    AppState,
    http::{
        bytes_response, client_ip, cookie, error_response, header_string, json_response,
        query_pairs, server_error, set_header,
    },
};

const SPAM_REFERRERS: &[&str] = &[
    "semalt.com",
    "buttons-for-website.com",
    "darodar.com",
    "best-seo-offer.com",
    "free-share-buttons.com",
    "blackhatworth.com",
    "hulfingtonpost.com",
    "o-o-6-o-o.com",
    "priceg.com",
    "make-money-online",
    "simple-share-buttons.com",
    "kambasoft.com",
];

const PIXEL_GIF: &[u8] = &[
    0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0x01, 0x00, 0x01, 0x00, 0x80, 0x00, 0x00, 0xFF, 0xFF, 0xFF,
    0x00, 0x00, 0x00, 0x21, 0xF9, 0x04, 0x01, 0x00, 0x00, 0x00, 0x00, 0x2C, 0x00, 0x00, 0x00, 0x00,
    0x01, 0x00, 0x01, 0x00, 0x00, 0x02, 0x02, 0x44, 0x01, 0x00,
];

#[derive(Debug, Deserialize)]
pub struct PixelPath {
    id: String,
}

#[derive(Debug, Default)]
pub(crate) struct UrlParts {
    pub(crate) path: Option<String>,
    pub(crate) query: Option<String>,
    pub(crate) hostname: Option<String>,
}

pub async fn send(State(state): State<AppState>, request: Request) -> Response {
    let (parts, body) = request.into_parts();
    let bytes = match body.collect().await {
        Ok(body) => body.to_bytes(),
        Err(error) => {
            tracing::debug!(?error, "failed to collect tracking request");
            return error_response(StatusCode::BAD_REQUEST, "Invalid JSON payload");
        }
    };
    let Ok(payload) = serde_json::from_slice::<TrackingPayload>(&bytes) else {
        return error_response(StatusCode::BAD_REQUEST, "Invalid JSON payload");
    };
    let request = Request::from_parts(parts, Body::empty());
    process_tracking(&state, &request, payload).await
}

pub async fn pixel(
    State(state): State<AppState>,
    Path(path): Path<PixelPath>,
    request: Request,
) -> Response {
    if Uuid::parse_str(&path.id).is_ok() {
        let query = query_pairs(&request);
        let referer = nonempty(header_string(request.headers(), header::REFERER));
        let url = query.get("url").cloned().or_else(|| referer.clone());
        let hostname = query
            .get("hostname")
            .cloned()
            .or_else(|| url.as_deref().and_then(|value| parse_url(value).hostname));
        let payload = TrackingPayload {
            kind: "event".to_owned(),
            payload: PayloadData {
                website: path.id,
                hostname,
                language: nonempty(header_string(request.headers(), header::ACCEPT_LANGUAGE)),
                referrer: query.get("referrer").cloned().or(referer),
                title: query.get("title").cloned(),
                url,
                name: query.get("name").cloned(),
                tag: query.get("tag").cloned(),
                utm_source: query.get("utm_source").cloned(),
                utm_medium: query.get("utm_medium").cloned(),
                utm_campaign: query.get("utm_campaign").cloned(),
                utm_term: query.get("utm_term").cloned(),
                utm_content: query.get("utm_content").cloned(),
                ..PayloadData::default()
            },
        };
        let result = process_tracking(&state, &request, payload).await;
        if result.status().is_client_error() || result.status().is_server_error() {
            tracing::debug!(status = %result.status(), "pixel tracking failed");
        }
    } else {
        tracing::debug!(website_id = %path.id, "pixel tracking received invalid website ID");
    }

    pixel_response()
}

async fn process_tracking(
    state: &AppState,
    request: &Request,
    payload: TrackingPayload,
) -> Response {
    let Ok(website_id) = Uuid::parse_str(&payload.payload.website) else {
        return error_response(StatusCode::BAD_REQUEST, "Invalid website ID");
    };
    let Ok(website) = crate::db::websites::get_by_id(&state.pool, website_id).await else {
        return error_response(StatusCode::NOT_FOUND, "Website not found");
    };

    if website_id.to_string() == SELF_WEBSITE_ID {
        let Some(token) = cookie(request.headers(), SESSION_COOKIE) else {
            return error_response(
                StatusCode::FORBIDDEN,
                "Self-tracking requires authentication",
            );
        };
        match crate::db::auth::validate_session(&state.pool, &crate::db::auth::hash_token(&token))
            .await
        {
            Ok(Some(_)) => {}
            _ => {
                return error_response(StatusCode::FORBIDDEN, "Invalid session for self-tracking");
            }
        }
    }

    let origin = {
        let origin = header_string(request.headers(), header::ORIGIN);
        if origin.is_empty() {
            referer_origin(&header_string(request.headers(), header::REFERER))
        } else {
            origin
        }
    };
    match crate::db::websites::validate_origin(&state.pool, website_id, &origin).await {
        Ok(true) => {}
        Ok(false) => {
            return tracking_cors(
                json_response(
                    StatusCode::FORBIDDEN,
                    json!({
                        "error": "Origin not allowed",
                        "origin": origin,
                        "hint": "Add this domain to the allowed list using: kaunta website add-domain"
                    }),
                ),
                &origin,
            );
        }
        Err(error) => return server_error(error, "Origin validation failed"),
    }

    let mut ip_address = client_ip(request, website.proxy_mode, state.config.proxy_mode);
    let mut user_agent = header_string(request.headers(), header::USER_AGENT);
    if let Some(ip) = payload.payload.ip.as_ref() {
        ip_address.clone_from(ip);
    }
    if let Some(agent) = payload.payload.user_agent.as_ref() {
        user_agent.clone_from(agent);
    }

    if is_excluded(state, &ip_address).await {
        return tracking_cors(
            json_response(StatusCode::ACCEPTED, json!({"excluded": true})),
            &origin,
        );
    }

    match crate::db::events::update_ip_metadata(&state.pool, &ip_address, &user_agent, None).await {
        Ok(true) => {
            return tracking_cors(
                json_response(
                    StatusCode::ACCEPTED,
                    json!({"beep": "boop", "bot_detected": true}),
                ),
                &origin,
            );
        }
        Ok(false) => {}
        Err(error) => {
            tracing::warn!(?error, ip = %ip_address, "bot detection failed");
        }
    }

    if payload
        .payload
        .url
        .as_ref()
        .is_some_and(|value| value.len() > MAX_URL_SIZE)
    {
        return error_response(
            StatusCode::BAD_REQUEST,
            "URL too long (max 2000 characters)",
        );
    }
    if payload
        .payload
        .referrer
        .as_deref()
        .is_some_and(is_spam_referrer)
    {
        return tracking_cors(
            json_response(StatusCode::ACCEPTED, json!({"dropped": "spam_referrer"})),
            &origin,
        );
    }

    let (browser, os, device) = parse_user_agent(&user_agent);
    let (country, city, region) = state.geoip.lookup(&ip_address);
    let created_at = payload
        .payload
        .timestamp
        .and_then(|timestamp| OffsetDateTime::from_unix_timestamp(timestamp).ok())
        .unwrap_or_else(OffsetDateTime::now_utc);
    let session_id = deterministic_uuid(&[
        website_id.to_string(),
        ip_address,
        user_agent,
        hash_date(created_at, DatePeriod::Month),
    ]);
    let url_parts = payload
        .payload
        .url
        .as_deref()
        .map(parse_url)
        .unwrap_or_default();

    let session = SessionUpsert {
        session_id,
        website_id,
        created_at,
        attributes: SessionAttributes {
            browser: Some(browser.clone()),
            os: Some(os.clone()),
            device: Some(device.clone()),
            screen: payload.payload.screen.clone(),
            language: payload.payload.language.clone(),
            country: Some(country.clone()),
            region: Some(region.clone()),
            city: Some(city.clone()),
            distinct_id: payload.payload.id.clone(),
            entry_page: url_parts.path.clone(),
            exit_page: url_parts.path.clone(),
            ..SessionAttributes::default()
        },
    };
    if let Err(error) = crate::db::events::upsert_session(&state.pool, &session).await {
        return server_error(error, "Failed to create session");
    }

    if payload.kind == "event" {
        let visit_id = deterministic_uuid(&[
            session_id.to_string(),
            hash_date(created_at, DatePeriod::Hour),
        ]);
        let event_type = if payload
            .payload
            .name
            .as_deref()
            .is_some_and(|name| !name.trim().is_empty())
        {
            2
        } else {
            1
        };
        let referrer = payload
            .payload
            .referrer
            .as_deref()
            .map(parse_referrer)
            .unwrap_or_default();
        let event_id = Uuid::new_v4();
        let event = EventInsert {
            event_id,
            website_id,
            session_id,
            visit_id,
            created_at,
            url_path: url_parts.path.clone(),
            url_query: url_parts.query,
            referrer_path: referrer.path,
            referrer_query: referrer.query,
            referrer_domain: referrer.hostname,
            page_title: payload.payload.title.clone(),
            hostname: payload.payload.hostname.clone().or(url_parts.hostname),
            event_type,
            event_name: payload.payload.name.clone(),
            tag: payload.payload.tag.clone(),
            scroll_depth: payload
                .payload
                .scroll_depth
                .filter(|value| (0..=100).contains(value))
                .and_then(|value| i16::try_from(value).ok()),
            engagement_time: payload.payload.engagement_time.filter(|value| *value >= 0),
            props: combined_props(&payload.payload),
            utm_source: payload.payload.utm_source.clone(),
            utm_medium: payload.payload.utm_medium.clone(),
            utm_campaign: payload.payload.utm_campaign.clone(),
            utm_term: payload.payload.utm_term.clone(),
            utm_content: payload.payload.utm_content.clone(),
            goal_id: None,
        };
        if let Err(error) = crate::db::events::insert_event(&state.pool, &event).await {
            return server_error(error, "Failed to save event");
        }
        record_goal(
            state,
            GoalRecord {
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

        let realtime_event = RealtimeEvent {
            kind: payload.kind,
            website_id,
            session_id,
            visit_id,
            path: payload.payload.url,
            title: payload.payload.title,
            created_at,
        };
        if let Err(error) = crate::db::realtime::notify(&state.pool, &realtime_event).await {
            tracing::warn!(?error, "failed to publish realtime event");
        }

        return tracking_cors(
            json_response(
                StatusCode::ACCEPTED,
                TrackingAccepted {
                    session_id,
                    visit_id,
                },
            ),
            &origin,
        );
    }

    if payload.kind == "engagement" {
        let engagement_time = payload.payload.engagement_time.filter(|value| *value >= 0);
        let scroll_depth = payload
            .payload
            .scroll_depth
            .filter(|value| (0..=100).contains(value))
            .and_then(|value| i16::try_from(value).ok());
        if (engagement_time.is_some() || scroll_depth.is_some())
            && let Err(error) = crate::db::events::update_engagement(
                &state.pool,
                website_id,
                session_id,
                url_parts.path.as_deref().unwrap_or("/"),
                engagement_time,
                scroll_depth,
            )
            .await
        {
            tracing::warn!(?error, %session_id, "failed to update engagement");
        }
        return tracking_cors(
            json_response(StatusCode::ACCEPTED, json!({"sessionId": session_id})),
            &origin,
        );
    }

    if payload.kind == "identify" && payload.payload.data.is_some() {
        return tracking_cors(
            json_response(StatusCode::ACCEPTED, json!({"sessionId": session_id})),
            &origin,
        );
    }

    error_response(StatusCode::BAD_REQUEST, "Invalid type")
}

pub(crate) struct GoalRecord<'a> {
    pub(crate) website_id: Uuid,
    pub(crate) session_id: Uuid,
    pub(crate) event_id: Uuid,
    pub(crate) created_at: OffsetDateTime,
    pub(crate) event_type: i16,
    pub(crate) url_path: Option<&'a str>,
    pub(crate) event_name: Option<&'a str>,
}

pub(crate) async fn record_goal(state: &AppState, record: GoalRecord<'_>) {
    let GoalRecord {
        website_id,
        session_id,
        event_id,
        created_at,
        event_type,
        url_path,
        event_name,
    } = record;
    let goal_id = match crate::db::goals::match_goal(
        &state.pool,
        website_id,
        event_type,
        url_path,
        event_name,
    )
    .await
    {
        Ok(goal_id) => goal_id,
        Err(error) => {
            tracing::warn!(?error, %website_id, "failed to match goals");
            None
        }
    };
    let Some(goal_id) = goal_id else {
        return;
    };
    if let Err(error) =
        crate::db::goals::record_completion(&state.pool, goal_id, session_id, event_id, website_id)
            .await
    {
        tracing::warn!(?error, %goal_id, "failed to record goal completion");
    }
    if let Err(error) =
        crate::db::events::set_event_goal(&state.pool, event_id, created_at, goal_id).await
    {
        tracing::warn!(?error, %goal_id, %event_id, "failed to tag event with goal");
    }
}

pub(crate) fn deterministic_uuid(parts: &[String]) -> Uuid {
    Uuid::from_bytes(md5::compute(parts.join("|")).0)
}

#[derive(Clone, Copy)]
pub(crate) enum DatePeriod {
    Month,
    Hour,
}

pub(crate) fn hash_date(timestamp: OffsetDateTime, period: DatePeriod) -> String {
    let date = timestamp.date();
    let key = match period {
        DatePeriod::Month => {
            format!("{:04}-{:02}", date.year(), u8::from(date.month()))
        }
        DatePeriod::Hour => format!(
            "{:04}-{:02}-{:02}T{:02}",
            date.year(),
            u8::from(date.month()),
            date.day(),
            timestamp.hour()
        ),
    };
    format!("{:x}", md5::compute(key))
}

pub(crate) fn parse_user_agent(user_agent: &str) -> (String, String, String) {
    use rama::ua::{DeviceKind, PlatformKind, UserAgent, UserAgentKind};

    let parsed = UserAgent::new(user_agent);
    let browser = match parsed.ua_kind() {
        Some(UserAgentKind::Chromium) => {
            if user_agent.to_ascii_lowercase().contains("edg") {
                "Edge"
            } else {
                "Chrome"
            }
        }
        Some(UserAgentKind::Firefox) => "Firefox",
        Some(UserAgentKind::Safari) => "Safari",
        None => "Unknown",
    };
    let os = match parsed.platform() {
        Some(PlatformKind::Windows) => "Windows",
        Some(PlatformKind::MacOS) => "macOS",
        Some(PlatformKind::Linux) => "Linux",
        Some(PlatformKind::Android) => "Android",
        Some(PlatformKind::IOS) => "iOS",
        None => "Unknown",
    };
    let device = match parsed.device() {
        Some(DeviceKind::Mobile) => "mobile",
        Some(DeviceKind::Desktop) | None => "desktop",
    };
    (browser.to_owned(), os.to_owned(), device.to_owned())
}

pub(crate) fn parse_url(raw: &str) -> UrlParts {
    if let Ok(url) = Url::parse(raw) {
        return UrlParts {
            path: Some(decode_path(url.path())),
            query: nonempty(url.query().unwrap_or_default().to_owned()),
            hostname: url.host_str().map(str::to_owned),
        };
    }

    let (path, query) = raw.split_once('?').map_or((raw, None), |(path, query)| {
        (path, nonempty(query.to_owned()))
    });
    UrlParts {
        path: Some(decode_path(path)),
        query,
        hostname: None,
    }
}

/// Percent-decode a URL path the way Go's `url.Parse(...).Path` does, so
/// stored paths (and goal `target_url` matches) use `/café` not `/caf%C3%A9`.
/// Invalid UTF-8 sequences are replaced rather than rejected; an empty path
/// normalizes to `/`.
pub(crate) fn decode_path(path: &str) -> String {
    let decoded = percent_encoding::percent_decode_str(path).decode_utf8_lossy();
    if decoded.is_empty() {
        "/".to_owned()
    } else {
        decoded.into_owned()
    }
}

pub(crate) fn parse_referrer(raw: &str) -> UrlParts {
    let mut parts = parse_url(raw);
    parts.hostname = parts
        .hostname
        .map(|hostname| hostname.trim_start_matches("www.").to_owned())
        .filter(|hostname| hostname != "localhost" && !hostname.is_empty());
    parts
}

fn combined_props(payload: &PayloadData) -> Option<Value> {
    let mut combined = payload.props.clone().unwrap_or_default();
    if let Some(data) = &payload.data {
        combined.extend(data.clone());
    }
    (!combined.is_empty()).then(|| {
        serde_json::to_value(combined).unwrap_or_else(|_| Value::Object(serde_json::Map::new()))
    })
}

fn is_spam_referrer(referrer: &str) -> bool {
    let Ok(url) = Url::parse(referrer) else {
        return false;
    };
    let domain = url
        .host_str()
        .unwrap_or_default()
        .to_lowercase()
        .trim_start_matches("www.")
        .to_owned();
    SPAM_REFERRERS
        .iter()
        .any(|candidate| domain.contains(candidate))
}

fn referer_origin(referer: &str) -> String {
    Url::parse(referer)
        .ok()
        .map(|url| url.origin().ascii_serialization())
        .filter(|origin| origin != "null")
        .unwrap_or_else(|| referer.to_owned())
}

fn tracking_cors(mut response: Response, origin: &str) -> Response {
    set_header(
        &mut response,
        header::ACCESS_CONTROL_ALLOW_ORIGIN,
        if origin.is_empty() || origin == "null" {
            "*"
        } else {
            origin
        },
    );
    response
}

fn pixel_response() -> Response {
    let mut response = bytes_response(StatusCode::OK, "image/gif", PIXEL_GIF);
    set_header(
        &mut response,
        header::CACHE_CONTROL,
        "no-store, no-cache, must-revalidate, private",
    );
    set_header(&mut response, header::PRAGMA, "no-cache");
    set_header(&mut response, header::EXPIRES, "0");
    set_header(&mut response, header::ACCESS_CONTROL_ALLOW_ORIGIN, "*");
    set_header(
        &mut response,
        header::ACCESS_CONTROL_ALLOW_METHODS,
        "GET, OPTIONS",
    );
    response
}

fn nonempty(value: String) -> Option<String> {
    (!value.is_empty()).then_some(value)
}

#[cfg(test)]
mod tests;

/// Whether a config rule or a stored rule covers this address.
async fn is_excluded(state: &AppState, ip_address: &str) -> bool {
    let Ok(address) = ip_address.parse::<std::net::IpAddr>() else {
        return false;
    };
    if state
        .config
        .excluded_ips
        .iter()
        .any(|rule| crate::domain::config::ip_matches(rule, &address))
    {
        return true;
    }
    state
        .exclusions
        .rules(&state.pool)
        .await
        .iter()
        .any(|rule| crate::domain::config::ip_matches(rule, &address))
}
