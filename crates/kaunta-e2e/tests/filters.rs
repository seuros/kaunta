//! Explicitly opt in with `cargo test -p kaunta-e2e --test filters -- --ignored`.
mod harness;

use anyhow::{Result, ensure};
use harness::{BrowserTest, Harness};

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
    tokio::time::sleep(std::time::Duration::from_millis(200)).await;
    let console = browser.finish().await?;
    env.close().await?;
    result?;
    ensure!(console.is_empty(), "browser console errors: {console:#?}");
    Ok(())
}

const PAGEVIEWS_CARD: &str = ".portal-stats article:nth-child(2) strong";

#[tokio::test]
#[ignore = "requires Chrome and PostgreSQL"]
async fn click_to_filter() -> Result<()> {
    scenario("click_to_filter", async |env, browser| {
        browser.login(&env.base).await?;
        browser.select_website(&env.website).await?;
        browser
            .wait_until(&format!(
                "document.querySelector('{PAGEVIEWS_CARD}')?.textContent === '2'"
            ))
            .await?;
        browser
            .wait_until(
                "[...document.querySelectorAll('#breakdown-data .filter-link')].some(b => b.textContent === '/home')",
            )
            .await?;
        browser
            .js("[...document.querySelectorAll('#breakdown-data .filter-link')].find(b => b.textContent === '/home').click()")
            .await?;
        browser
            .wait_until(&format!(
                "document.querySelector('{PAGEVIEWS_CARD}')?.textContent === '1'"
            ))
            .await?;
        browser
            .wait_until(
                "[...document.querySelectorAll('.filter-chip')].some(c => c.offsetParent !== null && c.textContent.includes('/home'))",
            )
            .await?;
        browser
            .js("[...document.querySelectorAll('.filter-chip')].find(c => c.textContent.includes('/home')).click()")
            .await?;
        browser
            .wait_until(&format!(
                "document.querySelector('{PAGEVIEWS_CARD}')?.textContent === '2'"
            ))
            .await
    })
    .await
}

#[tokio::test]
#[ignore = "requires Chrome and PostgreSQL"]
async fn realtime_feed() -> Result<()> {
    scenario("realtime_feed", async |env, browser| {
        browser.login(&env.base).await?;
        browser.select_website(&env.website).await?;
        browser.wait_for_selector("#live-feed").await?;
        let response = reqwest::Client::new()
            .post(format!("{}/api/send", env.base))
            .header(
                "user-agent",
                "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 \
                 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36",
            )
            .json(&serde_json::json!({
                "type": "event",
                "payload": {
                    "website": env.website,
                    "hostname": "e2e.example",
                    "url": "https://e2e.example/live-test",
                    "title": "Live test",
                }
            }))
            .send()
            .await?;
        ensure!(
            response.status().is_success(),
            "tracker send failed: {}",
            response.status()
        );
        browser
            .wait_until(
                "document.querySelector('#live-feed')?.textContent.includes('/live-test') === true",
            )
            .await
    })
    .await
}
