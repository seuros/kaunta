//! The `ui://kaunta/goals` panel: conversion numbers plus the controls to
//! change them. The write tools stay elicitation-gated, so a click in the
//! view still asks the human before anything is written.

use mcp_host::prelude::*;
use mcp_host::registry::router::McpToolRouter;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};

use crate::db::analytics::AnalyticsFilters;
use crate::domain::goal::{GoalKind, GoalRequest};
use crate::mcp::KauntaMcp;

pub const GOALS_URI: &str = "ui://kaunta/goals";

#[derive(Deserialize, JsonSchema)]
pub struct GoalsParams {
    /// Website UUID or domain (e.g. "derails.dev").
    pub website: String,
    /// Period in days (default 7).
    #[serde(default = "crate::mcp::tools::default_days")]
    pub days: i32,
}

#[derive(Deserialize, JsonSchema)]
pub struct AddGoalParams {
    /// Website UUID or domain.
    pub website: String,
    pub name: String,
    /// "page_view" (target is a URL path) or "custom_event" (an event name).
    pub kind: String,
    /// URL path (e.g. "/signup/done") or custom event name.
    pub target: String,
    #[serde(default = "crate::mcp::tools::default_days")]
    pub days: i32,
}

#[derive(Deserialize, JsonSchema)]
pub struct DropGoalParams {
    /// Website UUID or domain.
    pub website: String,
    /// Goal UUID or name.
    pub goal: String,
    #[serde(default = "crate::mcp::tools::default_days")]
    pub days: i32,
}

#[derive(Serialize, JsonSchema)]
pub struct GoalRow {
    pub id: String,
    pub name: String,
    /// "page_view" or "custom_event".
    pub kind: String,
    pub target: String,
    pub completions: i64,
    pub unique_sessions: i64,
    pub conversion_rate: f64,
}

#[derive(Serialize, JsonSchema)]
pub struct GoalsPanel {
    pub website: String,
    pub period_days: i32,
    pub total_sessions: i64,
    pub goals: Vec<GoalRow>,
}

impl KauntaMcp {
    async fn goals_panel(
        &self,
        ctx: &Ctx<'_>,
        website: &str,
        days: i32,
    ) -> Result<GoalsPanel, ToolError> {
        crate::mcp::validate_days(days)?;
        let website_id = self.resolve_website(ctx, website).await?;
        let goals = crate::db::goals::list(&self.pool, website_id)
            .await
            .map_err(crate::mcp::execution_error)?;
        let mut rows = Vec::with_capacity(goals.len());
        let mut total_sessions = 0;
        for goal in goals {
            let analytics = crate::db::analytics::goal_analytics(
                &self.pool,
                goal.id,
                days,
                AnalyticsFilters::default(),
            )
            .await
            .map_err(crate::mcp::execution_error)?;
            total_sessions = total_sessions.max(analytics.total_sessions);
            let (kind, target) = match (&goal.target_url, &goal.target_event) {
                (Some(url), _) => ("page_view", url.clone()),
                (None, Some(event)) => ("custom_event", event.clone()),
                (None, None) => ("page_view", String::new()),
            };
            rows.push(GoalRow {
                id: goal.id.to_string(),
                name: goal.name,
                kind: kind.to_owned(),
                target,
                completions: analytics.completions,
                unique_sessions: analytics.unique_sessions,
                conversion_rate: analytics.conversion_rate,
            });
        }
        Ok(GoalsPanel {
            website: website.to_owned(),
            period_days: days,
            total_sessions,
            goals: rows,
        })
    }

    /// Goals and how they are converting, as a panel that can also add and
    /// remove them.
    #[mcp_tool(
        name = "goals_panel",
        read_only = true,
        idempotent = true,
        ui = "ui://kaunta/goals",
        output = "GoalsPanel"
    )]
    async fn goals_panel_tool(&self, ctx: Ctx<'_>, params: Parameters<GoalsParams>) -> ToolResult {
        let GoalsParams { website, days } = params.0;
        structured(self.goals_panel(&ctx, &website, days).await?)
    }

    /// Re-read the panel. Called by the rendered view.
    #[mcp_tool(
        name = "goals_panel_refresh",
        read_only = true,
        idempotent = true,
        ui = "ui://kaunta/goals",
        ui_visibility = "app",
        output = "GoalsPanel"
    )]
    async fn goals_panel_refresh(
        &self,
        ctx: Ctx<'_>,
        params: Parameters<GoalsParams>,
    ) -> ToolResult {
        let GoalsParams { website, days } = params.0;
        structured(self.goals_panel(&ctx, &website, days).await?)
    }

    /// Create a goal from the panel and return the refreshed panel. The
    /// human still confirms through elicitation before anything is written.
    #[mcp_tool(
        name = "goals_panel_add",
        idempotent = false,
        task_support = "optional",
        ui = "ui://kaunta/goals",
        ui_visibility = "app",
        visible = "crate::mcp::operator_visible(ctx)",
        output = "GoalsPanel"
    )]
    async fn goals_panel_add(&self, ctx: Ctx<'_>, params: Parameters<AddGoalParams>) -> ToolResult {
        let AddGoalParams {
            website,
            name,
            kind,
            target,
            days,
        } = params.0;
        let kind = match kind.as_str() {
            "page_view" => GoalKind::PageView,
            "custom_event" => GoalKind::CustomEvent,
            other => {
                return Err(ToolError::InvalidArguments(format!(
                    "unknown goal kind {other:?}; expected page_view or custom_event"
                )));
            }
        };
        let website_id = self.resolve_website(&ctx, &website).await?;
        crate::mcp::require_confirmation(
            &ctx,
            &format!("Create goal {name:?} ({kind:?} -> {target:?}) on {website:?}?"),
        )
        .await?;
        crate::db::goals::create(
            &self.pool,
            &GoalRequest {
                website_id,
                name,
                kind,
                value: target,
            },
        )
        .await
        .map_err(crate::mcp::execution_error)?;
        structured(self.goals_panel(&ctx, &website, days).await?)
    }

    /// Delete a goal from the panel and return the refreshed panel. The
    /// human still confirms through elicitation.
    #[mcp_tool(
        name = "goals_panel_remove",
        destructive = true,
        idempotent = false,
        task_support = "optional",
        ui = "ui://kaunta/goals",
        ui_visibility = "app",
        visible = "crate::mcp::operator_visible(ctx)",
        output = "GoalsPanel"
    )]
    async fn goals_panel_remove(
        &self,
        ctx: Ctx<'_>,
        params: Parameters<DropGoalParams>,
    ) -> ToolResult {
        let DropGoalParams {
            website,
            goal,
            days,
        } = params.0;
        let goal = self.resolve_goal(&ctx, &website, &goal).await?;
        crate::mcp::require_confirmation(
            &ctx,
            &format!(
                "Delete goal {:?} on {website:?}? Its conversion history is removed.",
                goal.name
            ),
        )
        .await?;
        crate::db::goals::delete(&self.pool, goal.id)
            .await
            .map_err(crate::mcp::execution_error)?
            .ok_or_else(|| ToolError::Execution("goal disappeared during delete".into()))?;
        structured(self.goals_panel(&ctx, &website, days).await?)
    }
}

pub fn mount(router: McpToolRouter<KauntaMcp>) -> McpToolRouter<KauntaMcp> {
    router
        .with_tool(
            KauntaMcp::goals_panel_tool_tool_info(),
            KauntaMcp::goals_panel_tool_handler,
            None,
        )
        .with_tool(
            KauntaMcp::goals_panel_refresh_tool_info(),
            KauntaMcp::goals_panel_refresh_handler,
            None,
        )
        .with_tool(
            KauntaMcp::goals_panel_add_tool_info(),
            KauntaMcp::goals_panel_add_handler,
            None,
        )
        .with_tool(
            KauntaMcp::goals_panel_remove_tool_info(),
            KauntaMcp::goals_panel_remove_handler,
            None,
        )
}
