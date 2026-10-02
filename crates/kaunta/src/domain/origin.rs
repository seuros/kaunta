use serde::{Deserialize, Serialize};
use time::OffsetDateTime;
use url::Url;

#[derive(Debug, Clone, thiserror::Error, PartialEq, Eq)]
pub enum OriginValidationError {
    #[error("origin cannot be empty")]
    Empty,
    #[error("origin cannot contain whitespace")]
    Whitespace,
    #[error("wildcard origins are not allowed")]
    Wildcard,
    #[error("origin must contain a valid host")]
    MissingHost,
    #[error("origin cannot contain a path, query, or fragment")]
    PathQueryOrFragment,
    #[error("invalid origin")]
    Invalid,
}

pub fn sanitize_origin(value: &str) -> Result<String, OriginValidationError> {
    let origin = value.trim().to_lowercase();
    if origin.is_empty() {
        return Err(OriginValidationError::Empty);
    }
    if origin.chars().any(char::is_whitespace) {
        return Err(OriginValidationError::Whitespace);
    }
    if origin == "*" {
        return Err(OriginValidationError::Wildcard);
    }

    let origin = origin
        .strip_prefix("https://")
        .or_else(|| origin.strip_prefix("http://"))
        .unwrap_or(&origin)
        .trim_end_matches('/')
        .to_owned();

    if origin.is_empty() {
        return Err(OriginValidationError::MissingHost);
    }

    let parsed =
        Url::parse(&format!("http://{origin}")).map_err(|_| OriginValidationError::Invalid)?;
    if parsed.host_str().is_none() {
        return Err(OriginValidationError::MissingHost);
    }
    if parsed.path() != "/" || parsed.query().is_some() || parsed.fragment().is_some() {
        return Err(OriginValidationError::PathQueryOrFragment);
    }

    Ok(origin)
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct TrustedOrigin {
    pub id: i32,
    pub domain: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub description: Option<String>,
    pub is_active: bool,
    pub created_at: OffsetDateTime,
    pub updated_at: OffsetDateTime,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TrustedOriginRequest {
    pub domain: String,
    #[serde(default)]
    pub description: Option<String>,
}

#[cfg(test)]
mod tests;
