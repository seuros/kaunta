use crate::domain::{
    analytics::{
        BreakdownItem, DashboardStats, MapDataPoint, MapResponse, PeriodOverview, PublicStats,
        TimeSeriesPoint, TopPage,
    },
    goal::GoalAnalytics,
};
use sqlx::{FromRow, PgPool};
use time::OffsetDateTime;
use uuid::Uuid;

#[derive(Debug, Clone, Copy, Default)]
pub struct AnalyticsFilters<'a> {
    pub country: Option<&'a str>,
    pub browser: Option<&'a str>,
    pub device: Option<&'a str>,
    pub page: Option<&'a str>,
}

#[derive(Debug, FromRow)]
struct DashboardStatsRow {
    current_visitors: i64,
    today_pageviews: i64,
    today_visitors: i64,
    today_bounce_rate: f64,
}

#[derive(Debug, FromRow)]
struct TimeSeriesRow {
    time_bucket: OffsetDateTime,
    pageviews: i64,
}

#[derive(Debug, FromRow)]
struct BreakdownRow {
    name: String,
    count: i64,
    total_count: i64,
}

#[derive(Debug, FromRow)]
struct TopPageRow {
    path: String,
    views: i64,
    #[allow(dead_code)]
    unique_visitors: i64,
    #[allow(dead_code)]
    avg_engagement_time: Option<f64>,
    total_count: i64,
}

#[derive(Debug, FromRow)]
struct GoalAnalyticsRow {
    completions: i64,
    unique_sessions: i64,
    conversion_rate: f64,
    total_sessions: i64,
}

#[derive(Debug, FromRow)]
struct GoalTimeSeriesRow {
    time_bucket: OffsetDateTime,
    completions: i64,
}

#[derive(Debug, FromRow)]
struct ConvertingPageRow {
    page_path: String,
    conversions: i64,
    total_count: i64,
}

#[derive(Debug, FromRow)]
struct MapDataRow {
    country_code: String,
    visitors: i64,
    percentage: f64,
}

pub async fn dashboard_stats(
    pool: &PgPool,
    website_id: Uuid,
    days: i32,
    filters: AnalyticsFilters<'_>,
) -> Result<DashboardStats, sqlx::Error> {
    let row = sqlx::query_as::<_, DashboardStatsRow>(
        "SELECT
            current_visitors,
            today_pageviews,
            today_visitors,
            bounce_rate::double precision AS today_bounce_rate
         FROM get_dashboard_stats($1, $2, $3, $4, $5, $6)",
    )
    .bind(website_id)
    .bind(days)
    .bind(filters.country)
    .bind(filters.browser)
    .bind(filters.device)
    .bind(filters.page)
    .fetch_one(pool)
    .await?;

    Ok(DashboardStats {
        current_visitors: row.current_visitors,
        today_pageviews: row.today_pageviews,
        today_visitors: row.today_visitors,
        today_bounce_rate: format!("{:.1}%", row.today_bounce_rate),
    })
}

pub async fn timeseries(
    pool: &PgPool,
    website_id: Uuid,
    days: i32,
    filters: AnalyticsFilters<'_>,
) -> Result<Vec<TimeSeriesPoint>, sqlx::Error> {
    let rows = sqlx::query_as::<_, TimeSeriesRow>(
        "SELECT hour AS time_bucket, views AS pageviews
         FROM get_timeseries($1, $2, $3, $4, $5, $6)",
    )
    .bind(website_id)
    .bind(days)
    .bind(filters.country)
    .bind(filters.browser)
    .bind(filters.device)
    .bind(filters.page)
    .fetch_all(pool)
    .await?;

    Ok(rows
        .into_iter()
        .map(|row| TimeSeriesPoint {
            timestamp: row.time_bucket,
            value: row.pageviews,
        })
        .collect())
}

#[allow(clippy::too_many_arguments)]
pub async fn top_pages(
    pool: &PgPool,
    website_id: Uuid,
    days: i32,
    limit: i32,
    offset: i32,
    filters: AnalyticsFilters<'_>,
    sort_by: &str,
    sort_order: &str,
) -> Result<(Vec<TopPage>, i64), sqlx::Error> {
    let rows = sqlx::query_as::<_, TopPageRow>(
        "SELECT
            path,
            views,
            unique_visitors,
            avg_engagement_time::double precision AS avg_engagement_time,
            total_count
         FROM get_top_pages($1, $2, $3, $4, $5, $6, $7, $8, $9)",
    )
    .bind(website_id)
    .bind(days)
    .bind(limit)
    .bind(offset)
    .bind(filters.country)
    .bind(filters.browser)
    .bind(filters.device)
    .bind(sort_by)
    .bind(sort_order)
    .fetch_all(pool)
    .await?;

    let total_count = rows.first().map_or(0, |row| row.total_count);
    let pages = rows
        .into_iter()
        .map(|row| TopPage {
            path: row.path,
            views: row.views,
        })
        .collect();

    Ok((pages, total_count))
}

#[allow(clippy::too_many_arguments)]
pub async fn breakdown(
    pool: &PgPool,
    website_id: Uuid,
    dimension: &str,
    days: i32,
    limit: i32,
    offset: i32,
    filters: AnalyticsFilters<'_>,
    sort_by: &str,
    sort_order: &str,
) -> Result<(Vec<BreakdownItem>, i64), sqlx::Error> {
    let rows = sqlx::query_as::<_, BreakdownRow>(
        "SELECT name, count, total_count
         FROM get_breakdown($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)",
    )
    .bind(website_id)
    .bind(dimension)
    .bind(days)
    .bind(limit)
    .bind(offset)
    .bind(filters.country)
    .bind(filters.browser)
    .bind(filters.device)
    .bind(filters.page)
    .bind(sort_by)
    .bind(sort_order)
    .fetch_all(pool)
    .await?;

    let total_count = rows.first().map_or(0, |row| row.total_count);
    let items = rows
        .into_iter()
        .map(|row| BreakdownItem {
            name: row.name,
            code: None,
            count: row.count,
        })
        .collect();

    Ok((items, total_count))
}

pub async fn public_stats(pool: &PgPool, website_id: Uuid) -> Result<PublicStats, sqlx::Error> {
    let online = current_visitors(pool, website_id).await?;
    let (pageviews, visitors) = sqlx::query_as::<_, (i64, i64)>(
        "SELECT COUNT(*)::bigint, COUNT(DISTINCT session_id)::bigint
         FROM website_event
         WHERE website_id = $1 AND event_type = 1",
    )
    .bind(website_id)
    .fetch_one(pool)
    .await?;

    Ok(PublicStats {
        online,
        pageviews,
        visitors,
    })
}

pub async fn current_visitors(pool: &PgPool, website_id: Uuid) -> Result<i64, sqlx::Error> {
    sqlx::query_scalar(
        "SELECT COUNT(DISTINCT session_id)::bigint
         FROM website_event
         WHERE website_id = $1
           AND created_at >= NOW() - INTERVAL '5 minutes'
           AND event_type = 1",
    )
    .bind(website_id)
    .fetch_one(pool)
    .await
}

pub async fn map_data(
    pool: &PgPool,
    website_id: Uuid,
    days: i32,
    filters: AnalyticsFilters<'_>,
) -> Result<MapResponse, sqlx::Error> {
    let rows = sqlx::query_as::<_, MapDataRow>(
        "SELECT
            country AS country_code,
            visitors,
            percentage::double precision AS percentage
         FROM get_map_data($1, $2, $3, $4, $5, $6)",
    )
    .bind(website_id)
    .bind(days)
    .bind(filters.country)
    .bind(filters.browser)
    .bind(filters.device)
    .bind(filters.page)
    .fetch_all(pool)
    .await?;
    let total_visitors = rows.iter().map(|row| row.visitors).sum();
    let data = rows
        .into_iter()
        .map(|row| {
            let (code, country_name) = countries::resolve(&row.country_code);
            MapDataPoint {
                country_name,
                code,
                country: row.country_code,
                visitors: row.visitors,
                percentage: row.percentage,
            }
        })
        .collect();
    Ok(MapResponse {
        data,
        total_visitors,
        period_days: days,
    })
}

mod countries;

#[cfg(test)]
mod tests;

pub async fn goal_analytics(
    pool: &PgPool,
    goal_id: Uuid,
    days: i32,
    filters: AnalyticsFilters<'_>,
) -> Result<GoalAnalytics, sqlx::Error> {
    let row = sqlx::query_as::<_, GoalAnalyticsRow>(
        "SELECT
            completions,
            unique_sessions,
            conversion_rate::double precision AS conversion_rate,
            total_sessions
         FROM get_goal_analytics($1, $2, $3, $4, $5, $6)",
    )
    .bind(goal_id)
    .bind(days)
    .bind(filters.country)
    .bind(filters.browser)
    .bind(filters.device)
    .bind(filters.page)
    .fetch_one(pool)
    .await?;

    Ok(GoalAnalytics {
        completions: row.completions,
        unique_sessions: row.unique_sessions,
        conversion_rate: row.conversion_rate,
        total_sessions: row.total_sessions,
    })
}

pub async fn goal_timeseries(
    pool: &PgPool,
    goal_id: Uuid,
    days: i32,
    filters: AnalyticsFilters<'_>,
) -> Result<Vec<TimeSeriesPoint>, sqlx::Error> {
    let rows = sqlx::query_as::<_, GoalTimeSeriesRow>(
        "SELECT time_bucket, completions
         FROM get_goal_timeseries($1, $2, $3, $4, $5, $6)",
    )
    .bind(goal_id)
    .bind(days)
    .bind(filters.country)
    .bind(filters.browser)
    .bind(filters.device)
    .bind(filters.page)
    .fetch_all(pool)
    .await?;

    Ok(rows
        .into_iter()
        .map(|row| TimeSeriesPoint {
            timestamp: row.time_bucket,
            value: row.completions,
        })
        .collect())
}

#[allow(clippy::too_many_arguments)]
pub async fn goal_breakdown(
    pool: &PgPool,
    goal_id: Uuid,
    dimension: &str,
    days: i32,
    limit: i32,
    offset: i32,
    filters: AnalyticsFilters<'_>,
) -> Result<(Vec<BreakdownItem>, i64), sqlx::Error> {
    let rows = sqlx::query_as::<_, BreakdownRow>(
        "SELECT name, count, total_count
         FROM get_goal_breakdown($1, $2, $3, $4, $5, $6, $7, $8, $9)",
    )
    .bind(goal_id)
    .bind(dimension)
    .bind(days)
    .bind(limit)
    .bind(offset)
    .bind(filters.country)
    .bind(filters.browser)
    .bind(filters.device)
    .bind(filters.page)
    .fetch_all(pool)
    .await?;

    let total_count = rows.first().map_or(0, |row| row.total_count);
    let items = rows
        .into_iter()
        .map(|row| BreakdownItem {
            name: row.name,
            code: None,
            count: row.count,
        })
        .collect();

    Ok((items, total_count))
}

#[allow(clippy::too_many_arguments)]
pub async fn goal_converting_pages(
    pool: &PgPool,
    goal_id: Uuid,
    days: i32,
    limit: i32,
    offset: i32,
    filters: AnalyticsFilters<'_>,
) -> Result<(Vec<TopPage>, i64), sqlx::Error> {
    let rows = sqlx::query_as::<_, ConvertingPageRow>(
        "SELECT page_path, conversions, total_count
         FROM get_goal_converting_pages($1, $2, $3, $4, $5, $6, $7)",
    )
    .bind(goal_id)
    .bind(days)
    .bind(limit)
    .bind(offset)
    .bind(filters.country)
    .bind(filters.browser)
    .bind(filters.device)
    .fetch_all(pool)
    .await?;

    let total_count = rows.first().map_or(0, |row| row.total_count);
    let pages = rows
        .into_iter()
        .map(|row| TopPage {
            path: row.page_path,
            views: row.conversions,
        })
        .collect();

    Ok((pages, total_count))
}

#[derive(Debug, FromRow)]
struct PeriodOverviewRow {
    visitors: i64,
    pageviews: i64,
    bounce_rate: f64,
    prev_visitors: i64,
    prev_pageviews: i64,
    prev_bounce_rate: f64,
}

/// Whole-period totals for the current window and the previous window of
/// the same length, with the engagement-adjusted bounce definition.
pub async fn period_overview(
    pool: &PgPool,
    website_id: Uuid,
    days: i32,
    filters: AnalyticsFilters<'_>,
) -> Result<PeriodOverview, sqlx::Error> {
    let row = sqlx::query_as::<_, PeriodOverviewRow>(
        "SELECT
            visitors,
            pageviews,
            bounce_rate::double precision AS bounce_rate,
            prev_visitors,
            prev_pageviews,
            prev_bounce_rate::double precision AS prev_bounce_rate
         FROM get_period_overview($1, $2, $3, $4, $5, $6)",
    )
    .bind(website_id)
    .bind(days)
    .bind(filters.country)
    .bind(filters.browser)
    .bind(filters.device)
    .bind(filters.page)
    .fetch_one(pool)
    .await?;

    Ok(PeriodOverview {
        visitors: row.visitors,
        pageviews: row.pageviews,
        bounce_rate: format!("{:.1}%", row.bounce_rate),
        prev_visitors: row.prev_visitors,
        prev_pageviews: row.prev_pageviews,
        prev_bounce_rate: format!("{:.1}%", row.prev_bounce_rate),
        period_days: days,
    })
}

/// Custom-event name breakdown; the `event` dimension routes here instead
/// of `get_breakdown`.
#[allow(clippy::too_many_arguments)]
pub async fn event_breakdown(
    pool: &PgPool,
    website_id: Uuid,
    days: i32,
    limit: i32,
    offset: i32,
    filters: AnalyticsFilters<'_>,
    sort_by: &str,
    sort_order: &str,
) -> Result<(Vec<BreakdownItem>, i64), sqlx::Error> {
    let rows = sqlx::query_as::<_, BreakdownRow>(
        "SELECT name, count, total_count
         FROM get_event_breakdown($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)",
    )
    .bind(website_id)
    .bind(days)
    .bind(limit)
    .bind(offset)
    .bind(filters.country)
    .bind(filters.browser)
    .bind(filters.device)
    .bind(filters.page)
    .bind(sort_by)
    .bind(sort_order)
    .fetch_all(pool)
    .await?;

    let total_count = rows.first().map_or(0, |row| row.total_count);
    let items = rows
        .into_iter()
        .map(|row| BreakdownItem {
            name: row.name,
            code: None,
            count: row.count,
        })
        .collect();

    Ok((items, total_count))
}

/// Top property keys (when `prop` is `None`) or top values of one property
/// across a custom event's recorded occurrences.
pub async fn event_props(
    pool: &PgPool,
    website_id: Uuid,
    event_name: &str,
    prop: Option<&str>,
    days: i32,
    limit: i32,
) -> Result<Vec<BreakdownItem>, sqlx::Error> {
    let rows = match prop {
        None => {
            sqlx::query_as::<_, (String, i64)>(
                "SELECT key, COUNT(*)
                 FROM website_event e, jsonb_object_keys(e.props) AS key
                 WHERE e.website_id = $1 AND e.event_type = 2 AND e.event_name = $2
                   AND e.created_at >= CURRENT_DATE - ($3 || ' days')::INTERVAL
                   AND e.props IS NOT NULL
                 GROUP BY key ORDER BY COUNT(*) DESC, key LIMIT $4",
            )
            .bind(website_id)
            .bind(event_name)
            .bind(days.to_string())
            .bind(i64::from(limit))
            .fetch_all(pool)
            .await?
        }
        Some(prop) => {
            sqlx::query_as::<_, (String, i64)>(
                "SELECT e.props->>$4, COUNT(*)
                 FROM website_event e
                 WHERE e.website_id = $1 AND e.event_type = 2 AND e.event_name = $2
                   AND e.created_at >= CURRENT_DATE - ($3 || ' days')::INTERVAL
                   AND e.props ? $4
                 GROUP BY 1 ORDER BY COUNT(*) DESC, 1 LIMIT $5",
            )
            .bind(website_id)
            .bind(event_name)
            .bind(days.to_string())
            .bind(prop)
            .bind(i64::from(limit))
            .fetch_all(pool)
            .await?
        }
    };
    Ok(rows
        .into_iter()
        .map(|(name, count)| BreakdownItem {
            name,
            code: None,
            count,
        })
        .collect())
}

/// Breakdown by resolved source name or acquisition channel; the `source`
/// and `channel` dimensions route here instead of `get_breakdown`.
#[allow(clippy::too_many_arguments)]
pub async fn acquisition_breakdown(
    pool: &PgPool,
    website_id: Uuid,
    dimension: &str,
    days: i32,
    limit: i32,
    offset: i32,
    filters: AnalyticsFilters<'_>,
    sort_by: &str,
    sort_order: &str,
) -> Result<(Vec<BreakdownItem>, i64), sqlx::Error> {
    let rows = sqlx::query_as::<_, BreakdownRow>(
        "SELECT name, count, total_count
         FROM get_acquisition_breakdown($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)",
    )
    .bind(website_id)
    .bind(dimension)
    .bind(days)
    .bind(limit)
    .bind(offset)
    .bind(filters.country)
    .bind(filters.browser)
    .bind(filters.device)
    .bind(filters.page)
    .bind(sort_by)
    .bind(sort_order)
    .fetch_all(pool)
    .await?;

    let total_count = rows.first().map_or(0, |row| row.total_count);
    let items = rows
        .into_iter()
        .map(|row| BreakdownItem {
            name: row.name,
            code: None,
            count: row.count,
        })
        .collect();

    Ok((items, total_count))
}
