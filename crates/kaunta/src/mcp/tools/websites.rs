use mcp_host::prelude::*;
use mcp_host::registry::router::McpToolRouter;
use schemars::JsonSchema;
use serde::Serialize;

use crate::mcp::KauntaMcp;

#[derive(Serialize, JsonSchema)]
pub struct WebsiteSummary {
    /// Set when the website is pending deletion (30-day grace).
    #[serde(skip_serializing_if = "Option::is_none")]
    pub pending_delete_since: Option<String>,
    /// Website UUID, usable as the `website` argument of the other tools.
    pub id: String,
    pub name: String,
    pub domain: String,
    pub public_stats_enabled: bool,
}

#[derive(Serialize, JsonSchema)]
pub struct WebsiteList {
    pub websites: Vec<WebsiteSummary>,
}

impl KauntaMcp {
    /// List every website tracked by this Kaunta instance.
    #[mcp_tool(
        name = "list_websites",
        read_only = true,
        idempotent = true,
        output = "WebsiteList"
    )]
    async fn list_websites(&self, ctx: Ctx<'_>, _params: Parameters<()>) -> ToolResult {
        let websites = match crate::mcp::session_scope(&ctx) {
            crate::mcp::Scope::User(user_id) => {
                crate::db::websites::list_for_user(&self.pool, user_id)
                    .await
                    .map_err(crate::mcp::execution_error)?
            }
            crate::mcp::Scope::Operator => crate::db::websites::list(&self.pool)
                .await
                .map_err(crate::mcp::execution_error)?,
        };
        structured(WebsiteList {
            websites: websites
                .into_iter()
                .map(|site| WebsiteSummary {
                    pending_delete_since: site.pending_delete_at.map(|t| t.to_string()),
                    id: site.website_id.to_string(),
                    name: site.name,
                    domain: site.domain,
                    public_stats_enabled: site.public_stats_enabled,
                })
                .collect(),
        })
    }
}

pub fn mount(router: McpToolRouter<KauntaMcp>) -> McpToolRouter<KauntaMcp> {
    router.with_tool(
        KauntaMcp::list_websites_tool_info(),
        KauntaMcp::list_websites_handler,
        None,
    )
}
