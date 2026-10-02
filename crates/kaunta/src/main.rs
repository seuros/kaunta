use std::{env, fs, path::PathBuf};

use anyhow::{Context, bail};
use kaunta::domain::{config::Config, lifecycle::ApplicationLifecycle, website::ProxyMode};
use tracing_subscriber::{EnvFilter, layer::SubscriberExt, util::SubscriberInitExt};
use usage::Subcommands;

mod commands;
mod self_update;
mod website_sync;

const VERSION: &str = env!("CARGO_PKG_VERSION");

#[derive(Debug, usage::Cli)]
#[usage(bin = "kaunta", version = VERSION, about = "Analytics without bloat")]
struct Cli {
    #[usage(
        long,
        global = true,
        help = "Upgrade Kaunta to the latest release and exit"
    )]
    self_upgrade: bool,
    #[usage(
        long,
        global = true,
        help = "Only check whether a newer Kaunta release is available"
    )]
    self_upgrade_check: bool,
    #[usage(
        long,
        global = true,
        help = "Skip confirmation prompts when running --self-upgrade"
    )]
    self_upgrade_yes: bool,
    #[usage(long, global, env = "DATABASE_URL")]
    database_url: Option<String>,
    #[usage(long, global, env = "PORT")]
    port: Option<String>,
    #[usage(long, global, env = "DATA_DIR")]
    data_dir: Option<PathBuf>,
    #[usage(subcommand)]
    command: Option<Command>,
}

#[derive(Debug, Subcommands)]
enum Command {
    Serve,
    Healthcheck,
    Migrate {
        #[usage(default = "up")]
        action: String,
        #[usage(short = 's', long, default = "0")]
        step: u32,
    },
    MigrationStatus,
    User(commands::UserArgs),
    Website(commands::WebsiteArgs),
    #[usage(name = "apikey")]
    ApiKey(commands::ApiKeyArgs),
    Domain(commands::DomainArgs),
    Stats(commands::StatsArgs),
    Test(commands::TestArgs),
    Doctor {
        #[usage(long)]
        json: bool,
    },
    Diagnostics {
        #[usage(short = 'f', long)]
        full: bool,
    },
    Backup(commands::BackupArgs),
}

#[cfg(test)]
#[path = "main/tests.rs"]
mod tests;

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    init_tracing();
    let mut cli = Cli::parse();
    if cli.self_upgrade || cli.self_upgrade_check {
        return self_update::run(VERSION, cli.self_upgrade_check, cli.self_upgrade_yes).await;
    }
    let config = load_config(&cli)?;

    let command = cli.command.take().unwrap_or(Command::Serve);
    match command {
        Command::Serve => serve(config, &cli).await,
        Command::Healthcheck => healthcheck(config).await,
        Command::Migrate { action, step } => match action.as_str() {
            "up" if step == 0 => migrate(config).await,
            "version" => migration_status(config).await,
            "down" => bail!("rollback is not supported: the Go schema has no down migrations"),
            "up" => {
                bail!("partial migrations are not supported; omit --step to run all migrations")
            }
            _ => bail!("unknown migration action: {action} (use up or version)"),
        },
        Command::MigrationStatus => migration_status(config).await,
        Command::User(args) => commands::run_user(&config, args.command).await,
        Command::Website(args) => commands::run_website(&config, args.command).await,
        Command::ApiKey(args) => commands::run_api_key(&config, args.command).await,
        Command::Domain(args) => commands::run_domain(&config, args.command).await,
        Command::Stats(args) => commands::run_stats(&config, args.command).await,
        Command::Test(args) => commands::run_test(&config, args.command).await,
        Command::Doctor { json } => commands::run_doctor(&config, json).await,
        Command::Diagnostics { full } => commands::run_diagnostics(&config, full).await,
        Command::Backup(args) => commands::run_backup(&config, args.command, VERSION).await,
    }
}

fn init_tracing() {
    let level = env::var("KAUNTA_LOG_LEVEL").ok();
    let (directives, warning) = log_directives(level.as_deref());
    if let Some(warning) = warning {
        eprintln!("warning: {warning}");
    }
    let filter = EnvFilter::try_new(&directives).unwrap_or_else(|error| {
        eprintln!("warning: invalid KAUNTA_LOG_LEVEL {directives:?} ({error}); using info");
        EnvFilter::new("info")
    });
    let json = env::var("KAUNTA_LOG_FORMAT").is_ok_and(|format| log_format_is_json(&format));
    let source =
        env::var("KAUNTA_LOG_SOURCE").is_ok_and(|value| value.trim().eq_ignore_ascii_case("true"));

    let registry = tracing_subscriber::registry().with(filter);
    if json {
        registry
            .with(
                tracing_subscriber::fmt::layer()
                    .json()
                    .with_writer(std::io::stderr)
                    .with_file(source)
                    .with_line_number(source),
            )
            .init();
    } else {
        registry
            .with(
                tracing_subscriber::fmt::layer()
                    .with_writer(std::io::stderr)
                    .with_file(source)
                    .with_line_number(source),
            )
            .init();
    }
}

fn log_format_is_json(format: &str) -> bool {
    matches!(
        format.trim().to_ascii_lowercase().as_str(),
        "json" | "structured"
    )
}

/// Map `KAUNTA_LOG_LEVEL` to an `EnvFilter` directive string.
///
/// Go level names (`warning`, `fatal`, `critical`) are mapped to tracing
/// levels; unknown names fall back to `info` with a warning, matching Go.
/// Values containing `=` or `,` are treated as raw `EnvFilter` directives.
fn log_directives(value: Option<&str>) -> (String, Option<String>) {
    let Some(value) = value.map(str::trim).filter(|value| !value.is_empty()) else {
        return ("info".to_owned(), None);
    };
    if value.contains('=') || value.contains(',') {
        return (value.to_owned(), None);
    }
    match value.to_ascii_lowercase().as_str() {
        "trace" => ("trace".to_owned(), None),
        "debug" => ("debug".to_owned(), None),
        "info" => ("info".to_owned(), None),
        "warn" | "warning" => ("warn".to_owned(), None),
        "error" | "fatal" | "critical" => ("error".to_owned(), None),
        other => (
            "info".to_owned(),
            Some(format!("unknown KAUNTA_LOG_LEVEL {other:?}; using info")),
        ),
    }
}

fn load_config(cli: &Cli) -> anyhow::Result<Config> {
    let config = load_saved_config()?;
    apply_overrides(config, &env_value, cli)
}

/// Read an environment variable, treating empty or whitespace-only values as
/// unset so that `PORT=` in a unit file does not clobber the config file.
fn env_value(name: &str) -> Option<String> {
    env::var(name).ok().filter(|value| !value.trim().is_empty())
}

/// Precedence: defaults < config file < environment < command-line flags.
fn apply_overrides(
    mut config: Config,
    env: &dyn Fn(&str) -> Option<String>,
    cli: &Cli,
) -> anyhow::Result<Config> {
    if let Some(database_url) = env("DATABASE_URL") {
        config.database_url = database_url;
    }
    if let Some(port) = env("PORT") {
        config.port = port;
    }
    if let Some(data_dir) = env("DATA_DIR") {
        config.data_dir = data_dir.into();
    }
    if let Some(secure) = env("SECURE_COOKIES") {
        config.secure_cookies = secure.trim() == "true";
    }
    if let Some(origins) = env("TRUSTED_ORIGINS") {
        config.trusted_origins = kaunta::domain::config::parse_trusted_origins(&origins);
    }
    if let Some(mcp) = env("MCP_ENABLED") {
        config.mcp = mcp.trim().eq_ignore_ascii_case("true");
    }
    if let Some(excluded) = env("EXCLUDED_IPS") {
        config.excluded_ips = kaunta::domain::config::parse_trusted_origins(&excluded);
    }
    if let Some(mode) = env("PROXY_MODE") {
        let normalized = mode.trim().to_ascii_lowercase();
        config.proxy_mode = ProxyMode::try_from(normalized.as_str()).map_err(|_| {
            anyhow::anyhow!(
                "invalid PROXY_MODE {mode:?} (expected none, xforwarded, or cloudflare)"
            )
        })?;
    }
    if let Some(days) = env("EVENT_RETENTION_DAYS") {
        config.event_retention_days = days.trim().parse::<u32>().with_context(|| {
            format!("invalid EVENT_RETENTION_DAYS {days:?} (expected a non-negative integer)")
        })?;
    }

    if let Some(database_url) = cli.database_url.as_ref().filter(|value| !value.is_empty()) {
        config.database_url.clone_from(database_url);
    }
    if let Some(port) = cli.port.as_ref().filter(|value| !value.is_empty()) {
        config.port.clone_from(port);
    }
    if let Some(data_dir) = cli
        .data_dir
        .as_deref()
        .filter(|path| !path.as_os_str().is_empty())
    {
        config.data_dir = data_dir.to_path_buf();
    }

    Ok(config)
}

fn load_saved_config() -> anyhow::Result<Config> {
    let path = config_path();
    if path.exists() {
        let content = fs::read_to_string(&path)
            .with_context(|| format!("read configuration {}", path.display()))?;
        return toml::from_str(&content)
            .with_context(|| format!("parse configuration {}", path.display()));
    }
    Ok(Config::default())
}

fn config_path() -> PathBuf {
    let local = PathBuf::from("kaunta.toml");
    if local.exists() {
        return local;
    }
    let home = env::var_os("XDG_CONFIG_HOME")
        .map(PathBuf::from)
        .or_else(|| env::var_os("HOME").map(|home| PathBuf::from(home).join(".config")));
    home.map_or(local, |home| home.join("kaunta").join("kaunta.toml"))
}

async fn serve(mut config: Config, cli: &Cli) -> anyhow::Result<()> {
    loop {
        let status = kaunta::web::setup::check_status(&config).await;
        if !status.needs_setup {
            break;
        }
        tracing::info!(
            reason = status.reason.as_deref().unwrap_or("setup required"),
            "starting setup wizard"
        );
        let port = config
            .port
            .parse::<u16>()
            .with_context(|| format!("invalid port {}", config.port))?;
        if !kaunta::web::setup::serve(config_path(), port).await? {
            return Ok(());
        }
        config = load_config(cli).context("reload configuration after setup")?;
    }

    let lifecycle = ApplicationLifecycle::new(())
        .configure()
        .map_err(|error| anyhow::anyhow!("application configure transition failed: {error:?}"))?;

    let port = config
        .port
        .parse::<u16>()
        .with_context(|| format!("invalid port {}", config.port))?;
    let pool = commands::database(&config).await?;
    let lifecycle = lifecycle
        .connect()
        .map_err(|error| anyhow::anyhow!("application connect transition failed: {error:?}"))?;
    let migration_version = kaunta::db::migrations::run(&pool)
        .await
        .context("run database migrations")?;
    kaunta::db::startup::synchronize(&pool, &config.trusted_origins)
        .await
        .context("synchronize startup database state")?;
    let geoip = kaunta::web::geoip::GeoIp::initialize(&config.data_dir).await;
    let lifecycle = lifecycle
        .migrate()
        .map_err(|error| anyhow::anyhow!("application migrate transition failed: {error:?}"))?;
    let lifecycle = lifecycle
        .start()
        .map_err(|error| anyhow::anyhow!("application start transition failed: {error:?}"))?;

    tracing::info!(
        port,
        version = VERSION,
        migration_version,
        proxy_mode = config.proxy_mode.as_str(),
        mcp = config.mcp,
        "starting Kaunta"
    );
    let mcp = kaunta::mcp::http::service(
        pool.clone(),
        config.database_url.clone(),
        config.data_dir.clone(),
        VERSION,
    )
    .context("initialize MCP endpoint")?;
    let server_result = kaunta::web::server::serve(
        kaunta::web::AppState {
            pool,
            version: VERSION,
            config,
            geoip,
            realtime: kaunta::web::realtime::RealtimeHub::new(),
            login_attempts: kaunta::web::rate_limit::AttemptLimiter::default(),
            mcp,
            exclusions: kaunta::web::ExclusionCache::default(),
        },
        port,
    )
    .await;

    let lifecycle = lifecycle
        .drain()
        .map_err(|error| anyhow::anyhow!("application drain transition failed: {error:?}"))?;
    let _lifecycle = lifecycle
        .stop()
        .map_err(|error| anyhow::anyhow!("application stop transition failed: {error:?}"))?;

    server_result
}

async fn healthcheck(config: Config) -> anyhow::Result<()> {
    let port = config.port.parse::<u16>().context("invalid server port")?;
    reqwest::Client::builder()
        .timeout(std::time::Duration::from_secs(2))
        .build()?
        .get(format!("http://127.0.0.1:{port}/up"))
        .send()
        .await
        .context("server healthcheck failed")?
        .error_for_status()
        .context("server or database is unavailable")?;
    Ok(())
}

async fn migrate(config: Config) -> anyhow::Result<()> {
    let pool = commands::database(&config).await?;
    let version = kaunta::db::migrations::run(&pool)
        .await
        .context("run database migrations")?;
    println!("database migrated to version {version}");
    Ok(())
}

async fn migration_status(config: Config) -> anyhow::Result<()> {
    let pool = commands::database(&config).await?;
    let latest = kaunta::db::migrations::latest_version().context("load migrations")?;
    match kaunta::db::migrations::current_version(&pool)
        .await
        .context("read database migration state")?
    {
        Some((version, dirty)) => {
            println!("database={version} latest={latest} dirty={dirty}");
        }
        None => println!("database=none latest={latest} dirty=false"),
    }
    Ok(())
}
