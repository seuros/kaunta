use super::{WebsiteValidationError, validate_domain};

#[test]
fn matches_go_domain_validation() {
    assert!(validate_domain("localhost").is_ok());
    assert!(validate_domain("example.com:3000").is_ok());
    assert_eq!(
        validate_domain("https://example.com"),
        Err(WebsiteValidationError::InvalidDomainCharacters)
    );
}
