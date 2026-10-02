use serde::{Deserialize, Serialize};
use time::OffsetDateTime;
use uuid::Uuid;

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct WebsiteSummary {
    pub id: Uuid,
    pub domain: String,
    pub name: String,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct PeriodOverview {
    pub visitors: i64,
    pub pageviews: i64,
    pub bounce_rate: String,
    pub prev_visitors: i64,
    pub prev_pageviews: i64,
    pub prev_bounce_rate: String,
    pub period_days: i32,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct DashboardStats {
    pub current_visitors: i64,
    pub today_pageviews: i64,
    pub today_visitors: i64,
    pub today_bounce_rate: String,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct PublicStats {
    pub online: i64,
    pub pageviews: i64,
    pub visitors: i64,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct TopPage {
    pub path: String,
    pub views: i64,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct TimeSeriesPoint {
    #[serde(with = "time::serde::rfc3339")]
    pub timestamp: OffsetDateTime,
    pub value: i64,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct BreakdownItem {
    pub name: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub code: Option<String>,
    pub count: i64,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct MapDataPoint {
    pub country: String,
    pub country_name: String,
    pub code: String,
    pub visitors: i64,
    pub percentage: f64,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct MapResponse {
    pub data: Vec<MapDataPoint>,
    pub total_visitors: i64,
    pub period_days: i32,
}
