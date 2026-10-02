//! Goal tools: reads for any session, writes elicitation-gated and
//! operator-only, matching the website management tools.

use crate::db::analytics::AnalyticsFilters;
use crate::domain::goal::{Goal, GoalKind, GoalRequest};
use mcp_host::prelude::*;
use mcp_host::registry::router::McpToolRouter;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use time::format_description::well_known::Rfc3339;
use uuid::Uuid;

use crate::mcp::KauntaMcp;

#[derive(Serialize, JsonSchema)]
pub struct GoalSummary {
    /// Goal UUID, usable as the `goal` argument of the other goal tools.
    pub id: String,
    pub name: String,
    /// "page_view" or "custom_event".
    pub kind: String,
    /// The URL path or event name the goal matches.
    pub target: String,
    pub created_at: String,
}

fn summarize(goal: Goal) -> GoalSummary {
    let (kind, target) = match (&goal.target_url, &goal.target_event) {
        (Some(url), _) => ("page_view", url.clone()),
        (None, Some(event)) => ("custom_event", event.clone()),
        (None, None) => ("page_view", String::new()),
    };
    GoalSummary {
        id: goal.id.to_string(),
        name: goal.name,
        kind: kind.to_owned(),
        target,
        created_at: goal
            .created_at
            .format(&Rfc3339)
            .unwrap_or_else(|_| goal.created_at.to_string()),
    }
}

impl KauntaMcp {
    /// Resolve a goal by UUID or name within a website the session may
    /// access, returning the goal.
    pub(crate) async fn resolve_goal(
        &self,
        ctx: &Ctx<'_>,
        website: &str,
        goal: &str,
    ) -> Result<Goal, ToolError> {
        let website_id = self.resolve_website(ctx, website).await?;
        let goals = crate::db::goals::list(&self.pool, website_id)
            .await
            .map_err(crate::mcp::execution_error)?;
        let by_id = Uuid::parse_str(goal).ok();
        goals
            .into_iter()
            .find(|candidate| {
                by_id.is_some_and(|id| candidate.id == id)
                    || candidate.name.eq_ignore_ascii_case(goal)
            })
            .ok_or_else(|| {
                ToolError::InvalidArguments(format!("goal {goal:?} not found on {website:?}"))
            })
    }
}

#[derive(Deserialize, JsonSchema)]
pub struct ListGoalsParams {
    /// Website UUID or domain.
    pub website: String,
}

#[derive(Serialize, JsonSchema)]
pub struct GoalList {
    pub goals: Vec<GoalSummary>,
}

#[derive(Deserialize, JsonSchema)]
pub struct GoalStatsParams {
    /// Website UUID or domain.
    pub website: String,
    /// Goal UUID or goal name.
    pub goal: String,
    /// Period in days (default 7).
    #[serde(default = "crate::mcp::tools::default_days")]
    pub days: i32,
}

#[derive(Serialize, JsonSchema)]
pub struct GoalStatsOutput {
    pub goal: String,
    pub completions: i64,
    pub unique_sessions: i64,
    /// Percentage of sessions that completed the goal.
    pub conversion_rate: f64,
    pub total_sessions: i64,
    pub period_days: i32,
}

#[derive(Deserialize, JsonSchema)]
pub struct CreateGoalParams {
    /// Website UUID or domain.
    pub website: String,
    pub name: String,
    /// "page_view" (target is a URL path) or "custom_event" (target is an
    /// event name).
    pub kind: String,
    /// URL path (e.g. "/signup/done") or custom event name.
    pub target: String,
}

#[derive(Deserialize, JsonSchema)]
pub struct UpdateGoalParams {
    /// Website UUID or domain.
    pub website: String,
    /// Goal UUID or current goal name.
    pub goal: String,
    pub name: Option<String>,
    /// "page_view" or "custom_event"; required when changing target.
    pub kind: Option<String>,
    pub target: Option<String>,
}

#[derive(Deserialize, JsonSchema)]
pub struct DeleteGoalParams {
    /// Website UUID or domain.
    pub website: String,
    /// Goal UUID or goal name.
    pub goal: String,
}

#[derive(Serialize, JsonSchema)]
pub struct DeleteGoalOutput {
    pub deleted: String,
}

fn parse_kind(kind: &str) -> Result<GoalKind, ToolError> {
    match kind {
        "page_view" => Ok(GoalKind::PageView),
        "custom_event" => Ok(GoalKind::CustomEvent),
        other => Err(ToolError::InvalidArguments(format!(
            "unknown goal kind {other:?}; expected page_view or custom_event"
        ))),
    }
}

impl KauntaMcp {
    /// List the goals configured for a website.
    #[mcp_tool(
        name = "list_goals",
        read_only = true,
        idempotent = true,
        output = "GoalList"
    )]
    async fn list_goals(&self, ctx: Ctx<'_>, params: Parameters<ListGoalsParams>) -> ToolResult {
        let website_id = self.resolve_website(&ctx, &params.0.website).await?;
        let goals = crate::db::goals::list(&self.pool, website_id)
            .await
            .map_err(crate::mcp::execution_error)?;
        structured(GoalList {
            goals: goals.into_iter().map(summarize).collect(),
        })
    }

    /// Conversion stats for a goal: completions, unique converting
    /// sessions, and conversion rate over the period.
    #[mcp_tool(
        name = "goal_stats",
        read_only = true,
        idempotent = true,
        output = "GoalStatsOutput"
    )]
    async fn goal_stats(&self, ctx: Ctx<'_>, params: Parameters<GoalStatsParams>) -> ToolResult {
        let GoalStatsParams {
            website,
            goal,
            days,
        } = params.0;
        crate::mcp::validate_days(days)?;
        let goal = self.resolve_goal(&ctx, &website, &goal).await?;
        let analytics = crate::db::analytics::goal_analytics(
            &self.pool,
            goal.id,
            days,
            AnalyticsFilters::default(),
        )
        .await
        .map_err(crate::mcp::execution_error)?;
        structured(GoalStatsOutput {
            goal: goal.name,
            completions: analytics.completions,
            unique_sessions: analytics.unique_sessions,
            conversion_rate: analytics.conversion_rate,
            total_sessions: analytics.total_sessions,
            period_days: days,
        })
    }

    /// Create a goal. Requires human confirmation via elicitation;
    /// operator sessions only.
    #[mcp_tool(
        name = "create_goal",
        idempotent = false,
        task_support = "optional",
        visible = "crate::mcp::operator_visible(ctx)"
    )]
    async fn create_goal(&self, ctx: Ctx<'_>, params: Parameters<CreateGoalParams>) -> ToolResult {
        let CreateGoalParams {
            website,
            name,
            kind,
            target,
        } = params.0;
        let kind = parse_kind(&kind)?;
        let website_id = self.resolve_website(&ctx, &website).await?;
        crate::mcp::require_confirmation(
            &ctx,
            &format!("Create goal {name:?} ({kind:?} -> {target:?}) on {website:?}?"),
        )
        .await?;
        let goal = crate::db::goals::create(
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
        structured(summarize(goal))
    }

    /// Update a goal's name, kind, or target. Requires human confirmation
    /// via elicitation; operator sessions only.
    #[mcp_tool(
        name = "update_goal",
        idempotent = false,
        task_support = "optional",
        visible = "crate::mcp::operator_visible(ctx)"
    )]
    async fn update_goal(&self, ctx: Ctx<'_>, params: Parameters<UpdateGoalParams>) -> ToolResult {
        let UpdateGoalParams {
            website,
            goal,
            name,
            kind,
            target,
        } = params.0;
        if name.is_none() && target.is_none() {
            return Err(ToolError::InvalidArguments(
                "nothing to update: provide name and/or kind+target".into(),
            ));
        }
        let current = self.resolve_goal(&ctx, &website, &goal).await?;
        let current_summary = summarize(current.clone());
        let new_name = name.unwrap_or_else(|| current_summary.name.clone());
        let new_kind = match kind {
            Some(kind) => parse_kind(&kind)?,
            None => match current_summary.kind.as_str() {
                "custom_event" => GoalKind::CustomEvent,
                _ => GoalKind::PageView,
            },
        };
        let new_target = target.unwrap_or_else(|| current_summary.target.clone());
        crate::mcp::require_confirmation(
            &ctx,
            &format!(
                "Update goal {:?} on {website:?} to name {new_name:?}, {new_kind:?} -> {new_target:?}?",
                current_summary.name
            ),
        )
        .await?;
        let updated = crate::db::goals::update(
            &self.pool,
            current.id,
            &GoalRequest {
                website_id: current.website_id,
                name: new_name,
                kind: new_kind,
                value: new_target,
            },
        )
        .await
        .map_err(crate::mcp::execution_error)?
        .ok_or_else(|| ToolError::Execution("goal disappeared during update".into()))?;
        structured(summarize(updated))
    }

    /// Delete a goal and its recorded completions. Requires human
    /// confirmation via elicitation; operator sessions only.
    #[mcp_tool(
        name = "delete_goal",
        destructive = true,
        idempotent = false,
        task_support = "optional",
        visible = "crate::mcp::operator_visible(ctx)",
        output = "DeleteGoalOutput"
    )]
    async fn delete_goal(&self, ctx: Ctx<'_>, params: Parameters<DeleteGoalParams>) -> ToolResult {
        let DeleteGoalParams { website, goal } = params.0;
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
        structured(DeleteGoalOutput { deleted: goal.name })
    }
}

pub fn mount(router: McpToolRouter<KauntaMcp>) -> McpToolRouter<KauntaMcp> {
    router
        .with_tool(
            KauntaMcp::list_goals_tool_info(),
            KauntaMcp::list_goals_handler,
            None,
        )
        .with_tool(
            KauntaMcp::goal_stats_tool_info(),
            KauntaMcp::goal_stats_handler,
            None,
        )
        .with_tool(
            KauntaMcp::create_goal_tool_info(),
            KauntaMcp::create_goal_handler,
            None,
        )
        .with_tool(
            KauntaMcp::update_goal_tool_info(),
            KauntaMcp::update_goal_handler,
            None,
        )
        .with_tool(
            KauntaMcp::delete_goal_tool_info(),
            KauntaMcp::delete_goal_handler,
            None,
        )
}
