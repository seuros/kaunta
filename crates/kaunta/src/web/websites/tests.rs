use super::*;
use serde_json::json;
use std::collections::HashMap;

#[test]
fn pagination_defaults_clamps_and_saturates() {
    assert_eq!(pagination(&HashMap::new()), (1, 10, 0));
    let query = HashMap::from([("page".into(), "-1".into()), ("per".into(), "999".into())]);
    assert_eq!(pagination(&query), (1, 100, 0));
    let query = HashMap::from([
        ("page".into(), i64::MAX.to_string()),
        ("per".into(), "100".into()),
    ]);
    assert_eq!(pagination(&query), (i64::MAX, 100, i64::MAX));
}

#[test]
fn pagination_retains_total_on_out_of_range_pages() {
    let result = paginated_websites(Vec::new(), 3, 10, 11);
    assert_eq!(
        result,
        json!({"data": [], "pagination": {
            "page": 3, "per": 10, "total": 11, "total_pages": 2, "has_more": false
        }})
    );
}

#[test]
fn website_details_use_go_field_names_and_timestamp_format() {
    let detail = WebsiteDetailResponse {
        id: Uuid::nil(),
        domain: "example.com".into(),
        name: "Example".into(),
        allowed_domains: vec!["example.com".into()],
        public_stats_enabled: false,
        created_at: time::OffsetDateTime::UNIX_EPOCH,
    };
    let value = serde_json::to_value(detail).unwrap();
    assert_eq!(value["id"], Uuid::nil().to_string());
    assert!(value.get("website_id").is_none());
    assert_eq!(value["created_at"], "1970-01-01T00:00:00Z");
}
