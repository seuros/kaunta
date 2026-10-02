//! One tool per file; each module exposes `mount` to compose the router.

pub mod backup;
pub mod breakdown;
pub mod countries;
pub mod exclusions;
pub mod goals;
pub mod goals_app;
pub mod manage;
pub mod map_app;
pub mod overview;
pub mod overview_app;
pub mod props;
pub mod realtime_app;
pub mod stats;
pub mod timeseries;
pub mod websites;

use mcp_host::registry::router::McpToolRouter;

#[must_use]
pub fn router() -> McpToolRouter<crate::mcp::KauntaMcp> {
    let router = McpToolRouter::new();
    let router = websites::mount(router);
    let router = stats::mount(router);
    let router = timeseries::mount(router);
    let router = breakdown::mount(router);
    let router = props::mount(router);
    let router = exclusions::mount(router);
    let router = overview_app::mount(router);
    let router = realtime_app::mount(router);
    let router = map_app::mount(router);
    let router = goals_app::mount(router);
    let router = overview::mount(router);
    let router = manage::mount(router);
    let router = goals::mount(router);
    let router = backup::mount(router);
    countries::mount(router)
}

/// Default reporting window for every tool that takes one.
pub(crate) fn default_days() -> i32 {
    7
}
