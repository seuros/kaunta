//! MCP server exposing Kaunta's analytics and administration tools.
//!
//! The only transport is Streamable HTTP at `/mcp`, authenticated with a
//! Kaunta API key. A client therefore needs a key the operator can scope
//! and revoke, never the database credentials.

use std::sync::Arc;

use mcp_host::prelude::*;
use mcp_host::registry::router::McpRouter;
use sqlx::PgPool;
use uuid::Uuid;

pub mod http;
pub mod resources;
pub mod tools;

pub struct KauntaMcp {
    pool: PgPool,
    database_url: String,
    data_dir: std::path::PathBuf,
}

impl KauntaMcp {
    #[must_use]
    pub const fn new(pool: PgPool, database_url: String, data_dir: std::path::PathBuf) -> Self {
        Self {
            pool,
            database_url,
            data_dir,
        }
    }

    fn backups_dir(&self) -> std::path::PathBuf {
        self.data_dir.join("backups")
    }

    #[must_use]
    pub fn router() -> McpRouter<Self> {
        McpRouter::new(
            tools::router(),
            Default::default(),
            resources::router(),
            Default::default(),
        )
    }

    /// Resolve a website identifier that may be a UUID or a domain name,
    /// enforcing the session's access scope. Any accessible website may be
    /// named per call; that is how a session switches between projects.
    async fn resolve_website(&self, ctx: &Ctx<'_>, website: &str) -> Result<Uuid, ToolError> {
        let id = if let Ok(id) = Uuid::parse_str(website) {
            id
        } else {
            crate::db::websites::get_by_domain(&self.pool, website, None)
                .await
                .map(|site| site.website_id)
                .map_err(|error| {
                    ToolError::InvalidArguments(format!("unknown website {website:?}: {error}"))
                })?
        };
        if let Scope::User(user_id) = session_scope(ctx) {
            let accessible = crate::db::websites::is_accessible_by(&self.pool, id, user_id)
                .await
                .map_err(execution_error)?;
            if !accessible {
                return Err(ToolError::InvalidArguments(
                    "this website is not accessible with this API key".into(),
                ));
            }
        }
        Ok(id)
    }
}

/// Access scope of the current session.
enum Scope {
    /// Stdio sessions and CLI-minted API keys: every website.
    Operator,
    /// Dashboard-minted API keys: the creator's own plus shared websites.
    User(Uuid),
}

fn session_scope(ctx: &Ctx<'_>) -> Scope {
    ctx.get_state::<String>(http::USER_SCOPE_KEY)
        .and_then(|value| Uuid::parse_str(&value).ok())
        .map_or(Scope::Operator, Scope::User)
}

/// Write tools are visible to operator sessions only, which means keys
/// minted from the CLI (no creating user recorded).
pub(crate) fn operator_visible(ctx: &VisibilityContext) -> bool {
    ctx.get_state::<String>(http::USER_SCOPE_KEY).is_none()
}

/// Require an interactive human confirmation via MCP elicitation before a
/// write proceeds. Clients without the elicitation capability cannot use
/// the write tools at all. The human accept step is not optional.
pub(crate) async fn require_confirmation(ctx: &Ctx<'_>, message: &str) -> Result<(), ToolError> {
    use mcp_host::protocol::elicitation::ElicitationSchema;
    use mcp_host::protocol::types::ElicitationAction;

    let Some(requester) = ctx.client_requester() else {
        return Err(ToolError::Execution(
            "this operation requires a client with the MCP elicitation \
             capability; use the kaunta CLI instead"
                .into(),
        ));
    };
    if !requester.supports_elicitation() {
        return Err(ToolError::Execution(
            "this operation requires a client with the MCP elicitation \
             capability; use the kaunta CLI instead"
                .into(),
        ));
    }
    let schema = serde_json::to_value(ElicitationSchema::confirm())
        .map_err(|error| ToolError::Internal(error.to_string()))?;
    let result = requester
        .request_elicitation(
            message.to_owned(),
            schema,
            Some(std::time::Duration::from_secs(300)),
        )
        .await
        .map_err(|error| ToolError::Execution(format!("elicitation failed: {error}")))?;
    let confirmed = matches!(result.action, ElicitationAction::Accept)
        && result
            .content
            .as_ref()
            .and_then(|content| content.get("confirmed"))
            .and_then(serde_json::Value::as_bool)
            .unwrap_or(false);
    if confirmed {
        Ok(())
    } else {
        Err(ToolError::Execution(
            "the user did not confirm the operation".into(),
        ))
    }
}

/// Validate the `days` period argument shared by the analytics tools.
fn validate_days(days: i32) -> Result<(), ToolError> {
    if (1..=365).contains(&days) {
        Ok(())
    } else {
        Err(ToolError::InvalidArguments(
            "days must be between 1 and 365".into(),
        ))
    }
}

/// Map a database error into a tool execution error.
fn execution_error(error: impl std::fmt::Display) -> ToolError {
    ToolError::Execution(error.to_string())
}

/// Build the MCP server with kaunta's analytics tools registered.
#[must_use]
pub fn build_server(
    pool: PgPool,
    database_url: String,
    data_dir: std::path::PathBuf,
    version: &str,
) -> Server {
    let server = Server::builder("kaunta", version)
        .with_title("Kaunta Analytics")
        .with_description(
            "Read-only web analytics: tracked websites, visitor stats, \
             timeseries, breakdowns, and country data",
        )
        .with_tools(false)
        .with_mcp_apps()
        .with_circuit_breaker(ToolBreakerConfig {
            failure_threshold: 3,
            failure_window_secs: 60.0,
            half_open_timeout_secs: 10.0,
            success_threshold: 2,
            call_timeout_secs: 30.0,
        })
        .build();
    server.register_router(
        KauntaMcp::router(),
        Arc::new(KauntaMcp::new(pool, database_url, data_dir)),
    );
    server
}
