//! Explicitly opt in with `cargo test -p kaunta-e2e --test tracker -- --ignored --nocapture`.
mod harness;

use std::time::Duration;

use anyhow::{Context, Result, ensure};
use harness::{BrowserTest, Harness, USER};
use tokio::time::Instant;
use uuid::Uuid;

async fn scenario(
    name: &str,
    run: impl AsyncFnOnce(&Harness, &BrowserTest) -> Result<()>,
) -> Result<()> {
    let env = Harness::start().await?;
    let browser = match BrowserTest::start().await {
        Ok(browser) => browser,
        Err(error) => {
            env.close().await?;
            return Err(error);
        }
    };
    let result = run(&env, &browser).await;
    if result.is_err() {
        browser.screenshot(name).await;
    }
    tokio::time::sleep(Duration::from_millis(200)).await;
    if !browser.errors().is_empty() && result.is_ok() {
        browser.screenshot(name).await;
    }
    let console = browser.finish().await?;
    env.close().await?;
    result?;
    ensure!(console.is_empty(), "browser console errors: {console:#?}");
    Ok(())
}

async fn tracker_website(env: &Harness) -> Result<Uuid> {
    let user = kaunta::db::auth::get_user_by_username(env.pool(), USER)
        .await?
        .context("seeded user missing")?;
    let website = kaunta::db::websites::create(
        env.pool(),
        "127.0.0.1",
        "Tracker E2E",
        &["127.0.0.1".into()],
        Some(user.user_id),
    )
    .await?;
    Ok(website.website_id)
}

async fn insert_tracker(
    browser: &BrowserTest,
    base: &str,
    website: Uuid,
    href: &str,
) -> Result<()> {
    insert_tracker_at(browser, base, website, href, "").await
}

/// Loads the page with `query` appended, so a scenario can exercise the
/// `?kaunta_ignore=` opt-out switch.
async fn insert_tracker_at(
    browser: &BrowserTest,
    base: &str,
    website: Uuid,
    href: &str,
    query: &str,
) -> Result<()> {
    browser
        .page
        .set_user_agent(
            "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 \
             (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36",
        )
        .await?;
    browser
        .goto(base, &format!("/login{query}"), "#login-form")
        .await?;
    browser
        .js(&format!(
            "var link = document.createElement('a'); \
             link.id = 'dl'; link.href = {}; link.target = '_blank'; link.textContent = 'link'; \
             document.body.appendChild(link); \
             var script = document.createElement('script'); \
             script.src = {}; script.dataset.websiteId = {}; \
             document.body.appendChild(script)",
            serde_json::to_string(href)?,
            serde_json::to_string(&format!("{base}/k.js"))?,
            serde_json::to_string(&website.to_string())?,
        ))
        .await?;
    browser
        .wait_until("typeof window.kaunta?.track === 'function'")
        .await
}

async fn wait_for_event(env: &Harness, website: Uuid, name: &str) -> Result<()> {
    let deadline = Instant::now() + Duration::from_secs(15);
    loop {
        let found: bool = sqlx::query_scalar(
            "SELECT EXISTS(SELECT 1 FROM website_event
             WHERE website_id = $1 AND event_type = 2 AND event_name = $2)",
        )
        .bind(website)
        .bind(name)
        .fetch_one(env.pool())
        .await?;
        if found {
            return Ok(());
        }
        ensure!(Instant::now() < deadline, "timed out waiting for {name}");
        tokio::time::sleep(Duration::from_millis(100)).await;
    }
}

async fn event_count(env: &Harness, website: Uuid, name: &str) -> Result<i64> {
    Ok(sqlx::query_scalar(
        "SELECT count(*) FROM website_event
         WHERE website_id = $1 AND event_type = 2 AND event_name = $2",
    )
    .bind(website)
    .bind(name)
    .fetch_one(env.pool())
    .await?)
}

#[tokio::test]
#[ignore = "requires Chrome and PostgreSQL"]
async fn file_download_click() -> Result<()> {
    scenario("file_download_click", async |env, browser| {
        let website = tracker_website(env).await?;
        insert_tracker(
            browser,
            &env.base,
            website,
            &format!("{}/files/report.pdf", env.base),
        )
        .await?;
        browser.page.find_element("#dl").await?.click().await?;
        wait_for_event(env, website, "File Download").await?;
        ensure!(
            event_count(env, website, "File Download").await? == 1,
            "download click sent multiple events"
        );
        ensure!(
            event_count(env, website, "Outbound Link: Click").await? == 0,
            "download click also sent an outbound event"
        );
        Ok(())
    })
    .await
}

#[tokio::test]
#[ignore = "requires Chrome and PostgreSQL"]
async fn outbound_link_click() -> Result<()> {
    scenario("outbound_link_click", async |env, browser| {
        let website = tracker_website(env).await?;
        insert_tracker(browser, &env.base, website, "http://example.org/").await?;
        browser.page.find_element("#dl").await?.click().await?;
        wait_for_event(env, website, "Outbound Link: Click").await?;
        let downloads = event_count(env, website, "File Download").await?;
        ensure!(downloads == 0, "outbound click also tracked as a download");
        Ok(())
    })
    .await
}

#[tokio::test]
#[ignore = "requires Chrome and PostgreSQL"]
async fn browser_opt_out_survives_reloads() -> Result<()> {
    scenario("browser_opt_out", async |env, browser| {
        let website = tracker_website(env).await?;
        let local = format!("{}/login", env.base);
        insert_tracker_at(browser, &env.base, website, &local, "?kaunta_ignore=true").await?;
        browser
            .wait_until("localStorage.getItem('kaunta_ignore') === 'true'")
            .await?;
        insert_tracker_at(browser, &env.base, website, &local, "").await?;
        tokio::time::sleep(Duration::from_secs(2)).await;
        let recorded: i64 =
            sqlx::query_scalar("SELECT count(*) FROM website_event WHERE website_id = $1")
                .bind(website)
                .fetch_one(env.pool())
                .await?;
        ensure!(
            recorded == 0,
            "opted-out browser recorded {recorded} events"
        );
        Ok(())
    })
    .await
}
