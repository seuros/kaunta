use crate::web::http::config_origin_trusted;

#[test]
fn websocket_origin_policy_matches_http_origin_configuration() {
    let allowed = vec!["https://analytics.example.com".to_owned()];
    assert!(config_origin_trusted(
        &allowed,
        "https://analytics.example.com"
    ));
    assert!(config_origin_trusted(
        &allowed,
        "https://analytics.example.com/"
    ));
    assert!(!config_origin_trusted(&allowed, "https://attacker.example"));
    assert!(!config_origin_trusted(&[], "https://attacker.example"));
}
