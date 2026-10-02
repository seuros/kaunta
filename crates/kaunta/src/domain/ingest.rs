use std::collections::BTreeMap;

use serde::{Deserialize, Deserializer, Serialize};
use serde_json::Value;
use time::{Duration, OffsetDateTime};
use uuid::Uuid;

pub const MAX_BATCH_EVENTS: usize = 100;
pub const MAX_PROPERTIES_BYTES: usize = 100 * 1024;
pub const MAX_PROPERTIES_KEYS: usize = 100;
pub const MAX_PROPERTIES_DEPTH: usize = 5;

/// Maximum characters of `session.language` (`VARCHAR(35)` in the schema).
pub const MAX_LOCALE_LENGTH: usize = 35;
/// Maximum characters of `session.screen` (`VARCHAR(11)` in the schema).
pub const MAX_SCREEN_LENGTH: usize = 11;

/// Treat an explicit JSON `null` as the field's default, matching Go's
/// zero-value decoding, instead of rejecting the whole payload.
fn deserialize_null_default<'de, D, T>(deserializer: D) -> Result<T, D::Error>
where
    D: Deserializer<'de>,
    T: Default + Deserialize<'de>,
{
    Option::<T>::deserialize(deserializer).map(Option::unwrap_or_default)
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct IngestContext {
    #[serde(default, deserialize_with = "deserialize_null_default")]
    pub locale: String,
    #[serde(default, deserialize_with = "deserialize_null_default")]
    pub screen: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct IngestPayload {
    pub event: String,
    pub visitor_id: String,
    #[serde(default, deserialize_with = "deserialize_null_default")]
    pub url: String,
    #[serde(default, deserialize_with = "deserialize_null_default")]
    pub hostname: String,
    #[serde(default, deserialize_with = "deserialize_null_default")]
    pub referrer: String,
    #[serde(default, deserialize_with = "deserialize_null_default")]
    pub title: String,
    #[serde(default)]
    pub user_id: Option<String>,
    #[serde(default)]
    pub session_id: Option<String>,
    #[serde(default)]
    pub event_id: Option<String>,
    #[serde(default)]
    pub timestamp: Option<i64>,
    #[serde(default, deserialize_with = "deserialize_null_default")]
    pub properties: BTreeMap<String, Value>,
    #[serde(default)]
    pub context: Option<IngestContext>,
    #[serde(default)]
    pub utm_source: Option<String>,
    #[serde(default)]
    pub utm_medium: Option<String>,
    #[serde(default)]
    pub utm_campaign: Option<String>,
    #[serde(default)]
    pub utm_term: Option<String>,
    #[serde(default)]
    pub utm_content: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct BatchIngestRequest {
    pub events: Vec<IngestPayload>,
}

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct BatchIngestResponse {
    pub accepted: usize,
    pub failed: usize,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    pub errors: Vec<BatchError>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct BatchError {
    pub index: usize,
    pub error: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct IngestResponse {
    pub status: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub session_id: Option<Uuid>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub visit_id: Option<Uuid>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub idempotent: Option<bool>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub bot_detected: Option<bool>,
}

#[derive(Debug, Clone, thiserror::Error, PartialEq, Eq)]
pub enum IngestValidationError {
    #[error("event is required")]
    MissingEvent,
    #[error("event exceeds maximum length of 50")]
    EventTooLong,
    #[error("visitor_id is required")]
    MissingVisitorId,
    #[error("visitor_id exceeds maximum length of 500")]
    VisitorIdTooLong,
    #[error("url is required for page_view events")]
    MissingPageViewUrl,
    #[error("{0} exceeds maximum length of {1}")]
    FieldTooLong(&'static str, usize),
    #[error("timestamp must be within 30 days of now")]
    TimestampOutOfRange,
    #[error("event_id must be a valid UUID v4")]
    InvalidEventId,
    #[error("properties exceed 100KB limit")]
    PropertiesTooLarge,
    #[error("properties exceed 100 keys limit")]
    TooManyProperties,
    #[error("reserved property key: {0}")]
    ReservedProperty(String),
    #[error("properties exceed max depth of 5")]
    PropertiesTooDeep,
    #[error("maximum 100 events per batch")]
    BatchTooLarge,
    #[error("events array is required")]
    EmptyBatch,
}

pub fn validate_ingest(
    payload: &IngestPayload,
    now: OffsetDateTime,
) -> Result<(), IngestValidationError> {
    if payload.event.is_empty() {
        return Err(IngestValidationError::MissingEvent);
    }
    if payload.event.len() > 50 {
        return Err(IngestValidationError::EventTooLong);
    }
    if payload.visitor_id.is_empty() {
        return Err(IngestValidationError::MissingVisitorId);
    }
    if payload.visitor_id.len() > 500 {
        return Err(IngestValidationError::VisitorIdTooLong);
    }
    if payload.event == "page_view" && payload.url.is_empty() {
        return Err(IngestValidationError::MissingPageViewUrl);
    }

    validate_length("url", &payload.url, 2_000)?;
    validate_length("hostname", &payload.hostname, 100)?;
    validate_length("referrer", &payload.referrer, 2_000)?;
    validate_length("title", &payload.title, 500)?;
    validate_optional_length("user_id", payload.user_id.as_deref(), 500)?;
    validate_optional_length("session_id", payload.session_id.as_deref(), 100)?;
    validate_optional_length("utm_source", payload.utm_source.as_deref(), 100)?;
    validate_optional_length("utm_medium", payload.utm_medium.as_deref(), 100)?;
    validate_optional_length("utm_campaign", payload.utm_campaign.as_deref(), 100)?;
    validate_optional_length("utm_term", payload.utm_term.as_deref(), 100)?;
    validate_optional_length("utm_content", payload.utm_content.as_deref(), 100)?;
    if let Some(context) = &payload.context {
        validate_length("context.locale", &context.locale, MAX_LOCALE_LENGTH)?;
        validate_length("context.screen", &context.screen, MAX_SCREEN_LENGTH)?;
    }

    if payload.event_id.as_deref().is_some_and(|value| {
        Uuid::parse_str(value)
            .ok()
            .and_then(|uuid| uuid.get_version())
            != Some(uuid::Version::Random)
    }) {
        return Err(IngestValidationError::InvalidEventId);
    }

    if let Some(timestamp) = payload.timestamp {
        let event_time = OffsetDateTime::from_unix_timestamp(timestamp)
            .map_err(|_| IngestValidationError::TimestampOutOfRange)?;
        if event_time < now - Duration::days(30) || event_time > now + Duration::days(30) {
            return Err(IngestValidationError::TimestampOutOfRange);
        }
    }

    validate_properties(&payload.properties)
}

pub fn validate_batch(batch: &BatchIngestRequest) -> Result<(), IngestValidationError> {
    if batch.events.is_empty() {
        return Err(IngestValidationError::EmptyBatch);
    }
    if batch.events.len() > MAX_BATCH_EVENTS {
        return Err(IngestValidationError::BatchTooLarge);
    }
    Ok(())
}

fn validate_length(
    field: &'static str,
    value: &str,
    maximum: usize,
) -> Result<(), IngestValidationError> {
    if value.len() > maximum {
        return Err(IngestValidationError::FieldTooLong(field, maximum));
    }
    Ok(())
}

fn validate_optional_length(
    field: &'static str,
    value: Option<&str>,
    maximum: usize,
) -> Result<(), IngestValidationError> {
    value.map_or(Ok(()), |value| validate_length(field, value, maximum))
}

fn validate_properties(properties: &BTreeMap<String, Value>) -> Result<(), IngestValidationError> {
    if properties.len() > MAX_PROPERTIES_KEYS {
        return Err(IngestValidationError::TooManyProperties);
    }
    if properties
        .keys()
        .any(|key| key.starts_with('$') || key.starts_with('_'))
    {
        let key = properties
            .keys()
            .find(|key| key.starts_with('$') || key.starts_with('_'))
            .expect("reserved key exists");
        return Err(IngestValidationError::ReservedProperty(key.clone()));
    }
    if serde_json::to_vec(properties).map_or(MAX_PROPERTIES_BYTES + 1, |encoded| encoded.len())
        > MAX_PROPERTIES_BYTES
    {
        return Err(IngestValidationError::PropertiesTooLarge);
    }
    if properties
        .values()
        .map(|value| json_depth(value, 1))
        .max()
        .unwrap_or(0)
        > MAX_PROPERTIES_DEPTH
    {
        return Err(IngestValidationError::PropertiesTooDeep);
    }
    Ok(())
}

fn json_depth(value: &Value, current: usize) -> usize {
    match value {
        Value::Array(values) => values
            .iter()
            .map(|value| json_depth(value, current + 1))
            .max()
            .unwrap_or(current),
        Value::Object(values) => values
            .values()
            .map(|value| json_depth(value, current + 1))
            .max()
            .unwrap_or(current),
        _ => current,
    }
}

#[cfg(test)]
mod tests;
