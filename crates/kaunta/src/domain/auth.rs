use serde::{Deserialize, Serialize};
use time::OffsetDateTime;
use uuid::Uuid;

pub const SESSION_COOKIE: &str = "kaunta_session";
pub const CSRF_COOKIE: &str = "kaunta_csrf";
pub const SESSION_DURATION_DAYS: i64 = 7;

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct LoginRequest {
    pub username: String,
    pub password: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct User {
    pub user_id: Uuid,
    pub username: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub name: Option<String>,
    pub created_at: OffsetDateTime,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub updated_at: Option<OffsetDateTime>,
}

#[derive(Debug, Clone)]
pub struct UserCredential {
    pub user: User,
    pub password_hash: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AuthenticatedUser {
    pub user_id: Uuid,
    pub username: String,
    pub session_id: Uuid,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub name: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub created_at: Option<OffsetDateTime>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct LoginResponse {
    pub success: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub message: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub user: Option<AuthenticatedUser>,
}

#[derive(Debug, Clone)]
pub struct NewUserSession {
    pub session_id: Uuid,
    pub user_id: Uuid,
    pub token_hash: String,
    pub expires_at: OffsetDateTime,
    pub user_agent: Option<String>,
    pub ip_address: Option<String>,
}
