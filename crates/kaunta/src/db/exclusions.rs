//! Runtime-editable tracking exclusions, for operators whose address
//! changes and who cannot keep editing `excluded_ips` in the config file.

use sqlx::PgPool;
use time::OffsetDateTime;
use uuid::Uuid;

#[derive(Debug, Clone, serde::Serialize, sqlx::FromRow)]
pub struct Exclusion {
    pub excluded_address_id: Uuid,
    pub rule: String,
    pub note: String,
    #[serde(with = "time::serde::rfc3339")]
    pub created_at: OffsetDateTime,
}

pub async fn list(pool: &PgPool) -> Result<Vec<Exclusion>, sqlx::Error> {
    sqlx::query_as::<_, Exclusion>(
        "SELECT excluded_address_id, rule, note, created_at
         FROM excluded_address ORDER BY created_at DESC",
    )
    .fetch_all(pool)
    .await
}

pub async fn rules(pool: &PgPool) -> Result<Vec<String>, sqlx::Error> {
    sqlx::query_scalar("SELECT rule FROM excluded_address")
        .fetch_all(pool)
        .await
}

/// Adds a rule, or refreshes the note of an identical existing one.
pub async fn add(pool: &PgPool, rule: &str, note: &str) -> Result<Exclusion, sqlx::Error> {
    sqlx::query_as::<_, Exclusion>(
        "INSERT INTO excluded_address (rule, note) VALUES ($1, $2)
         ON CONFLICT (rule) DO UPDATE SET note = EXCLUDED.note
         RETURNING excluded_address_id, rule, note, created_at",
    )
    .bind(rule)
    .bind(note)
    .fetch_one(pool)
    .await
}

/// Removes a rule by id or by its exact text. Returns the removed rule.
pub async fn remove(pool: &PgPool, identifier: &str) -> Result<Option<String>, sqlx::Error> {
    sqlx::query_scalar(
        "DELETE FROM excluded_address
         WHERE excluded_address_id = $1::uuid OR rule = $2
         RETURNING rule",
    )
    .bind(Uuid::parse_str(identifier).unwrap_or(Uuid::nil()))
    .bind(identifier)
    .fetch_optional(pool)
    .await
}
