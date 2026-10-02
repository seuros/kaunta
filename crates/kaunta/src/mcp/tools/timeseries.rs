use crate::db::analytics::AnalyticsFilters;
use mcp_host::prelude::*;
use mcp_host::registry::router::McpToolRouter;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use time::format_description::well_known::Rfc3339;

use crate::mcp::KauntaMcp;

#[derive(Deserialize, JsonSchema)]
pub struct TimeseriesParams {
    /// Website UUID or domain (e.g. "derails.dev").
    pub website: String,
    /// Period in days (default 7).
    #[serde(default = "crate::mcp::tools::default_days")]
    pub days: i32,
}

#[derive(Serialize, JsonSchema)]
pub struct TimeseriesPoint {
    /// RFC 3339 bucket timestamp.
    pub timestamp: String,
    pub pageviews: i64,
}

#[derive(Serialize, JsonSchema)]
pub struct TimeseriesOutput {
    pub points: Vec<TimeseriesPoint>,
    pub period_days: i32,
}

impl KauntaMcp {
    /// Pageview timeseries for a website over the requested period.
    #[mcp_tool(
        name = "timeseries",
        read_only = true,
        idempotent = true,
        output = "TimeseriesOutput"
    )]
    async fn timeseries(&self, ctx: Ctx<'_>, params: Parameters<TimeseriesParams>) -> ToolResult {
        let TimeseriesParams { website, days } = params.0;
        crate::mcp::validate_days(days)?;
        let website_id = self.resolve_website(&ctx, &website).await?;
        let points = crate::db::analytics::timeseries(
            &self.pool,
            website_id,
            days,
            AnalyticsFilters::default(),
        )
        .await
        .map_err(crate::mcp::execution_error)?;
        structured(TimeseriesOutput {
            points: points
                .into_iter()
                .map(|point| TimeseriesPoint {
                    timestamp: point
                        .timestamp
                        .format(&Rfc3339)
                        .unwrap_or_else(|_| point.timestamp.to_string()),
                    pageviews: point.value,
                })
                .collect(),
            period_days: days,
        })
    }
}

pub fn mount(router: McpToolRouter<KauntaMcp>) -> McpToolRouter<KauntaMcp> {
    router.with_tool(
        KauntaMcp::timeseries_tool_info(),
        KauntaMcp::timeseries_handler,
        None,
    )
}
