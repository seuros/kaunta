use std::time::Duration;

use super::{RECONNECT_BACKOFF, reconnect_delay};
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

#[test]
fn reconnect_delay_grows_jittered_and_caps_without_running_out() {
    let mut failures = 0;
    let first = reconnect_delay(&mut failures);
    assert_eq!(failures, 1);
    assert!((Duration::from_millis(500)..=Duration::from_secs(1)).contains(&first));

    let cap = Duration::from_millis(RECONNECT_BACKOFF.max_delay_ms);
    for _ in 0..1_000 {
        assert!(reconnect_delay(&mut failures) <= cap);
    }
    assert_eq!(failures, RECONNECT_BACKOFF.max_attempts - 1);
    assert!(reconnect_delay(&mut failures) >= cap / 2);
}
