use crate::domain::origin::TrustedOrigin;
use sqlx::{FromRow, PgPool};

#[derive(Debug, FromRow)]
struct TrustedOriginRow {
    id: i32,
    domain: String,
    description: Option<String>,
    is_active: bool,
    created_at: time::OffsetDateTime,
    updated_at: time::OffsetDateTime,
}

impl From<TrustedOriginRow> for TrustedOrigin {
    fn from(row: TrustedOriginRow) -> Self {
        Self {
            id: row.id,
            domain: row.domain,
            description: row.description,
            is_active: row.is_active,
            created_at: row.created_at,
            updated_at: row.updated_at,
        }
    }
}

pub async fn is_trusted(pool: &PgPool, origin: &str) -> Result<bool, sqlx::Error> {
    sqlx::query_scalar("SELECT is_trusted_origin($1)")
        .bind(origin)
        .fetch_one(pool)
        .await
}

pub async fn active_domains(pool: &PgPool) -> Result<Vec<String>, sqlx::Error> {
    sqlx::query_scalar("SELECT get_trusted_origins()")
        .fetch_one(pool)
        .await
}

pub async fn list(pool: &PgPool) -> Result<Vec<TrustedOrigin>, sqlx::Error> {
    let rows = sqlx::query_as::<_, TrustedOriginRow>(
        "SELECT id, domain, description, is_active, created_at, updated_at
         FROM trusted_origin
         ORDER BY LOWER(domain)",
    )
    .fetch_all(pool)
    .await?;

    Ok(rows.into_iter().map(Into::into).collect())
}

pub async fn find(pool: &PgPool, identifier: &str) -> Result<Option<TrustedOrigin>, sqlx::Error> {
    let row = sqlx::query_as::<_, TrustedOriginRow>(
        "SELECT id, domain, description, is_active, created_at, updated_at
         FROM trusted_origin
         WHERE id::text = $1 OR LOWER(domain) = LOWER($1)
         LIMIT 1",
    )
    .bind(identifier)
    .fetch_optional(pool)
    .await?;

    Ok(row.map(Into::into))
}

pub async fn create(
    pool: &PgPool,
    domain: &str,
    description: Option<&str>,
) -> Result<TrustedOrigin, sqlx::Error> {
    let row = sqlx::query_as::<_, TrustedOriginRow>(
        "INSERT INTO trusted_origin (domain, description)
         VALUES ($1, $2)
         RETURNING id, domain, description, is_active, created_at, updated_at",
    )
    .bind(domain)
    .bind(description)
    .fetch_one(pool)
    .await?;

    Ok(row.into())
}

pub async fn set_active(pool: &PgPool, id: i32, active: bool) -> Result<bool, sqlx::Error> {
    let result = sqlx::query(
        "UPDATE trusted_origin
         SET is_active = $2
         WHERE id = $1",
    )
    .bind(id)
    .bind(active)
    .execute(pool)
    .await?;

    Ok(result.rows_affected() > 0)
}

pub async fn delete(pool: &PgPool, id: i32) -> Result<bool, sqlx::Error> {
    let result = sqlx::query("DELETE FROM trusted_origin WHERE id = $1")
        .bind(id)
        .execute(pool)
        .await?;

    Ok(result.rows_affected() > 0)
}

pub async fn delete_by_identifier(
    pool: &PgPool,
    identifier: &str,
) -> Result<Option<String>, sqlx::Error> {
    sqlx::query_scalar(
        "DELETE FROM trusted_origin
         WHERE id::text = $1 OR LOWER(domain) = LOWER($1)
         RETURNING domain",
    )
    .bind(identifier)
    .fetch_optional(pool)
    .await
}

pub async fn toggle(
    pool: &PgPool,
    identifier: &str,
) -> Result<Option<(String, bool)>, sqlx::Error> {
    sqlx::query_as(
        "UPDATE trusted_origin
         SET is_active = NOT is_active, updated_at = CURRENT_TIMESTAMP
         WHERE id::text = $1 OR LOWER(domain) = LOWER($1)
         RETURNING domain, is_active",
    )
    .bind(identifier)
    .fetch_optional(pool)
    .await
}
