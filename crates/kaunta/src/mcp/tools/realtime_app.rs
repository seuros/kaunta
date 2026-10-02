//! The `ui://kaunta/realtime` panel. The view polls `realtime_panel_poll`
//! itself, so a live feed costs one model call to open and nothing after.

use mcp_host::prelude::*;
use mcp_host::registry::router::McpToolRouter;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};

use crate::mcp::KauntaMcp;

pub const REALTIME_URI: &str = "ui://kaunta/realtime";

#[derive(Deserialize, JsonSchema)]
pub struct RealtimeParams {
    /// Website UUID or domain (e.g. "derails.dev").
    pub website: String,
    /// How many recent events to return (default 15, max 50).
    #[serde(default = "default_limit")]
    pub limit: i32,
}

fn default_limit() -> i32 {
    15
}

#[derive(Serialize, JsonSchema)]
pub struct RealtimeEntry {
    pub at: String,
    /// Page path for a pageview, or the custom event's name.
    pub what: String,
    /// "pageview" or "event".
    pub kind: String,
    pub country: Option<String>,
    pub browser: Option<String>,
}

#[derive(Serialize, JsonSchema)]
pub struct RealtimePanel {
    pub website: String,
    /// Sessions active in the last five minutes.
    pub current_visitors: i64,
    pub today_pageviews: i64,
    pub events: Vec<RealtimeEntry>,
}

impl KauntaMcp {
    async fn realtime_panel(
        &self,
        ctx: &Ctx<'_>,
        website: &str,
        limit: i32,
    ) -> Result<RealtimePanel, ToolError> {
        if !(1..=50).contains(&limit) {
            return Err(ToolError::InvalidArguments(
                "limit must be between 1 and 50".into(),
            ));
        }
        let website_id = self.resolve_website(ctx, website).await?;
        let stats = crate::db::analytics::dashboard_stats(
            &self.pool,
            website_id,
            1,
            crate::db::analytics::AnalyticsFilters::default(),
        )
        .await
        .map_err(crate::mcp::execution_error)?;
        let events = crate::db::realtime::recent_events(&self.pool, website_id, i64::from(limit))
            .await
            .map_err(crate::mcp::execution_error)?;
        Ok(RealtimePanel {
            website: website.to_owned(),
            current_visitors: stats.current_visitors,
            today_pageviews: stats.today_pageviews,
            events: events
                .into_iter()
                .map(|event| RealtimeEntry {
                    at: event
                        .created_at
                        .format(&time::format_description::well_known::Rfc3339)
                        .unwrap_or_else(|_| event.created_at.to_string()),
                    what: event
                        .event_name
                        .or(event.url_path)
                        .unwrap_or_else(|| "(unknown)".to_owned()),
                    kind: if event.event_type == 2 {
                        "event".to_owned()
                    } else {
                        "pageview".to_owned()
                    },
                    country: event.country,
                    browser: event.browser,
                })
                .collect(),
        })
    }

    /// Live visitor activity: who is on the site now and the events just
    /// recorded. Rendered as a self-refreshing panel in MCP Apps hosts.
    #[mcp_tool(
        name = "realtime_panel",
        read_only = true,
        idempotent = true,
        ui = "ui://kaunta/realtime",
        output = "RealtimePanel"
    )]
    async fn realtime_panel_tool(
        &self,
        ctx: Ctx<'_>,
        params: Parameters<RealtimeParams>,
    ) -> ToolResult {
        let RealtimeParams { website, limit } = params.0;
        structured(self.realtime_panel(&ctx, &website, limit).await?)
    }

    /// Poll for newer activity. Called by the rendered view on a timer, so
    /// the feed stays live without further model calls.
    #[mcp_tool(
        name = "realtime_panel_poll",
        read_only = true,
        idempotent = true,
        ui = "ui://kaunta/realtime",
        ui_visibility = "app",
        output = "RealtimePanel"
    )]
    async fn realtime_panel_poll(
        &self,
        ctx: Ctx<'_>,
        params: Parameters<RealtimeParams>,
    ) -> ToolResult {
        let RealtimeParams { website, limit } = params.0;
        structured(self.realtime_panel(&ctx, &website, limit).await?)
    }
}

pub fn mount(router: McpToolRouter<KauntaMcp>) -> McpToolRouter<KauntaMcp> {
    router
        .with_tool(
            KauntaMcp::realtime_panel_tool_tool_info(),
            KauntaMcp::realtime_panel_tool_handler,
            None,
        )
        .with_tool(
            KauntaMcp::realtime_panel_poll_tool_info(),
            KauntaMcp::realtime_panel_poll_handler,
            None,
        )
}
