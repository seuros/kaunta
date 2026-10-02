use crate::domain::event::{EventInsert, SessionUpsert};
use sqlx::{PgPool, types::Json};
use time::OffsetDateTime;
use uuid::Uuid;

pub async fn upsert_session(pool: &PgPool, session: &SessionUpsert) -> Result<(), sqlx::Error> {
    let attributes = &session.attributes;
    sqlx::query(
        "INSERT INTO session (
            session_id,
            website_id,
            hostname,
            browser,
            os,
            device,
            screen,
            language,
            country,
            subdivision1,
            subdivision2,
            city,
            region,
            created_at,
            distinct_id,
            entry_page,
            exit_page
         )
         VALUES (
            $1, $2, $3, $4, $5, $6, $7, $8, $9,
            $10, $11, $12, $13, $14, $15, $16, $17
         )
         ON CONFLICT (session_id) DO UPDATE
         SET exit_page = EXCLUDED.exit_page",
    )
    .bind(session.session_id)
    .bind(session.website_id)
    .bind(&attributes.hostname)
    .bind(&attributes.browser)
    .bind(&attributes.os)
    .bind(&attributes.device)
    .bind(&attributes.screen)
    .bind(&attributes.language)
    .bind(&attributes.country)
    .bind(&attributes.subdivision1)
    .bind(&attributes.subdivision2)
    .bind(&attributes.city)
    .bind(&attributes.region)
    .bind(session.created_at)
    .bind(&attributes.distinct_id)
    .bind(&attributes.entry_page)
    .bind(&attributes.exit_page)
    .execute(pool)
    .await?;

    Ok(())
}

pub async fn insert_event(pool: &PgPool, event: &EventInsert) -> Result<(), sqlx::Error> {
    sqlx::query(
        "INSERT INTO website_event (
            event_id,
            website_id,
            session_id,
            visit_id,
            created_at,
            url_path,
            url_query,
            referrer_path,
            referrer_query,
            referrer_domain,
            page_title,
            hostname,
            event_type,
            event_name,
            tag,
            scroll_depth,
            engagement_time,
            props,
            utm_source,
            utm_medium,
            utm_campaign,
            utm_term,
            utm_content,
            goal_id
         )
         VALUES (
            $1, $2, $3, $4, $5, $6, $7, $8,
            $9, $10, $11, $12, $13, $14, $15, $16,
            $17, $18, $19, $20, $21, $22, $23, $24
         )",
    )
    .bind(event.event_id)
    .bind(event.website_id)
    .bind(event.session_id)
    .bind(event.visit_id)
    .bind(event.created_at)
    .bind(&event.url_path)
    .bind(&event.url_query)
    .bind(&event.referrer_path)
    .bind(&event.referrer_query)
    .bind(&event.referrer_domain)
    .bind(&event.page_title)
    .bind(&event.hostname)
    .bind(event.event_type)
    .bind(&event.event_name)
    .bind(&event.tag)
    .bind(event.scroll_depth)
    .bind(event.engagement_time)
    .bind(event.props.clone().map(Json))
    .bind(&event.utm_source)
    .bind(&event.utm_medium)
    .bind(&event.utm_campaign)
    .bind(&event.utm_term)
    .bind(&event.utm_content)
    .bind(event.goal_id)
    .execute(pool)
    .await?;

    Ok(())
}

pub async fn set_event_goal(
    pool: &PgPool,
    event_id: Uuid,
    created_at: OffsetDateTime,
    goal_id: Uuid,
) -> Result<bool, sqlx::Error> {
    let result = sqlx::query(
        "UPDATE website_event
         SET goal_id = $1
         WHERE event_id = $2 AND created_at = $3",
    )
    .bind(goal_id)
    .bind(event_id)
    .bind(created_at)
    .execute(pool)
    .await?;

    Ok(result.rows_affected() > 0)
}

pub async fn update_ip_metadata(
    pool: &PgPool,
    ip_address: &str,
    user_agent: &str,
    country: Option<&str>,
) -> Result<bool, sqlx::Error> {
    let result = sqlx::query_scalar::<_, Option<bool>>(
        "SELECT update_ip_metadata($1::inet, $2, $3::char(2))",
    )
    .bind(ip_address)
    .bind(user_agent)
    .bind(country)
    .fetch_one(pool)
    .await?;

    Ok(result.unwrap_or(false))
}

/// Raise engagement metrics on the newest pageview of a session for a path.
///
/// Values only ever increase (GREATEST), so late, duplicate, or reordered
/// engagement beacons are harmless. Bounded to the last day for partition
/// pruning; returns the number of rows updated (0 or 1).
pub async fn update_engagement(
    pool: &PgPool,
    website_id: Uuid,
    session_id: Uuid,
    url_path: &str,
    engagement_time: Option<i32>,
    scroll_depth: Option<i16>,
) -> Result<u64, sqlx::Error> {
    let result = sqlx::query(
        "UPDATE website_event SET
            engagement_time = GREATEST(COALESCE(engagement_time, 0), COALESCE($4, 0)),
            scroll_depth = GREATEST(COALESCE(scroll_depth, 0), COALESCE($5, 0))
         WHERE created_at >= NOW() - INTERVAL '1 day'
           AND event_id = (
                SELECT event_id FROM website_event
                WHERE website_id = $1
                  AND session_id = $2
                  AND url_path = $3
                  AND event_type = 1
                  AND created_at >= NOW() - INTERVAL '1 day'
                ORDER BY created_at DESC
                LIMIT 1
           )",
    )
    .bind(website_id)
    .bind(session_id)
    .bind(url_path)
    .bind(engagement_time)
    .bind(scroll_depth)
    .execute(pool)
    .await?;
    Ok(result.rows_affected())
}
