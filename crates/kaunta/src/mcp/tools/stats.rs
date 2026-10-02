use crate::db::analytics::AnalyticsFilters;
use mcp_host::prelude::*;
use mcp_host::registry::router::McpToolRouter;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};

use crate::mcp::KauntaMcp;

#[derive(Deserialize, JsonSchema)]
pub struct StatsParams {
    /// Website UUID or domain (e.g. "derails.dev").
    pub website: String,
    /// Period in days the stats cover (default 7).
    #[serde(default = "crate::mcp::tools::default_days")]
    pub days: i32,
}

#[derive(Serialize, JsonSchema)]
pub struct StatsOutput {
    /// Visitors active right now.
    pub current_visitors: i64,
    /// Pageviews so far today (not the whole period).
    pub today_pageviews: i64,
    /// Unique visitors so far today (not the whole period).
    pub today_visitors: i64,
    /// Bounce rate over the requested period.
    pub bounce_rate: String,
    pub period_days: i32,
}

impl KauntaMcp {
    /// Live snapshot for a website: visitors online now, today's pageviews
    /// and unique visitors, and the bounce rate over the requested period.
    /// For period totals use the timeseries or breakdown tools.
    #[mcp_tool(
        name = "website_stats",
        read_only = true,
        idempotent = true,
        output = "StatsOutput"
    )]
    async fn website_stats(&self, ctx: Ctx<'_>, params: Parameters<StatsParams>) -> ToolResult {
        let StatsParams { website, days } = params.0;
        crate::mcp::validate_days(days)?;
        let website_id = self.resolve_website(&ctx, &website).await?;
        let stats = crate::db::analytics::dashboard_stats(
            &self.pool,
            website_id,
            days,
            AnalyticsFilters::default(),
        )
        .await
        .map_err(crate::mcp::execution_error)?;
        structured(StatsOutput {
            current_visitors: stats.current_visitors,
            today_pageviews: stats.today_pageviews,
            today_visitors: stats.today_visitors,
            bounce_rate: stats.today_bounce_rate,
            period_days: days,
        })
    }
}

pub fn mount(router: McpToolRouter<KauntaMcp>) -> McpToolRouter<KauntaMcp> {
    router.with_tool(
        KauntaMcp::website_stats_tool_info(),
        KauntaMcp::website_stats_handler,
        None,
    )
}
