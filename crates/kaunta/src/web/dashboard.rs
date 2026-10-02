use crate::db::analytics::AnalyticsFilters;
use crate::domain::goal::{GoalKind, GoalRequest};
use rama::error::BoxError;
use rama::http::{
    Request, Response, StatusCode,
    body::util::BodyExt as _,
    header,
    service::web::extract::{Path, State},
    sse::{
        JsonEventData,
        datastar::{PatchElements, PatchSignals},
    },
};
use serde::{Deserialize, Serialize};
use serde_json::{Value, json};
use time::OffsetDateTime;
use uuid::Uuid;

use crate::web::{
    AppState,
    auth::authenticate_session,
    http::{
        DatastarEvent, bytes_response, csrf_valid, datastar, error_response, json_response,
        query_pairs, server_error, set_header,
    },
};

#[derive(Debug, Deserialize)]
pub struct GoalPath {
    id: Uuid,
}

#[derive(Debug, Deserialize)]
pub struct GoalBreakdownPath {
    id: Uuid,
    r#type: String,
}

#[derive(Debug, Serialize)]
struct WebsiteInfo<'a> {
    id: Uuid,
    name: &'a str,
    domain: &'a str,
}

#[derive(Debug, Serialize)]
struct WebsiteCard<'a> {
    id: Uuid,
    domain: &'a str,
    name: &'a str,
    allowed_domains: &'a [String],
    public_stats_enabled: bool,
    /// Days left before the deletion policy removes this website, when one
    /// is pending. `None` for live websites.
    #[serde(skip_serializing_if = "Option::is_none")]
    deletion_in_days: Option<i64>,
}

#[derive(Debug)]
enum WebsiteIdError {
    Missing,
    Invalid,
}

impl WebsiteIdError {
    fn into_response(self) -> Response {
        let message = match self {
            Self::Missing => "Website ID is required",
            Self::Invalid => "Invalid website ID",
        };
        error_response(StatusCode::BAD_REQUEST, message)
    }
}

fn sse(signals: Value) -> Response {
    datastar([PatchSignals::new(JsonEventData(signals)).try_into_datastar_event()])
}

async fn user_and_query(
    state: &AppState,
    request: &Request,
) -> crate::web::http::HttpResult<(
    crate::domain::auth::AuthenticatedUser,
    std::collections::HashMap<String, String>,
)> {
    let user = authenticate_session(state, request.headers()).await?;
    Ok((user, query_pairs(request)))
}

fn selected_website(query: &std::collections::HashMap<String, String>) -> Option<String> {
    query
        .get("website_id")
        .or_else(|| query.get("website"))
        .or_else(|| query.get("selectedWebsite"))
        .map(String::as_str)
        .filter(|value| !value.is_empty())
        .map(str::to_owned)
        .or_else(|| {
            query
                .get("datastar")
                .and_then(|value| serde_json::from_str::<Value>(value).ok())
                .and_then(|signals| {
                    signals
                        .get("selectedWebsite")
                        .and_then(Value::as_str)
                        .map(str::to_owned)
                })
        })
}

fn website_id(query: &std::collections::HashMap<String, String>) -> Result<Uuid, WebsiteIdError> {
    selected_website(query)
        .ok_or(WebsiteIdError::Missing)
        .and_then(|value| Uuid::parse_str(&value).map_err(|_| WebsiteIdError::Invalid))
}

fn optional<'a>(
    query: &'a std::collections::HashMap<String, String>,
    key: &str,
) -> Option<&'a str> {
    query
        .get(key)
        .map(String::as_str)
        .filter(|value| !value.is_empty())
}

fn filters<'a>(query: &'a std::collections::HashMap<String, String>) -> AnalyticsFilters<'a> {
    AnalyticsFilters {
        country: optional(query, "country"),
        browser: optional(query, "browser"),
        device: optional(query, "device"),
        page: optional(query, "page"),
    }
}

/// Grace period the `website_delete_policy` trigger enforces before a
/// pending deletion completes.
const PENDING_DELETE_DAYS: i64 = 30;
/// Sort columns accepted by `get_breakdown`; the first entry is the default.
const BREAKDOWN_SORT_COLUMNS: &[&str] = &["count", "name"];
/// Sort columns accepted by `get_top_pages`; the first entry is the default.
const PAGES_SORT_COLUMNS: &[&str] = &["views", "path", "unique_visitors", "avg_engagement_time"];

/// Breakdown type in Go's precedence: datastar `activeTab`, then `type`,
/// then `tab`, defaulting to `pages`.
fn breakdown_type(query: &std::collections::HashMap<String, String>) -> String {
    query
        .get("datastar")
        .and_then(|value| serde_json::from_str::<Value>(value).ok())
        .and_then(|signals| {
            signals
                .get("activeTab")
                .and_then(Value::as_str)
                .filter(|value| !value.is_empty())
                .map(str::to_owned)
        })
        .or_else(|| optional(query, "type").map(str::to_owned))
        .or_else(|| optional(query, "tab").map(str::to_owned))
        .unwrap_or_else(|| "pages".to_owned())
}

/// Map both Go's plural/hyphenated vocabulary and the Rust singular names to
/// the SQL dimension names accepted by `get_breakdown`. `page` is returned
/// for the pages tab, which the handler routes to `get_top_pages`.
pub(crate) fn normalize_dimension(raw: &str) -> Option<&'static str> {
    Some(match raw.trim().to_ascii_lowercase().as_str() {
        "pages" | "page" => "page",
        "referrers" | "referrer" => "referrer",
        "browsers" | "browser" => "browser",
        "devices" | "device" => "device",
        "countries" | "country" => "country",
        "cities" | "city" => "city",
        "regions" | "region" => "region",
        "os" => "os",
        "utm_source" => "utm_source",
        "utm_medium" => "utm_medium",
        "utm_campaign" => "utm_campaign",
        "utm_term" => "utm_term",
        "utm_content" => "utm_content",
        "entry_page" | "entry-pages" | "entry_pages" => "entry_page",
        "exit_page" | "exit-pages" | "exit_pages" => "exit_page",
        "source" | "sources" => "source",
        "channel" | "channels" => "channel",
        "event" | "events" => "event",
        _ => return None,
    })
}

/// Goal breakdown dimensions accepted by `get_goal_breakdown`, plus the
/// `page`/`pages` alias routed to `get_goal_converting_pages`.
pub(crate) fn normalize_goal_dimension(raw: &str) -> Option<&'static str> {
    match normalize_dimension(raw)? {
        dimension @ ("page" | "referrer" | "country" | "browser" | "device" | "os") => {
            Some(dimension)
        }
        _ => None,
    }
}

/// `sort_by` restricted to `allowed` (case-insensitive); unknown values fall
/// back to the first entry, matching Go's `ParsePaginationParamsWithValidation`.
fn sort_column(
    query: &std::collections::HashMap<String, String>,
    allowed: &[&'static str],
) -> &'static str {
    let requested = optional(query, "sort_by").map(str::to_ascii_lowercase);
    allowed
        .iter()
        .copied()
        .find(|candidate| requested.as_deref() == Some(*candidate))
        .or_else(|| allowed.first().copied())
        .unwrap_or("count")
}

/// `sort_order` restricted to `asc`/`desc`, defaulting to `desc`.
fn sort_direction(query: &std::collections::HashMap<String, String>) -> &'static str {
    match optional(query, "sort_order")
        .map(str::to_ascii_lowercase)
        .as_deref()
    {
        Some("asc") => "asc",
        _ => "desc",
    }
}

fn int_param(
    query: &std::collections::HashMap<String, String>,
    key: &str,
    default: i32,
    maximum: i32,
) -> i32 {
    query
        .get(key)
        .and_then(|value| value.parse().ok())
        .unwrap_or(default)
        .clamp(1, maximum)
}

/// Shared preamble of the per-website dashboard endpoints: authenticate,
/// extract the query, and verify the requested website is accessible.
async fn authorized_website_query(
    state: &AppState,
    request: &Request,
) -> crate::web::http::HttpResult<(std::collections::HashMap<String, String>, Uuid)> {
    let (user, query) = user_and_query(state, request).await?;
    let website_id = website_id(&query)
        .map_err(|error| crate::web::http::HttpError::from(error.into_response()))?;
    verify_owned(state, user.user_id, website_id).await?;
    Ok((query, website_id))
}

async fn verify_owned(
    state: &AppState,
    user_id: Uuid,
    website_id: Uuid,
) -> crate::web::http::HttpResult<()> {
    match crate::db::websites::is_accessible_by(&state.pool, website_id, user_id).await {
        Ok(true) => Ok(()),
        Ok(false) => Err(error_response(StatusCode::NOT_FOUND, "Website not found").into()),
        Err(error) => Err(server_error(error, "Failed to verify website access").into()),
    }
}

async fn owned_goal(
    state: &AppState,
    user_id: Uuid,
    goal_id: Uuid,
) -> crate::web::http::HttpResult<crate::domain::goal::Goal> {
    let goal = crate::db::goals::get(&state.pool, goal_id)
        .await
        .map_err(|error| server_error(error, "Failed to load goal"))?
        .ok_or_else(|| error_response(StatusCode::NOT_FOUND, "Goal not found"))?;
    verify_owned(state, user_id, goal.website_id).await?;
    Ok(goal)
}

fn website_infos(websites: &[crate::domain::website::Website]) -> Vec<WebsiteInfo<'_>> {
    websites
        .iter()
        .map(|website| WebsiteInfo {
            id: website.website_id,
            name: &website.name,
            domain: &website.domain,
        })
        .collect()
}

fn selected_website_id(
    query: &std::collections::HashMap<String, String>,
    websites: &[crate::domain::website::Website],
) -> Option<Uuid> {
    selected_website(query)
        .and_then(|value| Uuid::parse_str(&value).ok())
        .filter(|candidate| {
            websites
                .iter()
                .any(|website| website.website_id == *candidate)
        })
        .or_else(|| websites.first().map(|website| website.website_id))
}

pub async fn init(State(state): State<AppState>, request: Request) -> Response {
    let (user, query) = match user_and_query(&state, &request).await {
        Ok(result) => result,
        Err(response) => return response.into(),
    };
    let websites = match crate::db::websites::list_for_user(&state.pool, user.user_id).await {
        Ok(websites) => websites,
        Err(error) => return server_error(error, "Failed to load websites"),
    };
    let selected = selected_website_id(&query, &websites);
    let dashboard_stats = if let Some(website_id) = selected {
        crate::db::analytics::dashboard_stats(
            &state.pool,
            website_id,
            int_param(&query, "days", 1, 90),
            AnalyticsFilters::default(),
        )
        .await
        .ok()
    } else {
        None
    };
    sse(json!({
        "selectedWebsite": selected.map(|value| value.to_string()).unwrap_or_default(),
        "_websites": website_infos(&websites),
        "websitesLoading": false,
        "websitesError": false,
        "stats": dashboard_stats.unwrap_or(crate::domain::analytics::DashboardStats {
            current_visitors: 0,
            today_pageviews: 0,
            today_visitors: 0,
            today_bounce_rate: "0%".to_owned(),
        })
    }))
}

pub async fn stats(State(state): State<AppState>, request: Request) -> Response {
    let (query, website_id) = match authorized_website_query(&state, &request).await {
        Ok(parts) => parts,
        Err(response) => return response.into(),
    };
    let days = int_param(&query, "days", 1, 90);
    let overview =
        crate::db::analytics::period_overview(&state.pool, website_id, days, filters(&query)).await;
    match crate::db::analytics::dashboard_stats(&state.pool, website_id, days, filters(&query))
        .await
    {
        Ok(dashboard_stats) => {
            let overview = match overview {
                Ok(overview) => serde_json::to_value(overview).unwrap_or(Value::Null),
                Err(error) => {
                    tracing::error!(?error, "period overview query failed");
                    Value::Null
                }
            };
            sse(json!({
                "stats": dashboard_stats,
                "overview": overview,
                "statsLoading": false,
                "statsError": false
            }))
        }
        Err(error) => {
            tracing::error!(?error, "dashboard stats query failed");
            sse(json!({
                "stats": {
                    "current_visitors": 0,
                    "today_pageviews": 0,
                    "today_visitors": 0,
                    "today_bounce_rate": "0%"
                },
                "statsLoading": false,
                "statsError": "Failed to load statistics"
            }))
        }
    }
}

pub async fn timeseries(State(state): State<AppState>, request: Request) -> Response {
    let (query, website_id) = match authorized_website_query(&state, &request).await {
        Ok(parts) => parts,
        Err(response) => return response.into(),
    };
    match crate::db::analytics::timeseries(
        &state.pool,
        website_id,
        int_param(&query, "days", 7, 90),
        filters(&query),
    )
    .await
    {
        Ok(points) => {
            sse(json!({"_timeseries": points, "chartLoading": false, "chartError": false}))
        }
        Err(error) => {
            tracing::error!(?error, "timeseries query failed");
            sse(
                json!({"_timeseries": [], "chartLoading": false, "chartError": "Failed to load chart"}),
            )
        }
    }
}

pub async fn breakdown(State(state): State<AppState>, request: Request) -> Response {
    let (query, website_id) = match authorized_website_query(&state, &request).await {
        Ok(parts) => parts,
        Err(response) => return response.into(),
    };
    let requested = breakdown_type(&query);
    let Some(dimension) = normalize_dimension(&requested) else {
        return sse(json!({
            "_breakdown": {"items": [], "total": 0},
            "breakdownLoading": false,
            "breakdownError": "Invalid breakdown type"
        }));
    };
    let page = int_param(&query, "page_number", 1, 10_000);
    let per_page = int_param(&query, "per", 10, 100);
    let offset = (page - 1) * per_page;
    let sort_order = sort_direction(&query);
    let result = if dimension == "page" {
        crate::db::analytics::top_pages(
            &state.pool,
            website_id,
            int_param(&query, "days", 7, 90),
            per_page,
            offset,
            filters(&query),
            sort_column(&query, PAGES_SORT_COLUMNS),
            sort_order,
        )
        .await
        .map(|(items, total)| json!({"items": items, "total": total}))
    } else if dimension == "event" {
        crate::db::analytics::event_breakdown(
            &state.pool,
            website_id,
            int_param(&query, "days", 7, 90),
            per_page,
            offset,
            filters(&query),
            sort_column(&query, BREAKDOWN_SORT_COLUMNS),
            sort_order,
        )
        .await
        .map(|(items, total)| json!({"items": items, "total": total}))
    } else if dimension == "source" || dimension == "channel" {
        crate::db::analytics::acquisition_breakdown(
            &state.pool,
            website_id,
            dimension,
            int_param(&query, "days", 7, 90),
            per_page,
            offset,
            filters(&query),
            sort_column(&query, BREAKDOWN_SORT_COLUMNS),
            sort_order,
        )
        .await
        .map(|(items, total)| json!({"items": items, "total": total}))
    } else {
        crate::db::analytics::breakdown(
            &state.pool,
            website_id,
            dimension,
            int_param(&query, "days", 7, 90),
            per_page,
            offset,
            filters(&query),
            sort_column(&query, BREAKDOWN_SORT_COLUMNS),
            sort_order,
        )
        .await
        .map(|(items, total)| json!({"items": items, "total": total}))
    };
    match result {
        Ok(value) => {
            sse(json!({"_breakdown": value, "breakdownLoading": false, "breakdownError": false}))
        }
        Err(error) => {
            tracing::error!(?error, "breakdown query failed");
            sse(
                json!({"_breakdown": {"items": [], "total": 0}, "breakdownLoading": false, "breakdownError": "Failed to load breakdown"}),
            )
        }
    }
}

fn csv_field(value: &str, text: bool) -> String {
    let protected = if text && matches!(value.chars().next(), Some('=' | '+' | '-' | '@')) {
        format!("'{value}")
    } else {
        value.to_owned()
    };
    if protected.contains([',', '"', '\r', '\n']) {
        format!("\"{}\"", protected.replace('"', "\"\""))
    } else {
        protected
    }
}

fn csv_row(body: &mut String, fields: &[(&str, bool)]) {
    for (index, (value, text)) in fields.iter().enumerate() {
        if index != 0 {
            body.push(',');
        }
        body.push_str(&csv_field(value, *text));
    }
    body.push('\n');
}

#[allow(clippy::too_many_lines)]
pub async fn export(State(state): State<AppState>, request: Request) -> Response {
    let (query, website_id) = match authorized_website_query(&state, &request).await {
        Ok(parts) => parts,
        Err(response) => return response.into(),
    };
    let days = int_param(&query, "days", 7, 90);
    let Some(kind) = optional(&query, "type") else {
        return error_response(StatusCode::BAD_REQUEST, "Export type is required");
    };
    let mut csv = String::new();
    match kind {
        "timeseries" => {
            csv.push_str("timestamp,pageviews\n");
            let points =
                crate::db::analytics::timeseries(&state.pool, website_id, days, filters(&query))
                    .await;
            let points = match points {
                Ok(points) => points,
                Err(error) => return server_error(error, "Failed to export timeseries"),
            };
            for point in points {
                let timestamp = match point
                    .timestamp
                    .format(&time::format_description::well_known::Rfc3339)
                {
                    Ok(timestamp) => timestamp,
                    Err(error) => return server_error(error, "Failed to format timestamp"),
                };
                csv_row(
                    &mut csv,
                    &[(&timestamp, false), (&point.value.to_string(), false)],
                );
            }
        }
        "breakdown" => {
            let Some(dimension) = optional(&query, "dimension").and_then(normalize_dimension)
            else {
                return error_response(StatusCode::BAD_REQUEST, "Invalid breakdown dimension");
            };
            csv.push_str("name,count\n");
            if dimension == "page" {
                let result = crate::db::analytics::top_pages(
                    &state.pool,
                    website_id,
                    days,
                    1000,
                    0,
                    filters(&query),
                    PAGES_SORT_COLUMNS[0],
                    "desc",
                )
                .await;
                let pages = match result {
                    Ok((pages, _)) => pages,
                    Err(error) => return server_error(error, "Failed to export breakdown"),
                };
                for page in pages {
                    csv_row(
                        &mut csv,
                        &[(&page.path, true), (&page.views.to_string(), false)],
                    );
                }
            } else {
                let result = if dimension == "event" {
                    crate::db::analytics::event_breakdown(
                        &state.pool,
                        website_id,
                        days,
                        1000,
                        0,
                        filters(&query),
                        BREAKDOWN_SORT_COLUMNS[0],
                        "desc",
                    )
                    .await
                } else if dimension == "source" || dimension == "channel" {
                    crate::db::analytics::acquisition_breakdown(
                        &state.pool,
                        website_id,
                        dimension,
                        days,
                        1000,
                        0,
                        filters(&query),
                        BREAKDOWN_SORT_COLUMNS[0],
                        "desc",
                    )
                    .await
                } else {
                    crate::db::analytics::breakdown(
                        &state.pool,
                        website_id,
                        dimension,
                        days,
                        1000,
                        0,
                        filters(&query),
                        BREAKDOWN_SORT_COLUMNS[0],
                        "desc",
                    )
                    .await
                };
                let items = match result {
                    Ok((items, _)) => items,
                    Err(error) => return server_error(error, "Failed to export breakdown"),
                };
                for item in items {
                    csv_row(
                        &mut csv,
                        &[(&item.name, true), (&item.count.to_string(), false)],
                    );
                }
            }
        }
        "countries" => {
            csv.push_str("country,country_name,visitors,percentage\n");
            let map = match crate::db::analytics::map_data(
                &state.pool,
                website_id,
                days,
                filters(&query),
            )
            .await
            {
                Ok(map) => map,
                Err(error) => return server_error(error, "Failed to export countries"),
            };
            for point in map.data {
                csv_row(
                    &mut csv,
                    &[
                        (&point.country, true),
                        (&point.country_name, true),
                        (&point.visitors.to_string(), false),
                        (&point.percentage.to_string(), false),
                    ],
                );
            }
        }
        "campaigns" => {
            csv.push_str("dimension,name,visitors\n");
            for dimension in ["source", "medium", "campaign", "term", "content"] {
                let items = crate::db::analytics::breakdown(
                    &state.pool,
                    website_id,
                    &format!("utm_{dimension}"),
                    days,
                    1000,
                    0,
                    AnalyticsFilters::default(),
                    BREAKDOWN_SORT_COLUMNS[0],
                    "desc",
                )
                .await;
                let items = match items {
                    Ok((items, _)) => items,
                    Err(error) => return server_error(error, "Failed to export campaigns"),
                };
                for item in items {
                    csv_row(
                        &mut csv,
                        &[
                            (dimension, true),
                            (&item.name, true),
                            (&item.count.to_string(), false),
                        ],
                    );
                }
            }
        }
        _ => return error_response(StatusCode::BAD_REQUEST, "Invalid export type"),
    }
    let mut response = bytes_response(StatusCode::OK, "text/csv; charset=utf-8", csv.into_bytes());
    set_header(
        &mut response,
        header::CONTENT_DISPOSITION,
        &format!("attachment; filename=\"kaunta-{kind}-{days}d.csv\""),
    );
    response
}

pub async fn map(State(state): State<AppState>, request: Request) -> Response {
    let (user, query) = match user_and_query(&state, &request).await {
        Ok(result) => result,
        Err(response) => return response.into(),
    };
    let Ok(website_id) = website_id(&query) else {
        return sse(json!({"mapError": "Website ID is required", "mapLoading": false}));
    };
    if let Err(response) = verify_owned(&state, user.user_id, website_id).await {
        return response.into();
    }
    let days = int_param(&query, "days", 7, 90);
    match crate::db::analytics::map_data(&state.pool, website_id, days, filters(&query)).await {
        Ok(response) => sse(json!({
            "_mapData": response.data,
            "mapTotalVisitors": response.total_visitors,
            "mapPeriodDays": response.period_days,
            "mapLoading": false,
            "mapError": false
        })),
        Err(error) => {
            tracing::error!(?error, "map data query failed");
            sse(json!({
                "_mapData": [],
                "mapTotalVisitors": 0,
                "mapPeriodDays": days,
                "mapLoading": false
            }))
        }
    }
}

pub async fn realtime(State(state): State<AppState>, request: Request) -> Response {
    let (_query, website_id) = match authorized_website_query(&state, &request).await {
        Ok(parts) => parts,
        Err(response) => return response.into(),
    };
    match crate::db::analytics::current_visitors(&state.pool, website_id).await {
        Ok(value) => sse(json!({
            "currentVisitors": value,
            "realtimeVisitors": value,
            "realtimeLoading": false
        })),
        Err(error) => server_error(error, "Failed to query current visitors"),
    }
}

pub async fn campaigns_init(State(state): State<AppState>, request: Request) -> Response {
    let (user, query) = match user_and_query(&state, &request).await {
        Ok(result) => result,
        Err(response) => return response.into(),
    };
    let websites = match crate::db::websites::list_for_user(&state.pool, user.user_id).await {
        Ok(websites) => websites,
        Err(error) => {
            tracing::error!(?error, "failed to load campaign websites");
            return sse(json!({
                "websitesError": "Failed to load websites",
                "websitesLoading": false,
                "_websites": []
            }));
        }
    };
    let selected = selected_website_id(&query, &websites);
    sse(json!({
        "_websites": website_infos(&websites),
        "selectedWebsite": selected.map(|value| value.to_string()).unwrap_or_default(),
        "websitesLoading": false,
        "websitesError": false
    }))
}

pub async fn campaigns(State(state): State<AppState>, request: Request) -> Response {
    let (user, query) = match user_and_query(&state, &request).await {
        Ok(result) => result,
        Err(response) => return response.into(),
    };
    let Ok(website_id) = website_id(&query) else {
        return sse(json!({"websitesError": "Website ID is required"}));
    };
    if let Err(response) = verify_owned(&state, user.user_id, website_id).await {
        return response.into();
    }

    let dimensions = match optional(&query, "dimension") {
        Some(dimension @ ("source" | "medium" | "campaign" | "term" | "content")) => {
            vec![dimension]
        }
        Some(_) => {
            return sse(json!({
                "websitesError": "Invalid campaign dimension"
            }));
        }
        None => vec!["source", "medium", "campaign", "term", "content"],
    };
    let sort_by = match optional(&query, "sort_by") {
        Some("name") => "name",
        _ => "count",
    };
    let sort_order = match optional(&query, "sort_order") {
        Some("asc") => "asc",
        _ => "desc",
    };

    let mut events: Vec<Result<DatastarEvent, BoxError>> = Vec::new();
    let mut campaign_signals = serde_json::Map::new();
    let mut loading = serde_json::Map::new();
    for dimension in dimensions {
        let result = crate::db::analytics::breakdown(
            &state.pool,
            website_id,
            &format!("utm_{dimension}"),
            int_param(&query, "days", 1, 90),
            50,
            0,
            AnalyticsFilters::default(),
            sort_by,
            sort_order,
        )
        .await;
        let items = match result {
            Ok((items, _)) => items,
            Err(error) => {
                tracing::error!(?error, %dimension, "campaign breakdown query failed");
                Vec::new()
            }
        };
        events.push(utm_table_patch(dimension, &items, sort_by, sort_order));
        campaign_signals.insert(dimension.to_owned(), json!(items));
        loading.insert(dimension.to_owned(), Value::Bool(false));
    }
    events.push(
        PatchSignals::new(JsonEventData(json!({
            "_campaigns": Value::Object(campaign_signals),
            "loading": Value::Object(loading)
        })))
        .try_into_datastar_event()
        .map_err(Into::into),
    );
    datastar(events)
}

fn utm_table_patch(
    dimension: &str,
    items: &[crate::domain::analytics::BreakdownItem],
    sort_by: &str,
    sort_order: &str,
) -> Result<DatastarEvent, BoxError> {
    Ok(
        PatchElements::new(build_utm_table(dimension, items, sort_by, sort_order).try_into()?)
            .with_selector(format!("#utm-{dimension}-content").try_into()?)
            .try_into_datastar_event()?,
    )
}

pub async fn websites_init(State(state): State<AppState>, request: Request) -> Response {
    let user = match authenticate_session(&state, request.headers()).await {
        Ok(user) => user,
        Err(response) => return response.into(),
    };
    match crate::db::websites::list_for_user(&state.pool, user.user_id).await {
        Ok(websites) => {
            let cards = websites
                .iter()
                .map(|website| WebsiteCard {
                    id: website.website_id,
                    domain: &website.domain,
                    name: &website.name,
                    allowed_domains: &website.allowed_domains,
                    public_stats_enabled: website.public_stats_enabled,
                    deletion_in_days: website.pending_delete_at.map(|pending| {
                        let elapsed = OffsetDateTime::now_utc() - pending;
                        (PENDING_DELETE_DAYS - elapsed.whole_days()).max(0)
                    }),
                })
                .collect::<Vec<_>>();
            sse(json!({
                "_websites": cards,
                "websitesError": false,
                "websitesLoading": false
            }))
        }
        Err(error) => {
            tracing::error!(?error, "failed to load website management data");
            sse(json!({
                "websitesError": "Failed to load websites",
                "websitesLoading": false
            }))
        }
    }
}

pub async fn websites_create(State(state): State<AppState>, request: Request) -> Response {
    if !csrf_valid(&state, &request).await {
        return error_response(StatusCode::FORBIDDEN, "CSRF validation failed");
    }
    let user = match authenticate_session(&state, request.headers()).await {
        Ok(user) => user,
        Err(response) => return response.into(),
    };
    let form = match decode_form(request).await {
        Ok(form) => form,
        Err(response) => return response.into(),
    };
    let domain = form
        .get("domain")
        .map(|value| value.trim())
        .unwrap_or_default();
    if domain.is_empty() {
        return sse(json!({"createError": "Domain is required", "creating": false}));
    }
    let name = form
        .get("name")
        .map(|value| value.trim())
        .filter(|value| !value.is_empty())
        .unwrap_or(domain);
    let allowed_domains = vec![domain.to_owned(), format!("www.{domain}")];
    if let Err(error) = crate::db::websites::create(
        &state.pool,
        domain,
        name,
        &allowed_domains,
        Some(user.user_id),
    )
    .await
    {
        tracing::error!(?error, "failed to create dashboard website");
        return sse(json!({
            "createError": "Failed to create website. Domain may already exist.",
            "creating": false
        }));
    }
    sse(json!({
        "showCreateModal": false,
        "creating": false,
        "createError": "",
        "newWebsite": {"domain": "", "name": ""},
        "toast": {
            "show": true,
            "message": "Website created successfully!",
            "type": "success"
        },
        "websitesReload": true
    }))
}

pub async fn map_init(State(state): State<AppState>, request: Request) -> Response {
    let (user, query) = match user_and_query(&state, &request).await {
        Ok(result) => result,
        Err(response) => return response.into(),
    };
    let websites = match crate::db::websites::list_for_user(&state.pool, user.user_id).await {
        Ok(websites) => websites,
        Err(error) => {
            tracing::error!(?error, "failed to load map websites");
            return sse(json!({
                "mapError": "Failed to load websites",
                "mapLoading": false
            }));
        }
    };
    let selected = selected_website_id(&query, &websites);
    let map = if let Some(website_id) = selected {
        crate::db::analytics::map_data(&state.pool, website_id, 7, AnalyticsFilters::default())
            .await
            .ok()
    } else {
        None
    };
    sse(json!({
        "_websites": website_infos(&websites),
        "selectedWebsite": selected.map(|value| value.to_string()).unwrap_or_default(),
        "websitesLoading": false,
        "websitesError": false,
        "_mapData": map.as_ref().map(|response| response.data.as_slice()).unwrap_or(&[]),
        "mapTotalVisitors": map.as_ref().map_or(0, |response| response.total_visitors),
        "mapPeriodDays": 7,
        "mapLoading": false
    }))
}

pub async fn goals(State(state): State<AppState>, request: Request) -> Response {
    let (_query, website_id) = match authorized_website_query(&state, &request).await {
        Ok(parts) => parts,
        Err(response) => return response.into(),
    };
    match crate::db::goals::list(&state.pool, website_id).await {
        Ok(goals) => sse(json!({"_goals": goals, "goalsLoading": false})),
        Err(error) => server_error(error, "Failed to load goals"),
    }
}

pub async fn create_goal(State(state): State<AppState>, request: Request) -> Response {
    if !csrf_valid(&state, &request).await {
        return error_response(StatusCode::FORBIDDEN, "CSRF validation failed");
    }
    let user = match authenticate_session(&state, request.headers()).await {
        Ok(user) => user,
        Err(response) => return response.into(),
    };
    let form = match decode_form(request).await {
        Ok(form) => form,
        Err(response) => return response.into(),
    };
    let goal = match goal_request(&form, None) {
        Ok(goal) => goal,
        Err(message) => {
            return sse(json!({
                "goalError": message,
                "goalLoading": false,
                "submitting": false
            }));
        }
    };
    if let Err(response) = verify_owned(&state, user.user_id, goal.website_id).await {
        return response.into();
    }
    if let Err(error) = crate::db::goals::create(&state.pool, &goal).await {
        tracing::error!(?error, "failed to create goal");
        return sse(json!({
            "goalError": "Failed to create goal",
            "goalLoading": false,
            "submitting": false
        }));
    }
    match crate::db::goals::list(&state.pool, goal.website_id).await {
        Ok(goals) => sse(json!({
            "_goals": goals,
            "goalsLoading": false,
            "showCreateModal": false,
            "goalLoading": false,
            "goalError": false,
            "goalForm": {"name": "", "type": "", "value": ""},
            "goalId": "",
            "submitting": false,
            "toast": {
                "show": true,
                "message": "Goal created successfully!",
                "type": "success"
            }
        })),
        Err(error) => server_error(error, "Failed to load goals"),
    }
}

pub async fn update_goal(
    State(state): State<AppState>,
    Path(path): Path<GoalPath>,
    request: Request,
) -> Response {
    if !csrf_valid(&state, &request).await {
        return error_response(StatusCode::FORBIDDEN, "CSRF validation failed");
    }
    let user = match authenticate_session(&state, request.headers()).await {
        Ok(user) => user,
        Err(response) => return response.into(),
    };
    let existing = match owned_goal(&state, user.user_id, path.id).await {
        Ok(goal) => goal,
        Err(response) => return response.into(),
    };
    let form = match decode_form(request).await {
        Ok(form) => form,
        Err(response) => return response.into(),
    };
    let goal = match goal_request(&form, Some(existing.website_id)) {
        Ok(goal) => goal,
        Err(message) => {
            return sse(json!({
                "goalError": message,
                "goalLoading": false,
                "submitting": false
            }));
        }
    };
    match crate::db::goals::update(&state.pool, path.id, &goal).await {
        Ok(Some(_)) => {}
        Ok(None) => return error_response(StatusCode::NOT_FOUND, "Goal not found"),
        Err(error) => {
            tracing::error!(?error, "failed to update goal");
            return sse(json!({"goalError": "Failed to update goal", "goalLoading": false}));
        }
    }
    match crate::db::goals::list(&state.pool, existing.website_id).await {
        Ok(goals) => sse(json!({
            "_goals": goals,
            "goalsLoading": false,
            "showEditModal": false,
            "goalLoading": false,
            "goalError": false,
            "submitting": false,
            "goalForm": {"name": "", "type": "", "value": ""},
            "goalId": "",
            "currentGoal": null,
            "toast": {
                "show": true,
                "message": "Goal updated successfully!",
                "type": "success"
            }
        })),
        Err(error) => server_error(error, "Failed to load goals"),
    }
}

pub async fn delete_goal(
    State(state): State<AppState>,
    Path(path): Path<GoalPath>,
    request: Request,
) -> Response {
    if !csrf_valid(&state, &request).await {
        return error_response(StatusCode::FORBIDDEN, "CSRF validation failed");
    }
    let user = match authenticate_session(&state, request.headers()).await {
        Ok(user) => user,
        Err(response) => return response.into(),
    };
    let existing = match owned_goal(&state, user.user_id, path.id).await {
        Ok(goal) => goal,
        Err(response) => return response.into(),
    };
    match crate::db::goals::delete(&state.pool, path.id).await {
        Ok(Some(_)) => {}
        Ok(None) => return error_response(StatusCode::NOT_FOUND, "Goal not found"),
        Err(error) => return server_error(error, "Failed to delete goal"),
    }
    match crate::db::goals::list(&state.pool, existing.website_id).await {
        Ok(goals) => sse(json!({
            "_goals": goals,
            "goalsLoading": false,
            "goalLoading": false,
            "goalError": false,
            "toast": {
                "show": true,
                "message": "Goal deleted successfully!",
                "type": "success"
            }
        })),
        Err(error) => server_error(error, "Failed to load goals"),
    }
}

pub async fn goal_analytics(
    State(state): State<AppState>,
    Path(path): Path<GoalPath>,
    request: Request,
) -> Response {
    let (user, query) = match user_and_query(&state, &request).await {
        Ok(result) => result,
        Err(response) => return response.into(),
    };
    if let Err(response) = owned_goal(&state, user.user_id, path.id).await {
        return response.into();
    }
    match crate::db::analytics::goal_analytics(
        &state.pool,
        path.id,
        int_param(&query, "days", 7, 90),
        filters(&query),
    )
    .await
    {
        Ok(analytics) => json_response(StatusCode::OK, analytics),
        Err(error) => server_error(error, "Failed to load goal analytics"),
    }
}

pub async fn goal_breakdown(
    State(state): State<AppState>,
    Path(path): Path<GoalBreakdownPath>,
    request: Request,
) -> Response {
    let (user, query) = match user_and_query(&state, &request).await {
        Ok(result) => result,
        Err(response) => return response.into(),
    };
    if let Err(response) = owned_goal(&state, user.user_id, path.id).await {
        return response.into();
    }
    let Some(dimension) = normalize_goal_dimension(&path.r#type) else {
        return error_response(StatusCode::BAD_REQUEST, "Invalid breakdown type");
    };
    let page = int_param(&query, "page_number", 1, 10_000);
    let per_page = int_param(&query, "per", 10, 100);
    let offset = (page - 1) * per_page;
    let result = if dimension == "page" {
        crate::db::analytics::goal_converting_pages(
            &state.pool,
            path.id,
            int_param(&query, "days", 7, 90),
            per_page,
            offset,
            filters(&query),
        )
        .await
        .map(|(items, total)| json!({"items": items, "total": total}))
    } else {
        crate::db::analytics::goal_breakdown(
            &state.pool,
            path.id,
            dimension,
            int_param(&query, "days", 7, 90),
            per_page,
            offset,
            filters(&query),
        )
        .await
        .map(|(items, total)| json!({"items": items, "total": total}))
    };
    match result {
        Ok(value) => json_response(StatusCode::OK, value),
        Err(error) => server_error(error, "Failed to load goal breakdown"),
    }
}

async fn decode_form(
    request: Request,
) -> crate::web::http::HttpResult<std::collections::HashMap<String, String>> {
    let bytes = request
        .into_body()
        .collect()
        .await
        .map_err(|error| {
            tracing::debug!(?error, "failed to collect form request");
            error_response(StatusCode::BAD_REQUEST, "Invalid form payload")
        })?
        .to_bytes();
    Ok(url::form_urlencoded::parse(&bytes).into_owned().collect())
}

fn goal_request(
    form: &std::collections::HashMap<String, String>,
    website_id: Option<Uuid>,
) -> Result<GoalRequest, &'static str> {
    let name = form.get("name").map(String::as_str).unwrap_or_default();
    let kind = form.get("type").map(String::as_str).unwrap_or_default();
    let value = form.get("value").map(String::as_str).unwrap_or_default();
    let website_id = website_id
        .or_else(|| {
            form.get("website_id")
                .and_then(|value| Uuid::parse_str(value).ok())
        })
        .ok_or("Invalid website ID")?;
    if name.is_empty() || kind.is_empty() || value.is_empty() {
        return Err("All fields are required");
    }
    let kind = match kind {
        "page_view" => GoalKind::PageView,
        "custom_event" => GoalKind::CustomEvent,
        _ => return Err("Invalid goal type"),
    };
    Ok(GoalRequest {
        website_id,
        name: name.to_owned(),
        kind,
        value: value.to_owned(),
    })
}

fn build_utm_table(
    dimension: &str,
    items: &[crate::domain::analytics::BreakdownItem],
    sort_by: &str,
    sort_order: &str,
) -> String {
    if items.is_empty() {
        return format!(
            "<div id=\"utm-{dimension}-content\" class=\"empty-state-mini\"><p>No campaign visits in this period.</p><small>Add UTM tags to your shared links.</small></div>"
        );
    }
    let label = format!(
        "{}{}",
        dimension
            .chars()
            .next()
            .unwrap_or_default()
            .to_ascii_uppercase(),
        &dimension[1..]
    );
    let mut rows = String::new();
    for item in items {
        rows.push_str(&format!(
            "<tr><td>{}</td><td style=\"text-align:right;font-weight:500;color:var(--accent-color)\">{}</td></tr>",
            escape_html(&item.name),
            item.count
        ));
    }
    format!(
        "<table id=\"utm-{dimension}-content\"><thead><tr>{}{}</tr></thead><tbody>{rows}</tbody></table>",
        sort_header(dimension, &label, "name", sort_by, sort_order, false),
        sort_header(dimension, "Visitors", "count", sort_by, sort_order, true),
    )
}

/// A `<th>` whose button re-fetches this dimension sorted by `column`,
/// toggling direction when the column is already active.
fn sort_header(
    dimension: &str,
    label: &str,
    column: &str,
    sort_by: &str,
    sort_order: &str,
    numeric: bool,
) -> String {
    let active = sort_by == column;
    let (aria, arrow) = match (active, sort_order) {
        (true, "asc") => ("ascending", "\u{25b2}"),
        (true, _) => ("descending", "\u{25bc}"),
        (false, _) => ("none", "\u{2195}"),
    };
    let next = if active && sort_order == "desc" {
        "asc"
    } else {
        "desc"
    };
    let style = if numeric {
        " style=\"text-align:right\""
    } else {
        ""
    };
    format!(
        "<th scope=\"col\" aria-sort=\"{aria}\"{style}><button type=\"button\" class=\"sort-button\" \
         data-on:click=\"@get('/api/dashboard/campaigns?' + window.kaunta.query($selectedWebsite, $days) + \
         '&dimension={dimension}&sort_by={column}&sort_order={next}')\">{label}<span aria-hidden=\"true\"> {arrow}</span></button></th>"
    )
}

fn escape_html(value: &str) -> String {
    value
        .replace('&', "&amp;")
        .replace('<', "&lt;")
        .replace('>', "&gt;")
        .replace('"', "&quot;")
        .replace('\'', "&#39;")
}

#[cfg(test)]
mod tests;
