use serde::{Deserialize, Serialize};
use serde_json::Value;
use time::OffsetDateTime;
use uuid::Uuid;

#[derive(Debug, Clone, Default)]
pub struct SessionAttributes {
    pub hostname: Option<String>,
    pub browser: Option<String>,
    pub os: Option<String>,
    pub device: Option<String>,
    pub screen: Option<String>,
    pub language: Option<String>,
    pub country: Option<String>,
    pub subdivision1: Option<String>,
    pub subdivision2: Option<String>,
    pub city: Option<String>,
    pub region: Option<String>,
    pub distinct_id: Option<String>,
    pub entry_page: Option<String>,
    pub exit_page: Option<String>,
}

#[derive(Debug, Clone)]
pub struct SessionUpsert {
    pub session_id: Uuid,
    pub website_id: Uuid,
    pub created_at: OffsetDateTime,
    pub attributes: SessionAttributes,
}

#[derive(Debug, Clone)]
pub struct EventInsert {
    pub event_id: Uuid,
    pub website_id: Uuid,
    pub session_id: Uuid,
    pub visit_id: Uuid,
    pub created_at: OffsetDateTime,
    pub url_path: Option<String>,
    pub url_query: Option<String>,
    pub referrer_path: Option<String>,
    pub referrer_query: Option<String>,
    pub referrer_domain: Option<String>,
    pub page_title: Option<String>,
    pub hostname: Option<String>,
    pub event_type: i16,
    pub event_name: Option<String>,
    pub tag: Option<String>,
    pub scroll_depth: Option<i16>,
    pub engagement_time: Option<i32>,
    pub props: Option<Value>,
    pub utm_source: Option<String>,
    pub utm_medium: Option<String>,
    pub utm_campaign: Option<String>,
    pub utm_term: Option<String>,
    pub utm_content: Option<String>,
    pub goal_id: Option<Uuid>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct TrackingAccepted {
    #[serde(rename = "sessionId")]
    pub session_id: Uuid,
    #[serde(rename = "visitId")]
    pub visit_id: Uuid,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct RealtimeEvent {
    #[serde(rename = "type")]
    pub kind: String,
    pub website_id: Uuid,
    pub session_id: Uuid,
    pub visit_id: Uuid,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub path: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub title: Option<String>,
    #[serde(with = "time::serde::rfc3339")]
    pub created_at: OffsetDateTime,
}

#[cfg(test)]
mod tests {
    use super::RealtimeEvent;
    use time::macros::datetime;
    use uuid::Uuid;

    #[test]
    fn realtime_event_serializes_created_at_as_rfc3339() {
        let event = RealtimeEvent {
            kind: "pageview".to_owned(),
            website_id: Uuid::nil(),
            session_id: Uuid::nil(),
            visit_id: Uuid::nil(),
            path: Some("/".to_owned()),
            title: None,
            created_at: datetime!(2026-09-27 12:34:56.5 UTC),
        };
        let json = serde_json::to_value(&event).unwrap();
        let created_at = json["created_at"].as_str().expect("created_at is a string");
        assert!(
            created_at.ends_with('Z') || created_at.ends_with("+00:00"),
            "unexpected timestamp format: {created_at}"
        );
        assert!(created_at.starts_with("2026-09-27T12:34:56"));
        assert!(json.get("title").is_none());

        let round_trip: RealtimeEvent = serde_json::from_value(json).unwrap();
        assert_eq!(round_trip.created_at, event.created_at);
    }
}
