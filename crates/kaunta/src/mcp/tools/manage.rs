//! Website CRUD for operator sessions. Every write asks the human to
//! confirm through MCP elicitation, and deletion policy is enforced by the
//! database's `website_delete_policy` trigger: sites with event data are
//! marked pending and only deletable after a 30-day grace period.

use crate::db::websites::DeleteOutcome;
use mcp_host::prelude::*;
use mcp_host::registry::router::McpToolRouter;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use time::format_description::well_known::Rfc3339;

use crate::mcp::KauntaMcp;

fn rfc3339(timestamp: time::OffsetDateTime) -> String {
    timestamp
        .format(&Rfc3339)
        .unwrap_or_else(|_| timestamp.to_string())
}

#[derive(Deserialize, JsonSchema)]
pub struct CreateWebsiteParams {
    /// Domain to track (e.g. "blog.example.com").
    pub domain: String,
    /// Display name; defaults to the domain.
    pub name: Option<String>,
}

#[derive(Serialize, JsonSchema)]
pub struct CreateWebsiteOutput {
    pub id: String,
    pub domain: String,
    pub name: String,
}

#[derive(Deserialize, JsonSchema)]
pub struct UpdateWebsiteParams {
    /// Website UUID or domain.
    pub website: String,
    /// New display name.
    pub name: Option<String>,
    /// Enable or disable the public stats page.
    pub public_stats_enabled: Option<bool>,
}

#[derive(Serialize, JsonSchema)]
pub struct UpdateWebsiteOutput {
    pub id: String,
    pub domain: String,
    pub name: String,
    pub public_stats_enabled: bool,
}

#[derive(Deserialize, JsonSchema)]
pub struct DeleteWebsiteParams {
    /// Website UUID or domain.
    pub website: String,
    /// Cancel a pending deletion instead of deleting.
    #[serde(default)]
    pub cancel: bool,
}

#[derive(Serialize, JsonSchema)]
pub struct DeleteWebsiteOutput {
    pub domain: String,
    /// "deleted", "pending" (30-day grace started or still running), or
    /// "cancelled".
    pub status: String,
    /// When the pending grace period started, if applicable.
    pub pending_since: Option<String>,
    /// Earliest moment the deletion can be completed, if pending.
    pub deletable_after: Option<String>,
}

impl KauntaMcp {
    /// Create a website to track. Requires human confirmation via
    /// elicitation; operator sessions only.
    #[mcp_tool(
        name = "create_website",
        idempotent = false,
        task_support = "optional",
        visible = "crate::mcp::operator_visible(ctx)",
        output = "CreateWebsiteOutput"
    )]
    async fn create_website(
        &self,
        ctx: Ctx<'_>,
        params: Parameters<CreateWebsiteParams>,
    ) -> ToolResult {
        let CreateWebsiteParams { domain, name } = params.0;
        let name = name.unwrap_or_else(|| domain.clone());
        crate::mcp::require_confirmation(
            &ctx,
            &format!("Create website {domain:?} (name: {name:?}) in kaunta?"),
        )
        .await?;
        let website = crate::db::websites::create(&self.pool, &domain, &name, &[], None)
            .await
            .map_err(|error| ToolError::Execution(error.to_string()))?;
        structured(CreateWebsiteOutput {
            id: website.website_id.to_string(),
            domain: website.domain,
            name: website.name,
        })
    }

    /// Update a website's name or public-stats flag. Requires human
    /// confirmation via elicitation; operator sessions only.
    #[mcp_tool(
        name = "update_website",
        idempotent = false,
        task_support = "optional",
        visible = "crate::mcp::operator_visible(ctx)",
        output = "UpdateWebsiteOutput"
    )]
    async fn update_website(
        &self,
        ctx: Ctx<'_>,
        params: Parameters<UpdateWebsiteParams>,
    ) -> ToolResult {
        let UpdateWebsiteParams {
            website,
            name,
            public_stats_enabled,
        } = params.0;
        if name.is_none() && public_stats_enabled.is_none() {
            return Err(ToolError::InvalidArguments(
                "nothing to update: provide name and/or public_stats_enabled".into(),
            ));
        }
        let website_id = self.resolve_website(&ctx, &website).await?;
        let current = crate::db::websites::get_by_id(&self.pool, website_id)
            .await
            .map_err(crate::mcp::execution_error)?;
        let mut changes = Vec::new();
        if let Some(name) = &name {
            changes.push(format!("name {:?} -> {name:?}", current.name));
        }
        if let Some(flag) = public_stats_enabled {
            changes.push(format!(
                "public stats {} -> {}",
                current.public_stats_enabled, flag
            ));
        }
        crate::mcp::require_confirmation(
            &ctx,
            &format!(
                "Update website {:?}: {}?",
                current.domain,
                changes.join(", ")
            ),
        )
        .await?;
        if let Some(name) = &name {
            crate::db::websites::update_name(&self.pool, website_id, name)
                .await
                .map_err(|error| ToolError::Execution(error.to_string()))?;
        }
        if let Some(flag) = public_stats_enabled {
            crate::db::websites::set_public_stats_enabled(&self.pool, website_id, flag)
                .await
                .map_err(|error| ToolError::Execution(error.to_string()))?;
        }
        let updated = crate::db::websites::get_by_id(&self.pool, website_id)
            .await
            .map_err(crate::mcp::execution_error)?;
        structured(UpdateWebsiteOutput {
            id: updated.website_id.to_string(),
            domain: updated.domain,
            name: updated.name,
            public_stats_enabled: updated.public_stats_enabled,
        })
    }

    /// Delete a website, or cancel a pending deletion. The database policy
    /// decides the outcome: a site with event data is marked pending and
    /// can only be deleted after a 30-day grace period. Requires human
    /// confirmation via elicitation; operator sessions only.
    #[mcp_tool(
        name = "delete_website",
        destructive = true,
        idempotent = false,
        task_support = "optional",
        visible = "crate::mcp::operator_visible(ctx)",
        output = "DeleteWebsiteOutput"
    )]
    async fn delete_website(
        &self,
        ctx: Ctx<'_>,
        params: Parameters<DeleteWebsiteParams>,
    ) -> ToolResult {
        let DeleteWebsiteParams { website, cancel } = params.0;
        let website_id = self.resolve_website(&ctx, &website).await?;
        let current = crate::db::websites::get_by_id(&self.pool, website_id)
            .await
            .map_err(crate::mcp::execution_error)?;

        if cancel {
            let cancelled = crate::db::websites::cancel_pending_delete(&self.pool, website_id)
                .await
                .map_err(|error| ToolError::Execution(error.to_string()))?;
            if !cancelled {
                return Err(ToolError::InvalidArguments(format!(
                    "website {:?} has no pending deletion",
                    current.domain
                )));
            }
            return structured(DeleteWebsiteOutput {
                domain: current.domain,
                status: "cancelled".into(),
                pending_since: None,
                deletable_after: None,
            });
        }

        let events = crate::db::websites::events_count(&self.pool, website_id)
            .await
            .map_err(|error| ToolError::Execution(error.to_string()))?;
        let message = match (events, current.pending_delete_at) {
            (0, _) => format!(
                "Delete website {:?}? It has no event data and will be \
                 soft-deleted immediately.",
                current.domain
            ),
            (n, None) => format!(
                "Website {:?} has {n} events. Deleting will mark it pending; \
                 it can be deleted for real 30 days later. Proceed?",
                current.domain
            ),
            (n, Some(since)) => format!(
                "Website {:?} ({n} events) has been pending deletion since {}. \
                 Attempt to complete the deletion now?",
                current.domain,
                rfc3339(since)
            ),
        };
        crate::mcp::require_confirmation(&ctx, &message).await?;

        match crate::db::websites::soft_delete(&self.pool, website_id)
            .await
            .map_err(|error| ToolError::Execution(error.to_string()))?
        {
            DeleteOutcome::Deleted(_) => structured(DeleteWebsiteOutput {
                domain: current.domain,
                status: "deleted".into(),
                pending_since: None,
                deletable_after: None,
            }),
            DeleteOutcome::PendingSince(since) => structured(DeleteWebsiteOutput {
                domain: current.domain,
                status: "pending".into(),
                pending_since: Some(rfc3339(since)),
                deletable_after: Some(rfc3339(since + time::Duration::days(30))),
            }),
        }
    }
}

#[derive(Deserialize, JsonSchema)]
pub struct RestoreWebsiteParams {
    /// Website UUID or domain of a soft-deleted website.
    pub website: String,
}

#[derive(Serialize, JsonSchema)]
pub struct RestoreWebsiteOutput {
    pub id: String,
    pub domain: String,
    pub name: String,
}

impl KauntaMcp {
    /// Restore a soft-deleted website (undo delete_website). Fails when a
    /// live website with the same domain exists. Requires human
    /// confirmation via elicitation; operator sessions only.
    #[mcp_tool(
        name = "restore_website",
        idempotent = false,
        task_support = "optional",
        visible = "crate::mcp::operator_visible(ctx)",
        output = "RestoreWebsiteOutput"
    )]
    async fn restore_website(
        &self,
        ctx: Ctx<'_>,
        params: Parameters<RestoreWebsiteParams>,
    ) -> ToolResult {
        let RestoreWebsiteParams { website } = params.0;
        let deleted = crate::db::websites::find_deleted(&self.pool, &website)
            .await
            .map_err(|error| ToolError::InvalidArguments(error.to_string()))?;
        crate::mcp::require_confirmation(
            &ctx,
            &format!(
                "Restore soft-deleted website {:?} (name: {:?})?",
                deleted.domain, deleted.name
            ),
        )
        .await?;
        let restored = crate::db::websites::restore(&self.pool, deleted.website_id)
            .await
            .map_err(|error| ToolError::Execution(error.to_string()))?;
        structured(RestoreWebsiteOutput {
            id: restored.website_id.to_string(),
            domain: restored.domain,
            name: restored.name,
        })
    }
}

pub fn mount(router: McpToolRouter<KauntaMcp>) -> McpToolRouter<KauntaMcp> {
    router
        .with_tool(
            KauntaMcp::create_website_tool_info(),
            KauntaMcp::create_website_handler,
            None,
        )
        .with_tool(
            KauntaMcp::update_website_tool_info(),
            KauntaMcp::update_website_handler,
            None,
        )
        .with_tool(
            KauntaMcp::delete_website_tool_info(),
            KauntaMcp::delete_website_handler,
            None,
        )
        .with_tool(
            KauntaMcp::restore_website_tool_info(),
            KauntaMcp::restore_website_handler,
            None,
        )
}
