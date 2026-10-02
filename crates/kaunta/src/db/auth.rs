use crate::domain::auth::{AuthenticatedUser, NewUserSession, User, UserCredential};
use sha2::{Digest, Sha256};
use sqlx::{FromRow, PgPool};
use time::OffsetDateTime;
use uuid::Uuid;

#[derive(Debug, FromRow)]
struct UserRow {
    user_id: Uuid,
    username: String,
    name: Option<String>,
    created_at: OffsetDateTime,
    updated_at: Option<OffsetDateTime>,
}

impl From<UserRow> for User {
    fn from(row: UserRow) -> Self {
        Self {
            user_id: row.user_id,
            username: row.username,
            name: row.name,
            created_at: row.created_at,
            updated_at: row.updated_at,
        }
    }
}

#[derive(Debug, FromRow)]
struct CredentialRow {
    user_id: Uuid,
    username: String,
    name: Option<String>,
    password_hash: String,
    created_at: OffsetDateTime,
    updated_at: Option<OffsetDateTime>,
}

#[derive(Debug, FromRow)]
struct AuthenticatedUserRow {
    user_id: Uuid,
    username: String,
    session_id: Uuid,
    name: Option<String>,
    created_at: Option<OffsetDateTime>,
}

#[must_use]
pub fn hash_token(token: &str) -> String {
    hex::encode(Sha256::digest(token.as_bytes()))
}

pub async fn has_any_users(pool: &PgPool) -> Result<bool, sqlx::Error> {
    let table_exists = sqlx::query_scalar::<_, bool>(
        "SELECT EXISTS (
            SELECT 1
            FROM information_schema.tables
            WHERE table_schema = 'public' AND table_name = 'users'
        )",
    )
    .fetch_one(pool)
    .await?;

    if !table_exists {
        return Ok(false);
    }

    sqlx::query_scalar("SELECT EXISTS(SELECT 1 FROM users LIMIT 1)")
        .fetch_one(pool)
        .await
}

pub async fn create_user(
    pool: &PgPool,
    username: &str,
    password: &str,
    name: &str,
) -> Result<User, sqlx::Error> {
    let row = sqlx::query_as::<_, UserRow>(
        "INSERT INTO users (username, password_hash, name)
         VALUES ($1, hash_password($2), NULLIF($3, ''))
         RETURNING user_id, username, name, created_at, updated_at",
    )
    .bind(username)
    .bind(password)
    .bind(name)
    .fetch_one(pool)
    .await?;

    Ok(row.into())
}

pub async fn list_users(pool: &PgPool) -> Result<Vec<User>, sqlx::Error> {
    let rows = sqlx::query_as::<_, UserRow>(
        "SELECT user_id, username, name, created_at, updated_at
         FROM users
         ORDER BY created_at DESC",
    )
    .fetch_all(pool)
    .await?;

    Ok(rows.into_iter().map(Into::into).collect())
}

pub async fn get_user_by_username(
    pool: &PgPool,
    username: &str,
) -> Result<Option<User>, sqlx::Error> {
    let row = sqlx::query_as::<_, UserRow>(
        "SELECT user_id, username, name, created_at, updated_at
         FROM users
         WHERE username = $1",
    )
    .bind(username)
    .fetch_optional(pool)
    .await?;

    Ok(row.map(Into::into))
}

pub async fn delete_user_by_username(pool: &PgPool, username: &str) -> Result<bool, sqlx::Error> {
    let result = sqlx::query("DELETE FROM users WHERE username = $1")
        .bind(username)
        .execute(pool)
        .await?;

    Ok(result.rows_affected() > 0)
}

pub async fn reset_password(
    pool: &PgPool,
    username: &str,
    password: &str,
) -> Result<bool, sqlx::Error> {
    let mut transaction = pool.begin().await?;
    let updated = sqlx::query_scalar::<_, Uuid>(
        "UPDATE users
         SET password_hash = hash_password($1), updated_at = NOW()
         WHERE username = $2
         RETURNING user_id",
    )
    .bind(password)
    .bind(username)
    .fetch_optional(&mut *transaction)
    .await?;

    let Some(user_id) = updated else {
        transaction.rollback().await?;
        return Ok(false);
    };

    sqlx::query("DELETE FROM user_sessions WHERE user_id = $1")
        .bind(user_id)
        .execute(&mut *transaction)
        .await?;
    transaction.commit().await?;
    Ok(true)
}

pub async fn find_credential(
    pool: &PgPool,
    username: &str,
) -> Result<Option<UserCredential>, sqlx::Error> {
    let row = sqlx::query_as::<_, CredentialRow>(
        "SELECT user_id, username, name, password_hash, created_at, updated_at
         FROM users
         WHERE username = $1",
    )
    .bind(username)
    .fetch_optional(pool)
    .await?;

    Ok(row.map(|row| UserCredential {
        user: User {
            user_id: row.user_id,
            username: row.username,
            name: row.name,
            created_at: row.created_at,
            updated_at: row.updated_at,
        },
        password_hash: row.password_hash,
    }))
}

pub async fn create_session(pool: &PgPool, session: &NewUserSession) -> Result<(), sqlx::Error> {
    sqlx::query(
        "INSERT INTO user_sessions (
            session_id,
            user_id,
            token_hash,
            expires_at,
            user_agent,
            ip_address
         )
         VALUES ($1, $2, $3, $4, $5, $6::inet)",
    )
    .bind(session.session_id)
    .bind(session.user_id)
    .bind(&session.token_hash)
    .bind(session.expires_at)
    .bind(&session.user_agent)
    .bind(&session.ip_address)
    .execute(pool)
    .await?;

    Ok(())
}

pub async fn validate_session(
    pool: &PgPool,
    token_hash: &str,
) -> Result<Option<AuthenticatedUser>, sqlx::Error> {
    let row = sqlx::query_as::<_, AuthenticatedUserRow>(
        "SELECT
            valid.user_id,
            valid.username,
            valid.session_id,
            users.name,
            users.created_at
         FROM validate_session($1) AS valid
         JOIN users ON users.user_id = valid.user_id",
    )
    .bind(token_hash)
    .fetch_optional(pool)
    .await?;

    Ok(row.map(|row| AuthenticatedUser {
        user_id: row.user_id,
        username: row.username,
        session_id: row.session_id,
        name: row.name,
        created_at: row.created_at,
    }))
}

pub async fn get_user(pool: &PgPool, user_id: Uuid) -> Result<Option<User>, sqlx::Error> {
    let row = sqlx::query_as::<_, UserRow>(
        "SELECT user_id, username, name, created_at, updated_at
         FROM users
         WHERE user_id = $1",
    )
    .bind(user_id)
    .fetch_optional(pool)
    .await?;

    Ok(row.map(Into::into))
}

pub async fn delete_session(pool: &PgPool, session_id: Uuid) -> Result<bool, sqlx::Error> {
    let result = sqlx::query("DELETE FROM user_sessions WHERE session_id = $1")
        .bind(session_id)
        .execute(pool)
        .await?;

    Ok(result.rows_affected() > 0)
}

pub async fn cleanup_expired_sessions(pool: &PgPool) -> Result<i32, sqlx::Error> {
    sqlx::query_scalar("SELECT cleanup_expired_sessions()")
        .fetch_one(pool)
        .await
}
