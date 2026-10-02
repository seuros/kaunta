//! Embeddable `/mcp` endpoint for the kaunta web server.
//!
//! Authenticates with kaunta API keys (`Authorization: Bearer <key>`). The
//! key identifies who is asking, not a single website: tools may query any
//! website the key's creator can access (own plus shared), naming the
//! target per call. Keys minted from the CLI have no creator and act as
//! operator credentials with access to every website.

use std::sync::Arc;

use mcp_host::prelude::AuthenticatedPrincipal;
pub use mcp_host::transport::http::McpHttpService;
use mcp_host::transport::http::{
    HttpAuthFuture, HttpAuthOutcome, HttpAuthRequest, HttpAuthenticator, HttpTransportConfig,
    HttpTransportError,
};
use serde_json::json;
use sqlx::PgPool;

/// Session-state key holding the user UUID that created the API key.
/// Absent for CLI-minted keys, which carry operator scope.
pub const USER_SCOPE_KEY: &str = "kaunta/user_id";

#[derive(Debug)]
struct ApiKeyAuthenticator {
    pool: PgPool,
}

impl HttpAuthenticator for ApiKeyAuthenticator {
    fn authenticate<'a>(&'a self, request: HttpAuthRequest<'a>) -> HttpAuthFuture<'a> {
        Box::pin(async move {
            let token = request
                .authorization
                .and_then(|value| value.strip_prefix("Bearer "))
                .ok_or(HttpTransportError::Unauthorized)?;
            let key = crate::db::api_keys::get_by_hash(
                &self.pool,
                &crate::db::api_keys::hash_api_key(token),
            )
            .await
            .map_err(|error| HttpTransportError::Internal(error.to_string()))?
            .ok_or(HttpTransportError::Unauthorized)?;
            let outcome = HttpAuthOutcome::new(AuthenticatedPrincipal::new(
                "api-key",
                key.key_id.to_string(),
            ));
            Ok(match key.created_by {
                Some(user_id) => outcome.with_state(USER_SCOPE_KEY, json!(user_id.to_string())),
                None => outcome,
            })
        })
    }
}

/// Build the embeddable `/mcp` service backed by kaunta API keys.
///
/// # Errors
///
/// Returns an error when the transport configuration is rejected or the MCP
/// server already has an active transport dispatcher.
pub fn service(
    pool: PgPool,
    database_url: String,
    data_dir: std::path::PathBuf,
    version: &str,
) -> anyhow::Result<McpHttpService> {
    let server = crate::mcp::build_server(pool.clone(), database_url, data_dir, version);
    let config = HttpTransportConfig {
        endpoint: "/mcp".into(),
        localhost_only: false,
        authenticator: Some(Arc::new(ApiKeyAuthenticator { pool })),
        ..Default::default()
    };
    McpHttpService::new(Arc::new(server), config)
        .map_err(|error| anyhow::anyhow!("mcp http service: {error}"))
}
