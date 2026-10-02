//! The `ui://kaunta/map` choropleth. The view carries pre-projected country
//! paths, so it needs no mapping library at runtime.

use mcp_host::prelude::*;
use mcp_host::registry::router::McpToolRouter;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};

use crate::mcp::KauntaMcp;

pub const MAP_URI: &str = "ui://kaunta/map";

#[derive(Deserialize, JsonSchema)]
pub struct MapParams {
    /// Website UUID or domain (e.g. "derails.dev").
    pub website: String,
    /// Period in days (default 7).
    #[serde(default = "crate::mcp::tools::default_days")]
    pub days: i32,
}

#[derive(Serialize, JsonSchema)]
pub struct MapCountry {
    /// ISO 3166-1 numeric code, which is what the view's paths are keyed by.
    pub code: String,
    pub name: String,
    pub visitors: i64,
    pub percentage: f64,
}

#[derive(Serialize, JsonSchema)]
pub struct MapPanel {
    pub website: String,
    pub period_days: i32,
    pub total_visitors: i64,
    pub countries: Vec<MapCountry>,
}

impl KauntaMcp {
    async fn map_panel(
        &self,
        ctx: &Ctx<'_>,
        website: &str,
        days: i32,
    ) -> Result<MapPanel, ToolError> {
        crate::mcp::validate_days(days)?;
        let website_id = self.resolve_website(ctx, website).await?;
        let map = crate::db::analytics::map_data(
            &self.pool,
            website_id,
            days,
            crate::db::analytics::AnalyticsFilters::default(),
        )
        .await
        .map_err(crate::mcp::execution_error)?;
        Ok(MapPanel {
            website: website.to_owned(),
            period_days: days,
            total_visitors: map.total_visitors,
            countries: map
                .data
                .into_iter()
                .map(|point| MapCountry {
                    code: point.code,
                    name: point.country_name,
                    visitors: point.visitors,
                    percentage: point.percentage,
                })
                .collect(),
        })
    }

    /// Visitors by country as a world choropleth with a ranked table.
    #[mcp_tool(
        name = "map_panel",
        read_only = true,
        idempotent = true,
        ui = "ui://kaunta/map",
        output = "MapPanel"
    )]
    async fn map_panel_tool(&self, ctx: Ctx<'_>, params: Parameters<MapParams>) -> ToolResult {
        let MapParams { website, days } = params.0;
        structured(self.map_panel(&ctx, &website, days).await?)
    }

    /// Re-read the map for another period. Called by the rendered view.
    #[mcp_tool(
        name = "map_panel_refresh",
        read_only = true,
        idempotent = true,
        ui = "ui://kaunta/map",
        ui_visibility = "app",
        output = "MapPanel"
    )]
    async fn map_panel_refresh(&self, ctx: Ctx<'_>, params: Parameters<MapParams>) -> ToolResult {
        let MapParams { website, days } = params.0;
        structured(self.map_panel(&ctx, &website, days).await?)
    }
}

pub fn mount(router: McpToolRouter<KauntaMcp>) -> McpToolRouter<KauntaMcp> {
    router
        .with_tool(
            KauntaMcp::map_panel_tool_tool_info(),
            KauntaMcp::map_panel_tool_handler,
            None,
        )
        .with_tool(
            KauntaMcp::map_panel_refresh_tool_info(),
            KauntaMcp::map_panel_refresh_handler,
            None,
        )
}
