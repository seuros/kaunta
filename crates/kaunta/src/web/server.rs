use std::{convert::Infallible, sync::Arc, time::Duration};

use rama::{
    Layer,
    graceful::Shutdown,
    http::{
        HeaderName, HeaderValue, Request, Response, StatusCode,
        layer::{
            error_handling::ErrorHandler,
            set_header::SetResponseHeaderLayer,
            trace::{DefaultMakeSpan, DefaultOnResponse, TraceLayer},
        },
        server::HttpServer,
        service::web::{
            Router,
            extract::{Path, State},
            response::{Html, IntoResponse, Json},
        },
    },
    net::address::SocketAddress,
    rt::Executor,
    tcp::server::TcpListener,
    telemetry::tracing::Level,
};
use serde_json::json;

use crate::web::{
    AppState, assets, auth, dashboard, http, ingest, pages, realtime, tracking, websites,
};

pub async fn serve(state: AppState, port: u16) -> anyhow::Result<()> {
    let graceful = Shutdown::default();
    let executor = Executor::graceful(graceful.guard());
    let listener = TcpListener::bind_address(SocketAddress::default_ipv4(port), executor.clone())
        .await
        .map_err(|error| anyhow::anyhow!("{error}"))?;
    let realtime_listener =
        tokio::spawn(realtime::listen(state.pool.clone(), state.realtime.clone()));
    let maintenance_tasks =
        crate::db::maintenance::spawn(state.pool.clone(), state.config.event_retention_days);
    let backups_dir = state.config.data_dir.join("backups");
    let backup_sweeper = tokio::spawn(async move {
        let mut ticker = tokio::time::interval(Duration::from_secs(600));
        loop {
            ticker.tick().await;
            match crate::db::backup::sweep_expired(&backups_dir, Duration::from_secs(3600)).await {
                Ok(0) => {}
                Ok(removed) => tracing::info!(removed, "swept expired backups"),
                Err(error) => tracing::warn!(?error, "backup sweep failed"),
            }
        }
    });

    graceful.spawn_task(async move {
        let version = HeaderValue::from_static(state.version);
        let mcp_enabled = state.config.mcp;
        let router = Router::new_with_state(state)
            .with_get("/", index)
            .with_get("/health", health)
            .with_get("/up", up)
            .with_get("/api/version", api_version)
            .with_get("/login", auth::login_page)
            .with_get("/dashboard", dashboard_home)
            .with_get("/dashboard/map", dashboard_map)
            .with_get("/dashboard/campaigns", dashboard_campaigns)
            .with_get("/dashboard/websites", dashboard_websites)
            .with_get("/dashboard/goals", dashboard_goals)
            .with_get("/k.js", assets::tracker)
            .with_get("/kaunta.js", assets::tracker)
            .with_get("/script.js", assets::tracker)
            .with_get("/p/{id}.gif", tracking::pixel)
            .with_get("/favicon.ico", assets::favicon)
            .with_get("/assets/datastar.js", assets::datastar)
            .with_get("/assets/datastar.js.map", assets::datastar_source_map)
            .with_get("/assets/{*path}", assets::asset)
            .with_get("/ws/realtime", realtime::websocket)
            .with_get("/api/auth/login", auth::login_sse)
            .with_post("/api/auth/login", auth::login)
            .with_post("/api/auth/logout", auth::logout)
            .with_get("/api/auth/me", auth::me)
            .with_get("/api/dashboard/whoami", websites::whoami)
            .with_get("/api/dashboard/exclusions", websites::list_exclusions)
            .with_post("/api/dashboard/exclusions", websites::add_exclusion)
            .with_delete(
                "/api/dashboard/exclusions/{exclusion}",
                websites::remove_exclusion,
            )
            .with_get("/api/websites", websites::list)
            .with_get("/api/websites/list", websites::list_details)
            .with_get("/api/websites/{website_id}", websites::show)
            .with_post("/api/websites", websites::create)
            .with_put("/api/websites/{website_id}", websites::update)
            .with_post("/api/websites/{website_id}/restore", websites::restore)
            .with_post("/api/websites/{website_id}/domains", websites::add_domain)
            .with_delete(
                "/api/websites/{website_id}/domains",
                websites::remove_domain,
            )
            .with_patch(
                "/api/websites/{website_id}/public-stats",
                websites::set_public_stats,
            )
            .with_get("/api/stats/realtime/{website_id}", websites::realtime)
            .with_options(
                "/api/public/stats/{website_id}",
                websites::public_stats_options,
            )
            .with_get("/api/public/stats/{website_id}", websites::public_stats)
            .with_get("/api/v1/stats/{website_id}", websites::api_stats)
            .with_get("/api/dashboard/init", dashboard::init)
            .with_get("/api/dashboard/stats", dashboard::stats)
            .with_get("/api/dashboard/timeseries", dashboard::timeseries)
            .with_get("/api/dashboard/chart", dashboard::timeseries)
            .with_get("/api/dashboard/breakdown", dashboard::breakdown)
            .with_get("/api/dashboard/export", dashboard::export)
            .with_get("/api/dashboard/map", dashboard::map)
            .with_get("/api/dashboard/realtime", dashboard::realtime)
            .with_get("/api/dashboard/campaigns-init", dashboard::campaigns_init)
            .with_get("/api/dashboard/campaigns", dashboard::campaigns)
            .with_get("/api/dashboard/websites-init", dashboard::websites_init)
            .with_post("/api/dashboard/websites-create", dashboard::websites_create)
            .with_get("/api/dashboard/map-init", dashboard::map_init)
            .with_get("/api/dashboard/goals", dashboard::goals)
            .with_post("/api/dashboard/goals", dashboard::create_goal)
            .with_put("/api/dashboard/goals/{id}", dashboard::update_goal)
            .with_delete("/api/dashboard/goals/{id}", dashboard::delete_goal)
            .with_get(
                "/api/dashboard/goals/{id}/analytics",
                dashboard::goal_analytics,
            )
            .with_get(
                "/api/dashboard/goals/{id}/breakdown/{type}",
                dashboard::goal_breakdown,
            )
            .with_options("/api/send", options)
            .with_post("/api/send", tracking::send)
            .with_options("/api/ingest", options)
            .with_post("/api/ingest", ingest::single)
            .with_options("/api/ingest/batch", options)
            .with_post("/api/ingest/batch", ingest::batch)
            .with_get("/backups/{name}", backup_download);

        let router = if mcp_enabled {
            router
                .with_post("/mcp", mcp_endpoint)
                .with_get("/mcp", mcp_endpoint)
                .with_delete("/mcp", mcp_endpoint)
        } else {
            router
        };

        let router = Arc::new(ErrorHandler::new(router));
        let app = SetResponseHeaderLayer::overriding(
            HeaderName::from_static("x-kaunta-version"),
            version,
        )
        .into_layer(router);
        let app = TraceLayer::new_for_http()
            .make_span_with(DefaultMakeSpan::new().with_level(Level::INFO))
            .on_response(DefaultOnResponse::new().with_level(Level::INFO))
            .into_layer(app);
        listener
            .serve(HttpServer::auto(executor).service(app))
            .await;
    });

    graceful
        .shutdown_with_limit(Duration::from_secs(30))
        .await?;
    realtime_listener.abort();
    backup_sweeper.abort();
    let _ = realtime_listener.await;
    for task in maintenance_tasks {
        task.abort();
        let _ = task.await;
    }
    Ok(())
}

/// Serve a generated backup by exact file name; the sha256 suffix in the
/// generated name is the access capability, so there is no auth and no
/// listing. Files disappear when the sweeper removes them.
async fn backup_download(State(state): State<AppState>, Path(path): Path<BackupPath>) -> Response {
    let backups_dir = state.config.data_dir.join("backups");
    let Some(file_path) = crate::db::backup::download_path(&backups_dir, &path.name) else {
        return StatusCode::NOT_FOUND.into_response();
    };
    match tokio::fs::read(&file_path).await {
        Ok(bytes) => {
            let mut response =
                crate::web::http::bytes_response(StatusCode::OK, "application/octet-stream", bytes);
            crate::web::http::set_header(
                &mut response,
                rama::http::header::CONTENT_DISPOSITION,
                &format!("attachment; filename=\"{}\"", path.name),
            );
            crate::web::http::set_header(
                &mut response,
                rama::http::header::CACHE_CONTROL,
                "no-store",
            );
            response
        }
        Err(_) => StatusCode::NOT_FOUND.into_response(),
    }
}

#[derive(Debug, serde::Deserialize)]
struct BackupPath {
    name: String,
}

/// MCP Streamable HTTP endpoint (POST JSON-RPC, GET SSE, DELETE session),
/// authenticated with kaunta API keys and scoped to the key's website.
async fn mcp_endpoint(State(state): State<AppState>, request: Request) -> Response {
    state.mcp.handle(request).await
}

async fn index(State(state): State<AppState>, request: Request) -> Response {
    let mut response = Html(pages::index(state.version).into_string()).into_response();
    http::ensure_csrf_cookie(&state, request.headers(), &mut response);
    response
}

async fn health() -> impl IntoResponse {
    Json(json!({"status": "healthy", "service": "kaunta"}))
}

async fn up(State(state): State<AppState>) -> impl IntoResponse {
    match crate::db::health::ping(&state.pool).await {
        Ok(()) => (StatusCode::OK, "OK").into_response(),
        Err(error) => {
            tracing::warn!(?error, "database health check failed");
            (StatusCode::SERVICE_UNAVAILABLE, "database unavailable").into_response()
        }
    }
}

async fn api_version(State(state): State<AppState>) -> Result<impl IntoResponse, Infallible> {
    Ok(Json(json!({"version": state.version})))
}

async fn options(request: Request) -> Response {
    http::options_response(&request)
}

async fn dashboard_home(State(state): State<AppState>, request: Request) -> Response {
    auth::protected_page(state, request, "Dashboard").await
}

async fn dashboard_map(State(state): State<AppState>, request: Request) -> Response {
    auth::protected_page(state, request, "Map").await
}

async fn dashboard_campaigns(State(state): State<AppState>, request: Request) -> Response {
    auth::protected_page(state, request, "Campaigns").await
}

async fn dashboard_websites(State(state): State<AppState>, request: Request) -> Response {
    auth::protected_page(state, request, "Websites").await
}

async fn dashboard_goals(State(state): State<AppState>, request: Request) -> Response {
    auth::protected_page(state, request, "Goals").await
}
