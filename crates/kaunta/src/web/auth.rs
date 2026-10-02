use crate::domain::{
    api_key::{API_KEY_PREFIX, ApiKey},
    auth::{AuthenticatedUser, LoginRequest, NewUserSession, SESSION_DURATION_DAYS},
};
use rama::http::{
    HeaderMap, Request, Response, StatusCode,
    body::util::BodyExt as _,
    header,
    service::web::{
        extract::{FromRequestBody as _, State, datastar::ReadSignals},
        response::{Html, IntoResponse},
    },
    sse::{
        JsonEventData,
        datastar::{ExecuteScript, PatchSignals},
    },
};
use rama::utils::str::non_empty_str;
use serde::Serialize;
use serde_json::json;
use time::{Duration, OffsetDateTime};
use uuid::Uuid;

use crate::web::{
    AppState,
    http::{
        SESSION_SECONDS, api_key, append_header, csrf_valid, datastar, ensure_csrf_cookie,
        error_response, json_response, server_client_ip, server_error, session_cookie,
        session_token,
    },
    pages,
};

pub async fn authenticate_session(
    state: &AppState,
    headers: &HeaderMap,
) -> crate::web::http::HttpResult<AuthenticatedUser> {
    let token = session_token(headers).ok_or_else(|| {
        error_response(
            StatusCode::UNAUTHORIZED,
            "Unauthorized - no session token provided",
        )
    })?;
    Ok(
        crate::db::auth::validate_session(&state.pool, &crate::db::auth::hash_token(&token))
            .await
            .map_err(|error| server_error(error, "Authentication error"))?
            .ok_or_else(|| {
                error_response(
                    StatusCode::UNAUTHORIZED,
                    "Unauthorized - invalid or expired session",
                )
            })?,
    )
}

pub async fn authenticate_api_key(
    state: &AppState,
    headers: &HeaderMap,
    required_scope: Option<&str>,
) -> crate::web::http::HttpResult<ApiKey> {
    let key = api_key(headers)
        .ok_or_else(|| error_response(StatusCode::UNAUTHORIZED, "Missing API key"))?;
    if !key.starts_with(API_KEY_PREFIX) {
        return Err(error_response(StatusCode::UNAUTHORIZED, "Invalid API key format").into());
    }
    let api_key =
        crate::db::api_keys::get_by_hash(&state.pool, &crate::db::api_keys::hash_api_key(&key))
            .await
            .map_err(|error| server_error(error, "Authentication error"))?
            .ok_or_else(|| error_response(StatusCode::UNAUTHORIZED, "Invalid API key"))?;
    if !api_key.is_valid_at(OffsetDateTime::now_utc()) {
        return Err(error_response(StatusCode::UNAUTHORIZED, "API key revoked or expired").into());
    }
    if let Some(scope) = required_scope
        && !api_key.has_scope(scope)
    {
        return Err(error_response(
            StatusCode::FORBIDDEN,
            format!("API key does not have {scope} permission"),
        )
        .into());
    }
    let pool = state.pool.clone();
    let key_id = api_key.key_id;
    tokio::spawn(async move {
        if let Err(error) = crate::db::api_keys::update_last_used(&pool, key_id).await {
            tracing::debug!(?error, %key_id, "failed to update API key last-used timestamp");
        }
    });
    Ok(api_key)
}

async fn create_login_session(
    state: &AppState,
    headers: &HeaderMap,
    ip_address: String,
    login: LoginRequest,
) -> crate::web::http::HttpResult<(LoginSuccess, String)> {
    if login.username.is_empty() || login.password.is_empty() {
        return Err(error_response(
            StatusCode::BAD_REQUEST,
            "Username and password are required",
        )
        .into());
    }
    let credential = crate::db::auth::find_credential(&state.pool, &login.username)
        .await
        .map_err(|error| server_error(error, "Authentication error"))?
        .ok_or_else(|| error_response(StatusCode::UNAUTHORIZED, "Invalid username or password"))?;
    if !bcrypt::verify(&login.password, &credential.password_hash).unwrap_or(false) {
        return Err(
            error_response(StatusCode::UNAUTHORIZED, "Invalid username or password").into(),
        );
    }

    let token = hex::encode(rand::random::<[u8; 32]>());
    let session_id = Uuid::new_v4();
    let expires_at = OffsetDateTime::now_utc() + Duration::days(SESSION_DURATION_DAYS);
    let mut user_agent = headers
        .get(header::USER_AGENT)
        .and_then(|value| value.to_str().ok())
        .unwrap_or_default()
        .to_owned();
    user_agent.truncate(500);
    crate::db::auth::create_session(
        &state.pool,
        &NewUserSession {
            session_id,
            user_id: credential.user.user_id,
            token_hash: crate::db::auth::hash_token(&token),
            expires_at,
            user_agent: (!user_agent.is_empty()).then_some(user_agent),
            ip_address: Some(ip_address),
        },
    )
    .await
    .map_err(|error| server_error(error, "Failed to create session"))?;

    Ok((
        LoginSuccess {
            success: true,
            message: "Login successful",
            user: LoginUser {
                user_id: credential.user.user_id,
                username: credential.user.username,
                name: credential.user.name,
            },
        },
        session_cookie(&token, state.config.secure_cookies, SESSION_SECONDS),
    ))
}

/// JSON login body matching Go: no `session_id` or timestamps are exposed.
#[derive(Debug, Serialize)]
struct LoginSuccess {
    success: bool,
    message: &'static str,
    user: LoginUser,
}

#[derive(Debug, Serialize)]
struct LoginUser {
    user_id: Uuid,
    username: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    name: Option<String>,
}

pub async fn login(State(state): State<AppState>, request: Request) -> Response {
    if let Some(response) = login_rate_limit(&state, &request) {
        return response;
    }
    if !csrf_valid(&state, &request).await {
        return error_response(StatusCode::FORBIDDEN, "CSRF validation failed");
    }
    let ip_address = server_client_ip(&state, &request);
    let headers = request.headers().clone();
    let body = request.into_body().collect().await.map_err(|error| {
        tracing::debug!(?error, "failed to collect login request");
    });
    let Ok(body) = body else {
        return error_response(StatusCode::BAD_REQUEST, "Invalid JSON payload");
    };
    let Ok(login) = serde_json::from_slice::<LoginRequest>(&body.to_bytes()) else {
        return error_response(StatusCode::BAD_REQUEST, "Invalid JSON payload");
    };
    match create_login_session(&state, &headers, ip_address, login).await {
        Ok((payload, cookie)) => {
            let mut response = json_response(StatusCode::OK, payload);
            append_header(&mut response, header::SET_COOKIE, &cookie);
            response
        }
        Err(response) => response.into(),
    }
}

pub async fn login_sse(State(state): State<AppState>, request: Request) -> Response {
    if let Some(response) = login_rate_limit(&state, &request) {
        return response;
    }
    let ip_address = server_client_ip(&state, &request);
    let (parts, body) = request.into_parts();
    let result = ReadSignals::<LoginRequest>::from_request_body(&parts, body).await;

    let (signals, cookie, redirect) = match result {
        Ok(ReadSignals(login)) if !login.username.is_empty() && !login.password.is_empty() => {
            match create_login_session(&state, &parts.headers, ip_address, login).await {
                Ok((_payload, cookie)) => {
                    (json!({"error": "", "loading": false}), Some(cookie), true)
                }
                Err(response) => {
                    let status = response.status();
                    let message = if status == StatusCode::UNAUTHORIZED {
                        "Invalid username or password"
                    } else {
                        "Authentication error"
                    };
                    (json!({"error": message, "loading": false}), None, false)
                }
            }
        }
        Ok(_) => (
            json!({"error": "Username and password are required", "loading": false}),
            None,
            false,
        ),
        Err(_) => (
            json!({"error": "Invalid request format", "loading": false}),
            None,
            false,
        ),
    };

    let mut events = vec![PatchSignals::new(JsonEventData(signals)).try_into_datastar_event()];
    if redirect {
        events.push(
            ExecuteScript::new(non_empty_str!("window.location.href = '/dashboard'"))
                .try_into_datastar_event(),
        );
    }
    let mut response = datastar(events);
    if let Some(cookie) = cookie {
        append_header(&mut response, header::SET_COOKIE, &cookie);
    }
    response
}

pub async fn logout(State(state): State<AppState>, request: Request) -> Response {
    if !csrf_valid(&state, &request).await {
        return error_response(StatusCode::FORBIDDEN, "CSRF validation failed");
    }
    let user = match authenticate_session(&state, request.headers()).await {
        Ok(user) => user,
        Err(response) => return response.into(),
    };
    let expired_cookie = session_cookie("", state.config.secure_cookies, -3600);
    if let Err(error) = crate::db::auth::delete_session(&state.pool, user.session_id).await {
        let mut response = server_error(error, "Failed to logout");
        append_header(&mut response, header::SET_COOKIE, &expired_cookie);
        return response;
    }
    let mut response = datastar([ExecuteScript::new(non_empty_str!(concat!(
        "localStorage.removeItem('kaunta_website');",
        "localStorage.removeItem('kaunta_dateRange');",
        "window.location.href = '/login'"
    )))
    .try_into_datastar_event()]);
    append_header(&mut response, header::SET_COOKIE, &expired_cookie);
    response
}

pub async fn me(State(state): State<AppState>, request: Request) -> Response {
    let user = match authenticate_session(&state, request.headers()).await {
        Ok(user) => user,
        Err(response) => return response.into(),
    };
    match crate::db::auth::get_user(&state.pool, user.user_id).await {
        Ok(Some(user)) => json_response(StatusCode::OK, user),
        Ok(None) => error_response(StatusCode::UNAUTHORIZED, "Not authenticated"),
        Err(error) => server_error(error, "Failed to get user info"),
    }
}

pub async fn login_page(State(state): State<AppState>, request: Request) -> Response {
    let mut response = Html(pages::login(state.version).into_string()).into_response();
    ensure_csrf_cookie(&state, request.headers(), &mut response);
    response
}

pub async fn protected_page(state: AppState, request: Request, title: &'static str) -> Response {
    if authenticate_session(&state, request.headers())
        .await
        .is_err()
    {
        return rama::http::service::web::response::Redirect::to("/login").into_response();
    }
    let mut response = Html(pages::dashboard(title, state.version).into_string()).into_response();
    ensure_csrf_cookie(&state, request.headers(), &mut response);
    response
}

fn login_rate_limit(state: &AppState, request: &Request) -> Option<Response> {
    let peer = server_client_ip(state, request);
    let retry_after = state.login_attempts.check(&peer).err()?;
    let mut response = json_response(
        StatusCode::TOO_MANY_REQUESTS,
        json!({
            "success": false,
            "error": "Too many login attempts. Please try again later."
        }),
    );
    crate::web::http::set_header(
        &mut response,
        header::RETRY_AFTER,
        &crate::web::rate_limit::retry_after_secs(retry_after).to_string(),
    );
    Some(response)
}
