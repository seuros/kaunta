pub mod analytics;
pub mod api_keys;
pub mod auth;
pub mod backup;
pub mod events;
pub mod exclusions;
pub mod goals;
pub mod health;
pub mod maintenance;
pub mod migrations;
pub mod origins;
pub mod realtime;
pub mod startup;
pub mod websites;

use std::str::FromStr;
use std::time::Duration;

use sqlx::{
    PgPool,
    postgres::{PgConnectOptions, PgPoolOptions, PgSslMode},
};

/// Connects to PostgreSQL.
///
/// Go's `lib/pq` defaulted to `sslmode=require` while sqlx defaults to
/// `prefer`; to keep deployments behaving identically, `require` is applied
/// when the URL carries no `sslmode` parameter and `PGSSLMODE` is unset.
/// Explicit values are left untouched.
pub async fn connect(database_url: &str) -> Result<PgPool, sqlx::Error> {
    let mut options = PgConnectOptions::from_str(database_url)?;
    if url_lacks_ssl_mode(database_url) && std::env::var_os("PGSSLMODE").is_none() {
        options = options.ssl_mode(PgSslMode::Require);
    }
    PgPoolOptions::new()
        .max_connections(20)
        .min_connections(1)
        .acquire_timeout(Duration::from_secs(10))
        .idle_timeout(Duration::from_mins(5))
        .connect_with(options)
        .await
}

/// Returns `true` when the connection URL has no `sslmode` query parameter,
/// i.e. when the `lib/pq`-compatible `require` default should be applied.
#[must_use]
fn url_lacks_ssl_mode(database_url: &str) -> bool {
    let Some((_, query)) = database_url.split_once('?') else {
        return true;
    };
    let query = query.split('#').next().unwrap_or_default();
    !query
        .split('&')
        .filter_map(|pair| pair.split_once('=').map(|(key, _)| key))
        .any(|key| key == "sslmode")
}

#[cfg(test)]
mod tests {
    use super::url_lacks_ssl_mode;

    #[test]
    fn absent_sslmode_gets_the_require_default() {
        assert!(url_lacks_ssl_mode("postgres://u:p@localhost:5432/kaunta"));
        assert!(url_lacks_ssl_mode("postgres://u:p@localhost/kaunta?"));
        assert!(url_lacks_ssl_mode(
            "postgres://u:p@localhost/kaunta?application_name=kaunta&connect_timeout=5"
        ));
        assert!(url_lacks_ssl_mode(
            "postgres://localhost/kaunta#sslmode=disable"
        ));
    }

    #[test]
    fn explicit_sslmode_is_left_untouched() {
        assert!(!url_lacks_ssl_mode(
            "postgres://u:p@localhost/kaunta?sslmode=disable"
        ));
        assert!(!url_lacks_ssl_mode(
            "postgres://u:p@localhost/kaunta?sslmode=prefer"
        ));
        assert!(!url_lacks_ssl_mode(
            "postgres://u:p@localhost/kaunta?application_name=x&sslmode=verify-full"
        ));
        assert!(!url_lacks_ssl_mode(
            "postgresql:///kaunta?sslmode=require&host=/tmp"
        ));
    }
}
