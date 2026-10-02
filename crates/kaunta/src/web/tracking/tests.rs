use super::{
    DatePeriod, decode_path, deterministic_uuid, hash_date, parse_url, parse_user_agent,
    referer_origin,
};
use time::{OffsetDateTime, format_description::well_known::Rfc3339};

#[test]
fn deterministic_uuid_matches_go_raw_md5_bytes() {
    assert_eq!(
        deterministic_uuid(&["a".to_owned(), "b".to_owned()]).to_string(),
        "d0726241-0206-76b1-4aa6-298ce6a18b21"
    );
}

#[test]
fn date_hash_uses_go_compatible_keys() {
    let value = OffsetDateTime::parse("2026-08-21T14:30:00Z", &Rfc3339)
        .expect("fixed RFC3339 timestamp is valid");
    assert_eq!(
        hash_date(value, DatePeriod::Month),
        format!("{:x}", md5::compute("2026-08"))
    );
    assert_eq!(
        hash_date(value, DatePeriod::Hour),
        format!("{:x}", md5::compute("2026-08-21T14"))
    );
}

#[test]
fn user_agent_parser_keeps_dashboard_vocabulary() {
    let cases = [
        (
            "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) \
             Chrome/124.0.0.0 Safari/537.36",
            ("Chrome", "Windows", "desktop"),
        ),
        (
            "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) \
             Chrome/124.0.0.0 Safari/537.36 Edg/124.0.0.0",
            ("Edge", "Windows", "desktop"),
        ),
        ("curl/8.4.0", ("Unknown", "Unknown", "desktop")),
    ];
    for (user_agent, (browser, os, device)) in cases {
        assert_eq!(
            parse_user_agent(user_agent),
            (browser.to_owned(), os.to_owned(), device.to_owned()),
            "user agent: {user_agent}"
        );
    }
}

#[test]
fn pixel_referer_is_normalized_for_origin_validation() {
    assert_eq!(
        referer_origin("https://example.com:8443/articles/port?source=pixel"),
        "https://example.com:8443"
    );
    assert_eq!(referer_origin("not-a-url"), "not-a-url");
}

#[test]
fn url_paths_are_percent_decoded_like_go() {
    assert_eq!(decode_path("/caf%C3%A9"), "/café");
    assert_eq!(decode_path("/a%20b"), "/a b");
    assert_eq!(decode_path("/plain"), "/plain");
    assert_eq!(decode_path(""), "/");
    assert_eq!(decode_path("/%FF"), "/\u{FFFD}");
}

#[test]
fn parse_url_stores_decoded_path_and_raw_query() {
    let parts = parse_url("https://example.com/caf%C3%A9?q=a%20b");
    assert_eq!(parts.path.as_deref(), Some("/café"));
    assert_eq!(parts.query.as_deref(), Some("q=a%20b"));
    assert_eq!(parts.hostname.as_deref(), Some("example.com"));

    let parts = parse_url("/a%20b?x=1");
    assert_eq!(parts.path.as_deref(), Some("/a b"));
    assert_eq!(parts.query.as_deref(), Some("x=1"));
    assert_eq!(parts.hostname, None);

    let parts = parse_url("https://example.com");
    assert_eq!(parts.path.as_deref(), Some("/"));
}
