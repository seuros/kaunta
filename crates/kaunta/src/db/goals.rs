use crate::domain::goal::{Goal, GoalRequest};
use sqlx::{FromRow, PgPool};
use time::OffsetDateTime;
use uuid::Uuid;

#[derive(Debug, FromRow)]
struct GoalRow {
    id: Uuid,
    website_id: Uuid,
    name: String,
    target_url: Option<String>,
    target_event: Option<String>,
    created_at: OffsetDateTime,
    updated_at: OffsetDateTime,
}

impl From<GoalRow> for Goal {
    fn from(row: GoalRow) -> Self {
        Self {
            id: row.id,
            website_id: row.website_id,
            name: row.name,
            target_url: row.target_url,
            target_event: row.target_event,
            created_at: row.created_at,
            updated_at: row.updated_at,
        }
    }
}

pub async fn create(pool: &PgPool, request: &GoalRequest) -> Result<Goal, sqlx::Error> {
    let (target_url, target_event) = request.targets();
    let row = sqlx::query_as::<_, GoalRow>(
        "INSERT INTO goals (website_id, name, target_url, target_event, created_at, updated_at)
         VALUES ($1, $2, $3, $4, NOW(), NOW())
         RETURNING
            id,
            website_id,
            name,
            target_url,
            target_event,
            COALESCE(created_at, NOW()) AS created_at,
            COALESCE(updated_at, created_at, NOW()) AS updated_at",
    )
    .bind(request.website_id)
    .bind(&request.name)
    .bind(target_url)
    .bind(target_event)
    .fetch_one(pool)
    .await?;

    Ok(row.into())
}

pub async fn update(
    pool: &PgPool,
    goal_id: Uuid,
    request: &GoalRequest,
) -> Result<Option<Goal>, sqlx::Error> {
    let (target_url, target_event) = request.targets();
    let row = sqlx::query_as::<_, GoalRow>(
        "UPDATE goals
         SET name = $2, target_url = $3, target_event = $4, updated_at = NOW()
         WHERE id = $1
         RETURNING
            id,
            website_id,
            name,
            target_url,
            target_event,
            COALESCE(created_at, NOW()) AS created_at,
            COALESCE(updated_at, created_at, NOW()) AS updated_at",
    )
    .bind(goal_id)
    .bind(&request.name)
    .bind(target_url)
    .bind(target_event)
    .fetch_optional(pool)
    .await?;

    Ok(row.map(Into::into))
}

pub async fn get(pool: &PgPool, goal_id: Uuid) -> Result<Option<Goal>, sqlx::Error> {
    let row = sqlx::query_as::<_, GoalRow>(
        "SELECT
            id,
            website_id,
            name,
            target_url,
            target_event,
            COALESCE(created_at, NOW()) AS created_at,
            COALESCE(updated_at, created_at, NOW()) AS updated_at
         FROM goals
         WHERE id = $1",
    )
    .bind(goal_id)
    .fetch_optional(pool)
    .await?;

    Ok(row.map(Into::into))
}

pub async fn list(pool: &PgPool, website_id: Uuid) -> Result<Vec<Goal>, sqlx::Error> {
    let rows = sqlx::query_as::<_, GoalRow>(
        "SELECT
            id,
            website_id,
            name,
            target_url,
            target_event,
            COALESCE(created_at, NOW()) AS created_at,
            COALESCE(updated_at, created_at, NOW()) AS updated_at
         FROM goals
         WHERE website_id = $1
         ORDER BY created_at DESC",
    )
    .bind(website_id)
    .fetch_all(pool)
    .await?;

    Ok(rows.into_iter().map(Into::into).collect())
}

pub async fn delete(pool: &PgPool, goal_id: Uuid) -> Result<Option<Uuid>, sqlx::Error> {
    sqlx::query_scalar("DELETE FROM goals WHERE id = $1 RETURNING website_id")
        .bind(goal_id)
        .fetch_optional(pool)
        .await
}

pub async fn match_goal(
    pool: &PgPool,
    website_id: Uuid,
    event_type: i16,
    url_path: Option<&str>,
    event_name: Option<&str>,
) -> Result<Option<Uuid>, sqlx::Error> {
    sqlx::query_scalar(
        "SELECT id
         FROM goals
         WHERE website_id = $1
           AND (
                ($2 = 1 AND target_url IS NOT NULL AND target_url = $3)
             OR ($2 = 2 AND target_event IS NOT NULL AND target_event = $4)
           )
         ORDER BY created_at
         LIMIT 1",
    )
    .bind(website_id)
    .bind(event_type)
    .bind(url_path)
    .bind(event_name)
    .fetch_optional(pool)
    .await
}

pub async fn record_completion(
    pool: &PgPool,
    goal_id: Uuid,
    session_id: Uuid,
    event_id: Uuid,
    website_id: Uuid,
) -> Result<bool, sqlx::Error> {
    let inserted = sqlx::query_scalar::<_, Uuid>(
        "INSERT INTO goal_completions (goal_id, session_id, event_id, website_id, completed_at)
         VALUES ($1, $2, $3, $4, NOW())
         ON CONFLICT (goal_id, session_id) DO NOTHING
         RETURNING id",
    )
    .bind(goal_id)
    .bind(session_id)
    .bind(event_id)
    .bind(website_id)
    .fetch_optional(pool)
    .await?;

    Ok(inserted.is_some())
}
