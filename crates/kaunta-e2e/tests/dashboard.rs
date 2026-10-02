//! Explicitly opt in with `cargo test -p kaunta-e2e --test dashboard -- --ignored --nocapture`.
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
    if !browser.errors().is_empty() && result.is_ok() {
        browser.screenshot(name).await;
    }
    let console = browser.finish().await?;
    let cleanup = env.close().await;
    cleanup?;
    result?;
    ensure!(console.is_empty(), "browser console errors: {console:#?}");
    Ok(())
}

#[tokio::test]
#[ignore = "requires Chrome and PostgreSQL"]
async fn login() -> Result<()> {
    scenario("login", async |env, browser| {
        browser.login(&env.base).await?;
        browser
            .wait_until("document.querySelectorAll('.portal-stats article').length === 4")
            .await
    })
    .await
}

#[tokio::test]
#[ignore = "requires Chrome and PostgreSQL"]
async fn tab_navigation_persistence() -> Result<()> {
    scenario("tab_navigation_persistence", async |env, browser| {
        browser.login(&env.base).await?;
        browser.select_website(&env.website).await?;
        for (path, selector) in [
            ("/dashboard/map", "#choropleth-map"),
            ("/dashboard/campaigns", "#utm-source-content"),
            ("/dashboard", ".portal-stats"),
        ] {
            browser.goto(&env.base, path, selector).await?;
            browser
                .wait_until(&format!(
                    "document.querySelector('#website-select').value === '{}'",
                    env.website
                ))
                .await?;
        }
        Ok(())
    })
    .await
}

#[tokio::test]
#[ignore = "requires Chrome and PostgreSQL"]
async fn breakdown_switching() -> Result<()> {
    scenario("breakdown_switching", async |env, browser| {
        browser.login(&env.base).await?;
        browser.select_website(&env.website).await?;
        for (kind, expected) in [
            ("page", "/home"),
            ("source", "newsletter"),
            ("channel", "Email"),
            ("referrer", "example.org"),
            ("event", "Signup Click"),
        ] {
            browser
                .js(&format!(
                    "{{ const sel = document.querySelector('#breakdown-type'); \
                     if (sel.value !== '{kind}') {{ sel.value = '{kind}'; \
                     sel.dispatchEvent(new Event('change', {{bubbles:true}})); }} }}"
                ))
                .await?;
            browser.wait_until(&format!(
                "document.querySelector('#breakdown-data')?.textContent.includes('{expected}') === true"
            )).await?;
        }
        Ok(())
    }).await
}

#[tokio::test]
#[ignore = "requires Chrome and PostgreSQL"]
async fn choropleth() -> Result<()> {
    scenario("choropleth", async |env, browser| {
        browser.login(&env.base).await?;
        browser.select_website(&env.website).await?;
        browser
            .goto(&env.base, "/dashboard/map", "#choropleth-map")
            .await?;
        browser
            .wait_until("document.querySelectorAll('#choropleth-map svg path').length > 0")
            .await
    })
    .await
}

#[tokio::test]
#[ignore = "requires Chrome and PostgreSQL"]
async fn goal_crud() -> Result<()> {
    scenario("goal_crud", async |env, browser| {
        browser.login(&env.base).await?;
        browser.select_website(&env.website).await?;
        browser.goto(&env.base, "/dashboard/goals", "#goals-container").await?;
        browser.js(
            "for (const [id, val, ev] of [['goal-name', 'E2E Signup', 'input'], \
             ['goal-value', '/signup', 'input'], ['goal-type', 'page_view', 'change']]) { \
             const el = document.getElementById(id); el.value = val; \
             el.dispatchEvent(new Event(ev, {bubbles: true})); }",
        ).await?;
        browser.page.find_element("button[type=submit]").await?.click().await?;
        browser.wait_until("document.querySelector('#goals-container')?.textContent.includes('E2E Signup') === true").await?;
        browser.js("window.confirm = () => true; [...document.querySelectorAll('#goals-container article button')].find(b => b.textContent === 'Delete').click()").await?;
        browser.wait_until("!document.querySelector('#goals-container')?.textContent.includes('E2E Signup')").await
    }).await
}

#[tokio::test]
#[ignore = "requires Chrome and PostgreSQL"]
async fn console_hygiene() -> Result<()> {
    scenario("console_hygiene", async |env, browser| {
        browser.login(&env.base).await?;
        browser.select_website(&env.website).await?;
        for (path, selector) in [
            ("/dashboard/map", "#choropleth-map"),
            ("/dashboard/campaigns", "#utm-source-content"),
            ("/dashboard/goals", "#goals-container"),
            ("/dashboard/websites", "#websites-container"),
            ("/dashboard", ".portal-stats"),
        ] {
            browser.goto(&env.base, path, selector).await?;
        }
        Ok(())
    })
    .await
}
