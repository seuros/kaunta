use std::collections::HashSet;

use crate::domain::website::{ProxyMode, Website, WebsiteValidationError, validate_domain};
use sqlx::{AssertSqlSafe, FromRow, PgPool, types::Json};
use time::OffsetDateTime;
use uuid::Uuid;

const SELECT_COLUMNS: &str = "
    website_id,
    domain,
    COALESCE(name, domain) AS name,
    COALESCE(allowed_domains, '[]'::jsonb) AS allowed_domains,
    share_id,
    COALESCE(proxy_mode, 'none') AS proxy_mode,
    COALESCE(public_stats_enabled, FALSE) AS public_stats_enabled,
    COALESCE(api_rate_limit_per_minute, 5000) AS api_rate_limit_per_minute,
    user_id,
    pending_delete_at,
    COALESCE(created_at, NOW()) AS created_at,
    COALESCE(updated_at, created_at, NOW()) AS updated_at
";

#[derive(Debug, thiserror::Error)]
pub enum WebsiteError {
    #[error(transparent)]
    Validation(#[from] WebsiteValidationError),
    #[error("website '{0}' not found")]
    NotFound(String),
    #[error("website with domain '{0}' already exists")]
    DuplicateDomain(String),
    #[error("domain '{0}' not found in allowed list")]
    AllowedDomainNotFound(String),
    #[error("cannot remove the last allowed domain (security: at least one domain must remain)")]
    LastAllowedDomain,
    #[error(transparent)]
    Database(#[from] sqlx::Error),
}

#[derive(Debug, FromRow)]
struct WebsiteRow {
    website_id: Uuid,
    domain: String,
    name: String,
    allowed_domains: Json<Vec<String>>,
    share_id: Option<String>,
    proxy_mode: String,
    public_stats_enabled: bool,
    api_rate_limit_per_minute: i32,
    user_id: Option<Uuid>,
    pending_delete_at: Option<OffsetDateTime>,
    created_at: OffsetDateTime,
    updated_at: OffsetDateTime,
}

impl TryFrom<WebsiteRow> for Website {
    type Error = WebsiteError;

    fn try_from(row: WebsiteRow) -> Result<Self, Self::Error> {
        Ok(Self {
            website_id: row.website_id,
            domain: row.domain,
            name: row.name,
            allowed_domains: row.allowed_domains.0,
            share_id: row.share_id,
            proxy_mode: ProxyMode::try_from(row.proxy_mode.as_str())?,
            public_stats_enabled: row.public_stats_enabled,
            api_rate_limit_per_minute: row.api_rate_limit_per_minute,
            user_id: row.user_id,
            pending_delete_at: row.pending_delete_at,
            created_at: row.created_at,
            updated_at: row.updated_at,
        })
    }
}

fn required(row: Option<WebsiteRow>, label: impl Into<String>) -> Result<Website, WebsiteError> {
    row.ok_or_else(|| WebsiteError::NotFound(label.into()))?
        .try_into()
}

pub async fn get_by_domain(
    pool: &PgPool,
    domain: &str,
    website_id: Option<Uuid>,
) -> Result<Website, WebsiteError> {
    let query = format!(
        "SELECT {SELECT_COLUMNS}
         FROM website
         WHERE deleted_at IS NULL
           AND (LOWER(domain) = LOWER($1) OR website_id = $2)
         LIMIT 1"
    );
    let row = sqlx::query_as::<_, WebsiteRow>(AssertSqlSafe(query))
        .bind(domain)
        .bind(website_id)
        .fetch_optional(pool)
        .await?;

    required(row, domain)
}

pub async fn get_by_id(pool: &PgPool, website_id: Uuid) -> Result<Website, WebsiteError> {
    let query = format!(
        "SELECT {SELECT_COLUMNS}
         FROM website
         WHERE deleted_at IS NULL AND website_id = $1
         LIMIT 1"
    );
    let row = sqlx::query_as::<_, WebsiteRow>(AssertSqlSafe(query))
        .bind(website_id)
        .fetch_optional(pool)
        .await?;

    required(row, website_id.to_string())
}

pub async fn get_by_share_id(pool: &PgPool, share_id: &str) -> Result<Website, WebsiteError> {
    let query = format!(
        "SELECT {SELECT_COLUMNS}
         FROM website
         WHERE deleted_at IS NULL AND share_id = $1
         LIMIT 1"
    );
    let row = sqlx::query_as::<_, WebsiteRow>(AssertSqlSafe(query))
        .bind(share_id)
        .fetch_optional(pool)
        .await?;

    required(row, share_id)
}

pub async fn list(pool: &PgPool) -> Result<Vec<Website>, WebsiteError> {
    let query = format!(
        "SELECT {SELECT_COLUMNS}
         FROM website
         WHERE deleted_at IS NULL
         ORDER BY LOWER(domain)"
    );
    let rows = sqlx::query_as::<_, WebsiteRow>(AssertSqlSafe(query))
        .fetch_all(pool)
        .await?;

    rows.into_iter().map(TryInto::try_into).collect()
}

/// Lists websites visible to `user_id`: the ones it owns plus shared
/// (unowned, `user_id IS NULL`) websites such as CLI-created sites without an
/// owner and the reserved self-tracking website.
pub async fn list_for_user(pool: &PgPool, user_id: Uuid) -> Result<Vec<Website>, WebsiteError> {
    let query = format!(
        "SELECT {SELECT_COLUMNS}
         FROM website
         WHERE deleted_at IS NULL AND (user_id = $1 OR user_id IS NULL)
         ORDER BY LOWER(domain)"
    );
    let rows = sqlx::query_as::<_, WebsiteRow>(AssertSqlSafe(query))
        .bind(user_id)
        .fetch_all(pool)
        .await?;

    rows.into_iter().map(TryInto::try_into).collect()
}

pub async fn list_page_for_user(
    pool: &PgPool,
    user_id: Uuid,
    limit: i64,
    offset: i64,
) -> Result<(Vec<Website>, i64), WebsiteError> {
    let total = sqlx::query_scalar(
        "SELECT COUNT(*) FROM website
         WHERE deleted_at IS NULL AND (user_id = $1 OR user_id IS NULL)",
    )
    .bind(user_id)
    .fetch_one(pool)
    .await?;
    let query = format!(
        "SELECT {SELECT_COLUMNS} FROM website
         WHERE deleted_at IS NULL AND (user_id = $1 OR user_id IS NULL)
         ORDER BY name, domain, website_id LIMIT $2 OFFSET $3"
    );
    let rows = sqlx::query_as::<_, WebsiteRow>(AssertSqlSafe(query))
        .bind(user_id)
        .bind(limit)
        .bind(offset)
        .fetch_all(pool)
        .await?;
    Ok((
        rows.into_iter()
            .map(TryInto::try_into)
            .collect::<Result<_, _>>()?,
        total,
    ))
}

pub async fn create(
    pool: &PgPool,
    domain: &str,
    name: &str,
    allowed_domains: &[String],
    user_id: Option<Uuid>,
) -> Result<Website, WebsiteError> {
    validate_domain(domain)?;

    let exists = sqlx::query_scalar::<_, bool>(
        "SELECT EXISTS(
            SELECT 1
            FROM website
            WHERE LOWER(domain) = LOWER($1) AND deleted_at IS NULL
        )",
    )
    .bind(domain)
    .fetch_one(pool)
    .await?;

    if exists {
        return Err(WebsiteError::DuplicateDomain(domain.to_owned()));
    }

    let name = if name.is_empty() { domain } else { name };
    let query = format!(
        "INSERT INTO website (domain, name, allowed_domains, user_id, created_at, updated_at)
         VALUES ($1, $2, $3, $4, NOW(), NOW())
         RETURNING {SELECT_COLUMNS}"
    );
    let row = sqlx::query_as::<_, WebsiteRow>(AssertSqlSafe(query))
        .bind(domain)
        .bind(name)
        .bind(Json(allowed_domains.to_vec()))
        .bind(user_id)
        .fetch_one(pool)
        .await?;

    row.try_into()
}

pub async fn update_name(
    pool: &PgPool,
    website_id: Uuid,
    name: &str,
) -> Result<Website, WebsiteError> {
    let query = format!(
        "UPDATE website
         SET name = $2, updated_at = NOW()
         WHERE website_id = $1 AND deleted_at IS NULL
         RETURNING {SELECT_COLUMNS}"
    );
    let row = sqlx::query_as::<_, WebsiteRow>(AssertSqlSafe(query))
        .bind(website_id)
        .bind(name)
        .fetch_optional(pool)
        .await?;

    required(row, website_id.to_string())
}

pub async fn set_allowed_domains(
    pool: &PgPool,
    website_id: Uuid,
    domains: &[String],
) -> Result<Website, WebsiteError> {
    let query = format!(
        "UPDATE website
         SET allowed_domains = $2, updated_at = NOW()
         WHERE website_id = $1 AND deleted_at IS NULL
         RETURNING {SELECT_COLUMNS}"
    );
    let row = sqlx::query_as::<_, WebsiteRow>(AssertSqlSafe(query))
        .bind(website_id)
        .bind(Json(domains.to_vec()))
        .fetch_optional(pool)
        .await?;

    required(row, website_id.to_string())
}

pub async fn add_allowed_domains(
    pool: &PgPool,
    website_id: Uuid,
    domains: &[String],
) -> Result<Website, WebsiteError> {
    let website = get_by_id(pool, website_id).await?;
    let mut known = website
        .allowed_domains
        .iter()
        .map(|domain| domain.to_lowercase())
        .collect::<HashSet<_>>();
    let mut merged = website.allowed_domains;

    for domain in domains {
        if known.insert(domain.to_lowercase()) {
            merged.push(domain.clone());
        }
    }

    set_allowed_domains(pool, website_id, &merged).await
}

pub async fn remove_allowed_domain(
    pool: &PgPool,
    website_id: Uuid,
    domain_to_remove: &str,
) -> Result<Website, WebsiteError> {
    let website = get_by_id(pool, website_id).await?;
    let mut found = false;
    let domains = website
        .allowed_domains
        .into_iter()
        .filter(|domain| {
            let keep = !domain.eq_ignore_ascii_case(domain_to_remove);
            found |= !keep;
            keep
        })
        .collect::<Vec<_>>();

    if !found {
        return Err(WebsiteError::AllowedDomainNotFound(
            domain_to_remove.to_owned(),
        ));
    }
    if domains.is_empty() {
        return Err(WebsiteError::LastAllowedDomain);
    }

    set_allowed_domains(pool, website_id, &domains).await
}

pub async fn set_public_stats_enabled(
    pool: &PgPool,
    website_id: Uuid,
    enabled: bool,
) -> Result<Website, WebsiteError> {
    let query = format!(
        "UPDATE website
         SET public_stats_enabled = $2, updated_at = NOW()
         WHERE website_id = $1 AND deleted_at IS NULL
         RETURNING {SELECT_COLUMNS}"
    );
    let row = sqlx::query_as::<_, WebsiteRow>(AssertSqlSafe(query))
        .bind(website_id)
        .bind(enabled)
        .fetch_optional(pool)
        .await?;

    required(row, website_id.to_string())
}

pub async fn set_proxy_mode(
    pool: &PgPool,
    website_id: Uuid,
    mode: ProxyMode,
) -> Result<Website, WebsiteError> {
    let query = format!(
        "UPDATE website
         SET proxy_mode = $2, updated_at = NOW()
         WHERE website_id = $1 AND deleted_at IS NULL
         RETURNING {SELECT_COLUMNS}"
    );
    let row = sqlx::query_as::<_, WebsiteRow>(AssertSqlSafe(query))
        .bind(website_id)
        .bind(mode.as_str())
        .fetch_optional(pool)
        .await?;

    required(row, website_id.to_string())
}

/// What the delete-policy trigger decided for a delete attempt.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum DeleteOutcome {
    Deleted(OffsetDateTime),
    PendingSince(OffsetDateTime),
}

/// Attempt a soft delete; the website_delete_policy trigger converts the
/// attempt into a pending mark when the site still has event data and the
/// 30-day grace has not elapsed. The returned outcome reports what the
/// database actually did.
pub async fn soft_delete(pool: &PgPool, website_id: Uuid) -> Result<DeleteOutcome, WebsiteError> {
    let row: Option<(Option<OffsetDateTime>, Option<OffsetDateTime>)> = sqlx::query_as(
        "UPDATE website
         SET deleted_at = NOW(), updated_at = NOW()
         WHERE website_id = $1 AND deleted_at IS NULL
         RETURNING deleted_at, pending_delete_at",
    )
    .bind(website_id)
    .fetch_optional(pool)
    .await?;
    match row.ok_or_else(|| WebsiteError::NotFound(website_id.to_string()))? {
        (Some(deleted_at), _) => Ok(DeleteOutcome::Deleted(deleted_at)),
        (None, Some(pending_since)) => Ok(DeleteOutcome::PendingSince(pending_since)),
        (None, None) => Err(WebsiteError::NotFound(website_id.to_string())),
    }
}

pub async fn cancel_pending_delete(pool: &PgPool, website_id: Uuid) -> Result<bool, WebsiteError> {
    let result = sqlx::query(
        "UPDATE website
         SET pending_delete_at = NULL, updated_at = NOW()
         WHERE website_id = $1 AND deleted_at IS NULL AND pending_delete_at IS NOT NULL",
    )
    .bind(website_id)
    .execute(pool)
    .await?;
    Ok(result.rows_affected() > 0)
}

pub async fn events_count(pool: &PgPool, website_id: Uuid) -> Result<i64, WebsiteError> {
    Ok(
        sqlx::query_scalar("SELECT COUNT(*)::bigint FROM website_event WHERE website_id = $1")
            .bind(website_id)
            .fetch_one(pool)
            .await?,
    )
}

pub async fn validate_origin(
    pool: &PgPool,
    website_id: Uuid,
    origin: &str,
) -> Result<bool, sqlx::Error> {
    sqlx::query_scalar("SELECT validate_origin($1, $2)")
        .bind(website_id)
        .bind(origin)
        .fetch_one(pool)
        .await
}

/// Returns whether `user_id` may access `website_id`: the website must be
/// active and either owned by the user or shared (`user_id IS NULL`).
pub async fn is_accessible_by(
    pool: &PgPool,
    website_id: Uuid,
    user_id: Uuid,
) -> Result<bool, sqlx::Error> {
    sqlx::query_scalar(
        "SELECT EXISTS(
            SELECT 1
            FROM website
            WHERE website_id = $1
              AND (user_id = $2 OR user_id IS NULL)
              AND deleted_at IS NULL
        )",
    )
    .bind(website_id)
    .bind(user_id)
    .fetch_one(pool)
    .await
}

/// Returns whether `website_id` exists and has not been soft-deleted.
/// Used by public stats endpoints, which have no user scope.
pub async fn exists_active(pool: &PgPool, website_id: Uuid) -> Result<bool, sqlx::Error> {
    sqlx::query_scalar(
        "SELECT EXISTS(
            SELECT 1
            FROM website
            WHERE website_id = $1 AND deleted_at IS NULL
        )",
    )
    .bind(website_id)
    .fetch_one(pool)
    .await
}

#[cfg(test)]
mod tests;

pub async fn find_deleted(pool: &PgPool, identifier: &str) -> Result<Website, WebsiteError> {
    let website_id = Uuid::parse_str(identifier).ok();
    let query = format!(
        "SELECT {SELECT_COLUMNS}
         FROM website
         WHERE deleted_at IS NOT NULL
           AND (LOWER(domain) = LOWER($1) OR website_id = $2)
         ORDER BY deleted_at DESC
         LIMIT 1"
    );
    let row = sqlx::query_as::<_, WebsiteRow>(AssertSqlSafe(query))
        .bind(identifier)
        .bind(website_id)
        .fetch_optional(pool)
        .await?;
    required(row, identifier)
}

/// Undo a soft delete, also clearing any pending mark. Fails with
/// `DuplicateDomain` when a live website with the same domain exists.
pub async fn restore(pool: &PgPool, website_id: Uuid) -> Result<Website, WebsiteError> {
    let query = format!(
        "UPDATE website
         SET deleted_at = NULL, pending_delete_at = NULL, updated_at = NOW()
         WHERE website_id = $1 AND deleted_at IS NOT NULL
         RETURNING {SELECT_COLUMNS}"
    );
    let result = sqlx::query_as::<_, WebsiteRow>(AssertSqlSafe(query))
        .bind(website_id)
        .fetch_optional(pool)
        .await;
    match result {
        Ok(row) => required(row, website_id.to_string()),
        Err(sqlx::Error::Database(error)) if error.is_unique_violation() => {
            let domain: String =
                sqlx::query_scalar("SELECT domain FROM website WHERE website_id = $1")
                    .bind(website_id)
                    .fetch_one(pool)
                    .await?;
            Err(WebsiteError::DuplicateDomain(domain))
        }
        Err(error) => Err(error.into()),
    }
}
