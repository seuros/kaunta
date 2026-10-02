use super::*;

#[test]
fn allows_five_attempts_per_peer_and_expires_window() {
    let limiter = AttemptLimiter::default();
    let now = Instant::now();
    for _ in 0..ATTEMPTS {
        assert!(limiter.allow_at("192.0.2.1", now));
    }
    assert!(!limiter.clone().allow_at("192.0.2.1", now));
    assert!(limiter.allow_at("192.0.2.2", now));
    assert!(!limiter.allow_at("192.0.2.1", now + WINDOW - Duration::from_nanos(1)));
    assert!(limiter.allow_at("192.0.2.1", now + WINDOW));
}

#[test]
fn full_table_evicts_stalest_peer_instead_of_blocking_new_ones() {
    let limiter = AttemptLimiter::default();
    let now = Instant::now();
    {
        let mut peers = limiter.attempts.lock().unwrap();
        for i in 0..MAX_PEERS {
            let last = if i == 0 {
                now
            } else {
                now + Duration::from_millis(1)
            };
            peers.insert(i.to_string(), VecDeque::from([last]));
        }
    }
    let later = now + Duration::from_millis(2);
    assert!(limiter.allow_at("new-peer", later));
    {
        let peers = limiter.attempts.lock().unwrap();
        assert_eq!(peers.len(), MAX_PEERS);
        assert!(!peers.contains_key("0"), "stalest peer is evicted");
        assert!(peers.contains_key("1"), "fresher peers are retained");
        assert!(peers.contains_key("new-peer"));
    }
    assert!(limiter.allow_at("1", later));
    assert!(limiter.allow_at("new-peer", later));
}

#[test]
fn eviction_does_not_bypass_quota_for_tracked_peer() {
    let limiter = AttemptLimiter::default();
    let now = Instant::now();
    {
        let mut peers = limiter.attempts.lock().unwrap();
        for i in 0..MAX_PEERS {
            peers.insert(i.to_string(), VecDeque::from([now]));
        }
        peers.insert("0".to_owned(), VecDeque::from([now; ATTEMPTS]));
    }
    assert!(!limiter.allow_at("0", now));
    assert!(limiter.allow_at("new-peer", now));
}
