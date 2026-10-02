use std::{borrow::Cow, convert::Infallible, net::IpAddr};

use crate::domain::{
    auth::{CSRF_COOKIE, SESSION_COOKIE},
    website::ProxyMode,
};
use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};
use rama::{
    extensions::ExtensionsRef as _,
    http::{
        Body, HeaderMap, HeaderName, HeaderValue, Request, Response, StatusCode,
        body::util::BodyExt as _,
        header,
        headers::{self, HeaderMapExt as _},
        service::web::response::{IntoResponse, Json, Sse},
        sse::{self, JsonEventData},
    },
    net::stream::SocketInfo,
};
use serde::{Serialize, de::DeserializeOwned};
use serde_json::json;

use crate::web::AppState;

pub const SESSION_SECONDS: i64 = 7 * 24 * 60 * 60;

pub type DatastarEvent = sse::datastar::DatastarEvent<JsonEventData<serde_json::Value>>;

/// A full error response boxed to keep the success path's stack small.
#[derive(Debug)]
pub struct HttpError(Box<Response>);

pub type HttpResult<T> = Result<T, HttpError>;

impl From<Response> for HttpError {
    fn from(response: Response) -> Self {
        Self(Box::new(response))
    }
}

impl From<HttpError> for Response {
    fn from(error: HttpError) -> Self {
        *error.0
    }
}

impl std::ops::Deref for HttpError {
    type Target = Response;

    fn deref(&self) -> &Response {
        &self.0
    }
}

pub fn json_response<T: Serialize>(status: StatusCode, value: T) -> Response {
    (status, Json(value)).into_response()
}

pub fn error_response(status: StatusCode, message: impl Into<String>) -> Response {
    json_response(status, json!({ "error": message.into() }))
}

pub fn server_error(error: impl std::fmt::Debug, message: &'static str) -> Response {
    tracing::error!(?error, "{message}");
    error_response(StatusCode::INTERNAL_SERVER_ERROR, message)
}

pub fn datastar<E: std::fmt::Debug>(
    events: impl IntoIterator<Item = Result<DatastarEvent, E>>,
) -> Response {
    match events.into_iter().collect::<Result<Vec<_>, _>>() {
        Ok(events) => Sse::new(futures_util::stream::iter(
            events.into_iter().map(Ok::<_, Infallible>),
        ))
        .into_response(),
        Err(error) => server_error(error, "Failed to build Datastar event"),
    }
}

pub async fn decode_json<T: DeserializeOwned>(request: Request) -> HttpResult<T> {
    let bytes = request
        .into_body()
        .collect()
        .await
        .map_err(|error| {
            tracing::debug!(?error, "failed to collect request body");
            error_response(StatusCode::BAD_REQUEST, "Invalid JSON payload")
        })?
        .to_bytes();
    Ok(serde_json::from_slice(&bytes)
        .map_err(|_| error_response(StatusCode::BAD_REQUEST, "Invalid JSON payload"))?)
}

pub fn cookie(headers: &HeaderMap, name: &str) -> Option<String> {
    headers
        .typed_get::<headers::Cookie>()?
        .get(name)
        .map(str::to_owned)
}

pub fn bearer(headers: &HeaderMap) -> Option<String> {
    headers
        .get(header::AUTHORIZATION)?
        .to_str()
        .ok()?
        .strip_prefix("Bearer ")
        .filter(|value| !value.is_empty())
        .map(str::to_owned)
}

pub fn session_token(headers: &HeaderMap) -> Option<String> {
    cookie(headers, SESSION_COOKIE).or_else(|| bearer(headers))
}

pub fn api_key(headers: &HeaderMap) -> Option<String> {
    bearer(headers).or_else(|| {
        headers
            .get("x-api-key")
            .and_then(|value| value.to_str().ok())
            .filter(|value| !value.is_empty())
            .map(str::to_owned)
    })
}

pub fn header_string(headers: &HeaderMap, name: impl rama::http::header::AsHeaderName) -> String {
    headers
        .get(name)
        .and_then(|value| value.to_str().ok())
        .unwrap_or_default()
        .to_owned()
}

pub fn direct_peer_ip(request: &Request) -> Option<IpAddr> {
    request
        .extensions()
        .get_ref::<SocketInfo>()
        .map(|info| info.peer_addr().ip_addr)
}

/// Resolve which proxy mode governs a request: the website's own setting wins
/// unless it is `None`, in which case the server-wide setting applies.
#[must_use]
pub const fn effective_proxy_mode(website: ProxyMode, server: ProxyMode) -> ProxyMode {
    match website {
        ProxyMode::None => server,
        mode => mode,
    }
}

/// First comma-separated element of a header value, validated as an IP.
fn first_header_ip(headers: &HeaderMap, name: &'static str) -> Option<IpAddr> {
    header_string(headers, name)
        .split(',')
        .next()
        .and_then(|value| value.trim().parse().ok())
}

/// Client address as seen through the configured proxy layer.
///
/// Forwarding headers are spoofable, so they are only honoured when the
/// effective mode says a trusted proxy sets them; `None` uses the TCP peer.
/// Header values that do not parse as IP addresses are ignored rather than
/// stored, falling through to the next source.
#[must_use]
pub fn client_ip(request: &Request, website_mode: ProxyMode, server_mode: ProxyMode) -> String {
    let headers = request.headers();
    let forwarded = || {
        headers
            .typed_get::<headers::forwarded::XForwardedFor>()
            .and_then(|chain| chain.iter().next().copied())
            .or_else(|| first_header_ip(headers, "x-real-ip"))
    };
    let cloudflare = || first_header_ip(headers, "cf-connecting-ip");

    match effective_proxy_mode(website_mode, server_mode) {
        ProxyMode::Cloudflare => cloudflare().or_else(forwarded),
        ProxyMode::Xforwarded => forwarded(),
        ProxyMode::None => None,
    }
    .or_else(|| direct_peer_ip(request))
    .map(|ip| ip.to_string())
    .unwrap_or_else(|| "127.0.0.1".to_owned())
}

/// Client address for endpoints that have no website scope (login, limiter).
#[must_use]
pub fn server_client_ip(state: &AppState, request: &Request) -> String {
    client_ip(request, ProxyMode::None, state.config.proxy_mode)
}

pub fn session_cookie(token: &str, secure: bool, max_age: i64) -> String {
    let same_site = if secure { "None" } else { "Lax" };
    let secure_attribute = if secure { "; Secure" } else { "" };
    format!(
        "{SESSION_COOKIE}={token}; Path=/; Max-Age={max_age}; HttpOnly; SameSite={same_site}{secure_attribute}"
    )
}

pub fn csrf_cookie(token: &str, secure: bool) -> String {
    let secure_attribute = if secure { "; Secure" } else { "" };
    format!(
        "{CSRF_COOKIE}={token}; Path=/; Max-Age={SESSION_SECONDS}; SameSite=Lax{secure_attribute}"
    )
}

pub fn new_csrf_token() -> String {
    let bytes: [u8; 32] = rand::random();
    URL_SAFE_NO_PAD.encode(bytes)
}

pub fn ensure_csrf_cookie(state: &AppState, request_headers: &HeaderMap, response: &mut Response) {
    if cookie(request_headers, CSRF_COOKIE).is_none() {
        append_header(
            response,
            header::SET_COOKIE,
            &csrf_cookie(&new_csrf_token(), state.config.secure_cookies),
        );
    }
}

pub async fn csrf_valid(state: &AppState, request: &Request) -> bool {
    let Some(cookie_token) = cookie(request.headers(), CSRF_COOKIE) else {
        return false;
    };
    let request_token = header_string(request.headers(), "x-csrf-token");
    if request_token.is_empty() || request_token != cookie_token {
        return false;
    }

    let origin = header_string(request.headers(), header::ORIGIN);
    origin_trusted(state, &origin).await
}

/// Whether `origin` is trusted: an empty origin always is (non-browser
/// clients), otherwise it must match the configured list or the DB table.
pub async fn origin_trusted(state: &AppState, origin: &str) -> bool {
    if origin.is_empty() {
        return true;
    }
    if config_origin_trusted(&state.config.trusted_origins, origin) {
        return true;
    }
    crate::db::origins::is_trusted(&state.pool, origin)
        .await
        .unwrap_or(false)
}

/// Synchronous half of [`origin_trusted`]: match against the config list only.
#[must_use]
pub fn config_origin_trusted(trusted_origins: &[String], origin: &str) -> bool {
    let normalized = origin.trim().trim_end_matches('/').to_lowercase();
    trusted_origins
        .iter()
        .any(|trusted| origin_matches(trusted, &normalized))
}

fn origin_matches(trusted: &str, origin: &str) -> bool {
    let trusted = trusted.trim().trim_end_matches('/').to_lowercase();
    origin == trusted
        || origin == format!("https://{trusted}")
        || origin == format!("http://{trusted}")
}

pub fn append_header(response: &mut Response, name: HeaderName, value: &str) {
    if let Ok(value) = HeaderValue::from_str(value) {
        response.headers_mut().append(name, value);
    }
}

pub fn set_header(response: &mut Response, name: HeaderName, value: &str) {
    if let Ok(value) = HeaderValue::from_str(value) {
        response.headers_mut().insert(name, value);
    }
}

pub fn set_content_type(response: &mut Response, value: &'static str) {
    response
        .headers_mut()
        .insert(header::CONTENT_TYPE, HeaderValue::from_static(value));
}

pub fn bytes_response(
    status: StatusCode,
    content_type: &'static str,
    bytes: impl Into<Cow<'static, [u8]>>,
) -> Response {
    let mut response = Response::new(Body::from(bytes.into()));
    *response.status_mut() = status;
    set_content_type(&mut response, content_type);
    response
}

pub fn options_response(request: &Request) -> Response {
    let mut response = StatusCode::NO_CONTENT.into_response();
    let origin = header_string(request.headers(), header::ORIGIN);
    set_header(
        &mut response,
        header::ACCESS_CONTROL_ALLOW_ORIGIN,
        if origin.is_empty() { "*" } else { &origin },
    );
    if !origin.is_empty() {
        append_header(&mut response, header::VARY, "Origin");
    }
    set_header(
        &mut response,
        header::ACCESS_CONTROL_ALLOW_METHODS,
        "GET, POST, PUT, PATCH, DELETE, OPTIONS",
    );
    let requested = header_string(request.headers(), header::ACCESS_CONTROL_REQUEST_HEADERS);
    set_header(
        &mut response,
        header::ACCESS_CONTROL_ALLOW_HEADERS,
        if requested.is_empty() {
            "Accept, Authorization, Content-Type, X-CSRF-Token, X-API-Key"
        } else {
            &requested
        },
    );
    if !requested.is_empty() {
        append_header(
            &mut response,
            header::VARY,
            "Access-Control-Request-Headers",
        );
    }
    set_header(&mut response, header::ACCESS_CONTROL_MAX_AGE, "300");
    response
}

pub fn query_pairs(request: &Request) -> std::collections::HashMap<String, String> {
    request
        .uri()
        .query()
        .map(|query| {
            url::form_urlencoded::parse(query.as_encoded_str().as_bytes())
                .into_owned()
                .collect()
        })
        .unwrap_or_default()
}

#[cfg(test)]
mod tests;
