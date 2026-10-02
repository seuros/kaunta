use crate::domain::api_key::{
    API_KEY_DISPLAY_LENGTH, API_KEY_PREFIX, API_KEY_RANDOM_BYTES, ApiKey, ApiKeyCreateResult,
    ApiKeyValidationError, default_scopes, validate_scopes,
};
use sha2::{Digest, Sha256};
use sqlx::{FromRow, PgPool};
use time::OffsetDateTime;
use uuid::Uuid;

const DEFAULT_RATE_LIMIT_PER_MINUTE: i32 = 1_000;

#[derive(Debug, thiserror::Error)]
pub enum ApiKeyError {
    #[error(transparent)]
    Validation(#[from] ApiKeyValidationError),
    #[error(transparent)]
    Database(#[from] sqlx::Error),
}

#[derive(Debug, FromRow)]
struct ApiKeyRow {
    key_id: Uuid,
    website_id: Uuid,
    created_by: Option<Uuid>,
    key_prefix: String,
    name: Option<String>,
    scopes: Vec<String>,
    rate_limit_per_minute: i32,
    created_at: OffsetDateTime,
    last_used_at: Option<OffsetDateTime>,
    revoked_at: Option<OffsetDateTime>,
    expires_at: Option<OffsetDateTime>,
    website_rate_limit: Option<i32>,
}

impl From<ApiKeyRow> for ApiKey {
    fn from(row: ApiKeyRow) -> Self {
        Self {
            key_id: row.key_id,
            website_id: row.website_id,
            created_by: row.created_by,
            key_prefix: row.key_prefix,
            name: row.name,
            scopes: row.scopes,
            rate_limit_per_minute: row.rate_limit_per_minute,
            created_at: row.created_at,
            last_used_at: row.last_used_at,
            revoked_at: row.revoked_at,
            expires_at: row.expires_at,
            website_rate_limit: row.website_rate_limit,
        }
    }
}

#[must_use]
pub fn hash_api_key(key: &str) -> String {
    hex::encode(Sha256::digest(key.as_bytes()))
}

pub async fn create(
    pool: &PgPool,
    website_id: Uuid,
    created_by: Option<Uuid>,
    name: Option<&str>,
    scopes: &[String],
    expires_at: Option<OffsetDateTime>,
) -> Result<ApiKeyCreateResult, ApiKeyError> {
    let scopes = if scopes.is_empty() {
        default_scopes()
    } else {
        scopes.to_vec()
    };
    validate_scopes(&scopes)?;

    let random_bytes: [u8; API_KEY_RANDOM_BYTES] = rand::random();
    let full_key = format!("{API_KEY_PREFIX}{}", hex::encode(random_bytes));
    let key_hash = hash_api_key(&full_key);
    let key_prefix = full_key[..API_KEY_DISPLAY_LENGTH].to_owned();

    let row = sqlx::query_as::<_, ApiKeyRow>(
        "INSERT INTO api_keys (
            website_id,
            created_by,
            key_hash,
            key_prefix,
            name,
            scopes,
            rate_limit_per_minute,
            expires_at
         )
         VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
         RETURNING
            key_id,
            website_id,
            created_by,
            key_prefix,
            name,
            scopes,
            rate_limit_per_minute,
            COALESCE(created_at, NOW()) AS created_at,
            last_used_at,
            revoked_at,
            expires_at,
            NULL::integer AS website_rate_limit",
    )
    .bind(website_id)
    .bind(created_by)
    .bind(key_hash)
    .bind(key_prefix)
    .bind(name)
    .bind(scopes)
    .bind(DEFAULT_RATE_LIMIT_PER_MINUTE)
    .bind(expires_at)
    .fetch_one(pool)
    .await?;

    Ok(ApiKeyCreateResult {
        api_key: full_key,
        key: row.into(),
    })
}

pub async fn get_by_hash(pool: &PgPool, key_hash: &str) -> Result<Option<ApiKey>, sqlx::Error> {
    let row = sqlx::query_as::<_, ApiKeyRow>(
        "SELECT
            api_keys.key_id,
            api_keys.website_id,
            api_keys.created_by,
            api_keys.key_prefix,
            api_keys.name,
            api_keys.scopes,
            COALESCE(api_keys.rate_limit_per_minute, 1000) AS rate_limit_per_minute,
            COALESCE(api_keys.created_at, NOW()) AS created_at,
            api_keys.last_used_at,
            api_keys.revoked_at,
            api_keys.expires_at,
            COALESCE(website.api_rate_limit_per_minute, 5000) AS website_rate_limit
         FROM api_keys
         JOIN website ON website.website_id = api_keys.website_id
         WHERE api_keys.key_hash = $1
           AND api_keys.revoked_at IS NULL
           AND website.deleted_at IS NULL
           AND (api_keys.expires_at IS NULL OR api_keys.expires_at > NOW())",
    )
    .bind(key_hash)
    .fetch_optional(pool)
    .await?;

    Ok(row.map(Into::into))
}

pub async fn get_by_id(pool: &PgPool, key_id: Uuid) -> Result<Option<ApiKey>, sqlx::Error> {
    let row = sqlx::query_as::<_, ApiKeyRow>(
        "SELECT
            key_id,
            website_id,
            created_by,
            key_prefix,
            name,
            scopes,
            COALESCE(rate_limit_per_minute, 1000) AS rate_limit_per_minute,
            COALESCE(created_at, NOW()) AS created_at,
            last_used_at,
            revoked_at,
            expires_at,
            NULL::integer AS website_rate_limit
         FROM api_keys
         WHERE key_id = $1",
    )
    .bind(key_id)
    .fetch_optional(pool)
    .await?;

    Ok(row.map(Into::into))
}

pub async fn get_by_prefix(pool: &PgPool, prefix: &str) -> Result<Option<ApiKey>, sqlx::Error> {
    let row = sqlx::query_as::<_, ApiKeyRow>(
        "SELECT
            key_id,
            website_id,
            created_by,
            key_prefix,
            name,
            scopes,
            COALESCE(rate_limit_per_minute, 1000) AS rate_limit_per_minute,
            COALESCE(created_at, NOW()) AS created_at,
            last_used_at,
            revoked_at,
            expires_at,
            NULL::integer AS website_rate_limit
         FROM api_keys
         WHERE key_prefix = $1",
    )
    .bind(prefix)
    .fetch_optional(pool)
    .await?;

    Ok(row.map(Into::into))
}

pub async fn list(pool: &PgPool, website_id: Uuid) -> Result<Vec<ApiKey>, sqlx::Error> {
    let rows = sqlx::query_as::<_, ApiKeyRow>(
        "SELECT
            key_id,
            website_id,
            created_by,
            key_prefix,
            name,
            scopes,
            COALESCE(rate_limit_per_minute, 1000) AS rate_limit_per_minute,
            COALESCE(created_at, NOW()) AS created_at,
            last_used_at,
            revoked_at,
            expires_at,
            NULL::integer AS website_rate_limit
         FROM api_keys
         WHERE website_id = $1
         ORDER BY created_at DESC",
    )
    .bind(website_id)
    .fetch_all(pool)
    .await?;

    Ok(rows.into_iter().map(Into::into).collect())
}

pub async fn revoke(pool: &PgPool, key_id: Uuid) -> Result<bool, sqlx::Error> {
    let result = sqlx::query(
        "UPDATE api_keys
         SET revoked_at = NOW()
         WHERE key_id = $1 AND revoked_at IS NULL",
    )
    .bind(key_id)
    .execute(pool)
    .await?;

    Ok(result.rows_affected() > 0)
}

pub async fn revoke_by_prefix(pool: &PgPool, prefix: &str) -> Result<bool, sqlx::Error> {
    let result = sqlx::query(
        "UPDATE api_keys
         SET revoked_at = NOW()
         WHERE key_prefix = $1 AND revoked_at IS NULL",
    )
    .bind(prefix)
    .execute(pool)
    .await?;

    Ok(result.rows_affected() > 0)
}

pub async fn update_last_used(pool: &PgPool, key_id: Uuid) -> Result<(), sqlx::Error> {
    sqlx::query("UPDATE api_keys SET last_used_at = NOW() WHERE key_id = $1")
        .bind(key_id)
        .execute(pool)
        .await?;

    Ok(())
}

pub async fn event_id_exists(
    pool: &PgPool,
    event_id: Uuid,
    website_id: Uuid,
) -> Result<bool, sqlx::Error> {
    sqlx::query_scalar(
        "SELECT EXISTS(
            SELECT 1
            FROM event_idempotency
            WHERE event_id = $1 AND website_id = $2
        )",
    )
    .bind(event_id)
    .bind(website_id)
    .fetch_one(pool)
    .await
}

pub async fn record_event_id(
    pool: &PgPool,
    event_id: Uuid,
    website_id: Uuid,
) -> Result<bool, sqlx::Error> {
    let inserted = sqlx::query_scalar::<_, Uuid>(
        "INSERT INTO event_idempotency (event_id, website_id, created_at)
         VALUES ($1, $2, NOW())
         ON CONFLICT DO NOTHING
         RETURNING event_id",
    )
    .bind(event_id)
    .bind(website_id)
    .fetch_optional(pool)
    .await?;

    Ok(inserted.is_some())
}

pub async fn cleanup_idempotency(pool: &PgPool, retention_days: i32) -> Result<i32, sqlx::Error> {
    sqlx::query_scalar("SELECT cleanup_event_idempotency($1)")
        .bind(retention_days)
        .fetch_one(pool)
        .await
}

pub async fn create_idempotency_partitions(
    pool: &PgPool,
    days_ahead: i32,
) -> Result<i32, sqlx::Error> {
    sqlx::query_scalar("SELECT create_event_idempotency_partitions($1)")
        .bind(days_ahead)
        .fetch_one(pool)
        .await
}

pub async fn cleanup_rate_limit_storage(pool: &PgPool) -> Result<i32, sqlx::Error> {
    sqlx::query_scalar("SELECT cleanup_rate_limit_storage()")
        .fetch_one(pool)
        .await
}

#[cfg(test)]
mod tests;
