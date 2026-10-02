use std::{collections::HashSet, path::Path};

use anyhow::{Context, bail};
use kaunta::domain::{SELF_WEBSITE_ID, website::validate_domain};
use serde::Deserialize;
use sqlx::{PgPool, types::Json};
use uuid::Uuid;

#[derive(Debug, Deserialize)]
struct Import {
    websites: Vec<ImportWebsite>,
}

#[derive(Debug, Deserialize)]
struct ImportWebsite {
    domain: String,
    #[serde(default)]
    name: String,
    #[serde(default)]
    allowed_domains: Vec<String>,
}

fn parse(path: &Path, bytes: &[u8]) -> anyhow::Result<Import> {
    let mut import: Import = match path.extension().and_then(|ext| ext.to_str()) {
        Some("json") => serde_json::from_slice(bytes).context("invalid JSON import")?,
        Some("yaml" | "yml") => serde_yaml_ng::from_slice(bytes).context("invalid YAML import")?,
        _ => bail!("unsupported file format (use .yaml, .yml or .json)"),
    };
    if import.websites.is_empty() {
        bail!("no websites found in file");
    }
    let mut domains = HashSet::new();
    for website in &mut import.websites {
        validate_domain(&website.domain)
            .with_context(|| format!("invalid website '{}'", website.domain))?;
        if !domains.insert(website.domain.to_lowercase()) {
            bail!("duplicate website '{}'", website.domain);
        }
        if website.name.is_empty() {
            website.name.clone_from(&website.domain);
        }
    }
    Ok(import)
}

pub async fn run(pool: &PgPool, path: &Path, dry_run: bool, replace: bool) -> anyhow::Result<()> {
    let import = parse(path, &std::fs::read(path).context("read website import")?)?;
    let self_id = Uuid::parse_str(SELF_WEBSITE_ID)?;
    let mut transaction = pool.begin().await?;
    if !dry_run {
        sqlx::query("LOCK TABLE website IN SHARE ROW EXCLUSIVE MODE")
            .execute(&mut *transaction)
            .await?;
    }
    let existing: Vec<(Uuid, String)> =
        sqlx::query_as("SELECT website_id, domain FROM website WHERE deleted_at IS NULL")
            .fetch_all(&mut *transaction)
            .await?;
    if import.websites.iter().any(|website| {
        existing
            .iter()
            .any(|(id, domain)| *id == self_id && domain.eq_ignore_ascii_case(&website.domain))
    }) {
        bail!("the reserved self-tracking website cannot be imported");
    }
    let domains: Vec<_> = import
        .websites
        .iter()
        .map(|website| website.domain.to_lowercase())
        .collect();
    let removed = if replace {
        existing
            .iter()
            .filter(|(id, domain)| *id != self_id && !domains.contains(&domain.to_lowercase()))
            .count()
    } else {
        0
    };
    if replace && !dry_run {
        sqlx::query(
            "UPDATE website SET deleted_at = NOW(), updated_at = NOW()
             WHERE website_id <> $1 AND deleted_at IS NULL AND NOT (LOWER(domain) = ANY($2))",
        )
        .bind(self_id)
        .bind(domains)
        .execute(&mut *transaction)
        .await?;
    }
    let mut created = 0;
    let mut updated = 0;
    for website in import.websites {
        let id = existing
            .iter()
            .find(|(_, domain)| domain.eq_ignore_ascii_case(&website.domain))
            .map(|(id, _)| *id);
        if let Some(id) = id {
            updated += 1;
            if !dry_run {
                sqlx::query(
                    "UPDATE website SET name = $2, allowed_domains = $3, updated_at = NOW()
                     WHERE website_id = $1",
                )
                .bind(id)
                .bind(&website.name)
                .bind(Json(&website.allowed_domains))
                .execute(&mut *transaction)
                .await?;
            }
        } else {
            created += 1;
            if !dry_run {
                sqlx::query(
                    "INSERT INTO website (domain, name, allowed_domains, created_at, updated_at)
                     VALUES ($1, $2, $3, NOW(), NOW())",
                )
                .bind(&website.domain)
                .bind(&website.name)
                .bind(Json(&website.allowed_domains))
                .execute(&mut *transaction)
                .await?;
            }
        }
    }
    if dry_run {
        transaction.rollback().await?;
        println!("[DRY RUN - No changes applied]");
    } else {
        transaction.commit().await?;
    }
    println!(
        "=== Website Sync Report ===\nCreated: {created}\nUpdated: {updated}\nRemoved: {removed}\nSkipped: 0"
    );
    Ok(())
}

#[cfg(test)]
mod tests;
