use super::*;
use std::assert_matches;
use std::ffi::OsStr;

fn parse(args: &[&str]) -> Cli {
    Cli::parse_from(&args.iter().map(OsStr::new).collect::<Vec<_>>()).unwrap()
}

#[test]
fn global_flags_work_after_subcommands() {
    let cli = parse(&[
        "serve",
        "--port",
        "3999",
        "--database-url",
        "postgresql://localhost/test",
    ]);
    assert_eq!(cli.port.as_deref(), Some("3999"));
    assert_eq!(
        cli.database_url.as_deref(),
        Some("postgresql://localhost/test")
    );
    assert_matches!(cli.command, Some(Command::Serve));
    let cli = parse(&["website", "list", "--port", "3999", "--format", "json"]);
    assert_eq!(cli.port.as_deref(), Some("3999"));
}

#[test]
fn migration_actions_and_defaults_are_compatible() {
    assert_matches!(parse(&["migrate"]).command,
        Some(Command::Migrate { action, step: 0 }) if action == "up");
    assert_matches!(parse(&["migrate", "version"]).command,
        Some(Command::Migrate { action, step: 0 }) if action == "version");
    assert_matches!(parse(&["migrate", "up", "--step", "2"]).command,
        Some(Command::Migrate { action, step: 2 }) if action == "up");
}

#[test]
fn website_sync_accepts_legacy_flags_and_rejects_conflicting_modes() {
    let cli = parse(&[
        "website",
        "sync",
        "--from",
        "websites.yaml",
        "--dry-run",
        "--merge",
    ]);
    assert_matches!(
        cli.command,
        Some(Command::Website(commands::WebsiteArgs {
            command: commands::WebsiteCommand::Sync {
                dry_run: true,
                merge: true,
                ..
            }
        }))
    );
    let args = [
        "website",
        "sync",
        "--from",
        "websites.yaml",
        "--merge",
        "--replace",
    ];
    assert!(Cli::parse_from(&args.map(OsStr::new)).is_err());
}

#[tokio::test]
async fn healthcheck_checks_http_without_database_credentials() {
    use std::io::{Read, Write};
    let listener = std::net::TcpListener::bind("127.0.0.1:0").unwrap();
    let port = listener.local_addr().unwrap().port();
    let server = std::thread::spawn(move || {
        let (mut stream, _) = listener.accept().unwrap();
        stream
            .set_read_timeout(Some(std::time::Duration::from_secs(5)))
            .unwrap();
        let mut request = [0; 1024];
        let size = stream.read(&mut request).unwrap();
        assert!(String::from_utf8_lossy(&request[..size]).starts_with("GET /up "));
        stream
            .write_all(b"HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: close\r\n\r\nOK")
            .unwrap();
    });
    let config = Config {
        port: port.to_string(),
        ..Config::default()
    };
    healthcheck(config.clone()).await.unwrap();
    server.join().unwrap();
    assert!(healthcheck(config).await.is_err());
}

fn env_from(pairs: &[(&str, &str)]) -> impl Fn(&str) -> Option<String> {
    let pairs: Vec<(String, String)> = pairs
        .iter()
        .map(|(key, value)| ((*key).to_owned(), (*value).to_owned()))
        .collect();
    move |name: &str| {
        pairs
            .iter()
            .find(|(key, _)| key == name)
            .map(|(_, value)| value.clone())
            .filter(|value| !value.trim().is_empty())
    }
}

#[test]
fn short_flags_match_go_cli() {
    let cli = parse(&["stats", "pages", "example.com", "-d", "30", "-t", "5"]);
    assert_matches!(
        cli.command,
        Some(Command::Stats(commands::StatsArgs {
            command: commands::StatsCommand::Pages {
                days: 30,
                top: 5,
                ..
            }
        }))
    );
    let cli = parse(&["stats", "live", "example.com", "-i", "10"]);
    assert_matches!(
        cli.command,
        Some(Command::Stats(commands::StatsArgs {
            command: commands::StatsCommand::Live { interval: 10, .. }
        }))
    );
    let cli = parse(&["diagnostics", "-f"]);
    assert_matches!(cli.command, Some(Command::Diagnostics { full: true }));
    let cli = parse(&["website", "create", "example.com", "-o", "alice"]);
    assert_matches!(
        cli.command,
        Some(Command::Website(commands::WebsiteArgs {
            command: commands::WebsiteCommand::Create { owner: Some(owner), .. }
        })) if owner == "alice"
    );
}

#[test]
fn env_overrides_file_and_flags_override_env() {
    let file = Config {
        port: "4000".to_owned(),
        database_url: "postgresql://file/db".to_owned(),
        ..Config::default()
    };
    let env = env_from(&[
        ("PORT", "5000"),
        ("DATABASE_URL", "postgresql://env/db"),
        ("DATA_DIR", "/srv/kaunta"),
        ("SECURE_COOKIES", "false"),
        ("TRUSTED_ORIGINS", " A.example.com,, b.example.com "),
        ("PROXY_MODE", "CloudFlare"),
        ("EVENT_RETENTION_DAYS", "45"),
    ]);
    let cli = parse(&["serve", "--port", "6000"]);
    let config = apply_overrides(file, &env, &cli).expect("apply overrides");
    assert_eq!(config.port, "6000");
    assert_eq!(config.database_url, "postgresql://env/db");
    assert_eq!(config.data_dir, PathBuf::from("/srv/kaunta"));
    assert!(!config.secure_cookies);
    assert_eq!(
        config.trusted_origins,
        vec!["a.example.com", "b.example.com"]
    );
    assert_eq!(config.proxy_mode, ProxyMode::Cloudflare);
    assert_eq!(config.event_retention_days, 45);
}

#[test]
fn empty_env_values_are_treated_as_unset() {
    let file = Config {
        port: "4000".to_owned(),
        database_url: "postgresql://file/db".to_owned(),
        data_dir: PathBuf::from("/file/data"),
        secure_cookies: true,
        trusted_origins: vec!["file.example.com".to_owned()],
        ..Config::default()
    };
    let env = env_from(&[
        ("PORT", ""),
        ("DATABASE_URL", "  "),
        ("DATA_DIR", ""),
        ("SECURE_COOKIES", ""),
        ("TRUSTED_ORIGINS", ""),
        ("PROXY_MODE", ""),
        ("EVENT_RETENTION_DAYS", ""),
    ]);
    let cli = parse(&["serve"]);
    let config = apply_overrides(file.clone(), &env, &cli).expect("apply overrides");
    assert_eq!(config.port, file.port);
    assert_eq!(config.database_url, file.database_url);
    assert_eq!(config.data_dir, file.data_dir);
    assert!(config.secure_cookies);
    assert_eq!(config.trusted_origins, file.trusted_origins);
    assert_eq!(config.proxy_mode, ProxyMode::None);
    assert_eq!(config.event_retention_days, 0);
}

#[test]
fn invalid_proxy_mode_or_retention_env_is_an_error() {
    let cli = parse(&["serve"]);
    let error = apply_overrides(
        Config::default(),
        &env_from(&[("PROXY_MODE", "haproxy")]),
        &cli,
    )
    .expect_err("invalid proxy mode")
    .to_string();
    assert!(error.contains("PROXY_MODE"), "{error}");
    let error = apply_overrides(
        Config::default(),
        &env_from(&[("EVENT_RETENTION_DAYS", "-1")]),
        &cli,
    )
    .expect_err("invalid retention")
    .to_string();
    assert!(error.contains("EVENT_RETENTION_DAYS"), "{error}");
}

#[test]
fn log_levels_map_go_names_and_pass_raw_directives() {
    assert_eq!(log_directives(None), ("info".to_owned(), None));
    assert_eq!(log_directives(Some("  ")), ("info".to_owned(), None));
    assert_eq!(log_directives(Some("WARNING")), ("warn".to_owned(), None));
    assert_eq!(log_directives(Some("fatal")), ("error".to_owned(), None));
    assert_eq!(log_directives(Some("critical")), ("error".to_owned(), None));
    assert_eq!(log_directives(Some("debug")), ("debug".to_owned(), None));
    let (level, warning) = log_directives(Some("verbose"));
    assert_eq!(level, "info");
    assert!(warning.is_some_and(|warning| warning.contains("verbose")));
    assert_eq!(
        log_directives(Some("info,kaunta_web=debug")),
        ("info,kaunta_web=debug".to_owned(), None)
    );
    assert!(log_format_is_json("JSON"));
    assert!(log_format_is_json("structured"));
    assert!(!log_format_is_json("text"));
}

#[test]
fn reads_go_wizard_config_layout() {
    let go_layout = r"
database_url = 'postgresql://localhost/kaunta'
data_dir = './data'
secure_cookies = false
trusted_origins = 'Analytics.example.com, ,localhost'

[security]
install_lock = true

[server]
host = '0.0.0.0'
port = '8080'
";
    let config: Config = toml::from_str(go_layout).expect("parse Go layout");
    assert_eq!(config.port, "8080");
    assert_eq!(
        config.trusted_origins,
        vec!["analytics.example.com", "localhost"]
    );
    assert!(config.security.install_lock);
    assert_eq!(
        config
            .server
            .as_ref()
            .and_then(|server| server.host.as_deref()),
        Some("0.0.0.0")
    );

    let empty: Config = toml::from_str("trusted_origins = ''").expect("parse empty origins");
    assert!(empty.trusted_origins.is_empty());
}
