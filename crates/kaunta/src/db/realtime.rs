use crate::domain::event::RealtimeEvent;
use sqlx::{PgPool, postgres::PgListener};
use time::OffsetDateTime;

pub const CHANNEL_NAME: &str = "kaunta_realtime_events";

#[derive(Debug, thiserror::Error)]
pub enum RealtimeError {
    #[error(transparent)]
    Database(#[from] sqlx::Error),
    #[error(transparent)]
    Json(#[from] serde_json::Error),
}

pub async fn notify(pool: &PgPool, event: &RealtimeEvent) -> Result<(), RealtimeError> {
    let payload = serde_json::to_string(event)?;
    sqlx::query("SELECT pg_notify($1, $2)")
        .bind(CHANNEL_NAME)
        .bind(payload)
        .execute(pool)
        .await?;

    Ok(())
}

pub async fn connect_listener(pool: &PgPool) -> Result<PgListener, sqlx::Error> {
    let mut listener = PgListener::connect_with(pool).await?;
    listener.listen(CHANNEL_NAME).await?;
    Ok(listener)
}

pub async fn receive(listener: &mut PgListener) -> Result<RealtimeEvent, RealtimeError> {
    let notification = listener.recv().await?;
    Ok(serde_json::from_str(notification.payload())?)
}

#[derive(Debug, Clone, serde::Serialize, sqlx::FromRow)]
pub struct RecentEvent {
    pub created_at: OffsetDateTime,
    pub url_path: Option<String>,
    pub event_name: Option<String>,
    pub country: Option<String>,
    pub browser: Option<String>,
    pub event_type: i16,
}

/// The newest events for a website, for the live feed. Bounded to the last
/// day so the scan stays inside recent partitions.
pub async fn recent_events(
    pool: &PgPool,
    website_id: uuid::Uuid,
    limit: i64,
) -> Result<Vec<RecentEvent>, sqlx::Error> {
    sqlx::query_as::<_, RecentEvent>(
        "SELECT e.created_at, e.url_path, e.event_name, s.country, s.browser, e.event_type
         FROM website_event e
         LEFT JOIN session s ON s.session_id = e.session_id
         WHERE e.website_id = $1 AND e.created_at >= NOW() - INTERVAL '1 day'
         ORDER BY e.created_at DESC
         LIMIT $2",
    )
    .bind(website_id)
    .bind(limit)
    .fetch_all(pool)
    .await
}
