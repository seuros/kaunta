use crate::db::analytics::AnalyticsFilters;
use mcp_host::prelude::*;
use mcp_host::registry::router::McpToolRouter;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};

use crate::mcp::KauntaMcp;

#[derive(Deserialize, JsonSchema)]
pub struct OverviewParams {
    /// Website UUID or domain (e.g. "derails.dev").
    pub website: String,
    /// Period in days (default 7); compared against the previous period of
    /// the same length.
    #[serde(default = "crate::mcp::tools::default_days")]
    pub days: i32,
}

#[derive(Serialize, JsonSchema)]
pub struct OverviewOutput {
    pub visitors: i64,
    pub pageviews: i64,
    /// Engagement-adjusted bounce rate for the period.
    pub bounce_rate: String,
    pub prev_visitors: i64,
    pub prev_pageviews: i64,
    pub prev_bounce_rate: String,
    /// Percentage change vs the previous period; absent when the previous
    /// period had no data.
    pub visitors_change_pct: Option<f64>,
    pub pageviews_change_pct: Option<f64>,
    pub period_days: i32,
}

fn change_pct(current: i64, previous: i64) -> Option<f64> {
    #[allow(clippy::cast_precision_loss)]
    if previous > 0 {
        Some((((current - previous) as f64 / previous as f64) * 1000.0).round() / 10.0)
    } else {
        None
    }
}

impl KauntaMcp {
    /// Whole-period totals for a website compared against the previous
    /// period of the same length: visitors, pageviews, and the
    /// engagement-adjusted bounce rate, with percentage changes.
    #[mcp_tool(
        name = "period_overview",
        read_only = true,
        idempotent = true,
        output = "OverviewOutput"
    )]
    async fn period_overview(
        &self,
        ctx: Ctx<'_>,
        params: Parameters<OverviewParams>,
    ) -> ToolResult {
        let OverviewParams { website, days } = params.0;
        crate::mcp::validate_days(days)?;
        let website_id = self.resolve_website(&ctx, &website).await?;
        let overview = crate::db::analytics::period_overview(
            &self.pool,
            website_id,
            days,
            AnalyticsFilters::default(),
        )
        .await
        .map_err(crate::mcp::execution_error)?;
        structured(OverviewOutput {
            visitors_change_pct: change_pct(overview.visitors, overview.prev_visitors),
            pageviews_change_pct: change_pct(overview.pageviews, overview.prev_pageviews),
            visitors: overview.visitors,
            pageviews: overview.pageviews,
            bounce_rate: overview.bounce_rate,
            prev_visitors: overview.prev_visitors,
            prev_pageviews: overview.prev_pageviews,
            prev_bounce_rate: overview.prev_bounce_rate,
            period_days: days,
        })
    }
}

pub fn mount(router: McpToolRouter<KauntaMcp>) -> McpToolRouter<KauntaMcp> {
    router.with_tool(
        KauntaMcp::period_overview_tool_info(),
        KauntaMcp::period_overview_handler,
        None,
    )
}
