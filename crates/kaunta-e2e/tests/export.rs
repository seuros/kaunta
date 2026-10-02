mod harness;

use anyhow::{Result, ensure};
use harness::{Harness, PASSWORD, USER};
use reqwest::header::{COOKIE, SET_COOKIE};

#[tokio::test]
#[ignore = "requires PostgreSQL"]
async fn csv_exports() -> Result<()> {
    let env = Harness::start().await?;
    let result = check_exports(&env).await;
    let cleanup = env.close().await;
    cleanup?;
    result
}

async fn check_exports(env: &Harness) -> Result<()> {
    let client = reqwest::Client::new();
    let login_page = client.get(format!("{}/login", env.base)).send().await?;
    let csrf = login_page
        .headers()
        .get_all(SET_COOKIE)
        .iter()
        .filter_map(|value| value.to_str().ok())
        .find_map(|value| value.split(';').next()?.strip_prefix("kaunta_csrf="))
        .ok_or_else(|| anyhow::anyhow!("missing CSRF cookie"))?
        .to_owned();
    let login = client
        .post(format!("{}/api/auth/login", env.base))
        .header(COOKIE, format!("kaunta_csrf={csrf}"))
        .header("x-csrf-token", &csrf)
        .json(&serde_json::json!({"username": USER, "password": PASSWORD}))
        .send()
        .await?;
    ensure!(login.status() == 200, "login returned {}", login.status());
    let session = login
        .headers()
        .get_all(SET_COOKIE)
        .iter()
        .filter_map(|value| value.to_str().ok())
        .find_map(|value| value.split(';').next()?.strip_prefix("kaunta_session="))
        .ok_or_else(|| anyhow::anyhow!("missing session cookie"))?
        .to_owned();
    let cookies = format!("kaunta_csrf={csrf}; kaunta_session={session}");
    for (kind, extra, header, marker) in [
        ("timeseries", "", "timestamp,pageviews", None),
        ("breakdown", "&dimension=page", "name,count", Some("/home")),
        (
            "breakdown",
            "&dimension=event",
            "name,count",
            Some("Signup Click"),
        ),
        (
            "countries",
            "",
            "country,country_name,visitors,percentage",
            Some("US"),
        ),
        (
            "campaigns",
            "",
            "dimension,name,visitors",
            Some("newsletter"),
        ),
    ] {
        let url = format!(
            "{}/api/dashboard/export?website_id={}&days=7&type={kind}{extra}",
            env.base, env.website
        );
        let response = client.get(&url).header(COOKIE, &cookies).send().await?;
        ensure!(response.status() == 200, "{kind}: {}", response.status());
        ensure!(
            response.headers()["content-type"] == "text/csv; charset=utf-8",
            "{kind}: wrong content type"
        );
        ensure!(
            response.headers()["content-disposition"]
                == format!("attachment; filename=\"kaunta-{kind}-7d.csv\""),
            "{kind}: wrong filename"
        );
        let body = response.text().await?;
        ensure!(body.lines().next() == Some(header), "{kind}: wrong header");
        if let Some(marker) = marker {
            ensure!(body.contains(marker), "{kind}: missing {marker}");
        } else {
            ensure!(body.lines().count() > 1, "{kind}: no data rows");
        }
    }
    let url = format!(
        "{}/api/dashboard/export?website_id={}&type=unknown",
        env.base, env.website
    );
    ensure!(
        client
            .get(&url)
            .header(COOKIE, cookies)
            .send()
            .await?
            .status()
            == 400,
        "unknown type should be rejected"
    );
    ensure!(
        client.get(&url).send().await?.status() == 401,
        "missing session should be rejected"
    );
    Ok(())
}

#[tokio::test]
#[ignore = "requires PostgreSQL"]
async fn excluded_ips_are_not_recorded() -> Result<()> {
    let env = Harness::start_with_excluded_ips(vec!["127.0.0.0/8".to_owned()]).await?;
    let result = check_exclusion(&env).await;
    env.close().await?;
    result
}

async fn check_exclusion(env: &Harness) -> Result<()> {
    let before: i64 = sqlx::query_scalar("SELECT count(*) FROM website_event")
        .fetch_one(env.pool())
        .await?;
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
                "url": "https://e2e.example/should-not-record",
            }
        }))
        .send()
        .await?;
    ensure!(
        response.status().is_success(),
        "tracker send failed: {}",
        response.status()
    );
    let body: serde_json::Value = response.json().await?;
    ensure!(
        body.get("excluded") == Some(&serde_json::Value::Bool(true)),
        "{body}"
    );
    let after: i64 = sqlx::query_scalar("SELECT count(*) FROM website_event")
        .fetch_one(env.pool())
        .await?;
    ensure!(after == before, "excluded request was recorded");
    Ok(())
}
