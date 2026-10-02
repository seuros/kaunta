use std::{
    fs,
    path::{Path, PathBuf},
    sync::{
        Arc,
        atomic::{AtomicBool, Ordering},
    },
    time::Duration,
};

use crate::domain::{
    SELF_WEBSITE_ID,
    auth::{NewUserSession, SESSION_DURATION_DAYS},
    config::Config,
};
use rama::{
    Layer,
    graceful::{Shutdown, default_signal},
    http::{
        Request, Response, StatusCode,
        body::util::BodyExt as _,
        header,
        layer::{error_handling::ErrorHandler, trace::TraceLayer},
        server::HttpServer,
        service::web::{
            Router,
            extract::State,
            response::{Html, IntoResponse},
        },
    },
    net::address::SocketAddress,
    rt::Executor,
    tcp::server::TcpListener,
};
use serde::{Deserialize, Serialize};
use serde_json::json;
use sqlx::{PgPool, types::Json};
use time::OffsetDateTime;
use tokio::sync::mpsc;
use url::Url;
use uuid::Uuid;

use crate::web::{
    assets, http,
    rate_limit::{self, AttemptLimiter},
};

const SETUP_HTML: &str = include_str!(concat!(
    env!("CARGO_MANIFEST_DIR"),
    "/../../assets/setup.html"
));
const SETUP_COMPLETE_HTML: &str = include_str!(concat!(
    env!("CARGO_MANIFEST_DIR"),
    "/../../assets/setup_complete.html"
));

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct SetupStatus {
    pub needs_setup: bool,
    pub has_database_config: bool,
    pub has_users: bool,
    pub reason: Option<String>,
}

impl SetupStatus {
    fn complete(has_database_config: bool, has_users: bool) -> Self {
        Self {
            needs_setup: false,
            has_database_config,
            has_users,
            reason: None,
        }
    }

    fn required(has_database_config: bool, has_users: bool, reason: impl Into<String>) -> Self {
        Self {
            needs_setup: true,
            has_database_config,
            has_users,
            reason: Some(reason.into()),
        }
    }
}

#[derive(Clone)]
struct SetupState {
    config_path: PathBuf,
    completion: mpsc::Sender<()>,
    completed: Arc<AtomicBool>,
    attempts: AttemptLimiter,
}

#[derive(Debug, Clone, Default, Deserialize, Serialize)]
#[serde(default)]
struct SetupForm {
    db_host: String,
    db_port: String,
    db_name: String,
    db_user: String,
    db_password: String,
    db_ssl_mode: String,
    server_port: String,
    data_dir: String,
    admin_username: String,
    admin_name: String,
    admin_password: String,
    admin_password_confirm: String,
}

#[derive(Debug, Deserialize)]
#[serde(untagged)]
enum SetupPayload {
    Wrapped { form: SetupForm },
    Direct(SetupForm),
}

impl SetupPayload {
    fn into_form(self) -> SetupForm {
        match self {
            Self::Wrapped { form } | Self::Direct(form) => form,
        }
    }
}

pub async fn check_status(config: &Config) -> SetupStatus {
    if config.security.install_lock {
        return SetupStatus::complete(!config.database_url.is_empty(), false);
    }
    if config.database_url.is_empty() {
        return SetupStatus::required(false, false, "No database configured");
    }

    let pool = match crate::db::connect(&config.database_url).await {
        Ok(pool) => pool,
        Err(error) => {
            tracing::warn!(%error, "setup status could not reach configured database");
            return SetupStatus::required(true, false, "Cannot reach database");
        }
    };
    let has_users = match crate::db::auth::has_any_users(&pool).await {
        Ok(has_users) => has_users,
        Err(error) => {
            tracing::warn!(%error, "setup status could not inspect users");
            pool.close().await;
            return SetupStatus::required(true, false, "Database not initialized");
        }
    };
    pool.close().await;

    if has_users {
        SetupStatus::complete(true, true)
    } else {
        SetupStatus::required(true, false, "No users found")
    }
}

pub async fn serve(config_path: PathBuf, port: u16) -> anyhow::Result<bool> {
    let (completion, mut completion_rx) = mpsc::channel(1);
    let completed = Arc::new(AtomicBool::new(false));
    let shutdown = Shutdown::new(async move {
        tokio::select! {
            () = default_signal() => {}
            _ = completion_rx.recv() => {}
        }
    });
    let executor = Executor::graceful(shutdown.guard());
    let listener = TcpListener::bind_address(SocketAddress::default_ipv4(port), executor.clone())
        .await
        .map_err(|error| anyhow::anyhow!("{error}"))?;
    let state = SetupState {
        config_path,
        completion,
        completed: completed.clone(),
        attempts: AttemptLimiter::default(),
    };

    tracing::info!(
        url = %format!("http://localhost:{port}/setup"),
        "setup wizard available"
    );

    shutdown.spawn_task(async move {
        let router = Router::new_with_state(state)
            .with_get("/", setup_redirect)
            .with_get("/up", up)
            .with_get("/setup", show)
            .with_post("/setup", submit)
            .with_post("/setup/test-db", test_database)
            .with_get("/setup/complete", complete)
            .with_get("/assets/datastar.js", assets::datastar)
            .with_get("/assets/datastar.js.map", assets::datastar_source_map)
            .with_get("/assets/{*path}", assets::asset)
            .with_get("/favicon.ico", assets::favicon);
        let router = Arc::new(ErrorHandler::new(router));
        let app = TraceLayer::new_for_http().into_layer(router);
        listener
            .serve(HttpServer::auto(executor).service(app))
            .await;
    });

    let _ = shutdown.shutdown_with_limit(Duration::from_secs(5)).await;
    Ok(completed.load(Ordering::Acquire))
}

async fn setup_redirect() -> Response {
    let mut response = StatusCode::FOUND.into_response();
    http::set_header(&mut response, header::LOCATION, "/setup");
    response
}

async fn up() -> impl IntoResponse {
    (StatusCode::OK, "OK")
}

async fn show(State(state): State<SetupState>) -> Response {
    if config_is_locked(&state.config_path) {
        let mut response = StatusCode::SEE_OTHER.into_response();
        http::set_header(&mut response, header::LOCATION, "/");
        return response;
    }
    Html(SETUP_HTML).into_response()
}

async fn complete() -> impl IntoResponse {
    Html(SETUP_COMPLETE_HTML)
}

async fn test_database(State(state): State<SetupState>, request: Request) -> Response {
    if let Some(response) = rate_limited(&state, &request) {
        return response;
    }
    let form = match decode_setup_payload(request).await {
        Ok(form) => form,
        Err(message) => return setup_error(message),
    };
    if form.db_host.trim().is_empty()
        || form.db_port.trim().is_empty()
        || form.db_name.trim().is_empty()
        || form.db_user.trim().is_empty()
    {
        return setup_error("Missing required database fields");
    }

    let database_url = match build_database_url(&form) {
        Ok(url) => url,
        Err(error) => return setup_error(format!("Invalid config: {error}")),
    };
    let pool = match crate::db::connect(&database_url).await {
        Ok(pool) => pool,
        Err(error) => return setup_error(format!("Connection failed: {error}")),
    };
    let version = match require_postgres_18(&pool).await {
        Ok(version) => version,
        Err(error) => {
            pool.close().await;
            return setup_error(error);
        }
    };
    pool.close().await;

    http::json_response(
        StatusCode::OK,
        json!({
            "testing": false,
            "submitting": false,
            "message": format!("Database connection successful! Version: {version}"),
            "messageType": "success",
            "version": version,
        }),
    )
}

async fn submit(State(state): State<SetupState>, request: Request) -> Response {
    if let Some(response) = rate_limited(&state, &request) {
        return response;
    }
    if config_is_locked(&state.config_path) {
        return setup_error("Setup already completed.");
    }

    let mut form = match decode_setup_payload(request).await {
        Ok(form) => form,
        Err(message) => return setup_error(message),
    };
    if let Err(error) = validate_setup_form(&mut form) {
        return setup_error(error);
    }
    let database_url = match build_database_url(&form) {
        Ok(url) => url,
        Err(error) => return setup_error(format!("Invalid database configuration: {error}")),
    };
    let pool = match crate::db::connect(&database_url).await {
        Ok(pool) => pool,
        Err(error) => return setup_error(format!("Cannot connect to database: {error}")),
    };
    if let Err(error) = require_postgres_18(&pool).await {
        pool.close().await;
        return setup_error(error);
    }
    match crate::db::auth::has_any_users(&pool).await {
        Ok(true) => {
            pool.close().await;
            return setup_error("Setup already completed. Users already exist in the database.");
        }
        Ok(false) => {}
        Err(error) => {
            tracing::debug!(%error, "users table is not available before setup migration");
        }
    }
    if let Err(error) = crate::db::migrations::run(&pool).await {
        pool.close().await;
        return setup_error(format!("Failed to initialize database: {error}"));
    }
    if let Err(error) = crate::db::startup::ensure_self_website(&pool).await {
        pool.close().await;
        return setup_error(format!("Failed to initialize self website: {error}"));
    }
    match crate::db::auth::has_any_users(&pool).await {
        Ok(true) => {
            pool.close().await;
            return setup_error("Setup already completed. Users already exist in the database.");
        }
        Ok(false) => {}
        Err(error) => {
            pool.close().await;
            return setup_error(format!("Failed to inspect users: {error}"));
        }
    }

    let result = complete_setup(&pool, &state.config_path, &form, &database_url).await;
    pool.close().await;
    let (user_id, token) = match result {
        Ok(result) => result,
        Err(error) => return setup_error(error),
    };

    let mut response = http::json_response(
        StatusCode::OK,
        json!({
            "success": true,
            "submitting": false,
            "message": "Setup completed successfully. Server is restarting...",
            "user": {
                "id": user_id,
                "username": form.admin_username,
            },
            "redirect": "/dashboard",
        }),
    );
    http::append_header(
        &mut response,
        header::SET_COOKIE,
        &http::session_cookie(&token, false, http::SESSION_SECONDS),
    );

    let completion = state.completion.clone();
    let completed = state.completed.clone();
    tokio::spawn(async move {
        tokio::time::sleep(Duration::from_millis(500)).await;
        completed.store(true, Ordering::Release);
        let _ = completion.send(()).await;
    });
    response
}

async fn complete_setup(
    pool: &PgPool,
    config_path: &Path,
    form: &SetupForm,
    database_url: &str,
) -> Result<(Uuid, String), String> {
    let config = Config {
        database_url: database_url.to_owned(),
        port: form.server_port.clone(),
        data_dir: PathBuf::from(&form.data_dir),
        secure_cookies: false,
        trusted_origins: vec!["localhost".to_owned()],
        security: crate::domain::config::SecurityConfig { install_lock: true },
        ..Config::default()
    };
    let staged_config = stage_config(config_path, &config)
        .map_err(|error| format!("Failed to save configuration: {error}"))?;

    let mut transaction = match pool.begin().await {
        Ok(transaction) => transaction,
        Err(error) => {
            remove_staged_config(&staged_config);
            return Err(format!("Failed to start setup transaction: {error}"));
        }
    };
    let user_id = match sqlx::query_scalar::<_, Uuid>(
        "INSERT INTO users (username, password_hash, name)
         VALUES ($1, hash_password($2), NULLIF($3, ''))
         RETURNING user_id",
    )
    .bind(&form.admin_username)
    .bind(&form.admin_password)
    .bind(&form.admin_name)
    .fetch_one(&mut *transaction)
    .await
    {
        Ok(user_id) => user_id,
        Err(error) => {
            remove_staged_config(&staged_config);
            return Err(format!("Failed to create admin user: {error}"));
        }
    };

    let self_id = Uuid::parse_str(SELF_WEBSITE_ID).expect("SELF_WEBSITE_ID must be a UUID");
    let allowed_domains = vec![
        "localhost".to_owned(),
        format!("localhost:{}", form.server_port),
        "http://localhost".to_owned(),
        format!("http://localhost:{}", form.server_port),
        "https://localhost".to_owned(),
        format!("https://localhost:{}", form.server_port),
    ];
    if let Err(error) = sqlx::query(
        "UPDATE website
         SET user_id = $2, allowed_domains = $3, updated_at = NOW()
         WHERE website_id = $1",
    )
    .bind(self_id)
    .bind(user_id)
    .bind(Json(allowed_domains))
    .execute(&mut *transaction)
    .await
    {
        remove_staged_config(&staged_config);
        return Err(format!("Failed to configure self website: {error}"));
    }

    let token_bytes: [u8; 32] = rand::random();
    let token = hex::encode(token_bytes);
    let session = NewUserSession {
        session_id: Uuid::new_v4(),
        user_id,
        token_hash: crate::db::auth::hash_token(&token),
        expires_at: OffsetDateTime::now_utc() + time::Duration::days(SESSION_DURATION_DAYS),
        user_agent: None,
        ip_address: None,
    };
    if let Err(error) = sqlx::query(
        "INSERT INTO user_sessions (
            session_id, user_id, token_hash, expires_at, user_agent, ip_address
         )
         VALUES ($1, $2, $3, $4, $5, $6::inet)",
    )
    .bind(session.session_id)
    .bind(session.user_id)
    .bind(&session.token_hash)
    .bind(session.expires_at)
    .bind(&session.user_agent)
    .bind(&session.ip_address)
    .execute(&mut *transaction)
    .await
    {
        remove_staged_config(&staged_config);
        return Err(format!("Failed to create admin session: {error}"));
    }

    if let Err(error) = transaction.commit().await {
        remove_staged_config(&staged_config);
        return Err(format!("Failed to commit setup: {error}"));
    }
    if let Err(error) = finalize_config(&staged_config, config_path) {
        let _ = sqlx::query("DELETE FROM users WHERE user_id = $1")
            .bind(user_id)
            .execute(pool)
            .await;
        remove_staged_config(&staged_config);
        return Err(format!("Failed to finalize configuration: {error}"));
    }

    Ok((user_id, token))
}

async fn decode_setup_payload(request: Request) -> Result<SetupForm, String> {
    let bytes = request
        .into_body()
        .collect()
        .await
        .map_err(|error| format!("Invalid request: {error}"))?
        .to_bytes();
    serde_json::from_slice::<SetupPayload>(&bytes)
        .map(SetupPayload::into_form)
        .map_err(|error| format!("Invalid request: {error}"))
}

fn validate_setup_form(form: &mut SetupForm) -> Result<(), String> {
    if form.db_port.trim().is_empty() {
        "5432".clone_into(&mut form.db_port);
    }
    if form.db_ssl_mode.trim().is_empty() {
        "disable".clone_into(&mut form.db_ssl_mode);
    }
    if form.server_port.trim().is_empty() {
        "3000".clone_into(&mut form.server_port);
    }
    if form.data_dir.trim().is_empty() {
        "./data".clone_into(&mut form.data_dir);
    }
    if form.db_host.trim().is_empty() {
        return Err("database host is required".to_owned());
    }
    if form.db_name.trim().is_empty() {
        return Err("database name is required".to_owned());
    }
    if form.db_user.trim().is_empty() {
        return Err("database user is required".to_owned());
    }
    form.db_port
        .parse::<u16>()
        .map_err(|_| "database port must be a valid TCP port".to_owned())?;
    form.server_port
        .parse::<u16>()
        .map_err(|_| "server port must be a valid TCP port".to_owned())?;
    if !matches!(
        form.db_ssl_mode.as_str(),
        "disable" | "require" | "verify-ca" | "verify-full"
    ) {
        return Err("invalid database SSL mode".to_owned());
    }
    if form.admin_username.is_empty() {
        return Err("admin username is required".to_owned());
    }
    if !(3..=30).contains(&form.admin_username.len()) {
        return Err("username must be between 3 and 30 characters".to_owned());
    }
    if !form
        .admin_username
        .chars()
        .all(|value| value.is_ascii_alphanumeric() || value == '_')
    {
        return Err("username can only contain letters, numbers, and underscores".to_owned());
    }
    if form.admin_password.is_empty() {
        return Err("admin password is required".to_owned());
    }
    if form.admin_password.len() < 8 {
        return Err("password must be at least 8 characters".to_owned());
    }
    if form.admin_password != form.admin_password_confirm {
        return Err("passwords do not match".to_owned());
    }
    Ok(())
}

fn build_database_url(form: &SetupForm) -> Result<String, String> {
    let port = if form.db_port.trim().is_empty() {
        5432
    } else {
        form.db_port
            .parse::<u16>()
            .map_err(|_| "database port must be a valid TCP port".to_owned())?
    };
    let ssl_mode = if form.db_ssl_mode.trim().is_empty() {
        "disable"
    } else {
        form.db_ssl_mode.as_str()
    };
    let mut url =
        Url::parse("postgresql://localhost").map_err(|error| format!("invalid URL: {error}"))?;
    url.set_username(&form.db_user)
        .map_err(|()| "invalid database username".to_owned())?;
    url.set_password((!form.db_password.is_empty()).then_some(form.db_password.as_str()))
        .map_err(|()| "invalid database password".to_owned())?;
    url.set_host(Some(&form.db_host))
        .map_err(|error| format!("invalid database host: {error}"))?;
    url.set_port(Some(port))
        .map_err(|()| "invalid database port".to_owned())?;
    url.set_path(&format!("/{}", form.db_name));
    url.query_pairs_mut().append_pair("sslmode", ssl_mode);
    Ok(url.into())
}

async fn require_postgres_18(pool: &PgPool) -> Result<String, String> {
    let version_number: i32 =
        sqlx::query_scalar("SELECT current_setting('server_version_num')::INTEGER")
            .fetch_one(pool)
            .await
            .map_err(|error| format!("Failed to read PostgreSQL version: {error}"))?;
    let version: String = sqlx::query_scalar("SELECT version()")
        .fetch_one(pool)
        .await
        .map_err(|error| format!("Failed to read PostgreSQL version: {error}"))?;
    if version_number < 180_000 {
        return Err(format!(
            "PostgreSQL 18 or newer is required; connected server is {version}"
        ));
    }
    Ok(version)
}

fn setup_error(message: impl Into<String>) -> Response {
    http::json_response(
        StatusCode::OK,
        json!({
            "testing": false,
            "submitting": false,
            "message": message.into(),
            "messageType": "error",
        }),
    )
}

fn rate_limited(state: &SetupState, request: &Request) -> Option<Response> {
    let key =
        http::direct_peer_ip(request).map_or_else(|| "unknown".to_owned(), |ip| ip.to_string());
    let retry_after = state.attempts.check(&key).err()?;
    let mut response = http::json_response(
        StatusCode::TOO_MANY_REQUESTS,
        json!({"error": "Too many requests, slow down."}),
    );
    http::set_header(
        &mut response,
        header::RETRY_AFTER,
        &rate_limit::retry_after_secs(retry_after).to_string(),
    );
    Some(response)
}

fn config_is_locked(path: &Path) -> bool {
    fs::read_to_string(path)
        .ok()
        .and_then(|content| toml::from_str::<Config>(&content).ok())
        .is_some_and(|config| config.security.install_lock)
}

fn stage_config(path: &Path, config: &Config) -> std::io::Result<PathBuf> {
    if let Some(parent) = path.parent()
        && !parent.as_os_str().is_empty()
    {
        fs::create_dir_all(parent)?;
    }
    let file_name = path
        .file_name()
        .and_then(|value| value.to_str())
        .unwrap_or("kaunta.toml");
    let staged = path.with_file_name(format!(".{file_name}.{}.tmp", Uuid::new_v4()));
    fs::write(
        &staged,
        toml::to_string_pretty(config).map_err(std::io::Error::other)?,
    )?;
    set_private_permissions(&staged)?;
    Ok(staged)
}

fn finalize_config(staged: &Path, destination: &Path) -> std::io::Result<()> {
    fs::rename(staged, destination)?;
    set_private_permissions(destination)
}

fn remove_staged_config(path: &Path) {
    let _ = fs::remove_file(path);
}

#[cfg(unix)]
fn set_private_permissions(path: &Path) -> std::io::Result<()> {
    use std::os::unix::fs::PermissionsExt;

    fs::set_permissions(path, fs::Permissions::from_mode(0o600))
}

#[cfg(not(unix))]
fn set_private_permissions(_path: &Path) -> std::io::Result<()> {
    Ok(())
}

#[cfg(test)]
mod tests;
