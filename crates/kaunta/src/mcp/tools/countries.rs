use crate::db::analytics::AnalyticsFilters;
use mcp_host::prelude::*;
use mcp_host::registry::router::McpToolRouter;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};

use crate::mcp::KauntaMcp;

#[derive(Deserialize, JsonSchema)]
pub struct CountriesParams {
    /// Website UUID or domain (e.g. "derails.dev").
    pub website: String,
    /// Period in days (default 7).
    #[serde(default = "crate::mcp::tools::default_days")]
    pub days: i32,
}

#[derive(Serialize, JsonSchema)]
pub struct CountryEntry {
    /// ISO 3166-1 alpha-2 code.
    pub code: String,
    pub name: String,
    pub visitors: i64,
    pub percentage: f64,
}

#[derive(Serialize, JsonSchema)]
pub struct CountriesOutput {
    pub countries: Vec<CountryEntry>,
    pub total_visitors: i64,
    pub period_days: i32,
}

impl KauntaMcp {
    /// Visitors by country for a website over the requested period.
    #[mcp_tool(
        name = "country_stats",
        read_only = true,
        idempotent = true,
        output = "CountriesOutput"
    )]
    async fn country_stats(&self, ctx: Ctx<'_>, params: Parameters<CountriesParams>) -> ToolResult {
        let CountriesParams { website, days } = params.0;
        crate::mcp::validate_days(days)?;
        let website_id = self.resolve_website(&ctx, &website).await?;
        let map = crate::db::analytics::map_data(
            &self.pool,
            website_id,
            days,
            AnalyticsFilters::default(),
        )
        .await
        .map_err(crate::mcp::execution_error)?;
        structured(CountriesOutput {
            countries: map
                .data
                .into_iter()
                .map(|point| CountryEntry {
                    code: point.code,
                    name: point.country_name,
                    visitors: point.visitors,
                    percentage: point.percentage,
                })
                .collect(),
            total_visitors: map.total_visitors,
            period_days: days,
        })
    }
}

pub fn mount(router: McpToolRouter<KauntaMcp>) -> McpToolRouter<KauntaMcp> {
    router.with_tool(
        KauntaMcp::country_stats_tool_info(),
        KauntaMcp::country_stats_handler,
        None,
    )
}
