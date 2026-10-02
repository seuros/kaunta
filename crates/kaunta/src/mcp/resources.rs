//! MCP resources: the `ui://` views that the panel tools render in
//! MCP Apps hosts.

use mcp_host::prelude::*;
use mcp_host::registry::router::McpResourceRouter;

use crate::mcp::KauntaMcp;
use crate::mcp::tools::goals_app::GOALS_URI;
use crate::mcp::tools::map_app::MAP_URI;
use crate::mcp::tools::overview_app::OVERVIEW_URI;
use crate::mcp::tools::realtime_app::REALTIME_URI;

/// Self-contained HTML with no external origins, so the default deny-all
/// Same self-contained rule; the country outlines are pre-projected at
/// Every view is delivered as one self-contained document, so the shared
/// stylesheet, the JSON-RPC bridge, and (for the map) the country outlines
/// are spliced in here rather than fetched.
fn render(html: &str) -> String {
    html.replace("__SHARED_CSS__", include_str!("views/shared.css"))
        .replace("__BRIDGE_JS__", include_str!("views/bridge.js"))
        .replace("__WORLD_PATHS__", include_str!("views/world-paths.json"))
}

/// Views share this: a border helps the panel read as one surface inside a
/// chat transcript.
fn panel_ui() -> UiResourceMeta {
    UiResourceMeta {
        prefers_border: Some(true),
        ..Default::default()
    }
}

impl KauntaMcp {
    /// Visitor overview panel rendered by MCP Apps hosts.
    #[mcp_resource(
        uri = "ui://kaunta/overview",
        name = "overview_view",
        mime_type = "text/html;profile=mcp-app",
        ui_meta = "panel_ui()"
    )]
    async fn overview_view(&self, _ctx: Ctx<'_>) -> ResourceResult {
        Ok(vec![
            ResourceContent::mcp_app(OVERVIEW_URI, render(include_str!("views/overview.html")))
                .with_meta(panel_ui().into_meta()?),
        ])
    }

    /// Live activity panel rendered by MCP Apps hosts.
    #[mcp_resource(
        uri = "ui://kaunta/realtime",
        name = "realtime_view",
        mime_type = "text/html;profile=mcp-app",
        ui_meta = "panel_ui()"
    )]
    async fn realtime_view(&self, _ctx: Ctx<'_>) -> ResourceResult {
        Ok(vec![
            ResourceContent::mcp_app(REALTIME_URI, render(include_str!("views/realtime.html")))
                .with_meta(panel_ui().into_meta()?),
        ])
    }

    /// World choropleth rendered by MCP Apps hosts.
    #[mcp_resource(
        uri = "ui://kaunta/map",
        name = "map_view",
        mime_type = "text/html;profile=mcp-app",
        ui_meta = "panel_ui()"
    )]
    async fn map_view(&self, _ctx: Ctx<'_>) -> ResourceResult {
        Ok(vec![
            ResourceContent::mcp_app(MAP_URI, render(include_str!("views/map.html")))
                .with_meta(panel_ui().into_meta()?),
        ])
    }

    /// Goal panel rendered by MCP Apps hosts; it can also edit goals.
    #[mcp_resource(
        uri = "ui://kaunta/goals",
        name = "goals_view",
        mime_type = "text/html;profile=mcp-app",
        ui_meta = "panel_ui()"
    )]
    async fn goals_view(&self, _ctx: Ctx<'_>) -> ResourceResult {
        Ok(vec![
            ResourceContent::mcp_app(GOALS_URI, render(include_str!("views/goals.html")))
                .with_meta(panel_ui().into_meta()?),
        ])
    }
}

pub fn router() -> McpResourceRouter<KauntaMcp> {
    McpResourceRouter::new()
        .with_resource(
            KauntaMcp::overview_view_resource_info(),
            KauntaMcp::overview_view_handler,
            None,
        )
        .with_resource(
            KauntaMcp::realtime_view_resource_info(),
            KauntaMcp::realtime_view_handler,
            None,
        )
        .with_resource(
            KauntaMcp::map_view_resource_info(),
            KauntaMcp::map_view_handler,
            None,
        )
        .with_resource(
            KauntaMcp::goals_view_resource_info(),
            KauntaMcp::goals_view_handler,
            None,
        )
}
