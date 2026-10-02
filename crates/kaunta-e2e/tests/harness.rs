#![allow(dead_code)]

use std::{net::TcpListener, time::Duration};

use anyhow::{Context, Result, ensure};
use chromiumoxide::{
    Browser, BrowserConfig, Page, cdp::js_protocol::runtime::EventConsoleApiCalled,
};
use futures_util::StreamExt;
use kaunta::domain::event::{EventInsert, SessionAttributes, SessionUpsert};
use sqlx::PgPool;
use tempfile::TempDir;
use time::OffsetDateTime;
use tokio::{task::JoinHandle, time::Instant};
use uuid::Uuid;

pub const USER: &str = "e2e_user";
pub const PASSWORD: &str = "e2e_password_123";

pub struct Harness {
    pub base: String,
    pub website: String,
    admin: PgPool,
    pool: PgPool,
    db_name: String,
    server: JoinHandle<Result<()>>,
    _data: TempDir,
}

impl Harness {
    pub fn pool(&self) -> &PgPool {
        &self.pool
    }

    pub async fn start() -> Result<Self> {
        Self::start_with_excluded_ips(Vec::new()).await
    }

    /// Boots a server that refuses to record traffic from `excluded_ips`.
    pub async fn start_with_excluded_ips(excluded_ips: Vec<String>) -> Result<Self> {
        let admin_url = std::env::var("KAUNTA_E2E_ADMIN_URL").unwrap_or_else(|_| {
            "postgresql://ubuntu:ubuntu@localhost:5432/ubuntu?sslmode=disable".into()
        });
        let admin = kaunta::db::connect(&admin_url)
            .await
            .context("connect admin database")?;
        let db_name = format!("kaunta_e2e_{}", Uuid::new_v4().simple());
        sqlx::query(sqlx::AssertSqlSafe(format!("CREATE DATABASE {db_name}")))
            .execute(&admin)
            .await?;
        let mut url = url::Url::parse(&admin_url)?;
        url.set_path(&format!("/{db_name}"));
        let database_url = url.to_string();
        let result =
            Self::initialize(admin.clone(), db_name.clone(), database_url, excluded_ips).await;
        if result.is_err() {
            let _ = sqlx::query(sqlx::AssertSqlSafe(format!(
                "DROP DATABASE {db_name} WITH (FORCE)"
            )))
            .execute(&admin)
            .await;
        }
        result
    }

    #[allow(clippy::too_many_lines)]
    async fn initialize(
        admin: PgPool,
        db_name: String,
        database_url: String,
        excluded_ips: Vec<String>,
    ) -> Result<Self> {
        let pool = kaunta::db::connect(&database_url).await?;
        kaunta::db::migrations::run(&pool).await?;
        let user = kaunta::db::auth::create_user(&pool, USER, PASSWORD, "E2E Tester").await?;
        let website = kaunta::db::websites::create(
            &pool,
            "e2e.example",
            "E2E Website",
            &["e2e.example".into()],
            Some(user.user_id),
        )
        .await?;
        let session_id = Uuid::new_v4();
        let now = OffsetDateTime::now_utc();
        kaunta::db::events::upsert_session(
            &pool,
            &SessionUpsert {
                session_id,
                website_id: website.website_id,
                created_at: now,
                attributes: SessionAttributes {
                    hostname: Some("e2e.example".into()),
                    country: Some("US".into()),
                    browser: Some("Chrome".into()),
                    device: Some("desktop".into()),
                    ..Default::default()
                },
            },
        )
        .await?;
        for (path, source) in [("/home", "newsletter"), ("/pricing", "search")] {
            kaunta::db::events::insert_event(
                &pool,
                &EventInsert {
                    event_id: Uuid::new_v4(),
                    website_id: website.website_id,
                    session_id,
                    visit_id: Uuid::new_v4(),
                    created_at: now,
                    url_path: Some(path.into()),
                    url_query: None,
                    referrer_path: None,
                    referrer_query: None,
                    referrer_domain: Some("example.org".into()),
                    page_title: Some("E2E page".into()),
                    hostname: Some("e2e.example".into()),
                    event_type: 1,
                    event_name: None,
                    tag: None,
                    scroll_depth: None,
                    engagement_time: None,
                    props: None,
                    utm_source: Some(source.into()),
                    utm_medium: Some("email".into()),
                    utm_campaign: Some("launch".into()),
                    utm_term: None,
                    utm_content: None,
                    goal_id: None,
                },
            )
            .await?;
        }
        kaunta::db::events::insert_event(
            &pool,
            &EventInsert {
                event_id: Uuid::new_v4(),
                website_id: website.website_id,
                session_id,
                visit_id: Uuid::new_v4(),
                created_at: now,
                url_path: Some("/pricing".into()),
                url_query: None,
                referrer_path: None,
                referrer_query: None,
                referrer_domain: None,
                page_title: None,
                hostname: Some("e2e.example".into()),
                event_type: 2,
                event_name: Some("Signup Click".into()),
                tag: None,
                scroll_depth: None,
                engagement_time: None,
                props: Some(serde_json::json!({"plan": "team"})),
                utm_source: None,
                utm_medium: None,
                utm_campaign: None,
                utm_term: None,
                utm_content: None,
                goal_id: None,
            },
        )
        .await?;
        let data = tempfile::tempdir()?;
        let listener = TcpListener::bind("127.0.0.1:0")?;
        let port = listener.local_addr()?.port();
        drop(listener);
        let config = kaunta::domain::config::Config {
            database_url: database_url.clone(),
            data_dir: data.path().to_path_buf(),
            secure_cookies: false,
            port: port.to_string(),
            trusted_origins: vec![format!("127.0.0.1:{port}")],
            excluded_ips,
            ..Default::default()
        };
        let mcp = kaunta::mcp::http::service(
            pool.clone(),
            database_url,
            data.path().to_path_buf(),
            env!("CARGO_PKG_VERSION"),
        )?;
        let state = kaunta::web::AppState {
            pool: pool.clone(),
            version: env!("CARGO_PKG_VERSION"),
            config,
            geoip: kaunta::web::geoip::GeoIp::default(),
            realtime: kaunta::web::realtime::RealtimeHub::new(),
            login_attempts: kaunta::web::rate_limit::AttemptLimiter::default(),
            mcp,
            exclusions: kaunta::web::ExclusionCache::default(),
        };
        let server = tokio::spawn(async move { kaunta::web::server::serve(state, port).await });
        let base = format!("http://127.0.0.1:{port}");
        let client = reqwest::Client::new();
        let deadline = Instant::now() + Duration::from_secs(15);
        loop {
            if client
                .get(format!("{base}/up"))
                .send()
                .await
                .is_ok_and(|r| r.status().is_success())
            {
                break;
            }
            ensure!(Instant::now() < deadline, "server did not start");
            tokio::time::sleep(Duration::from_millis(100)).await;
        }
        Ok(Self {
            base,
            website: website.website_id.to_string(),
            admin,
            pool,
            db_name,
            server,
            _data: data,
        })
    }

    pub async fn close(self) -> Result<()> {
        self.server.abort();
        let _ = self.server.await;
        self.pool.close().await;
        sqlx::query(sqlx::AssertSqlSafe(format!(
            "DROP DATABASE {} WITH (FORCE)",
            self.db_name
        )))
        .execute(&self.admin)
        .await?;
        self.admin.close().await;
        Ok(())
    }
}

pub struct BrowserTest {
    pub browser: Browser,
    pub page: Page,
    handler: JoinHandle<()>,
    console: JoinHandle<()>,
    errors: std::sync::Arc<std::sync::Mutex<Vec<String>>>,
    _profile: tempfile::TempDir,
}

impl BrowserTest {
    pub async fn start() -> Result<Self> {
        let chrome = std::env::var("CHROME").unwrap_or_else(|_| {
            "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome".into()
        });
        let profile = tempfile::tempdir()?;
        let config = BrowserConfig::builder()
            .chrome_executable(chrome)
            .no_sandbox()
            .user_data_dir(profile.path())
            .build()
            .map_err(|e| anyhow::anyhow!("{e}"))?;
        let (browser, mut handler) = Browser::launch(config).await?;
        let task = tokio::spawn(async move {
            while let Some(event) = handler.next().await {
                if let Err(error) = event {
                    eprintln!("CDP: {error}");
                }
            }
        });
        let page = browser.new_page("about:blank").await?;
        let mut events = page.event_listener::<EventConsoleApiCalled>().await?;
        let errors = std::sync::Arc::new(std::sync::Mutex::new(Vec::new()));
        let collected = errors.clone();
        let console = tokio::spawn(async move {
            while let Some(event) = events.next().await {
                let text = format!("{event:?}");
                eprintln!("browser console: {text}");
                if format!("{:?}", event.r#type)
                    .to_ascii_lowercase()
                    .contains("error")
                {
                    collected.lock().expect("console lock").push(text);
                }
            }
        });
        Ok(Self {
            browser,
            page,
            handler: task,
            console,
            errors,
            _profile: profile,
        })
    }

    pub async fn goto(&self, base: &str, path: &str, selector: &str) -> Result<()> {
        self.page.goto(format!("{base}{path}")).await?;
        self.wait_for_selector(selector).await
    }

    pub async fn wait_for_selector(&self, selector: &str) -> Result<()> {
        let script = format!(
            "!!document.querySelector({})",
            serde_json::to_string(selector)?
        );
        self.wait_until(&script).await
    }

    pub async fn wait_until(&self, js: &str) -> Result<()> {
        let deadline = Instant::now() + Duration::from_secs(30);
        let mut last_error = None;
        loop {
            match self.page.evaluate(js).await {
                Ok(value) => {
                    if value.into_value::<bool>().unwrap_or(false) {
                        return Ok(());
                    }
                }
                Err(error) => last_error = Some(error),
            }
            ensure!(
                Instant::now() < deadline,
                "timed out waiting for: {js}{}",
                last_error.map_or_else(String::new, |e| format!(" (last error: {e})"))
            );
            tokio::time::sleep(Duration::from_millis(100)).await;
        }
    }

    pub async fn js(&self, script: &str) -> Result<()> {
        self.page.evaluate(script).await?;
        Ok(())
    }

    pub async fn login(&self, base: &str) -> Result<()> {
        self.goto(base, "/login", "#login-form").await?;
        self.page
            .find_element("#username")
            .await?
            .click()
            .await?
            .type_str(USER)
            .await?;
        self.page
            .find_element("#password")
            .await?
            .click()
            .await?
            .type_str(PASSWORD)
            .await?;
        self.page
            .find_element("#login-form button")
            .await?
            .click()
            .await?;
        if let Err(error) = self.wait_for_selector(".portal-stats article").await {
            let debug = self
                .page
                .evaluate(
                    "JSON.stringify({href: location.href, error: document.querySelector('#login-error')?.textContent, cookies: document.cookie})",
                )
                .await?
                .into_value::<String>()
                .unwrap_or_default();
            eprintln!("login debug: {debug}");
            return Err(error);
        }
        Ok(())
    }

    pub async fn select_website(&self, website: &str) -> Result<()> {
        self.wait_for_selector(&format!("#website-select option[value='{website}']"))
            .await?;
        self.js(&format!(
            "const sel = document.querySelector('#website-select'); \
             if (sel.value !== '{website}') {{ sel.value = '{website}'; \
             sel.dispatchEvent(new Event('change', {{bubbles:true}})); }} \
             else {{ localStorage.setItem('kaunta.website', '{website}'); }}"
        ))
        .await?;
        self.wait_until(&format!(
            "localStorage.getItem('kaunta.website') === '{website}'"
        ))
        .await
    }

    pub async fn finish(mut self) -> Result<Vec<String>> {
        self.browser.close().await?;
        let errors = self.errors.lock().expect("console lock").clone();
        self.handler.abort();
        self.console.abort();
        Ok(errors)
    }

    #[must_use]
    pub fn errors(&self) -> Vec<String> {
        self.errors.lock().expect("console lock").clone()
    }

    pub async fn screenshot(&self, name: &str) {
        let dir =
            std::path::Path::new(env!("CARGO_MANIFEST_DIR")).join("../../target/e2e-artifacts");
        if let Err(error) = std::fs::create_dir_all(&dir) {
            eprintln!("artifact directory: {error}");
            return;
        }
        let path = dir.join(format!("{name}.png"));
        match self
            .page
            .screenshot(
                chromiumoxide::page::ScreenshotParams::builder()
                    .full_page(true)
                    .build(),
            )
            .await
        {
            Ok(bytes) => {
                if let Err(error) = std::fs::write(&path, bytes) {
                    eprintln!("screenshot {}: {error}", path.display());
                }
            }
            Err(error) => eprintln!("screenshot {}: {error}", path.display()),
        }
    }
}
