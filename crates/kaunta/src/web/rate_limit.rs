use std::{
    collections::HashMap,
    sync::{Arc, Mutex},
    time::{Duration, Instant},
};

use throttle_machines::{Gate, Gcra, GcraParams};

/// Burst a fresh peer gets before being throttled.
const ATTEMPTS: u32 = 5;
/// Time for a drained peer to earn its full burst back.
const WINDOW_MS: u32 = 60_000;
const MAX_PEERS: usize = 10_000;

/// Shared per-peer GCRA limiter for unauthenticated credential endpoints.
///
/// Time is tracked in whole milliseconds so GCRA arithmetic stays exact in
/// `f64` and the burst boundary never flips on rounding.
#[derive(Clone)]
pub struct AttemptLimiter {
    epoch: Instant,
    params: GcraParams,
    /// Theoretical arrival time per peer, in ms since `epoch`.
    peers: Arc<Mutex<HashMap<String, f64>>>,
}

impl Default for AttemptLimiter {
    fn default() -> Self {
        let emission_interval = f64::from(WINDOW_MS / ATTEMPTS);
        Self {
            epoch: Instant::now(),
            params: GcraParams {
                emission_interval,
                delay_tolerance: emission_interval * f64::from(ATTEMPTS - 1),
            },
            peers: Arc::default(),
        }
    }
}

impl AttemptLimiter {
    /// `Err` carries how long the peer must wait before its next attempt.
    pub fn check(&self, peer: &str) -> Result<(), Duration> {
        self.check_at(peer, Instant::now())
    }

    fn check_at(&self, peer: &str, now: Instant) -> Result<(), Duration> {
        let now = self.millis_since_epoch(now);
        let Ok(mut peers) = self.peers.lock() else {
            return Err(Duration::from_millis(WINDOW_MS.into()));
        };
        // A TAT in the past is indistinguishable from a fresh peer.
        peers.retain(|_, tat| *tat > now);
        if peers.len() >= MAX_PEERS && !peers.contains_key(peer) {
            let stalest = peers
                .iter()
                .min_by(|(_, a), (_, b)| a.total_cmp(b))
                .map(|(key, _)| key.clone());
            if let Some(key) = stalest {
                peers.remove(&key);
            }
        }
        let tat = peers.get(peer).copied().unwrap_or_default();
        let decision = Gcra::check(tat, now, self.params);
        if !decision.allowed {
            return Err(Duration::from_secs_f64(decision.retry_after / 1000.0));
        }
        peers.insert(peer.to_owned(), decision.state);
        Ok(())
    }

    #[allow(clippy::cast_precision_loss)] // whole ms stay exact below 2^53
    fn millis_since_epoch(&self, now: Instant) -> f64 {
        now.saturating_duration_since(self.epoch).as_millis() as f64
    }
}

/// Whole seconds for a `Retry-After` header, rounded up so clients never
/// retry early.
#[must_use]
pub fn retry_after_secs(wait: Duration) -> u64 {
    wait.as_secs() + u64::from(wait.subsec_nanos() > 0)
}

#[cfg(test)]
mod tests;
