use super::{csv_field, parse_csv};

#[test]
fn parses_comma_separated_flags() {
    assert_eq!(
        parse_csv(Some(" ingest, stats ,, ")),
        vec!["ingest", "stats"]
    );
}

#[test]
fn quotes_csv_values() {
    assert_eq!(csv_field("hello"), "hello");
    assert_eq!(csv_field("hello,world"), "\"hello,world\"");
    assert_eq!(csv_field("say \"hi\""), "\"say \"\"hi\"\"\"");
}

#[test]
fn reset_password_requires_explicit_password_when_not_interactive() {
    let error = super::reset_password_value(None, false)
        .expect_err("non-interactive reset without --password must fail")
        .to_string();
    assert_eq!(
        error,
        "password required: pass --password or run interactively"
    );
    assert_eq!(
        super::reset_password_value(Some("correct horse".to_owned()), false).expect("explicit"),
        "correct horse"
    );
    assert!(super::reset_password_value(Some("short".to_owned()), false).is_err());
}
