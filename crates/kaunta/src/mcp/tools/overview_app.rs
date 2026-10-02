//! The `ui://kaunta/overview` panel: one tool renders it, a second one
//! refreshes it from inside the view so changing the period costs no model
//! tokens.

use mcp_host::prelude::*;
use mcp_host::registry::router::McpToolRouter;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};

use crate::db::analytics::AnalyticsFilters;
use crate::mcp::KauntaMcp;

pub const OVERVIEW_URI: &str = "ui://kaunta/overview";

#[derive(Deserialize, JsonSchema)]
pub struct OverviewParams {
    /// Website UUID or domain (e.g. "derails.dev").
    pub website: String,
    /// Period in days (default 7).
    #[serde(default = "crate::mcp::tools::default_days")]
    pub days: i32,
}

#[derive(Serialize, JsonSchema)]
pub struct OverviewPoint {
    pub timestamp: String,
    pub value: i64,
}

#[derive(Serialize, JsonSchema)]
pub struct OverviewRow {
    pub name: String,
    pub count: i64,
}

#[derive(Serialize, JsonSchema)]
pub struct OverviewPanel {
    pub website: String,
    pub period_days: i32,
    pub visitors: i64,
    pub pageviews: i64,
    pub bounce_rate: String,
    pub current_visitors: i64,
    /// Same metrics over the preceding period, for the deltas.
    pub prev_visitors: i64,
    pub prev_pageviews: i64,
    pub timeseries: Vec<OverviewPoint>,
    pub top_pages: Vec<OverviewRow>,
    pub top_sources: Vec<OverviewRow>,
}

impl KauntaMcp {
    async fn overview_panel(
        &self,
        ctx: &Ctx<'_>,
        website: &str,
        days: i32,
    ) -> Result<OverviewPanel, ToolError> {
        crate::mcp::validate_days(days)?;
        let website_id = self.resolve_website(ctx, website).await?;
        let filters = AnalyticsFilters::default();
        let overview = crate::db::analytics::period_overview(&self.pool, website_id, days, filters)
            .await
            .map_err(crate::mcp::execution_error)?;
        let stats = crate::db::analytics::dashboard_stats(&self.pool, website_id, days, filters)
            .await
            .map_err(crate::mcp::execution_error)?;
        let timeseries = crate::db::analytics::timeseries(&self.pool, website_id, days, filters)
            .await
            .map_err(crate::mcp::execution_error)?;
        let (pages, _) = crate::db::analytics::top_pages(
            &self.pool, website_id, days, 5, 0, filters, "views", "desc",
        )
        .await
        .map_err(crate::mcp::execution_error)?;
        let (sources, _) = crate::db::analytics::acquisition_breakdown(
            &self.pool, website_id, "source", days, 5, 0, filters, "count", "desc",
        )
        .await
        .map_err(crate::mcp::execution_error)?;

        Ok(OverviewPanel {
            website: website.to_owned(),
            period_days: days,
            visitors: overview.visitors,
            pageviews: overview.pageviews,
            bounce_rate: overview.bounce_rate,
            current_visitors: stats.current_visitors,
            prev_visitors: overview.prev_visitors,
            prev_pageviews: overview.prev_pageviews,
            timeseries: timeseries
                .into_iter()
                .map(|point| OverviewPoint {
                    timestamp: point
                        .timestamp
                        .format(&time::format_description::well_known::Rfc3339)
                        .unwrap_or_else(|_| point.timestamp.to_string()),
                    value: point.value,
                })
                .collect(),
            top_pages: pages
                .into_iter()
                .map(|page| OverviewRow {
                    name: page.path,
                    count: page.views,
                })
                .collect(),
            top_sources: sources
                .into_iter()
                .map(|source| OverviewRow {
                    name: source.name,
                    count: source.count,
                })
                .collect(),
        })
    }

    /// Visitor overview for a website as an interactive panel: headline
    /// numbers, pageviews over time, top pages and sources.
    #[mcp_tool(
        name = "overview_panel",
        read_only = true,
        idempotent = true,
        ui = "ui://kaunta/overview",
        output = "OverviewPanel"
    )]
    async fn overview_panel_tool(
        &self,
        ctx: Ctx<'_>,
        params: Parameters<OverviewParams>,
    ) -> ToolResult {
        let OverviewParams { website, days } = params.0;
        structured(self.overview_panel(&ctx, &website, days).await?)
    }

    /// Re-read the panel for another website or period. Called by the
    /// rendered view itself, so switching periods does not go through the
    /// model.
    #[mcp_tool(
        name = "overview_panel_refresh",
        read_only = true,
        idempotent = true,
        ui = "ui://kaunta/overview",
        ui_visibility = "app",
        output = "OverviewPanel"
    )]
    async fn overview_panel_refresh(
        &self,
        ctx: Ctx<'_>,
        params: Parameters<OverviewParams>,
    ) -> ToolResult {
        let OverviewParams { website, days } = params.0;
        structured(self.overview_panel(&ctx, &website, days).await?)
    }
}

pub fn mount(router: McpToolRouter<KauntaMcp>) -> McpToolRouter<KauntaMcp> {
    router
        .with_tool(
            KauntaMcp::overview_panel_tool_tool_info(),
            KauntaMcp::overview_panel_tool_handler,
            None,
        )
        .with_tool(
            KauntaMcp::overview_panel_refresh_tool_info(),
            KauntaMcp::overview_panel_refresh_handler,
            None,
        )
}

#[cfg(test)]
mod tests {
    use super::OVERVIEW_URI;

    #[test]
    fn views_are_self_contained_and_wired_to_their_app_tools() {
        for (html, app_tool) in [
            (
                include_str!("../views/overview.html"),
                "overview_panel_refresh",
            ),
            (
                include_str!("../views/realtime.html"),
                "realtime_panel_poll",
            ),
            (include_str!("../views/map.html"), "map_panel_refresh"),
            (include_str!("../views/goals.html"), "goals_panel_add"),
        ] {
            let scanned = html.replace("http://www.w3.org/2000/svg", "");
            for forbidden in [
                "http://",
                "https://",
                "//cdn",
                "<link rel=\"stylesheet\" href",
            ] {
                assert!(!scanned.contains(forbidden), "{app_tool}: {forbidden}");
            }
            assert!(html.contains(app_tool), "{app_tool}");
        }
        assert_eq!(OVERVIEW_URI, "ui://kaunta/overview");
        assert_eq!(
            crate::mcp::tools::realtime_app::REALTIME_URI,
            "ui://kaunta/realtime"
        );
        assert_eq!(crate::mcp::tools::map_app::MAP_URI, "ui://kaunta/map");
        assert_eq!(crate::mcp::tools::goals_app::GOALS_URI, "ui://kaunta/goals");
        let paths = include_str!("../views/world-paths.json");
        assert!(
            paths.starts_with('{') && paths.contains("\"250\""),
            "world paths"
        );
    }
}
