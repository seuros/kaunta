use std::{
    collections::{HashMap, VecDeque},
    sync::{Arc, Mutex},
    time::{Duration, Instant},
};

const WINDOW: Duration = Duration::from_secs(60);
const ATTEMPTS: usize = 5;
const MAX_PEERS: usize = 10_000;

/// Shared rolling-window limiter for unauthenticated credential endpoints.
#[derive(Clone, Default)]
pub struct AttemptLimiter {
    attempts: Arc<Mutex<HashMap<String, VecDeque<Instant>>>>,
}

impl AttemptLimiter {
    #[must_use]
    pub fn allow(&self, peer: &str) -> bool {
        self.allow_at(peer, Instant::now())
    }

    fn allow_at(&self, peer: &str, now: Instant) -> bool {
        let Ok(mut peers) = self.attempts.lock() else {
            return false;
        };
        peers.retain(|_, attempts| {
            while attempts
                .front()
                .is_some_and(|time| now.duration_since(*time) >= WINDOW)
            {
                attempts.pop_front();
            }
            !attempts.is_empty()
        });
        if peers.len() >= MAX_PEERS && !peers.contains_key(peer) {
            let stalest = peers
                .iter()
                .min_by_key(|(_, attempts)| attempts.back().copied())
                .map(|(key, _)| key.clone());
            if let Some(key) = stalest {
                peers.remove(&key);
            }
        }
        let attempts = peers.entry(peer.to_owned()).or_default();
        if attempts.len() >= ATTEMPTS {
            return false;
        }
        attempts.push_back(now);
        true
    }
}

#[cfg(test)]
mod tests;
