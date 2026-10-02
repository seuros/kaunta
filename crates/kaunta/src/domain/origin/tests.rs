use super::{OriginValidationError, sanitize_origin};

#[test]
fn sanitizes_hosts_like_the_go_configuration() {
    assert_eq!(
        sanitize_origin(" HTTPS://Example.COM:8443/ "),
        Ok("example.com:8443".to_owned())
    );
    assert_eq!(sanitize_origin("localhost"), Ok("localhost".to_owned()));
}

#[test]
fn rejects_unsafe_or_non_origin_values() {
    assert_eq!(sanitize_origin(""), Err(OriginValidationError::Empty));
    assert_eq!(sanitize_origin("*"), Err(OriginValidationError::Wildcard));
    assert_eq!(
        sanitize_origin("example.com/path"),
        Err(OriginValidationError::PathQueryOrFragment)
    );
    assert_eq!(
        sanitize_origin("example .com"),
        Err(OriginValidationError::Whitespace)
    );
}
