use serde::{Deserialize, Serialize};
use time::OffsetDateTime;
use uuid::Uuid;

pub const API_KEY_PREFIX: &str = "kaunta_live_";
pub const API_KEY_RANDOM_BYTES: usize = 32;
pub const API_KEY_DISPLAY_LENGTH: usize = 16;

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ApiKey {
    pub key_id: Uuid,
    pub website_id: Uuid,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub created_by: Option<Uuid>,
    pub key_prefix: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub name: Option<String>,
    pub scopes: Vec<String>,
    pub rate_limit_per_minute: i32,
    pub created_at: OffsetDateTime,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub last_used_at: Option<OffsetDateTime>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub revoked_at: Option<OffsetDateTime>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub expires_at: Option<OffsetDateTime>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub website_rate_limit: Option<i32>,
}

impl ApiKey {
    #[must_use]
    pub fn has_scope(&self, scope: &str) -> bool {
        self.scopes.iter().any(|candidate| candidate == scope)
    }

    #[must_use]
    pub fn is_valid_at(&self, now: OffsetDateTime) -> bool {
        self.revoked_at.is_none() && self.expires_at.is_none_or(|expires_at| expires_at > now)
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct CreateApiKeyRequest {
    #[serde(default)]
    pub name: Option<String>,
    #[serde(default = "default_scopes")]
    pub scopes: Vec<String>,
    #[serde(default)]
    pub expires_at: Option<OffsetDateTime>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ApiKeyCreateResult {
    pub api_key: String,
    pub key: ApiKey,
}

#[derive(Debug, Clone, thiserror::Error, PartialEq, Eq)]
pub enum ApiKeyValidationError {
    #[error("invalid scope: {0} (valid: ingest, stats)")]
    InvalidScope(String),
}

#[must_use]
pub fn default_scopes() -> Vec<String> {
    vec!["ingest".to_owned()]
}

pub fn validate_scopes(scopes: &[String]) -> Result<(), ApiKeyValidationError> {
    for scope in scopes {
        if !matches!(scope.as_str(), "ingest" | "stats") {
            return Err(ApiKeyValidationError::InvalidScope(scope.clone()));
        }
    }
    Ok(())
}
