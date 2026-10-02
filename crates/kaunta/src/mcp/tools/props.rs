use mcp_host::prelude::*;
use mcp_host::registry::router::McpToolRouter;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};

use crate::mcp::KauntaMcp;

#[derive(Deserialize, JsonSchema)]
pub struct PropsBreakdownParams {
    /// Website UUID or domain (e.g. "derails.dev").
    pub website: String,
    /// Custom event name whose properties to inspect.
    pub event: String,
    /// Property key to break down by value; omit to list the event's
    /// property keys instead.
    pub prop: Option<String>,
    /// Period in days (default 7).
    #[serde(default = "crate::mcp::tools::default_days")]
    pub days: i32,
    /// Maximum rows to return (default 10, max 100).
    #[serde(default = "default_limit")]
    pub limit: i32,
}

fn default_limit() -> i32 {
    10
}

#[derive(Serialize, JsonSchema)]
pub struct PropsEntry {
    pub name: String,
    pub count: i64,
}

#[derive(Serialize, JsonSchema)]
pub struct PropsBreakdownOutput {
    pub event: String,
    /// The property key the entries are values of, or null when the
    /// entries are the event's property keys themselves.
    pub prop: Option<String>,
    pub entries: Vec<PropsEntry>,
    pub period_days: i32,
}

impl KauntaMcp {
    /// Property keys of a custom event, or the top values of one property.
    /// Call without `prop` to discover keys, then with `prop` to drill in.
    #[mcp_tool(
        name = "props_breakdown",
        read_only = true,
        idempotent = true,
        output = "PropsBreakdownOutput"
    )]
    async fn props_breakdown(
        &self,
        ctx: Ctx<'_>,
        params: Parameters<PropsBreakdownParams>,
    ) -> ToolResult {
        let PropsBreakdownParams {
            website,
            event,
            prop,
            days,
            limit,
        } = params.0;
        crate::mcp::validate_days(days)?;
        if !(1..=100).contains(&limit) {
            return Err(ToolError::InvalidArguments(
                "limit must be between 1 and 100".into(),
            ));
        }
        let website_id = self.resolve_website(&ctx, &website).await?;
        let items = crate::db::analytics::event_props(
            &self.pool,
            website_id,
            &event,
            prop.as_deref(),
            days,
            limit,
        )
        .await
        .map_err(crate::mcp::execution_error)?;
        structured(PropsBreakdownOutput {
            event,
            prop,
            entries: items
                .into_iter()
                .map(|item| PropsEntry {
                    name: item.name,
                    count: item.count,
                })
                .collect(),
            period_days: days,
        })
    }
}

pub fn mount(router: McpToolRouter<KauntaMcp>) -> McpToolRouter<KauntaMcp> {
    router.with_tool(
        KauntaMcp::props_breakdown_tool_info(),
        KauntaMcp::props_breakdown_handler,
        None,
    )
}
