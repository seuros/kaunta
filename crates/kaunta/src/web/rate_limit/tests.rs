use super::*;

const INTERVAL: Duration = Duration::from_secs(12);

fn drain(limiter: &AttemptLimiter, peer: &str, now: Instant) {
    for _ in 0..ATTEMPTS {
        assert!(limiter.check_at(peer, now).is_ok());
    }
}

#[test]
fn allows_burst_per_peer_then_reports_retry_after() {
    let limiter = AttemptLimiter::default();
    let now = Instant::now();
    drain(&limiter, "192.0.2.1", now);
    assert_eq!(limiter.clone().check_at("192.0.2.1", now), Err(INTERVAL));
    assert!(limiter.check_at("192.0.2.2", now).is_ok());
}

#[test]
fn full_window_restores_full_burst_and_forgets_peer() {
    let limiter = AttemptLimiter::default();
    let now = Instant::now();
    drain(&limiter, "192.0.2.1", now);
    let later = now + Duration::from_millis(WINDOW_MS.into());
    assert!(limiter.check_at("192.0.2.2", later).is_ok());
    assert!(
        !limiter.peers.lock().unwrap().contains_key("192.0.2.1"),
        "recovered peer is purged"
    );
    drain(&limiter, "192.0.2.1", later);
}

#[test]
fn full_table_evicts_stalest_peer_instead_of_blocking_new_ones() {
    let limiter = AttemptLimiter::default();
    let now = Instant::now();
    let base = limiter.millis_since_epoch(now);
    {
        let mut peers = limiter.peers.lock().unwrap();
        for i in 0..MAX_PEERS {
            let tat = if i == 0 { base + 1.0 } else { base + 2.0 };
            peers.insert(i.to_string(), tat);
        }
    }
    assert!(limiter.check_at("new-peer", now).is_ok());
    {
        let peers = limiter.peers.lock().unwrap();
        assert_eq!(peers.len(), MAX_PEERS);
        assert!(!peers.contains_key("0"), "stalest peer is evicted");
        assert!(peers.contains_key("1"), "fresher peers are retained");
        assert!(peers.contains_key("new-peer"));
    }
    assert!(limiter.check_at("1", now).is_ok());
}

#[test]
fn eviction_does_not_bypass_quota_for_tracked_peer() {
    let limiter = AttemptLimiter::default();
    let now = Instant::now();
    let base = limiter.millis_since_epoch(now);
    {
        let mut peers = limiter.peers.lock().unwrap();
        for i in 0..MAX_PEERS {
            peers.insert(i.to_string(), base + 1.0);
        }
        peers.insert("0".to_owned(), base + f64::from(WINDOW_MS));
    }
    assert!(limiter.check_at("0", now).is_err());
    assert!(limiter.check_at("new-peer", now).is_ok());
}

#[test]
fn retry_after_rounds_up_to_whole_seconds() {
    assert_eq!(retry_after_secs(Duration::ZERO), 0);
    assert_eq!(retry_after_secs(Duration::from_secs(12)), 12);
    assert_eq!(retry_after_secs(Duration::from_millis(1)), 1);
    assert_eq!(retry_after_secs(Duration::from_millis(11_001)), 12);
}
