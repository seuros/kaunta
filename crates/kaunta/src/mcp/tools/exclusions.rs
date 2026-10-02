use mcp_host::prelude::*;
use mcp_host::registry::router::McpToolRouter;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};

use crate::mcp::KauntaMcp;

#[derive(Serialize, JsonSchema)]
pub struct ExclusionEntry {
    pub id: String,
    /// IP address or CIDR block.
    pub rule: String,
    pub note: String,
    pub created_at: String,
}

#[derive(Serialize, JsonSchema)]
pub struct ExclusionList {
    /// Rules stored in the database. Rules set in the config file are not
    /// listed here and cannot be changed through MCP.
    pub exclusions: Vec<ExclusionEntry>,
}

#[derive(Deserialize, JsonSchema)]
pub struct AddExclusionParams {
    /// IP address or CIDR block whose traffic should never be recorded,
    /// e.g. "203.0.113.4" or "10.0.0.0/8".
    pub rule: String,
    /// Why it is excluded, shown in the dashboard.
    #[serde(default)]
    pub note: String,
}

#[derive(Deserialize, JsonSchema)]
pub struct RemoveExclusionParams {
    /// Exclusion id or the exact rule text.
    pub exclusion: String,
}

#[derive(Serialize, JsonSchema)]
pub struct RemovedExclusion {
    pub removed: String,
}

impl KauntaMcp {
    /// Addresses whose traffic kaunta discards.
    #[mcp_tool(
        name = "list_exclusions",
        read_only = true,
        idempotent = true,
        visible = "crate::mcp::operator_visible(ctx)",
        output = "ExclusionList"
    )]
    async fn list_exclusions(&self, _ctx: Ctx<'_>, _params: Parameters<()>) -> ToolResult {
        let stored = crate::db::exclusions::list(&self.pool)
            .await
            .map_err(crate::mcp::execution_error)?;
        structured(ExclusionList {
            exclusions: stored
                .into_iter()
                .map(|entry| ExclusionEntry {
                    id: entry.excluded_address_id.to_string(),
                    rule: entry.rule,
                    note: entry.note,
                    created_at: entry
                        .created_at
                        .format(&time::format_description::well_known::Rfc3339)
                        .unwrap_or_else(|_| entry.created_at.to_string()),
                })
                .collect(),
        })
    }

    /// Stop recording traffic from an address. Useful when your own address
    /// changes and you do not want your visits in the data. Requires human
    /// confirmation; operator sessions only.
    #[mcp_tool(
        name = "add_exclusion",
        idempotent = true,
        task_support = "optional",
        visible = "crate::mcp::operator_visible(ctx)"
    )]
    async fn add_exclusion(
        &self,
        ctx: Ctx<'_>,
        params: Parameters<AddExclusionParams>,
    ) -> ToolResult {
        let AddExclusionParams { rule, note } = params.0;
        let rule = rule.trim();
        if !crate::domain::config::is_ip_rule(rule) {
            return Err(ToolError::InvalidArguments(format!(
                "{rule:?} is not an IP address or CIDR block"
            )));
        }
        crate::mcp::require_confirmation(&ctx, &format!("Stop recording traffic from {rule}?"))
            .await?;
        let entry = crate::db::exclusions::add(&self.pool, rule, note.trim())
            .await
            .map_err(crate::mcp::execution_error)?;
        structured(ExclusionEntry {
            id: entry.excluded_address_id.to_string(),
            rule: entry.rule,
            note: entry.note,
            created_at: entry
                .created_at
                .format(&time::format_description::well_known::Rfc3339)
                .unwrap_or_else(|_| entry.created_at.to_string()),
        })
    }

    /// Record traffic from an address again. Requires human confirmation;
    /// operator sessions only.
    #[mcp_tool(
        name = "remove_exclusion",
        idempotent = true,
        task_support = "optional",
        visible = "crate::mcp::operator_visible(ctx)",
        output = "RemovedExclusion"
    )]
    async fn remove_exclusion(
        &self,
        ctx: Ctx<'_>,
        params: Parameters<RemoveExclusionParams>,
    ) -> ToolResult {
        let exclusion = params.0.exclusion;
        crate::mcp::require_confirmation(&ctx, &format!("Record traffic from {exclusion} again?"))
            .await?;
        let removed = crate::db::exclusions::remove(&self.pool, exclusion.trim())
            .await
            .map_err(crate::mcp::execution_error)?
            .ok_or_else(|| ToolError::InvalidArguments(format!("no exclusion {exclusion:?}")))?;
        structured(RemovedExclusion { removed })
    }
}

pub fn mount(router: McpToolRouter<KauntaMcp>) -> McpToolRouter<KauntaMcp> {
    router
        .with_tool(
            KauntaMcp::list_exclusions_tool_info(),
            KauntaMcp::list_exclusions_handler,
            None,
        )
        .with_tool(
            KauntaMcp::add_exclusion_tool_info(),
            KauntaMcp::add_exclusion_handler,
            None,
        )
        .with_tool(
            KauntaMcp::remove_exclusion_tool_info(),
            KauntaMcp::remove_exclusion_handler,
            None,
        )
}
