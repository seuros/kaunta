use crate::domain::website::{
    CreateWebsiteRequest, DomainRequest, PublicStatsRequest, UpdateWebsiteRequest, Website,
};
use rama::http::{
    Request, Response, StatusCode,
    service::web::extract::{Path, State},
};
use serde::{Deserialize, Serialize};
use uuid::Uuid;

use crate::web::{
    AppState,
    auth::{authenticate_api_key, authenticate_session},
    http::{csrf_valid, decode_json, error_response, json_response, server_error},
};

#[derive(Debug, Deserialize)]
pub struct WebsitePath {
    website_id: Uuid,
}

#[derive(Debug, Serialize)]
struct WebsiteDetailResponse {
    id: Uuid,
    domain: String,
    name: String,
    allowed_domains: Vec<String>,
    public_stats_enabled: bool,
    #[serde(with = "time::serde::rfc3339")]
    created_at: time::OffsetDateTime,
}

impl From<Website> for WebsiteDetailResponse {
    fn from(website: Website) -> Self {
        Self {
            id: website.website_id,
            domain: website.domain,
            name: website.name,
            allowed_domains: website.allowed_domains,
            public_stats_enabled: website.public_stats_enabled,
            created_at: website.created_at,
        }
    }
}

async fn owned_website(
    state: &AppState,
    request: &Request,
    website_id: Uuid,
) -> crate::web::http::HttpResult<crate::domain::auth::AuthenticatedUser> {
    let user = authenticate_session(state, request.headers()).await?;
    let accessible = crate::db::websites::is_accessible_by(&state.pool, website_id, user.user_id)
        .await
        .map_err(|error| server_error(error, "Failed to verify website access"))?;
    if !accessible {
        return Err(error_response(StatusCode::NOT_FOUND, "Website not found").into());
    }
    Ok(user)
}

pub async fn list(State(state): State<AppState>, request: Request) -> Response {
    let user = match authenticate_session(&state, request.headers()).await {
        Ok(user) => user,
        Err(error) => return error.into(),
    };
    let (page, per, offset) = pagination(&crate::web::http::query_pairs(&request));
    match crate::db::websites::list_page_for_user(&state.pool, user.user_id, per, offset).await {
        Ok((websites, total)) => json_response(
            StatusCode::OK,
            paginated_websites(websites, page, per, total),
        ),
        Err(error) => server_error(error, "Failed to query websites"),
    }
}

fn pagination(query: &std::collections::HashMap<String, String>) -> (i64, i64, i64) {
    let number = |key: &str, default| {
        query
            .get(key)
            .and_then(|value| value.parse::<i64>().ok())
            .unwrap_or(default)
    };
    let page = number("page", 1).max(1);
    let per = number("per", 10).clamp(1, 100);
    (page, per, (page - 1).saturating_mul(per))
}

fn paginated_websites(
    websites: Vec<Website>,
    page: i64,
    per: i64,
    total: i64,
) -> serde_json::Value {
    let total_pages = total / per + i64::from(total % per != 0);
    let data: Vec<_> = websites
        .into_iter()
        .map(|website| {
            serde_json::json!({
                "id": website.website_id, "name": website.name, "domain": website.domain
            })
        })
        .collect();
    serde_json::json!({
        "data": data,
        "pagination": {
            "page": page, "per": per, "total": total,
            "total_pages": total_pages, "has_more": page < total_pages
        }
    })
}

pub async fn list_details(State(state): State<AppState>, request: Request) -> Response {
    let user = match authenticate_session(&state, request.headers()).await {
        Ok(user) => user,
        Err(response) => return response.into(),
    };
    match crate::db::websites::list_for_user(&state.pool, user.user_id).await {
        Ok(websites) => json_response(
            StatusCode::OK,
            websites
                .into_iter()
                .map(WebsiteDetailResponse::from)
                .collect::<Vec<_>>(),
        ),
        Err(error) => server_error(error, "Failed to query websites"),
    }
}

#[cfg(test)]
mod tests;

pub async fn show(
    State(state): State<AppState>,
    Path(path): Path<WebsitePath>,
    request: Request,
) -> Response {
    if let Err(response) = owned_website(&state, &request, path.website_id).await {
        return response.into();
    }
    match crate::db::websites::get_by_id(&state.pool, path.website_id).await {
        Ok(website) => json_response(StatusCode::OK, WebsiteDetailResponse::from(website)),
        Err(error) => error_response(StatusCode::NOT_FOUND, error.to_string()),
    }
}

pub async fn create(State(state): State<AppState>, request: Request) -> Response {
    if !csrf_valid(&state, &request).await {
        return error_response(StatusCode::FORBIDDEN, "CSRF validation failed");
    }
    let user = match authenticate_session(&state, request.headers()).await {
        Ok(user) => user,
        Err(response) => return response.into(),
    };
    let payload = match decode_json::<CreateWebsiteRequest>(request).await {
        Ok(payload) => payload,
        Err(response) => return response.into(),
    };
    if payload.domain.is_empty() {
        return error_response(StatusCode::BAD_REQUEST, "Domain is required");
    }
    let allowed = vec![
        payload.domain.clone(),
        format!("www.{}", payload.domain),
        format!("https://{}", payload.domain),
        format!("http://{}", payload.domain),
        format!("https://www.{}", payload.domain),
        format!("http://www.{}", payload.domain),
    ];
    match crate::db::websites::create(
        &state.pool,
        &payload.domain,
        &payload.name,
        &allowed,
        Some(user.user_id),
    )
    .await
    {
        Ok(website) => json_response(StatusCode::CREATED, WebsiteDetailResponse::from(website)),
        Err(error) => error_response(StatusCode::BAD_REQUEST, error.to_string()),
    }
}

pub async fn update(
    State(state): State<AppState>,
    Path(path): Path<WebsitePath>,
    request: Request,
) -> Response {
    if !csrf_valid(&state, &request).await {
        return error_response(StatusCode::FORBIDDEN, "CSRF validation failed");
    }
    if let Err(response) = owned_website(&state, &request, path.website_id).await {
        return response.into();
    }
    let payload = match decode_json::<UpdateWebsiteRequest>(request).await {
        Ok(payload) => payload,
        Err(response) => return response.into(),
    };
    match crate::db::websites::update_name(&state.pool, path.website_id, &payload.name).await {
        Ok(website) => json_response(StatusCode::OK, WebsiteDetailResponse::from(website)),
        Err(error) => error_response(StatusCode::BAD_REQUEST, error.to_string()),
    }
}

pub async fn add_domain(
    State(state): State<AppState>,
    Path(path): Path<WebsitePath>,
    request: Request,
) -> Response {
    if !csrf_valid(&state, &request).await {
        return error_response(StatusCode::FORBIDDEN, "CSRF validation failed");
    }
    if let Err(response) = owned_website(&state, &request, path.website_id).await {
        return response.into();
    }
    let payload = match decode_json::<DomainRequest>(request).await {
        Ok(payload) => payload,
        Err(response) => return response.into(),
    };
    if payload.domain.is_empty() {
        return error_response(StatusCode::BAD_REQUEST, "Domain is required");
    }
    match crate::db::websites::add_allowed_domains(&state.pool, path.website_id, &[payload.domain])
        .await
    {
        Ok(website) => json_response(StatusCode::OK, WebsiteDetailResponse::from(website)),
        Err(error) => error_response(StatusCode::BAD_REQUEST, error.to_string()),
    }
}

pub async fn remove_domain(
    State(state): State<AppState>,
    Path(path): Path<WebsitePath>,
    request: Request,
) -> Response {
    if !csrf_valid(&state, &request).await {
        return error_response(StatusCode::FORBIDDEN, "CSRF validation failed");
    }
    if let Err(response) = owned_website(&state, &request, path.website_id).await {
        return response.into();
    }
    let payload = match decode_json::<DomainRequest>(request).await {
        Ok(payload) => payload,
        Err(response) => return response.into(),
    };
    if payload.domain.is_empty() {
        return error_response(StatusCode::BAD_REQUEST, "Domain is required");
    }
    match crate::db::websites::remove_allowed_domain(&state.pool, path.website_id, &payload.domain)
        .await
    {
        Ok(website) => json_response(StatusCode::OK, WebsiteDetailResponse::from(website)),
        Err(error) => error_response(StatusCode::BAD_REQUEST, error.to_string()),
    }
}

pub async fn set_public_stats(
    State(state): State<AppState>,
    Path(path): Path<WebsitePath>,
    request: Request,
) -> Response {
    if !csrf_valid(&state, &request).await {
        return error_response(StatusCode::FORBIDDEN, "CSRF validation failed");
    }
    if let Err(response) = owned_website(&state, &request, path.website_id).await {
        return response.into();
    }
    let payload = match decode_json::<PublicStatsRequest>(request).await {
        Ok(payload) => payload,
        Err(response) => return response.into(),
    };
    match crate::db::websites::set_public_stats_enabled(
        &state.pool,
        path.website_id,
        payload.enabled,
    )
    .await
    {
        Ok(website) => json_response(StatusCode::OK, WebsiteDetailResponse::from(website)),
        Err(error) => server_error(error, "Failed to update website"),
    }
}

/// Cancels a pending deletion, keeping the website and its data.
pub async fn restore(
    State(state): State<AppState>,
    Path(path): Path<WebsitePath>,
    request: Request,
) -> Response {
    if !csrf_valid(&state, &request).await {
        return error_response(StatusCode::FORBIDDEN, "CSRF validation failed");
    }
    if let Err(response) = owned_website(&state, &request, path.website_id).await {
        return response.into();
    }
    match crate::db::websites::restore(&state.pool, path.website_id).await {
        Ok(website) => json_response(StatusCode::OK, WebsiteDetailResponse::from(website)),
        Err(error) => server_error(error, "Failed to restore website"),
    }
}

/// Public CORS headers for the unauthenticated stats endpoint. Applied to every
/// response (including 404/500) so browsers can read the error body too.
fn public_cors(mut response: Response) -> Response {
    use rama::http::header;
    crate::web::http::set_header(&mut response, header::ACCESS_CONTROL_ALLOW_ORIGIN, "*");
    crate::web::http::set_header(
        &mut response,
        header::ACCESS_CONTROL_ALLOW_METHODS,
        "GET, OPTIONS",
    );
    crate::web::http::set_header(
        &mut response,
        header::ACCESS_CONTROL_ALLOW_HEADERS,
        "Content-Type",
    );
    response
}

pub async fn public_stats_options() -> Response {
    use rama::http::service::web::response::IntoResponse as _;
    public_cors(StatusCode::NO_CONTENT.into_response())
}

pub async fn public_stats(
    State(state): State<AppState>,
    Path(path): Path<WebsitePath>,
) -> Response {
    public_cors(public_stats_inner(&state, path.website_id).await)
}

async fn public_stats_inner(state: &AppState, website_id: Uuid) -> Response {
    let Ok(website) = crate::db::websites::get_by_id(&state.pool, website_id).await else {
        return error_response(StatusCode::NOT_FOUND, "Website not found");
    };
    if !website.public_stats_enabled {
        return error_response(
            StatusCode::NOT_FOUND,
            "Public stats not enabled for this website",
        );
    }
    match crate::db::analytics::public_stats(&state.pool, website_id).await {
        Ok(public_metrics) => json_response(StatusCode::OK, public_metrics),
        Err(error) => server_error(error, "Failed to fetch stats"),
    }
}

pub async fn api_stats(
    State(state): State<AppState>,
    Path(path): Path<WebsitePath>,
    request: Request,
) -> Response {
    let key = match authenticate_api_key(&state, request.headers(), Some("stats")).await {
        Ok(key) => key,
        Err(response) => return response.into(),
    };
    if key.website_id != path.website_id {
        return error_response(
            StatusCode::FORBIDDEN,
            "API key not authorized for this website",
        );
    }
    match crate::db::websites::exists_active(&state.pool, path.website_id).await {
        Ok(true) => {}
        Ok(false) => return error_response(StatusCode::NOT_FOUND, "Website not found"),
        Err(error) => return server_error(error, "Failed to verify website"),
    }
    match crate::db::analytics::public_stats(&state.pool, path.website_id).await {
        Ok(public_metrics) => json_response(StatusCode::OK, public_metrics),
        Err(error) => server_error(error, "Failed to fetch stats"),
    }
}

pub async fn realtime(
    State(state): State<AppState>,
    Path(path): Path<WebsitePath>,
    request: Request,
) -> Response {
    if let Err(response) = owned_website(&state, &request, path.website_id).await {
        return response.into();
    }
    match crate::db::analytics::current_visitors(&state.pool, path.website_id).await {
        Ok(value) => json_response(StatusCode::OK, serde_json::json!({"value": value})),
        Err(error) => server_error(error, "Failed to query current visitors"),
    }
}

/// The address Kaunta resolves for this request, and whether it is already
/// excluded. Lets an operator on a changing address exclude the right one.
pub async fn whoami(State(state): State<AppState>, request: Request) -> Response {
    if let Err(response) = crate::web::auth::authenticate_session(&state, request.headers()).await {
        return response.into();
    }
    let ip_address = crate::web::http::server_client_ip(&state, &request);
    let excluded = match ip_address.parse::<std::net::IpAddr>() {
        Ok(address) => {
            let stored = state.exclusions.rules(&state.pool).await;
            state
                .config
                .excluded_ips
                .iter()
                .chain(stored.iter())
                .any(|rule| crate::domain::config::ip_matches(rule, &address))
        }
        Err(_) => false,
    };
    json_response(
        StatusCode::OK,
        serde_json::json!({"ip": ip_address, "excluded": excluded}),
    )
}

pub async fn list_exclusions(State(state): State<AppState>, request: Request) -> Response {
    if let Err(response) = crate::web::auth::authenticate_session(&state, request.headers()).await {
        return response.into();
    }
    match crate::db::exclusions::list(&state.pool).await {
        Ok(rules) => json_response(StatusCode::OK, serde_json::json!({"exclusions": rules})),
        Err(error) => server_error(error, "Failed to list exclusions"),
    }
}

#[derive(Debug, serde::Deserialize)]
pub struct ExclusionRequest {
    rule: String,
    #[serde(default)]
    note: String,
}

pub async fn add_exclusion(State(state): State<AppState>, request: Request) -> Response {
    if !csrf_valid(&state, &request).await {
        return error_response(StatusCode::FORBIDDEN, "CSRF validation failed");
    }
    if let Err(response) = crate::web::auth::authenticate_session(&state, request.headers()).await {
        return response.into();
    }
    let payload = match decode_json::<ExclusionRequest>(request).await {
        Ok(payload) => payload,
        Err(response) => return response.into(),
    };
    let rule = payload.rule.trim();
    if !crate::domain::config::is_ip_rule(rule) {
        return error_response(
            StatusCode::BAD_REQUEST,
            "Expected an IP address or CIDR block",
        );
    }
    match crate::db::exclusions::add(&state.pool, rule, payload.note.trim()).await {
        Ok(exclusion) => {
            state.exclusions.invalidate().await;
            json_response(StatusCode::OK, exclusion)
        }
        Err(error) => server_error(error, "Failed to add exclusion"),
    }
}

pub async fn remove_exclusion(
    State(state): State<AppState>,
    Path(path): Path<ExclusionPath>,
    request: Request,
) -> Response {
    if !csrf_valid(&state, &request).await {
        return error_response(StatusCode::FORBIDDEN, "CSRF validation failed");
    }
    if let Err(response) = crate::web::auth::authenticate_session(&state, request.headers()).await {
        return response.into();
    }
    match crate::db::exclusions::remove(&state.pool, &path.exclusion).await {
        Ok(Some(rule)) => {
            state.exclusions.invalidate().await;
            json_response(StatusCode::OK, serde_json::json!({"removed": rule}))
        }
        Ok(None) => error_response(StatusCode::NOT_FOUND, "Exclusion not found"),
        Err(error) => server_error(error, "Failed to remove exclusion"),
    }
}

#[derive(Debug, serde::Deserialize)]
pub struct ExclusionPath {
    exclusion: String,
}
