pub mod assets;
pub mod auth;
pub mod dashboard;
pub mod geoip;
pub mod http;
pub mod ingest;
pub mod pages;
pub mod rate_limit;
pub mod realtime;
pub mod server;
pub mod setup;
pub mod tracking;
pub mod websites;

#[derive(Clone)]
pub struct AppState {
    pub pool: sqlx::PgPool,
    pub version: &'static str,
    pub config: crate::domain::config::Config,
    pub geoip: geoip::GeoIp,
    pub realtime: realtime::RealtimeHub,
    pub login_attempts: rate_limit::AttemptLimiter,
    pub mcp: crate::mcp::http::McpHttpService,
    /// Database-backed exclusion rules, re-read periodically so edits take
    /// effect without a restart while keeping them off the tracking path.
    pub exclusions: ExclusionCache,
}

/// Snapshot of `excluded_address`, refreshed at most once per
/// [`EXCLUSION_TTL`].
type CachedRules = std::sync::Arc<tokio::sync::RwLock<Option<(std::time::Instant, Vec<String>)>>>;

#[derive(Clone, Default)]
pub struct ExclusionCache(CachedRules);

const EXCLUSION_TTL: std::time::Duration = std::time::Duration::from_secs(60);

impl ExclusionCache {
    pub async fn rules(&self, pool: &sqlx::PgPool) -> Vec<String> {
        if let Some((fetched, rules)) = self.0.read().await.as_ref()
            && fetched.elapsed() < EXCLUSION_TTL
        {
            return rules.clone();
        }
        match crate::db::exclusions::rules(pool).await {
            Ok(rules) => {
                *self.0.write().await = Some((std::time::Instant::now(), rules.clone()));
                rules
            }
            Err(error) => {
                tracing::warn!(?error, "failed to load tracking exclusions");
                self.0
                    .read()
                    .await
                    .as_ref()
                    .map(|(_, rules)| rules.clone())
                    .unwrap_or_default()
            }
        }
    }

    /// Drops the snapshot so the next check reflects a just-made edit.
    pub async fn invalidate(&self) {
        *self.0.write().await = None;
    }
}
