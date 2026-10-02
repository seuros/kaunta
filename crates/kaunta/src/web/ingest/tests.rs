use crate::domain::ingest::IngestPayload;
use time::{OffsetDateTime, format_description::well_known::Rfc3339};
use uuid::Uuid;

use super::resolve_session_id;

#[test]
fn explicit_non_uuid_session_ids_are_deterministic() {
    let payload: IngestPayload = serde_json::from_value(serde_json::json!({
        "event": "signup",
        "visitor_id": "visitor",
        "session_id": "checkout"
    }))
    .expect("payload is valid");
    let timestamp =
        OffsetDateTime::parse("2026-08-21T14:30:00Z", &Rfc3339).expect("fixed timestamp is valid");
    let website_id =
        Uuid::parse_str("11223344-5566-7788-99aa-bbccddeeff00").expect("fixed UUID is valid");
    assert_eq!(
        resolve_session_id(&payload, website_id, timestamp),
        resolve_session_id(&payload, website_id, timestamp)
    );
}
