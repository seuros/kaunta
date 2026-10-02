use super::{IngestPayload, IngestValidationError, validate_ingest};
use time::OffsetDateTime;

#[test]
fn page_view_requires_url() {
    let payload = IngestPayload {
        event: "page_view".to_owned(),
        visitor_id: "visitor".to_owned(),
        url: String::new(),
        hostname: String::new(),
        referrer: String::new(),
        title: String::new(),
        user_id: None,
        session_id: None,
        event_id: None,
        timestamp: None,
        properties: std::collections::BTreeMap::default(),
        context: None,
        utm_source: None,
        utm_medium: None,
        utm_campaign: None,
        utm_term: None,
        utm_content: None,
    };

    assert_eq!(
        validate_ingest(&payload, OffsetDateTime::now_utc()),
        Err(IngestValidationError::MissingPageViewUrl)
    );
}

fn minimal_payload(extra: serde_json::Value) -> IngestPayload {
    let mut value = serde_json::json!({ "event": "signup", "visitor_id": "visitor" });
    if let (Some(base), Some(extra)) = (value.as_object_mut(), extra.as_object()) {
        base.extend(extra.clone());
    }
    serde_json::from_value(value).expect("payload deserializes")
}

#[test]
fn explicit_json_nulls_decode_as_empty_like_go() {
    let payload = minimal_payload(serde_json::json!({
        "url": null,
        "hostname": null,
        "referrer": null,
        "title": null,
        "properties": null,
        "context": { "locale": null, "screen": null }
    }));
    assert_eq!(payload.url, "");
    assert_eq!(payload.hostname, "");
    assert_eq!(payload.referrer, "");
    assert_eq!(payload.title, "");
    assert!(payload.properties.is_empty());
    let context = payload.context.as_ref().expect("context is present");
    assert_eq!(context.locale, "");
    assert_eq!(context.screen, "");
    assert_eq!(validate_ingest(&payload, OffsetDateTime::now_utc()), Ok(()));

    let payload = minimal_payload(serde_json::json!({ "context": null }));
    assert!(payload.context.is_none());
}

#[test]
fn context_locale_and_screen_are_bounded_by_session_columns() {
    let payload = minimal_payload(serde_json::json!({
        "context": { "locale": "a".repeat(super::MAX_LOCALE_LENGTH + 1) }
    }));
    assert_eq!(
        validate_ingest(&payload, OffsetDateTime::now_utc()),
        Err(IngestValidationError::FieldTooLong(
            "context.locale",
            super::MAX_LOCALE_LENGTH
        ))
    );

    let payload = minimal_payload(serde_json::json!({
        "context": { "screen": "1920x1080x999" }
    }));
    assert_eq!(
        validate_ingest(&payload, OffsetDateTime::now_utc()),
        Err(IngestValidationError::FieldTooLong(
            "context.screen",
            super::MAX_SCREEN_LENGTH
        ))
    );

    let payload = minimal_payload(serde_json::json!({
        "context": { "locale": "en-US", "screen": "1920x1080" }
    }));
    assert_eq!(validate_ingest(&payload, OffsetDateTime::now_utc()), Ok(()));
}
