use serde::{Deserialize, Serialize};
use serde_json::Value;
use time::OffsetDateTime;
use uuid::Uuid;

#[derive(Debug, Clone, Copy, Default, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "lowercase")]
pub enum ProxyMode {
    #[default]
    None,
    Xforwarded,
    Cloudflare,
}

impl ProxyMode {
    #[must_use]
    pub const fn as_str(self) -> &'static str {
        match self {
            Self::None => "none",
            Self::Xforwarded => "xforwarded",
            Self::Cloudflare => "cloudflare",
        }
    }
}

impl TryFrom<&str> for ProxyMode {
    type Error = WebsiteValidationError;

    fn try_from(value: &str) -> Result<Self, Self::Error> {
        match value {
            "none" => Ok(Self::None),
            "xforwarded" => Ok(Self::Xforwarded),
            "cloudflare" => Ok(Self::Cloudflare),
            _ => Err(WebsiteValidationError::InvalidProxyMode),
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct Website {
    pub website_id: Uuid,
    pub domain: String,
    pub name: String,
    pub allowed_domains: Vec<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub share_id: Option<String>,
    pub proxy_mode: ProxyMode,
    pub public_stats_enabled: bool,
    pub api_rate_limit_per_minute: i32,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub user_id: Option<Uuid>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub pending_delete_at: Option<OffsetDateTime>,
    pub created_at: OffsetDateTime,
    pub updated_at: OffsetDateTime,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct CreateWebsiteRequest {
    pub domain: String,
    #[serde(default)]
    pub name: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct UpdateWebsiteRequest {
    pub name: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct DomainRequest {
    pub domain: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct PublicStatsRequest {
    pub enabled: bool,
}

#[derive(Debug, thiserror::Error, Clone, PartialEq, Eq)]
pub enum WebsiteValidationError {
    #[error("invalid domain format: domain cannot be empty")]
    EmptyDomain,
    #[error("invalid domain format: domain cannot exceed 253 characters (DNS standard)")]
    DomainTooLong,
    #[error("invalid domain format: contains invalid characters")]
    InvalidDomainCharacters,
    #[error("invalid proxy mode")]
    InvalidProxyMode,
    #[error("allowed_domains must be a JSON array of strings")]
    InvalidAllowedDomains,
}

pub fn validate_domain(domain: &str) -> Result<(), WebsiteValidationError> {
    if domain.is_empty() {
        return Err(WebsiteValidationError::EmptyDomain);
    }
    if domain.len() > 253 {
        return Err(WebsiteValidationError::DomainTooLong);
    }
    if domain == "localhost" {
        return Ok(());
    }
    if domain.chars().any(|character| {
        !character.is_ascii_alphanumeric() && !matches!(character, '.' | '-' | ':')
    }) {
        return Err(WebsiteValidationError::InvalidDomainCharacters);
    }
    Ok(())
}

pub fn parse_allowed_domains(value: Value) -> Result<Vec<String>, WebsiteValidationError> {
    serde_json::from_value(value).map_err(|_| WebsiteValidationError::InvalidAllowedDomains)
}

#[cfg(test)]
mod tests;
