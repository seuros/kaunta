use serde::{Deserialize, Serialize};
use time::OffsetDateTime;
use uuid::Uuid;

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct Goal {
    pub id: Uuid,
    pub website_id: Uuid,
    pub name: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub target_url: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub target_event: Option<String>,
    #[serde(with = "time::serde::rfc3339")]
    pub created_at: OffsetDateTime,
    #[serde(with = "time::serde::rfc3339")]
    pub updated_at: OffsetDateTime,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct GoalRequest {
    pub website_id: Uuid,
    pub name: String,
    #[serde(rename = "type")]
    pub kind: GoalKind,
    pub value: String,
}

#[derive(Debug, Clone, Copy, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub enum GoalKind {
    PageView,
    CustomEvent,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct GoalAnalytics {
    pub completions: i64,
    pub unique_sessions: i64,
    pub conversion_rate: f64,
    pub total_sessions: i64,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct GoalTimeSeriesPoint {
    #[serde(with = "time::serde::rfc3339")]
    pub timestamp: OffsetDateTime,
    pub completions: i64,
}

impl GoalRequest {
    #[must_use]
    pub fn targets(&self) -> (Option<&str>, Option<&str>) {
        match self.kind {
            GoalKind::PageView => (Some(self.value.as_str()), None),
            GoalKind::CustomEvent => (None, Some(self.value.as_str())),
        }
    }
}
