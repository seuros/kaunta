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
