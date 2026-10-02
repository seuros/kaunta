use std::{
    fs,
    io::{self, IsTerminal, Write},
    path::Path,
};

use anyhow::{Context, bail};
use base64::{Engine as _, engine::general_purpose::URL_SAFE};
use kaunta::domain::{
    api_key::ApiKey,
    config::Config,
    origin::sanitize_origin,
    website::{Website, validate_domain},
};
use serde::Serialize;
use sqlx::{AssertSqlSafe, PgPool};
use time::{OffsetDateTime, format_description::well_known::Rfc3339};
use url::Url;
use usage::{Args, Subcommands};
use uuid::Uuid;

#[derive(Debug, Args)]
pub struct UserArgs {
    #[usage(subcommand)]
    pub command: UserCommand,
}

#[derive(Debug, Subcommands)]
pub enum UserCommand {
    Create {
        username: String,
        #[usage(short, long)]
        name: Option<String>,
        #[usage(short, long)]
        password: Option<String>,
    },
    List,
    Delete {
        username: String,
        #[usage(short, long)]
        force: bool,
    },
    ResetPassword {
        username: String,
        #[usage(short, long)]
        password: Option<String>,
    },
}

#[derive(Debug, Args)]
pub struct WebsiteArgs {
    #[usage(subcommand)]
    pub command: WebsiteCommand,
}

#[derive(Debug, Subcommands)]
pub enum WebsiteCommand {
    List {
        #[usage(short, long, default = "table")]
        format: String,
    },
    Show {
        identifier: String,
        #[usage(short, long, default = "table")]
        format: String,
    },
    Create {
        domain: String,
        #[usage(short, long)]
        name: Option<String>,
        #[usage(short, long)]
        allowed: Option<String>,
        #[usage(
            short = 'o',
            long,
            help = "Username that owns the website; omit to create a shared website visible to all users"
        )]
        owner: Option<String>,
    },
    Update {
        identifier: String,
        #[usage(short, long)]
        name: Option<String>,
        #[usage(short, long)]
        allowed: Option<String>,
    },
    Delete {
        identifier: String,
        #[usage(short, long)]
        force: bool,
    },
    Restore {
        identifier: String,
    },
    TrackingCode {
        identifier: String,
    },
    AddDomain {
        identifier: String,
        domain: String,
        #[usage(short, long)]
        allowed: Option<String>,
    },
    RemoveDomain {
        identifier: String,
        domain: String,
    },
    ListDomains {
        identifier: String,
        #[usage(short, long, default = "text")]
        format: String,
    },
    EnablePublicStats {
        identifier: String,
    },
    DisablePublicStats {
        identifier: String,
    },
    Check {
        identifier: String,
        #[usage(long)]
        origin: Option<String>,
    },
    /// Import websites from a JSON or YAML file.
    Sync {
        #[usage(short = 'f', long)]
        from: std::path::PathBuf,
        #[usage(short = 'd', long)]
        dry_run: bool,
        #[usage(short = 'r', long, conflicts = "merge")]
        replace: bool,
        #[usage(long)]
        merge: bool,
    },
}

#[derive(Debug, Args)]
pub struct ApiKeyArgs {
    #[usage(subcommand)]
    pub command: ApiKeyCommand,
}

#[derive(Debug, Subcommands)]
pub enum ApiKeyCommand {
    Create {
        website: String,
        #[usage(short, long)]
        name: Option<String>,
        #[usage(short, long = "scope")]
        scopes: Option<String>,
    },
    List {
        website: String,
        #[usage(short, long, default = "table")]
        format: String,
    },
    Revoke {
        identifier: String,
    },
    Show {
        identifier: String,
    },
}

#[derive(Debug, Args)]
pub struct DomainArgs {
    #[usage(subcommand)]
    pub command: DomainCommand,
}

#[derive(Debug, Subcommands)]
pub enum DomainCommand {
    Add {
        domain: String,
        #[usage(short, long)]
        description: Option<String>,
    },
    List {
        #[usage(long)]
        active: bool,
    },
    Remove {
        identifier: String,
        #[usage(short, long)]
        force: bool,
    },
    Toggle {
        identifier: String,
    },
    Verify {
        origin: String,
    },
}

#[derive(Debug, Args)]
pub struct StatsArgs {
    #[usage(subcommand)]
    pub command: StatsCommand,
}

#[derive(Debug, Subcommands)]
pub enum StatsCommand {
    Overview {
        website: String,
        #[usage(short = 'd', long, default = "7")]
        days: i32,
        #[usage(short, long, default = "table")]
        format: String,
    },
    Pages {
        website: String,
        #[usage(short = 'd', long, default = "7")]
        days: i32,
        #[usage(short = 't', long, default = "10")]
        top: i32,
        #[usage(short, long, default = "table")]
        format: String,
    },
    Breakdown {
        website: String,
        #[usage(long = "by")]
        dimension: String,
        #[usage(short = 'd', long, default = "7")]
        days: i32,
        #[usage(short = 't', long, default = "10")]
        top: i32,
        #[usage(short, long, default = "table")]
        format: String,
    },
    Live {
        website: String,
        #[usage(short = 'i', long, default = "5")]
        interval: u64,
        #[usage(short, long, default = "text")]
        format: String,
    },
}

#[derive(Debug, Args)]
pub struct BackupArgs {
    #[usage(subcommand)]
    pub command: BackupCommand,
}

#[derive(Debug, Subcommands)]
pub enum BackupCommand {
    Create {
        #[usage(
            short = 'd',
            long,
            help = "Back up only the last N days of events (omit for a full pg_dump)"
        )]
        days: Option<i32>,
        #[usage(short, long, help = "Directory to write the backup into (default: .)")]
        output: Option<std::path::PathBuf>,
    },
    Restore {
        #[usage(help = "Full backup file (kaunta-full-*.dump) to restore")]
        file: std::path::PathBuf,
        #[usage(short, long, help = "Skip the confirmation prompt")]
        yes: bool,
    },
}

#[derive(Debug, Args)]
pub struct TestArgs {
    #[usage(subcommand)]
    pub command: TestCommand,
}

#[derive(Debug, Subcommands)]
pub enum TestCommand {
    Tracking {
        website: String,
        #[usage(short = 'o', long)]
        origin: Option<String>,
        #[usage(short = 'p', long)]
        payload: Option<std::path::PathBuf>,
    },
}

#[derive(Debug, Clone, Serialize)]
struct CheckResult {
    name: String,
    pass: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    error: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    suggestion: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    details: Option<String>,
}

impl CheckResult {
    fn passed(name: &str, details: Option<String>) -> Self {
        Self {
            name: name.to_owned(),
            pass: true,
            error: None,
            suggestion: None,
            details,
        }
    }

    fn failed(name: &str, error: impl Into<String>, suggestion: Option<&str>) -> Self {
        Self {
            name: name.to_owned(),
            pass: false,
            error: Some(error.into()),
            suggestion: suggestion.map(str::to_owned),
            details: None,
        }
    }
}

#[derive(Debug, Serialize)]
struct Overview {
    total_visitors: i64,
    total_pageviews: i64,
    current_visitors: i64,
    top_page: Option<NamedMetric>,
    top_referrer: Option<NamedMetric>,
    browser_distribution: Vec<NamedMetric>,
    device_distribution: Vec<NamedMetric>,
    country_distribution: Vec<NamedMetric>,
    avg_engagement_seconds: f64,
}

#[derive(Debug, Clone, Serialize, sqlx::FromRow)]
struct NamedMetric {
    name: String,
    count: i64,
}

#[derive(Debug, Serialize, sqlx::FromRow)]
struct PageMetric {
    path: String,
    pageviews: i64,
    unique_visitors: i64,
    bounce_rate: f64,
    avg_time_seconds: f64,
}

#[derive(Debug, Serialize, sqlx::FromRow)]
struct BreakdownMetric {
    name: String,
    visitors: i64,
    pageviews: i64,
    bounce_rate: f64,
}

#[derive(Debug, Serialize)]
struct LiveStats {
    timestamp: OffsetDateTime,
    active_visitors_now: i64,
    pageviews_last_minute: i64,
    recent_events: i64,
    top_page_now: Option<String>,
}

pub async fn run_user(config: &Config, command: UserCommand) -> anyhow::Result<()> {
    let pool = database(config).await?;
    match command {
        UserCommand::Create {
            username,
            name,
            password,
        } => user_create(&pool, &username, name.as_deref(), password).await,
        UserCommand::List => user_list(&pool).await,
        UserCommand::Delete { username, force } => user_delete(&pool, &username, force).await,
        UserCommand::ResetPassword { username, password } => {
            user_reset_password(&pool, &username, password).await
        }
    }
}

pub async fn run_website(config: &Config, command: WebsiteCommand) -> anyhow::Result<()> {
    let pool = database(config).await?;
    match command {
        WebsiteCommand::List { format } => website_list(&pool, &format).await,
        WebsiteCommand::Show { identifier, format } => {
            let website = website_by_identifier(&pool, &identifier).await?;
            output_website(&website, &format)
        }
        WebsiteCommand::Create {
            domain,
            name,
            allowed,
            owner,
        } => {
            website_create(
                &pool,
                &domain,
                name.as_deref(),
                allowed.as_deref(),
                owner.as_deref(),
            )
            .await
        }
        WebsiteCommand::Update {
            identifier,
            name,
            allowed,
        } => website_update(&pool, &identifier, name.as_deref(), allowed.as_deref()).await,
        WebsiteCommand::Delete { identifier, force } => {
            website_delete(&pool, &identifier, force).await
        }
        WebsiteCommand::Restore { identifier } => {
            let deleted = kaunta::db::websites::find_deleted(&pool, &identifier).await?;
            let restored = kaunta::db::websites::restore(&pool, deleted.website_id).await?;
            println!("Website '{}' restored", restored.domain);
            Ok(())
        }
        WebsiteCommand::TrackingCode { identifier } => {
            let website = website_by_identifier(&pool, &identifier).await?;
            println!(
                "<script async src=\"/k.js\" data-website-id=\"{}\"></script>",
                website.website_id
            );
            Ok(())
        }
        WebsiteCommand::AddDomain {
            identifier,
            domain,
            allowed,
        } => website_add_domains(&pool, &identifier, &domain, allowed.as_deref()).await,
        WebsiteCommand::RemoveDomain { identifier, domain } => {
            let website = website_by_identifier(&pool, &identifier).await?;
            let updated =
                kaunta::db::websites::remove_allowed_domain(&pool, website.website_id, &domain)
                    .await?;
            output_domains(&updated, "table")
        }
        WebsiteCommand::ListDomains { identifier, format } => {
            let website = website_by_identifier(&pool, &identifier).await?;
            output_domains(&website, &format)
        }
        WebsiteCommand::EnablePublicStats { identifier } => {
            website_public_stats(&pool, &identifier, true).await
        }
        WebsiteCommand::DisablePublicStats { identifier } => {
            website_public_stats(&pool, &identifier, false).await
        }
        WebsiteCommand::Check { identifier, origin } => {
            tracking_check(&pool, &identifier, origin.as_deref(), None).await
        }
        WebsiteCommand::Sync {
            from,
            dry_run,
            replace,
            merge,
        } => {
            anyhow::ensure!(
                !(replace && merge),
                "--replace and --merge are mutually exclusive"
            );
            crate::website_sync::run(&pool, &from, dry_run, replace).await
        }
    }
}

pub async fn run_api_key(config: &Config, command: ApiKeyCommand) -> anyhow::Result<()> {
    let pool = database(config).await?;
    match command {
        ApiKeyCommand::Create {
            website,
            name,
            scopes,
        } => api_key_create(&pool, &website, name.as_deref(), scopes.as_deref()).await,
        ApiKeyCommand::List { website, format } => api_key_list(&pool, &website, &format).await,
        ApiKeyCommand::Revoke { identifier } => api_key_revoke(&pool, &identifier).await,
        ApiKeyCommand::Show { identifier } => api_key_show(&pool, &identifier).await,
    }
}

pub async fn run_domain(config: &Config, command: DomainCommand) -> anyhow::Result<()> {
    let pool = database(config).await?;
    match command {
        DomainCommand::Add {
            domain,
            description,
        } => domain_add(&pool, &domain, description.as_deref()).await,
        DomainCommand::List { active } => domain_list(&pool, active).await,
        DomainCommand::Remove { identifier, force } => {
            domain_remove(&pool, &identifier, force).await
        }
        DomainCommand::Toggle { identifier } => domain_toggle(&pool, &identifier).await,
        DomainCommand::Verify { origin } => domain_verify(&pool, &origin).await,
    }
}

pub async fn run_stats(config: &Config, command: StatsCommand) -> anyhow::Result<()> {
    let pool = database(config).await?;
    match command {
        StatsCommand::Overview {
            website,
            days,
            format,
        } => stats_overview(&pool, &website, days, &format).await,
        StatsCommand::Pages {
            website,
            days,
            top,
            format,
        } => stats_pages(&pool, &website, days, top, &format).await,
        StatsCommand::Breakdown {
            website,
            dimension,
            days,
            top,
            format,
        } => stats_breakdown(&pool, &website, &dimension, days, top, &format).await,
        StatsCommand::Live {
            website,
            interval,
            format,
        } => stats_live(&pool, &website, interval, &format).await,
    }
}

pub async fn run_test(config: &Config, command: TestCommand) -> anyhow::Result<()> {
    let pool = database(config).await?;
    match command {
        TestCommand::Tracking {
            website,
            origin,
            payload,
        } => tracking_check(&pool, &website, origin.as_deref(), payload.as_deref()).await,
    }
}

pub(crate) async fn database(config: &Config) -> anyhow::Result<PgPool> {
    if config.database_url.is_empty() {
        bail!("DATABASE_URL is required");
    }
    kaunta::db::connect(&config.database_url)
        .await
        .context("connect to PostgreSQL")
}

async fn user_create(
    pool: &PgPool,
    username: &str,
    name: Option<&str>,
    password: Option<String>,
) -> anyhow::Result<()> {
    if username.len() < 3 {
        bail!("username must be at least 3 characters long");
    }
    if kaunta::db::auth::get_user_by_username(pool, username)
        .await?
        .is_some()
    {
        bail!("user '{username}' already exists");
    }
    let (password, generated) = password_value(password, "Password: ", "Confirm password: ")?;
    let user = kaunta::db::auth::create_user(pool, username, &password, name.unwrap_or("")).await?;
    println!("\n✓ User created successfully");
    println!("  ID:       {}", user.user_id);
    println!("  Username: {}", user.username);
    if let Some(name) = user.name {
        println!("  Name:     {name}");
    }
    if generated {
        println!("  Password: {password} (auto-generated)");
    }
    println!("  Created:  {}", timestamp(user.created_at));
    Ok(())
}

async fn user_list(pool: &PgPool) -> anyhow::Result<()> {
    let users = kaunta::db::auth::list_users(pool).await?;
    if users.is_empty() {
        println!("No users found");
        return Ok(());
    }
    println!("\nTotal users: {}\n", users.len());
    println!("ID\tUSERNAME\tNAME\tCREATED");
    for user in users {
        println!(
            "{}\t{}\t{}\t{}",
            user.user_id,
            user.username,
            user.name.as_deref().unwrap_or("-"),
            timestamp(user.created_at)
        );
    }
    Ok(())
}

async fn user_delete(pool: &PgPool, username: &str, force: bool) -> anyhow::Result<()> {
    if !force
        && !confirm(&format!(
            "Are you sure you want to delete user '{username}'?"
        ))?
    {
        println!("Deletion cancelled");
        return Ok(());
    }
    if !kaunta::db::auth::delete_user_by_username(pool, username).await? {
        bail!("user '{username}' not found");
    }
    println!("✓ User '{username}' deleted successfully");
    Ok(())
}

async fn user_reset_password(
    pool: &PgPool,
    username: &str,
    password: Option<String>,
) -> anyhow::Result<()> {
    let password = reset_password_value(password, io::stdin().is_terminal())?;
    if !kaunta::db::auth::reset_password(pool, username, &password).await? {
        bail!("user '{username}' not found");
    }
    println!("✓ Password reset successfully for '{username}'");
    println!("  All existing sessions have been invalidated");
    Ok(())
}

/// Resolve the password for `user reset-password`.
///
/// Unlike `user create`, a reset never generates a password: without
/// `--password` the caller must be interactive so the new password can be
/// prompted for, otherwise the command fails.
fn reset_password_value(password: Option<String>, interactive: bool) -> anyhow::Result<String> {
    let password = match password {
        Some(password) => password,
        None if interactive => {
            let password = rpassword::prompt_password("New password: ")?;
            let confirmation = rpassword::prompt_password("Confirm password: ")?;
            if password != confirmation {
                bail!("passwords do not match");
            }
            password
        }
        None => bail!("password required: pass --password or run interactively"),
    };
    if password.len() < 8 {
        bail!("password must be at least 8 characters long");
    }
    Ok(password)
}

fn password_value(
    password: Option<String>,
    prompt: &str,
    confirm_prompt: &str,
) -> anyhow::Result<(String, bool)> {
    let (password, generated) = if let Some(password) = password {
        (password, false)
    } else if io::stdin().is_terminal() {
        let password = rpassword::prompt_password(prompt)?;
        let confirmation = rpassword::prompt_password(confirm_prompt)?;
        if password != confirmation {
            bail!("passwords do not match");
        }
        (password, false)
    } else {
        let bytes: [u8; 16] = rand::random();
        (URL_SAFE.encode(bytes)[..16].to_owned(), true)
    };
    if password.len() < 8 {
        bail!("password must be at least 8 characters long");
    }
    Ok((password, generated))
}

async fn website_list(pool: &PgPool, format: &str) -> anyhow::Result<()> {
    let websites = kaunta::db::websites::list(pool).await?;
    match format {
        "json" => println!("{}", serde_json::to_string_pretty(&websites)?),
        "csv" => {
            println!("domain,name,website_id,created_at");
            for website in websites {
                println!(
                    "{},{},{},{}",
                    csv_field(&website.domain),
                    csv_field(&website.name),
                    website.website_id,
                    timestamp(website.created_at)
                );
            }
        }
        "table" => {
            if websites.is_empty() {
                println!("No websites found");
            } else {
                println!("DOMAIN\tNAME\tWEBSITE ID\tCREATED AT");
                for website in websites {
                    println!(
                        "{}\t{}\t{}\t{}",
                        website.domain,
                        website.name,
                        website.website_id,
                        timestamp(website.created_at)
                    );
                }
            }
        }
        _ => bail!("invalid format: {format}"),
    }
    Ok(())
}

async fn website_create(
    pool: &PgPool,
    domain: &str,
    name: Option<&str>,
    allowed: Option<&str>,
    owner: Option<&str>,
) -> anyhow::Result<()> {
    validate_domain(domain)?;
    let owner_id = match owner.map(str::trim).filter(|owner| !owner.is_empty()) {
        Some(username) => Some(
            kaunta::db::auth::get_user_by_username(pool, username)
                .await?
                .with_context(|| format!("owner user '{username}' not found"))?
                .user_id,
        ),
        None => None,
    };
    let mut allowed_domains = parse_csv(allowed);
    for candidate in [
        domain.to_owned(),
        format!("www.{domain}"),
        format!("https://{domain}"),
        format!("http://{domain}"),
        format!("https://www.{domain}"),
        format!("http://www.{domain}"),
    ] {
        if !allowed_domains.contains(&candidate) {
            allowed_domains.push(candidate);
        }
    }
    let website = kaunta::db::websites::create(
        pool,
        domain,
        name.unwrap_or(domain),
        &allowed_domains,
        owner_id,
    )
    .await?;
    println!("Website created successfully!\n");
    output_website_table(&website);
    match owner {
        Some(username) => println!("\nOwner: {username}"),
        None => println!("\nOwner: none (shared website, visible to all users)"),
    }
    println!("\nTracking Code ID: {}", website.website_id);
    println!(
        "\nNext: Use 'kaunta website tracking-code <domain>' to generate the tracking snippet"
    );
    Ok(())
}

async fn website_update(
    pool: &PgPool,
    identifier: &str,
    name: Option<&str>,
    allowed: Option<&str>,
) -> anyhow::Result<()> {
    if name.is_none() && allowed.is_none() {
        bail!("must specify at least one option: --name or --allowed");
    }
    let mut website = website_by_identifier(pool, identifier).await?;
    if let Some(name) = name {
        website = kaunta::db::websites::update_name(pool, website.website_id, name).await?;
    }
    if let Some(allowed) = allowed {
        website = kaunta::db::websites::set_allowed_domains(
            pool,
            website.website_id,
            &parse_csv(Some(allowed)),
        )
        .await?;
    }
    println!("Website updated successfully!\n");
    output_website_table(&website);
    Ok(())
}

async fn website_delete(pool: &PgPool, identifier: &str, force: bool) -> anyhow::Result<()> {
    let website = website_by_identifier(pool, identifier).await?;
    if !force
        && !confirm(&format!(
            "Are you sure you want to delete website '{}'?",
            website.domain
        ))?
    {
        println!("Deletion cancelled");
        return Ok(());
    }
    match kaunta::db::websites::soft_delete(pool, website.website_id).await? {
        kaunta::db::websites::DeleteOutcome::Deleted(deleted_at) => {
            println!("Website '{}' deleted successfully", website.domain);
            println!("Deleted at: {}", timestamp(deleted_at));
        }
        kaunta::db::websites::DeleteOutcome::PendingSince(pending_since) => {
            println!(
                "Website '{}' still has event data; marked pending deletion",
                website.domain
            );
            println!("Pending since: {}", timestamp(pending_since));
            println!("Run delete again after 30 days to complete it.");
        }
    }
    Ok(())
}

async fn website_add_domains(
    pool: &PgPool,
    identifier: &str,
    domain: &str,
    additional: Option<&str>,
) -> anyhow::Result<()> {
    validate_domain(domain)?;
    let mut domains = vec![domain.to_owned()];
    for candidate in parse_csv(additional) {
        validate_domain(&candidate)?;
        domains.push(candidate);
    }
    let website = website_by_identifier(pool, identifier).await?;
    let updated =
        kaunta::db::websites::add_allowed_domains(pool, website.website_id, &domains).await?;
    println!("Allowed domains updated successfully!\n");
    output_domains(&updated, "table")
}

async fn website_public_stats(
    pool: &PgPool,
    identifier: &str,
    enabled: bool,
) -> anyhow::Result<()> {
    let website = website_by_identifier(pool, identifier).await?;
    let updated =
        kaunta::db::websites::set_public_stats_enabled(pool, website.website_id, enabled).await?;
    println!(
        "Public stats {} for website '{}' (ID: {})",
        if enabled { "enabled" } else { "disabled" },
        updated.domain,
        updated.website_id
    );
    Ok(())
}

fn output_website(website: &Website, format: &str) -> anyhow::Result<()> {
    match format {
        "json" => println!("{}", serde_json::to_string_pretty(website)?),
        "table" => output_website_table(website),
        _ => bail!("invalid format: {format}"),
    }
    Ok(())
}

fn output_website_table(website: &Website) {
    println!("Domain:\t{}", website.domain);
    println!("Name:\t{}", website.name);
    println!("Website ID:\t{}", website.website_id);
    println!("Created:\t{}", timestamp(website.created_at));
    println!("Updated:\t{}", timestamp(website.updated_at));
    println!(
        "Share ID:\t{}",
        website.share_id.as_deref().unwrap_or("(none)")
    );
    println!(
        "Allowed Domains:\t{}",
        if website.allowed_domains.is_empty() {
            "(none)".to_owned()
        } else {
            website.allowed_domains.join(", ")
        }
    );
}

fn output_domains(website: &Website, format: &str) -> anyhow::Result<()> {
    match format {
        "json" => println!(
            "{}",
            serde_json::to_string_pretty(&website.allowed_domains)?
        ),
        "text" => {
            for domain in &website.allowed_domains {
                println!("{domain}");
            }
        }
        "table" => {
            println!("Website: {}", website.domain);
            println!("#\tDOMAIN");
            for (index, domain) in website.allowed_domains.iter().enumerate() {
                println!("{}\t{}", index + 1, domain);
            }
        }
        _ => bail!("invalid format: {format} (use text, json, or table)"),
    }
    Ok(())
}

async fn website_by_identifier(pool: &PgPool, identifier: &str) -> anyhow::Result<Website> {
    if let Ok(website_id) = Uuid::parse_str(identifier) {
        return Ok(kaunta::db::websites::get_by_id(pool, website_id).await?);
    }
    Ok(kaunta::db::websites::get_by_domain(pool, identifier, None).await?)
}

async fn api_key_create(
    pool: &PgPool,
    website_identifier: &str,
    name: Option<&str>,
    scopes: Option<&str>,
) -> anyhow::Result<()> {
    let website = website_by_identifier(pool, website_identifier).await?;
    let scopes = parse_csv(scopes);
    let result =
        kaunta::db::api_keys::create(pool, website.website_id, None, name, &scopes, None).await?;
    println!("\nAPI Key created successfully!\n");
    println!("============================================================");
    println!("IMPORTANT: Save this key now. It will NOT be shown again.");
    println!("============================================================\n");
    println!("API Key: {}\n", result.api_key);
    println!("Key ID:     {}", result.key.key_id);
    println!("Website:    {} ({})", website.domain, website.website_id);
    if let Some(name) = result.key.name {
        println!("Name:       {name}");
    }
    println!("Scopes:     {}", result.key.scopes.join(", "));
    println!("Rate Limit: {} req/min", result.key.rate_limit_per_minute);
    println!("Created:    {}", timestamp(result.key.created_at));
    Ok(())
}

async fn api_key_list(pool: &PgPool, website: &str, format: &str) -> anyhow::Result<()> {
    let website = website_by_identifier(pool, website).await?;
    let keys = kaunta::db::api_keys::list(pool, website.website_id).await?;
    if format == "json" {
        println!("{}", serde_json::to_string_pretty(&keys)?);
        return Ok(());
    }
    if format != "table" {
        bail!("invalid format: {format} (use table or json)");
    }
    if keys.is_empty() {
        println!("No API keys found for website '{}'", website.domain);
        return Ok(());
    }
    println!("\nAPI Keys for {} ({} total)\n", website.domain, keys.len());
    println!("PREFIX\tNAME\tSTATUS\tLAST USED\tCREATED");
    for key in keys {
        println!(
            "{}\t{}\t{}\t{}\t{}",
            key.key_prefix,
            key.name.as_deref().unwrap_or("-"),
            api_key_status(&key),
            key.last_used_at
                .map_or_else(|| "never".to_owned(), timestamp),
            timestamp(key.created_at)
        );
    }
    Ok(())
}

async fn api_key_revoke(pool: &PgPool, identifier: &str) -> anyhow::Result<()> {
    let revoked = if let Ok(key_id) = Uuid::parse_str(identifier) {
        kaunta::db::api_keys::revoke(pool, key_id).await?
    } else {
        kaunta::db::api_keys::revoke_by_prefix(pool, identifier).await?
    };
    if !revoked {
        bail!("API key '{identifier}' not found or already revoked");
    }
    println!("API key '{identifier}' revoked successfully");
    Ok(())
}

async fn api_key_show(pool: &PgPool, identifier: &str) -> anyhow::Result<()> {
    let key = api_key_by_identifier(pool, identifier)
        .await?
        .with_context(|| format!("API key '{identifier}' not found"))?;
    println!("Key ID:\t{}", key.key_id);
    println!("Prefix:\t{}", key.key_prefix);
    println!("Website ID:\t{}", key.website_id);
    println!("Name:\t{}", key.name.as_deref().unwrap_or("(none)"));
    println!("Scopes:\t{}", key.scopes.join(", "));
    println!("Rate Limit:\t{} req/min", key.rate_limit_per_minute);
    println!("Status:\t{}", api_key_status(&key));
    println!("Created:\t{}", timestamp(key.created_at));
    println!(
        "Last Used:\t{}",
        key.last_used_at
            .map_or_else(|| "never".to_owned(), timestamp)
    );
    if let Some(expires_at) = key.expires_at {
        println!("Expires:\t{}", timestamp(expires_at));
    }
    Ok(())
}

async fn api_key_by_identifier(
    pool: &PgPool,
    identifier: &str,
) -> Result<Option<ApiKey>, sqlx::Error> {
    if let Ok(key_id) = Uuid::parse_str(identifier) {
        kaunta::db::api_keys::get_by_id(pool, key_id).await
    } else {
        kaunta::db::api_keys::get_by_prefix(pool, identifier).await
    }
}

fn api_key_status(key: &ApiKey) -> String {
    if let Some(revoked_at) = key.revoked_at {
        format!("revoked ({})", timestamp(revoked_at))
    } else if let Some(expires_at) = key
        .expires_at
        .filter(|expires_at| *expires_at <= OffsetDateTime::now_utc())
    {
        format!("expired ({})", timestamp(expires_at))
    } else {
        "active".to_owned()
    }
}

async fn domain_add(pool: &PgPool, domain: &str, description: Option<&str>) -> anyhow::Result<()> {
    let domain = sanitize_origin(domain)?;
    if kaunta::db::origins::find(pool, &domain).await?.is_some() {
        bail!("domain '{domain}' already exists");
    }
    let origin = kaunta::db::origins::create(pool, &domain, description).await?;
    println!("\n✓ Trusted domain added successfully");
    println!("  ID:     {}", origin.id);
    println!("  Domain: {}", origin.domain);
    if let Some(description) = origin.description {
        println!("  Desc:   {description}");
    }
    println!("  Active: {}", origin.is_active);
    println!("  Added:  {}", timestamp(origin.created_at));
    Ok(())
}

async fn domain_list(pool: &PgPool, active_only: bool) -> anyhow::Result<()> {
    let mut origins = kaunta::db::origins::list(pool).await?;
    if active_only {
        origins.retain(|origin| origin.is_active);
    }
    if origins.is_empty() {
        println!(
            "{}",
            if active_only {
                "No active trusted domains found"
            } else {
                "No trusted domains found"
            }
        );
        return Ok(());
    }
    println!("\nTotal domains: {}\n", origins.len());
    println!("ID\tACTIVE\tDOMAIN\tDESCRIPTION\tCREATED");
    for origin in origins {
        println!(
            "{}\t{}\t{}\t{}\t{}",
            origin.id,
            if origin.is_active { "✓" } else { "✗" },
            origin.domain,
            origin.description.as_deref().unwrap_or("-"),
            timestamp(origin.created_at)
        );
    }
    Ok(())
}

async fn domain_remove(pool: &PgPool, identifier: &str, force: bool) -> anyhow::Result<()> {
    let origin = kaunta::db::origins::find(pool, identifier)
        .await?
        .with_context(|| format!("domain '{identifier}' not found"))?;
    if !force && !confirm(&format!("Remove trusted domain '{}'?", origin.domain))? {
        println!("Removal cancelled");
        return Ok(());
    }
    kaunta::db::origins::delete_by_identifier(pool, identifier)
        .await?
        .with_context(|| format!("domain '{identifier}' not found"))?;
    println!("✓ Trusted domain '{}' removed successfully", origin.domain);
    Ok(())
}

async fn domain_toggle(pool: &PgPool, identifier: &str) -> anyhow::Result<()> {
    let (domain, active) = kaunta::db::origins::toggle(pool, identifier)
        .await?
        .with_context(|| format!("domain '{identifier}' not found"))?;
    println!(
        "✓ Domain '{}' {} successfully",
        domain,
        if active { "enabled" } else { "disabled" }
    );
    Ok(())
}

async fn domain_verify(pool: &PgPool, origin: &str) -> anyhow::Result<()> {
    let trusted = kaunta::db::origins::is_trusted(pool, origin).await?;
    println!(
        "{} Origin '{}' is {}",
        if trusted { "✓" } else { "✗" },
        origin,
        if trusted { "TRUSTED" } else { "NOT TRUSTED" }
    );
    Ok(())
}

async fn stats_overview(
    pool: &PgPool,
    website: &str,
    days: i32,
    format: &str,
) -> anyhow::Result<()> {
    validate_days(days)?;
    let website = website_by_identifier(pool, website).await?;
    let (total_pageviews, total_visitors, avg_engagement_seconds) =
        sqlx::query_as::<_, (i64, i64, f64)>(
            "SELECT
                COUNT(*)::bigint,
                COUNT(DISTINCT session_id)::bigint,
                COALESCE(AVG(engagement_time)::double precision / 1000.0, 0)
             FROM website_event
             WHERE website_id = $1
               AND created_at >= NOW() - ($2 * INTERVAL '1 day')
               AND event_type = 1",
        )
        .bind(website.website_id)
        .bind(days)
        .fetch_one(pool)
        .await?;
    let overview = Overview {
        total_visitors,
        total_pageviews,
        current_visitors: kaunta::db::analytics::current_visitors(pool, website.website_id).await?,
        top_page: top_named_metric(pool, website.website_id, days, "url_path").await?,
        top_referrer: top_named_metric(pool, website.website_id, days, "referrer_domain").await?,
        browser_distribution: distribution(pool, website.website_id, days, "browser", 3).await?,
        device_distribution: distribution(pool, website.website_id, days, "device", 10).await?,
        country_distribution: distribution(pool, website.website_id, days, "country", 3).await?,
        avg_engagement_seconds,
    };
    match format {
        "json" => println!("{}", serde_json::to_string_pretty(&overview)?),
        "table" | "text" => {
            println!(
                "Analytics overview for {} (last {days} days)",
                website.domain
            );
            println!("Total Visitors:\t{}", overview.total_visitors);
            println!("Total Pageviews:\t{}", overview.total_pageviews);
            println!("Active Visitors:\t{}", overview.current_visitors);
            println!("Top Page:\t{}", metric_label(overview.top_page.as_ref()));
            println!(
                "Top Referrer:\t{}",
                metric_label(overview.top_referrer.as_ref())
            );
            println!(
                "Average Engagement:\t{:.1}s",
                overview.avg_engagement_seconds
            );
            print_distribution("Browsers", &overview.browser_distribution);
            print_distribution("Devices", &overview.device_distribution);
            print_distribution("Countries", &overview.country_distribution);
        }
        _ => bail!("invalid format: {format} (use json, table, or text)"),
    }
    Ok(())
}

async fn stats_pages(
    pool: &PgPool,
    website: &str,
    days: i32,
    top: i32,
    format: &str,
) -> anyhow::Result<()> {
    validate_days(days)?;
    validate_top(top)?;
    let website = website_by_identifier(pool, website).await?;
    let pages = sqlx::query_as::<_, PageMetric>(
        "WITH per_session AS (
             SELECT session_id, url_path, COUNT(*) AS views
             FROM website_event
             WHERE website_id = $1
               AND created_at >= NOW() - ($2 * INTERVAL '1 day')
               AND event_type = 1
               AND url_path IS NOT NULL
             GROUP BY session_id, url_path
         )
         SELECT
             event.url_path AS path,
             COUNT(*)::bigint AS pageviews,
             COUNT(DISTINCT event.session_id)::bigint AS unique_visitors,
             COALESCE(
                 100.0 * COUNT(DISTINCT event.session_id) FILTER (WHERE per_session.views = 1)
                 / NULLIF(COUNT(DISTINCT event.session_id), 0),
                 0
             )::double precision AS bounce_rate,
             COALESCE(AVG(event.engagement_time)::double precision / 1000.0, 0) AS avg_time_seconds
         FROM website_event event
         JOIN per_session
           ON per_session.session_id = event.session_id
          AND per_session.url_path = event.url_path
         WHERE event.website_id = $1
           AND event.created_at >= NOW() - ($2 * INTERVAL '1 day')
           AND event.event_type = 1
           AND event.url_path IS NOT NULL
         GROUP BY event.url_path
         ORDER BY pageviews DESC
         LIMIT $3",
    )
    .bind(website.website_id)
    .bind(days)
    .bind(top)
    .fetch_all(pool)
    .await?;
    match format {
        "json" => println!("{}", serde_json::to_string_pretty(&pages)?),
        "csv" => {
            println!("path,pageviews,unique_visitors,bounce_rate,avg_time_seconds");
            for page in pages {
                println!(
                    "{},{},{},{:.2},{:.2}",
                    csv_field(&page.path),
                    page.pageviews,
                    page.unique_visitors,
                    page.bounce_rate,
                    page.avg_time_seconds
                );
            }
        }
        "table" => {
            println!("PATH\tPAGEVIEWS\tUNIQUE VISITORS\tBOUNCE RATE\tAVG TIME");
            for page in pages {
                println!(
                    "{}\t{}\t{}\t{:.1}%\t{:.1}s",
                    page.path,
                    page.pageviews,
                    page.unique_visitors,
                    page.bounce_rate,
                    page.avg_time_seconds
                );
            }
        }
        _ => bail!("invalid format: {format} (use json, table, or csv)"),
    }
    Ok(())
}

async fn stats_breakdown(
    pool: &PgPool,
    website: &str,
    dimension: &str,
    days: i32,
    top: i32,
    format: &str,
) -> anyhow::Result<()> {
    validate_days(days)?;
    validate_top(top)?;
    let expression = match dimension {
        "country" => "COALESCE(session.country, 'Unknown')",
        "browser" => "COALESCE(session.browser, 'Unknown')",
        "device" => "COALESCE(session.device, 'Unknown')",
        "referrer" => "COALESCE(event.referrer_domain, 'Direct / None')",
        "os" => "COALESCE(session.os, 'Unknown')",
        _ => {
            bail!("invalid dimension: {dimension} (valid: country, browser, device, referrer, os)")
        }
    };
    let website = website_by_identifier(pool, website).await?;
    let query = format!(
        "WITH session_totals AS (
             SELECT session_id, COUNT(*) AS views
             FROM website_event
             WHERE website_id = $1
               AND created_at >= NOW() - ($2 * INTERVAL '1 day')
               AND event_type = 1
             GROUP BY session_id
         )
         SELECT
             {expression} AS name,
             COUNT(DISTINCT event.session_id)::bigint AS visitors,
             COUNT(*)::bigint AS pageviews,
             COALESCE(
                 100.0 * COUNT(DISTINCT event.session_id) FILTER (WHERE session_totals.views = 1)
                 / NULLIF(COUNT(DISTINCT event.session_id), 0),
                 0
             )::double precision AS bounce_rate
         FROM website_event event
         JOIN session ON session.session_id = event.session_id
         JOIN session_totals ON session_totals.session_id = event.session_id
         WHERE event.website_id = $1
           AND event.created_at >= NOW() - ($2 * INTERVAL '1 day')
           AND event.event_type = 1
         GROUP BY {expression}
         ORDER BY visitors DESC
         LIMIT $3"
    );
    let rows = sqlx::query_as::<_, BreakdownMetric>(AssertSqlSafe(query))
        .bind(website.website_id)
        .bind(days)
        .bind(top)
        .fetch_all(pool)
        .await?;
    match format {
        "json" => println!("{}", serde_json::to_string_pretty(&rows)?),
        "csv" => {
            println!("name,visitors,pageviews,bounce_rate");
            for row in rows {
                println!(
                    "{},{},{},{:.2}",
                    csv_field(&row.name),
                    row.visitors,
                    row.pageviews,
                    row.bounce_rate
                );
            }
        }
        "table" => {
            println!("NAME\tVISITORS\tPAGEVIEWS\tBOUNCE RATE");
            for row in rows {
                println!(
                    "{}\t{}\t{}\t{:.1}%",
                    row.name, row.visitors, row.pageviews, row.bounce_rate
                );
            }
        }
        _ => bail!("invalid format: {format} (use json, table, or csv)"),
    }
    Ok(())
}

async fn stats_live(
    pool: &PgPool,
    website: &str,
    interval_seconds: u64,
    format: &str,
) -> anyhow::Result<()> {
    if !(2..=60).contains(&interval_seconds) {
        bail!("interval must be between 2 and 60 seconds");
    }
    if !matches!(format, "json" | "text") {
        bail!("invalid format: {format} (use json or text)");
    }
    let website = website_by_identifier(pool, website).await?;
    println!(
        "Live stats for {} (updating every {} seconds, press Ctrl+C to exit)\n",
        website.domain, interval_seconds
    );
    let mut ticker = tokio::time::interval(std::time::Duration::from_secs(interval_seconds));
    loop {
        tokio::select! {
            result = tokio::signal::ctrl_c() => {
                result?;
                println!("\nExiting live stats...");
                return Ok(());
            }
            _ = ticker.tick() => {
                let live = live_stats(pool, website.website_id).await?;
                if format == "json" {
                    println!("{}", serde_json::to_string(&live)?);
                } else {
                    println!(
                        "{}  active={}  last_minute={}  recent={}  top={}",
                        timestamp(live.timestamp),
                        live.active_visitors_now,
                        live.pageviews_last_minute,
                        live.recent_events,
                        live.top_page_now.as_deref().unwrap_or("-")
                    );
                }
            }
        }
    }
}

async fn live_stats(pool: &PgPool, website_id: Uuid) -> anyhow::Result<LiveStats> {
    let (active, last_minute, recent) = sqlx::query_as::<_, (i64, i64, i64)>(
        "SELECT
             COUNT(DISTINCT session_id) FILTER (WHERE created_at >= NOW() - INTERVAL '5 minutes')::bigint,
             COUNT(*) FILTER (WHERE created_at >= NOW() - INTERVAL '1 minute')::bigint,
             COUNT(*) FILTER (WHERE created_at >= NOW() - INTERVAL '5 minutes')::bigint
         FROM website_event
         WHERE website_id = $1 AND event_type = 1",
    )
    .bind(website_id)
    .fetch_one(pool)
    .await?;
    let top_page_now = sqlx::query_scalar::<_, String>(
        "SELECT url_path
         FROM website_event
         WHERE website_id = $1
           AND created_at >= NOW() - INTERVAL '5 minutes'
           AND event_type = 1
           AND url_path IS NOT NULL
         GROUP BY url_path
         ORDER BY COUNT(*) DESC
         LIMIT 1",
    )
    .bind(website_id)
    .fetch_optional(pool)
    .await?;
    Ok(LiveStats {
        timestamp: OffsetDateTime::now_utc(),
        active_visitors_now: active,
        pageviews_last_minute: last_minute,
        recent_events: recent,
        top_page_now,
    })
}

async fn tracking_check(
    pool: &PgPool,
    website_identifier: &str,
    origin: Option<&str>,
    payload: Option<&Path>,
) -> anyhow::Result<()> {
    println!("=== Kaunta Tracking Setup Test ===");
    let website = website_by_identifier(pool, website_identifier).await?;
    println!("Step 1: Checking website exists... PASS");
    println!("  Website ID: {}", website.website_id);
    println!("  Domain: {}", website.domain);

    let origin = origin.map_or_else(|| format!("https://{}", website.domain), str::to_owned);
    let origin_host = origin_host(&origin)?;
    let allowed = website.allowed_domains.iter().any(|candidate| {
        candidate.eq_ignore_ascii_case(&origin_host)
            || candidate.eq_ignore_ascii_case(&origin)
            || candidate.eq_ignore_ascii_case(&website.domain)
    });
    println!(
        "\nStep 2: Validating CORS origin... {}",
        if allowed { "PASS" } else { "WARN" }
    );
    println!("  Origin: {origin_host}");

    let database_valid =
        kaunta::db::websites::validate_origin(pool, website.website_id, &origin).await?;
    println!(
        "\nStep 3: Testing database origin validation... {}",
        if database_valid { "PASS" } else { "WARN" }
    );

    let payload = if let Some(path) = payload {
        let contents =
            fs::read_to_string(path).with_context(|| format!("read payload {}", path.display()))?;
        serde_json::from_str::<serde_json::Value>(&contents)
            .with_context(|| format!("parse payload {}", path.display()))?
    } else {
        serde_json::json!({
            "type": "event",
            "payload": {
                "website": website.website_id,
                "hostname": origin_host,
                "url": "/test",
                "title": "Test Page",
                "language": "en-US",
                "screen": "1920x1080",
                "timestamp": OffsetDateTime::now_utc().unix_timestamp_nanos() / 1_000_000
            }
        })
    };
    println!("\nStep 4: Preparing test payload... PASS");
    println!("{}", serde_json::to_string_pretty(&payload)?);
    println!("\nStep 5: Tracking event send... PASS (validation only)");
    println!(
        "\nStatus: {}",
        if allowed && database_valid {
            "Ready for tracking ✓"
        } else {
            "Configuration needed ⚠"
        }
    );
    if !allowed {
        println!(
            "To fix: kaunta website add-domain {} {}",
            website.domain, origin_host
        );
    }
    Ok(())
}

async fn top_named_metric(
    pool: &PgPool,
    website_id: Uuid,
    days: i32,
    field: &str,
) -> anyhow::Result<Option<NamedMetric>> {
    let expression = match field {
        "url_path" => "COALESCE(url_path, 'Unknown')",
        "referrer_domain" => "COALESCE(referrer_domain, 'Direct / None')",
        _ => bail!("unsupported metric field"),
    };
    let query = format!(
        "SELECT {expression} AS name, COUNT(*)::bigint AS count
         FROM website_event
         WHERE website_id = $1
           AND created_at >= NOW() - ($2 * INTERVAL '1 day')
           AND event_type = 1
         GROUP BY {expression}
         ORDER BY count DESC
         LIMIT 1"
    );
    Ok(sqlx::query_as::<_, NamedMetric>(AssertSqlSafe(query))
        .bind(website_id)
        .bind(days)
        .fetch_optional(pool)
        .await?)
}

async fn distribution(
    pool: &PgPool,
    website_id: Uuid,
    days: i32,
    field: &str,
    limit: i32,
) -> anyhow::Result<Vec<NamedMetric>> {
    let expression = match field {
        "browser" => "COALESCE(session.browser, 'Unknown')",
        "device" => "COALESCE(session.device, 'Unknown')",
        "country" => "COALESCE(session.country, 'Unknown')",
        _ => bail!("unsupported distribution field"),
    };
    let query = format!(
        "SELECT {expression} AS name, COUNT(DISTINCT event.session_id)::bigint AS count
         FROM website_event event
         JOIN session ON session.session_id = event.session_id
         WHERE event.website_id = $1
           AND event.created_at >= NOW() - ($2 * INTERVAL '1 day')
           AND event.event_type = 1
         GROUP BY {expression}
         ORDER BY count DESC
         LIMIT $3"
    );
    Ok(sqlx::query_as::<_, NamedMetric>(AssertSqlSafe(query))
        .bind(website_id)
        .bind(days)
        .bind(limit)
        .fetch_all(pool)
        .await?)
}

pub async fn run_doctor(config: &Config, json: bool) -> anyhow::Result<()> {
    let mut results = vec![check_data_directory(config), check_geoip_database(config)];
    match database(config).await {
        Ok(pool) => {
            results.push(CheckResult::passed("Database Connection", None));
            results.push(check_postgresql_version(&pool).await);
            results.push(check_migrations(&pool).await);
            results.push(
                check_named_objects(
                    &pool,
                    "PostgreSQL Functions",
                    "SELECT proname FROM pg_proc JOIN pg_namespace ON pg_proc.pronamespace = pg_namespace.oid WHERE nspname = 'public' AND proname = ANY($1)",
                    &[
                        "hash_password", "verify_password", "validate_session",
                        "cleanup_expired_sessions", "cleanup_old_partitions",
                        "cleanup_old_bot_logs", "get_partition_stats",
                        "reset_stale_request_counters", "is_trusted_origin",
                        "update_trusted_origin_timestamp", "get_trusted_origins",
                        "get_dashboard_stats", "get_top_pages", "get_timeseries",
                        "get_breakdown", "validate_origin",
                    ],
                ).await,
            );
            results.push(
                check_named_objects(
                    &pool,
                    "PostgreSQL Triggers",
                    "SELECT tgname FROM pg_trigger WHERE tgname = ANY($1)",
                    &[
                        "trg_website_event_realtime_stats",
                        "trigger_update_trusted_origin_timestamp",
                    ],
                )
                .await,
            );
            results.push(
                check_named_objects(
                    &pool,
                    "Materialized Views",
                    "SELECT matviewname FROM pg_matviews WHERE schemaname = 'public' AND matviewname = ANY($1)",
                    &["daily_website_stats", "hourly_website_stats", "realtime_website_stats", "bot_stats_by_country"],
                ).await,
            );
        }
        Err(error) => results.push(CheckResult::failed(
            "Database Connection",
            error.to_string(),
            Some("Verify DATABASE_URL and ensure PostgreSQL 18 is running"),
        )),
    }

    if json {
        println!("{}", serde_json::to_string_pretty(&results)?);
    } else {
        println!("\nKaunta Health Check");
        for result in &results {
            println!(
                "{} {}{}",
                if result.pass { "✓" } else { "✗" },
                result.name,
                result
                    .details
                    .as_ref()
                    .map_or_else(String::new, |details| format!(" ({details})"))
            );
            if let Some(error) = &result.error {
                println!("  Error: {error}");
            }
            if let Some(suggestion) = &result.suggestion {
                println!("  Suggestion: {suggestion}");
            }
        }
        let passed = results.iter().filter(|result| result.pass).count();
        println!("\n{passed}/{} checks passed\n", results.len());
    }
    if results.iter().all(|result| result.pass) {
        Ok(())
    } else {
        bail!("one or more health checks failed")
    }
}

pub async fn run_diagnostics(config: &Config, full: bool) -> anyhow::Result<()> {
    let pool = database(config).await?;
    let version: String = sqlx::query_scalar("SHOW server_version")
        .fetch_one(&pool)
        .await?;
    let extensions: Vec<String> =
        sqlx::query_scalar("SELECT extname FROM pg_extension ORDER BY extname")
            .fetch_all(&pool)
            .await?;
    let website_count: i64 =
        sqlx::query_scalar("SELECT COUNT(*)::bigint FROM website WHERE deleted_at IS NULL")
            .fetch_one(&pool)
            .await?;
    let session_count: i64 = sqlx::query_scalar("SELECT COUNT(*)::bigint FROM session")
        .fetch_one(&pool)
        .await?;
    let event_count: i64 = sqlx::query_scalar("SELECT COUNT(*)::bigint FROM website_event")
        .fetch_one(&pool)
        .await?;
    let (oldest, newest): (Option<OffsetDateTime>, Option<OffsetDateTime>) =
        sqlx::query_as("SELECT MIN(created_at), MAX(created_at) FROM website_event")
            .fetch_one(&pool)
            .await?;
    let partitions: i64 = sqlx::query_scalar(
        "SELECT COUNT(*)::bigint FROM pg_inherits
         JOIN pg_class parent ON pg_inherits.inhparent = parent.oid
         WHERE parent.relname IN ('website_event', 'bot_detection_log', 'event_idempotency')",
    )
    .fetch_one(&pool)
    .await?;
    let bytes: i64 = sqlx::query_scalar("SELECT pg_database_size(current_database())::bigint")
        .fetch_one(&pool)
        .await?;
    let rate: f64 = sqlx::query_scalar(
        "SELECT COUNT(*)::double precision / 60.0 FROM website_event
         WHERE created_at >= NOW() - INTERVAL '1 hour'",
    )
    .fetch_one(&pool)
    .await?;
    let retention: i32 = sqlx::query_scalar(
        "SELECT COALESCE(EXTRACT(DAY FROM MAX(created_at) - MIN(created_at))::integer, 0)
         FROM website_event",
    )
    .fetch_one(&pool)
    .await?;

    println!("=== Kaunta System Diagnostics ===");
    println!("Database Connected:\tPASS");
    println!("PostgreSQL Version:\t{version}");
    println!("Extensions Loaded:\t{}", extensions.join(", "));
    println!("Websites:\t{website_count}");
    println!("Sessions:\t{session_count}");
    println!("Events:\t{event_count}");
    if let Some(value) = oldest {
        println!("Oldest Event:\t{}", timestamp(value));
    }
    if let Some(value) = newest {
        println!("Newest Event:\t{}", timestamp(value));
    }
    println!("Data Retention:\t{retention} days");
    println!("Events Per Minute:\t{rate:.1}");
    println!("Partitions:\t{partitions}");
    println!("Disk Usage:\t{:.2} GB", gigabytes(bytes));
    println!("Status:\tPASS");

    if full {
        println!("\nIndex Status:");
        for (name, scans, read, fetched) in sqlx::query_as::<_, (String, i64, i64, i64)>(
            "SELECT indexrelname, idx_scan, idx_tup_read, idx_tup_fetch
             FROM pg_stat_user_indexes WHERE schemaname = 'public'
             ORDER BY idx_scan DESC LIMIT 10",
        )
        .fetch_all(&pool)
        .await?
        {
            println!("{name}\t{scans}\t{read}\t{fetched}");
        }
        println!("\nTable Sizes:");
        for (name, size) in sqlx::query_as::<_, (String, String)>(
            "SELECT tablename, pg_size_pretty(pg_total_relation_size(quote_ident(schemaname) || '.' || quote_ident(tablename)))
             FROM pg_tables WHERE schemaname = 'public'
             ORDER BY pg_total_relation_size(quote_ident(schemaname) || '.' || quote_ident(tablename)) DESC LIMIT 10",
        ).fetch_all(&pool).await? {
            println!("{name}\t{size}");
        }
    }
    Ok(())
}

fn check_data_directory(config: &Config) -> CheckResult {
    if let Err(error) = fs::create_dir_all(&config.data_dir) {
        return CheckResult::failed(
            "Data Directory Writable",
            error.to_string(),
            Some("Ensure DATA_DIR can be created"),
        );
    }
    let path = config.data_dir.join(".kaunta-write-test");
    match fs::write(&path, b"test") {
        Ok(()) => {
            let _ = fs::remove_file(path);
            CheckResult::passed("Data Directory Writable", None)
        }
        Err(error) => CheckResult::failed(
            "Data Directory Writable",
            error.to_string(),
            Some("Ensure DATA_DIR has write permissions"),
        ),
    }
}

fn check_geoip_database(config: &Config) -> CheckResult {
    match fs::metadata(config.data_dir.join("GeoLite2-City.mmdb")) {
        Ok(metadata) => CheckResult::passed(
            "GeoIP Database",
            Some(format!("{:.1} MB", megabytes(metadata.len()))),
        ),
        Err(error) => CheckResult::failed(
            "GeoIP Database",
            error.to_string(),
            Some("The database auto-downloads on first server start"),
        ),
    }
}

async fn check_postgresql_version(pool: &PgPool) -> CheckResult {
    match sqlx::query_scalar::<_, String>("SHOW server_version")
        .fetch_one(pool)
        .await
    {
        Ok(version) => {
            let major = version
                .split('.')
                .next()
                .and_then(|value| value.parse::<u32>().ok())
                .unwrap_or_default();
            if major >= 18 {
                CheckResult::passed("PostgreSQL Version", Some(version))
            } else {
                CheckResult::failed(
                    "PostgreSQL Version",
                    format!("Version {version} found, need PostgreSQL 18 or newer"),
                    Some("Upgrade PostgreSQL to version 18 or higher"),
                )
            }
        }
        Err(error) => CheckResult::failed("PostgreSQL Version", error.to_string(), None),
    }
}

async fn check_migrations(pool: &PgPool) -> CheckResult {
    let latest = match kaunta::db::migrations::latest_version() {
        Ok(version) => version,
        Err(error) => return CheckResult::failed("Database Migrations", error.to_string(), None),
    };
    match kaunta::db::migrations::current_version(pool).await {
        Ok(Some((version, false))) if version == latest => {
            CheckResult::passed("Database Migrations", Some(format!("v{version}")))
        }
        Ok(Some((version, dirty))) => CheckResult::failed(
            "Database Migrations",
            format!("database={version}, latest={latest}, dirty={dirty}"),
            Some("Run: kaunta migrate"),
        ),
        Ok(None) => CheckResult::failed(
            "Database Migrations",
            "database has not been migrated",
            Some("Run: kaunta migrate"),
        ),
        Err(error) => CheckResult::failed("Database Migrations", error.to_string(), None),
    }
}

async fn check_named_objects(
    pool: &PgPool,
    label: &str,
    query: &'static str,
    required: &[&str],
) -> CheckResult {
    let names = required
        .iter()
        .map(|name| (*name).to_owned())
        .collect::<Vec<_>>();
    match sqlx::query_scalar::<_, String>(query)
        .bind(&names)
        .fetch_all(pool)
        .await
    {
        Ok(found) => {
            let missing = required
                .iter()
                .filter(|name| !found.iter().any(|candidate| candidate == **name))
                .copied()
                .collect::<Vec<_>>();
            if missing.is_empty() {
                CheckResult::passed(
                    label,
                    Some(format!("{}/{} found", required.len(), required.len())),
                )
            } else {
                CheckResult::failed(
                    label,
                    format!("missing: {}", missing.join(", ")),
                    Some("Run database migrations"),
                )
            }
        }
        Err(error) => CheckResult::failed(label, error.to_string(), None),
    }
}

fn parse_csv(value: Option<&str>) -> Vec<String> {
    value
        .into_iter()
        .flat_map(|value| value.split(','))
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(str::to_owned)
        .collect()
}

fn csv_field(value: &str) -> String {
    if value.contains([',', '"', '\n']) {
        format!("\"{}\"", value.replace('"', "\"\""))
    } else {
        value.to_owned()
    }
}

fn confirm(prompt: &str) -> anyhow::Result<bool> {
    print!("{prompt} (yes/no): ");
    io::stdout().flush()?;
    let mut response = String::new();
    io::stdin().read_line(&mut response)?;
    Ok(matches!(
        response.trim().to_lowercase().as_str(),
        "yes" | "y"
    ))
}

fn timestamp(value: OffsetDateTime) -> String {
    value.format(&Rfc3339).unwrap_or_else(|_| value.to_string())
}

#[expect(
    clippy::cast_precision_loss,
    reason = "display-only conversion; sizes never approach 2^53 bytes"
)]
fn gigabytes(bytes: i64) -> f64 {
    bytes as f64 / 1_073_741_824.0
}

#[expect(
    clippy::cast_precision_loss,
    reason = "display-only conversion; sizes never approach 2^53 bytes"
)]
fn megabytes(bytes: u64) -> f64 {
    bytes as f64 / 1_048_576.0
}

fn validate_days(days: i32) -> anyhow::Result<()> {
    if !(1..=365).contains(&days) {
        bail!("days must be between 1 and 365");
    }
    Ok(())
}

fn validate_top(top: i32) -> anyhow::Result<()> {
    if !(1..=100).contains(&top) {
        bail!("top must be between 1 and 100");
    }
    Ok(())
}

fn origin_host(origin: &str) -> anyhow::Result<String> {
    let value = if origin.contains("://") {
        origin.to_owned()
    } else {
        format!("https://{origin}")
    };
    Url::parse(&value)
        .context("invalid origin URL")?
        .host_str()
        .map(str::to_owned)
        .context("origin URL has no host")
}

fn metric_label(metric: Option<&NamedMetric>) -> String {
    metric.map_or_else(
        || "-".to_owned(),
        |metric| format!("{} ({})", metric.name, metric.count),
    )
}

fn print_distribution(label: &str, metrics: &[NamedMetric]) {
    let value = if metrics.is_empty() {
        "-".to_owned()
    } else {
        metrics
            .iter()
            .map(|metric| format!("{}={}", metric.name, metric.count))
            .collect::<Vec<_>>()
            .join(", ")
    };
    println!("{label}:\t{value}");
}

#[cfg(test)]
mod tests;

pub async fn run_backup(
    config: &Config,
    command: BackupCommand,
    version: &str,
) -> anyhow::Result<()> {
    let (days, output) = match command {
        BackupCommand::Create { days, output } => (days, output),
        BackupCommand::Restore { file, yes } => {
            anyhow::ensure!(file.is_file(), "{} is not a file", file.display());
            let name = file.file_name().unwrap_or_default().to_string_lossy();
            anyhow::ensure!(
                !name.ends_with(".tar.gz"),
                "period archives are data exports; restore only applies full dumps (kaunta-full-*.dump)"
            );
            if !yes
                && !confirm(&format!(
                    "Restore {} into the configured database, replacing its current contents? Stop the kaunta server first",
                    file.display()
                ))?
            {
                println!("Restore cancelled.");
                return Ok(());
            }
            let pool = database(config).await?;
            kaunta::db::backup::restore_full(&pool, &config.database_url, &file).await?;
            println!(
                "Restore complete. Run `kaunta migrate up` if the dump predates this binary's schema."
            );
            return Ok(());
        }
    };
    let pool = database(config).await?;
    let target_dir = output.unwrap_or_else(|| std::path::PathBuf::from("."));
    let mode = match days {
        Some(days) => {
            anyhow::ensure!(days > 0, "--days must be positive");
            let to = OffsetDateTime::now_utc();
            kaunta::db::backup::BackupMode::Period {
                from: to - time::Duration::days(i64::from(days)),
                to,
            }
        }
        None => kaunta::db::backup::BackupMode::Full,
    };
    let backup =
        kaunta::db::backup::create_backup(&pool, &config.database_url, &target_dir, mode, version)
            .await?;
    println!(
        "Backup written: {}",
        target_dir.join(&backup.file_name).display()
    );
    println!("SHA-256: {}", backup.sha256);
    println!("Size: {} bytes", backup.size_bytes);
    Ok(())
}
