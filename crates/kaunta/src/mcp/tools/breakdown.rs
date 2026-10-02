use crate::db::analytics::AnalyticsFilters;
use mcp_host::prelude::*;
use mcp_host::registry::router::McpToolRouter;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};

use crate::mcp::KauntaMcp;

#[derive(Deserialize, JsonSchema)]
pub struct BreakdownParams {
    /// Website UUID or domain (e.g. "derails.dev").
    pub website: String,
    /// Dimension: page, source (resolved referrer name, e.g. "Reddit"),
    /// channel (acquisition channel, e.g. "Organic Search", "AI Assistants"),
    /// referrer, browser, device, country, city, region, os, utm_source,
    /// utm_medium, utm_campaign, utm_term, utm_content, entry_page, exit_page,
    /// event (custom event names; see props_breakdown for their properties).
    pub dimension: String,
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
pub struct BreakdownEntry {
    pub name: String,
    pub count: i64,
}

#[derive(Serialize, JsonSchema)]
pub struct BreakdownOutput {
    pub dimension: String,
    pub entries: Vec<BreakdownEntry>,
    pub total_count: i64,
    pub period_days: i32,
}

impl KauntaMcp {
    /// Breakdown of traffic by a dimension (pages, referrers, browsers,
    /// devices, countries, UTM parameters, entry/exit pages).
    #[mcp_tool(
        name = "breakdown",
        read_only = true,
        idempotent = true,
        output = "BreakdownOutput"
    )]
    async fn breakdown(&self, ctx: Ctx<'_>, params: Parameters<BreakdownParams>) -> ToolResult {
        let BreakdownParams {
            website,
            dimension,
            days,
            limit,
        } = params.0;
        let Some(dimension) = crate::web::dashboard::normalize_dimension(&dimension) else {
            return Err(ToolError::InvalidArguments(format!(
                "unknown dimension {dimension:?}; see this parameter's description for accepted values"
            )));
        };
        let dimension = dimension.to_owned();
        crate::mcp::validate_days(days)?;
        if !(1..=100).contains(&limit) {
            return Err(ToolError::InvalidArguments(
                "limit must be between 1 and 100".into(),
            ));
        }
        let website_id = self.resolve_website(&ctx, &website).await?;
        let breakdown_call = if dimension == "event" {
            crate::db::analytics::event_breakdown(
                &self.pool,
                website_id,
                days,
                limit,
                0,
                AnalyticsFilters::default(),
                "count",
                "desc",
            )
            .await
        } else if dimension == "source" || dimension == "channel" {
            crate::db::analytics::acquisition_breakdown(
                &self.pool,
                website_id,
                &dimension,
                days,
                limit,
                0,
                AnalyticsFilters::default(),
                "count",
                "desc",
            )
            .await
        } else {
            crate::db::analytics::breakdown(
                &self.pool,
                website_id,
                &dimension,
                days,
                limit,
                0,
                AnalyticsFilters::default(),
                "count",
                "desc",
            )
            .await
        };
        let (items, total_count) = breakdown_call.map_err(crate::mcp::execution_error)?;
        structured(BreakdownOutput {
            dimension,
            entries: items
                .into_iter()
                .map(|item| BreakdownEntry {
                    name: item.name,
                    count: item.count,
                })
                .collect(),
            total_count,
            period_days: days,
        })
    }
}

pub fn mount(router: McpToolRouter<KauntaMcp>) -> McpToolRouter<KauntaMcp> {
    router.with_tool(
        KauntaMcp::breakdown_tool_info(),
        KauntaMcp::breakdown_handler,
        None,
    )
}
