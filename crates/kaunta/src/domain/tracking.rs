use std::collections::BTreeMap;

use serde::{Deserialize, Serialize};
use serde_json::Value;

pub const MAX_URL_SIZE: usize = 2_000;

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TrackingPayload {
    #[serde(rename = "type")]
    pub kind: String,
    pub payload: PayloadData,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct PayloadData {
    pub website: String,
    pub hostname: Option<String>,
    pub language: Option<String>,
    pub referrer: Option<String>,
    pub screen: Option<String>,
    pub title: Option<String>,
    pub url: Option<String>,
    pub name: Option<String>,
    pub tag: Option<String>,
    pub data: Option<BTreeMap<String, Value>>,
    pub ip: Option<String>,
    #[serde(rename = "userAgent")]
    pub user_agent: Option<String>,
    pub timestamp: Option<i64>,
    pub id: Option<String>,
    pub scroll_depth: Option<i32>,
    pub engagement_time: Option<i32>,
    pub props: Option<BTreeMap<String, Value>>,
    pub utm_source: Option<String>,
    pub utm_medium: Option<String>,
    pub utm_campaign: Option<String>,
    pub utm_term: Option<String>,
    pub utm_content: Option<String>,
}
