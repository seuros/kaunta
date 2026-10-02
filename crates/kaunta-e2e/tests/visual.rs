//! Screenshot tour for design review: captures every dashboard page into
//! target/e2e-artifacts/page-*.png. Not an assertion suite.
//! Run with `cargo test -p kaunta-e2e --test visual -- --ignored`.
mod harness;

use anyhow::Result;
use harness::{BrowserTest, Harness};

#[tokio::test]
#[ignore = "requires Chrome and PostgreSQL"]
async fn screenshot_tour() -> Result<()> {
    let env = Harness::start().await?;
    let browser = BrowserTest::start().await?;
    browser.login(&env.base).await?;
    browser.select_website(&env.website).await?;
    for (path, selector, name) in [
        ("/dashboard", ".portal-stats", "page-dashboard"),
        ("/dashboard/map", "#choropleth-map", "page-map"),
        (
            "/dashboard/campaigns",
            "#utm-source-content",
            "page-campaigns",
        ),
        ("/dashboard/goals", "#goals-container", "page-goals"),
        (
            "/dashboard/websites",
            "#websites-container",
            "page-websites",
        ),
    ] {
        browser.goto(&env.base, path, selector).await?;
        tokio::time::sleep(std::time::Duration::from_millis(1200)).await;
        browser.screenshot(name).await;
    }
    browser.finish().await?;
    env.close().await
}
