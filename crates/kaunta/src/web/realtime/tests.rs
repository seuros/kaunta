use super::{RECONNECT_BACKOFF, reconnect_delay};

#[test]
fn reconnect_failures_saturate_below_max_attempts() {
    let mut failures = 0;
    for _ in 0..1_000 {
        reconnect_delay(&mut failures);
    }
    assert_eq!(failures, RECONNECT_BACKOFF.max_attempts - 1);
}
