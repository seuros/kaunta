use crate::domain::{SELF_WEBSITE_ID, origin::sanitize_origin};
use sqlx::{AssertSqlSafe, PgPool, Postgres, Transaction, types::Json};
use uuid::Uuid;

const SELF_DOMAIN: &str = "self";
const SELF_NAME: &str = "Kaunta Dashboard";
const SELF_ALLOWED_DOMAINS: &[&str] = &["localhost", "http://localhost", "https://localhost"];

pub async fn synchronize(pool: &PgPool, trusted_origins: &[String]) -> Result<(), sqlx::Error> {
    sync_trusted_origins(pool, trusted_origins).await;
    ensure_self_website(pool).await
}

pub async fn sync_trusted_origins(pool: &PgPool, origins: &[String]) {
    for configured_origin in origins {
        let origin = match sanitize_origin(configured_origin) {
            Ok(origin) => origin,
            Err(error) => {
                tracing::warn!(
                    origin = configured_origin,
                    %error,
                    "ignoring invalid configured trusted origin"
                );
                continue;
            }
        };

        if let Err(error) = sqlx::query(
            "INSERT INTO trusted_origin (domain, is_active, description)
             VALUES ($1, TRUE, 'Auto-synced from TRUSTED_ORIGINS env var')
             ON CONFLICT (domain) DO UPDATE
             SET is_active = TRUE, updated_at = NOW()",
        )
        .bind(&origin)
        .execute(pool)
        .await
        {
            tracing::warn!(%origin, %error, "failed to synchronize trusted origin");
        }
    }
}

pub async fn ensure_self_website(pool: &PgPool) -> Result<(), sqlx::Error> {
    let self_id = Uuid::parse_str(SELF_WEBSITE_ID).expect("SELF_WEBSITE_ID must be a UUID");
    let mut transaction = pool.begin().await?;

    let fixed_exists: bool =
        sqlx::query_scalar("SELECT EXISTS(SELECT 1 FROM website WHERE website_id = $1)")
            .bind(self_id)
            .fetch_one(&mut *transaction)
            .await?;
    if fixed_exists {
        transaction.commit().await?;
        return Ok(());
    }

    let old_self = sqlx::query_as::<_, (Uuid, Option<String>)>(
        "SELECT website_id, share_id
         FROM website
         WHERE LOWER(domain) = 'self' AND deleted_at IS NULL
         LIMIT 1
         FOR UPDATE",
    )
    .fetch_optional(&mut *transaction)
    .await?;

    if let Some((old_id, share_id)) = old_self {
        migrate_legacy_self_website(&mut transaction, old_id, self_id, share_id).await?;
    } else {
        insert_self_website(&mut transaction, self_id).await?;
    }

    transaction.commit().await
}

async fn insert_self_website(
    transaction: &mut Transaction<'_, Postgres>,
    self_id: Uuid,
) -> Result<(), sqlx::Error> {
    let allowed_domains = SELF_ALLOWED_DOMAINS
        .iter()
        .map(|value| (*value).to_owned())
        .collect::<Vec<_>>();
    sqlx::query(
        "INSERT INTO website (
             website_id, domain, name, allowed_domains, created_at, updated_at
         )
         VALUES ($1, $2, $3, $4, NOW(), NOW())",
    )
    .bind(self_id)
    .bind(SELF_DOMAIN)
    .bind(SELF_NAME)
    .bind(Json(allowed_domains))
    .execute(&mut **transaction)
    .await?;
    Ok(())
}

async fn migrate_legacy_self_website(
    transaction: &mut Transaction<'_, Postgres>,
    old_id: Uuid,
    self_id: Uuid,
    share_id: Option<String>,
) -> Result<(), sqlx::Error> {
    let temporary_domain = format!("self-legacy-{old_id}");
    sqlx::query(
        "UPDATE website
         SET domain = $2, share_id = NULL, updated_at = NOW()
         WHERE website_id = $1",
    )
    .bind(old_id)
    .bind(&temporary_domain)
    .execute(&mut **transaction)
    .await?;

    sqlx::query(
        "INSERT INTO website (
             website_id, domain, name, share_id, allowed_domains, created_at, updated_at,
             deleted_at, proxy_mode, user_id, api_rate_limit_per_minute, public_stats_enabled
         )
         SELECT
             $2, 'self', name, NULL, allowed_domains, created_at, NOW(),
             NULL, proxy_mode, user_id, api_rate_limit_per_minute, public_stats_enabled
         FROM website
         WHERE website_id = $1",
    )
    .bind(old_id)
    .bind(self_id)
    .execute(&mut **transaction)
    .await?;

    for table in [
        "session",
        "website_event",
        "goals",
        "goal_completions",
        "api_keys",
        "event_idempotency",
        "bot_detection_log",
    ] {
        let query = format!("UPDATE {table} SET website_id = $2 WHERE website_id = $1");
        sqlx::query(AssertSqlSafe(query))
            .bind(old_id)
            .bind(self_id)
            .execute(&mut **transaction)
            .await?;
    }

    sqlx::query(
        "UPDATE realtime_stats_cache
         SET website_id = $2
         WHERE website_id = $1",
    )
    .bind(old_id)
    .bind(self_id)
    .execute(&mut **transaction)
    .await?;

    sqlx::query("UPDATE website SET deleted_at = NOW() WHERE website_id = $1")
        .bind(old_id)
        .execute(&mut **transaction)
        .await?;

    sqlx::query("DELETE FROM website WHERE website_id = $1")
        .bind(old_id)
        .execute(&mut **transaction)
        .await?;

    sqlx::query("UPDATE website SET share_id = $2 WHERE website_id = $1")
        .bind(self_id)
        .bind(share_id)
        .execute(&mut **transaction)
        .await?;

    Ok(())
}
