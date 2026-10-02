use super::*;

#[test]
fn csv_escaping_and_formula_protection() {
    for (input, expected) in [
        ("plain", "plain"),
        ("a,b", "\"a,b\""),
        ("a\"b", "\"a\"\"b\""),
        ("a\rb\nc", "\"a\rb\nc\""),
        (
            "=HYPERLINK(\"x\",\"y\")",
            "\"'=HYPERLINK(\"\"x\"\",\"\"y\"\")\"",
        ),
        ("+sum", "'+sum"),
        ("-sum", "'-sum"),
        ("@sum", "'@sum"),
    ] {
        assert_eq!(csv_field(input, true), expected);
    }
    assert_eq!(csv_field("-42", false), "-42");
    let mut row = String::new();
    csv_row(&mut row, &[("a,b", true), ("7", false)]);
    assert_eq!(row, "\"a,b\",7\n");
}

#[test]
fn empty_campaign_patch_preserves_target() {
    let html = build_utm_table("source", &[], "count", "desc");
    assert!(html.contains("id=\"utm-source-content\""));
}

#[test]
fn campaign_data_is_escaped() {
    let html = build_utm_table(
        "source",
        &[crate::domain::analytics::BreakdownItem {
            name: "<script>alert('x')</script>".to_owned(),
            code: None,
            count: 1,
        }],
        "count",
        "desc",
    );
    assert!(!html.contains("<script>"));
    assert!(html.contains("&lt;script&gt;"));
}

fn query(pairs: &[(&str, &str)]) -> std::collections::HashMap<String, String> {
    pairs
        .iter()
        .map(|(key, value)| ((*key).to_owned(), (*value).to_owned()))
        .collect()
}

#[test]
fn normalize_dimension_accepts_go_and_rust_vocabulary() {
    for (raw, expected) in [
        ("pages", "page"),
        ("page", "page"),
        ("Pages", "page"),
        ("referrers", "referrer"),
        ("referrer", "referrer"),
        ("browsers", "browser"),
        ("devices", "device"),
        ("countries", "country"),
        ("cities", "city"),
        ("regions", "region"),
        ("os", "os"),
        ("utm_source", "utm_source"),
        ("utm_medium", "utm_medium"),
        ("utm_campaign", "utm_campaign"),
        ("utm_term", "utm_term"),
        ("utm_content", "utm_content"),
        ("entry_page", "entry_page"),
        ("entry-pages", "entry_page"),
        ("exit_page", "exit_page"),
        ("exit-pages", "exit_page"),
    ] {
        assert_eq!(normalize_dimension(raw), Some(expected), "input {raw:?}");
    }
    assert_eq!(normalize_dimension("sessions"), None);
    assert_eq!(normalize_dimension("page; DROP TABLE"), None);
    assert_eq!(normalize_dimension(""), None);
}

#[test]
fn goal_dimension_accepts_pages_alias_and_rejects_unsupported() {
    assert_eq!(normalize_goal_dimension("pages"), Some("page"));
    assert_eq!(normalize_goal_dimension("page"), Some("page"));
    assert_eq!(normalize_goal_dimension("referrers"), Some("referrer"));
    assert_eq!(normalize_goal_dimension("os"), Some("os"));
    assert_eq!(normalize_goal_dimension("utm_source"), None);
    assert_eq!(normalize_goal_dimension("cities"), None);
    assert_eq!(normalize_goal_dimension("bogus"), None);
}

#[test]
fn breakdown_type_follows_go_precedence() {
    assert_eq!(breakdown_type(&query(&[])), "pages");
    assert_eq!(breakdown_type(&query(&[("tab", "browsers")])), "browsers");
    assert_eq!(
        breakdown_type(&query(&[("type", "devices"), ("tab", "browsers")])),
        "devices"
    );
    assert_eq!(
        breakdown_type(&query(&[
            ("datastar", r#"{"activeTab":"countries"}"#),
            ("type", "devices"),
        ])),
        "countries"
    );
    assert_eq!(
        breakdown_type(&query(&[
            ("datastar", r#"{"activeTab":""}"#),
            ("type", "os")
        ])),
        "os"
    );
}

#[test]
fn sort_parameters_are_whitelisted() {
    assert_eq!(sort_column(&query(&[]), BREAKDOWN_SORT_COLUMNS), "count");
    assert_eq!(
        sort_column(&query(&[("sort_by", "NAME")]), BREAKDOWN_SORT_COLUMNS),
        "name"
    );
    assert_eq!(
        sort_column(&query(&[("sort_by", "views")]), BREAKDOWN_SORT_COLUMNS),
        "count"
    );
    assert_eq!(sort_column(&query(&[]), PAGES_SORT_COLUMNS), "views");
    assert_eq!(
        sort_column(
            &query(&[("sort_by", "unique_visitors")]),
            PAGES_SORT_COLUMNS
        ),
        "unique_visitors"
    );
    assert_eq!(
        sort_column(&query(&[("sort_by", "1; DROP")]), PAGES_SORT_COLUMNS),
        "views"
    );

    assert_eq!(sort_direction(&query(&[])), "desc");
    assert_eq!(sort_direction(&query(&[("sort_order", "ASC")])), "asc");
    assert_eq!(
        sort_direction(&query(&[("sort_order", "sideways")])),
        "desc"
    );
}
