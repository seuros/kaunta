//! Backup generation shared by the CLI and the MCP tool.
//!
//! Two modes:
//! - Full: shells out to `pg_dump --format=custom`, the canonical
//!   restorable snapshot (`pg_restore` on the other end).
//! - Period: a kaunta-native `tar.gz` holding a `manifest.json` plus one
//!   CSV (with header) per event-bearing table, restricted to the window.
//!   Restorable with `psql \copy` per the manifest.
//!
//! Files are first written under a temporary name, hashed, then renamed to
//! `kaunta-{mode}-{timestamp}-{sha256 prefix}.{ext}`. The hash suffix is
//! what makes the download URL a capability when served over HTTP.

use std::path::{Path, PathBuf};

use futures_util::TryStreamExt;
use sha2::{Digest, Sha256};
use sqlx::{AssertSqlSafe, PgPool};
use time::OffsetDateTime;
use time::format_description::well_known::Rfc3339;
use tokio::io::AsyncWriteExt;

#[derive(Debug, thiserror::Error)]
pub enum BackupError {
    #[error(transparent)]
    Database(#[from] sqlx::Error),
    #[error(transparent)]
    Io(#[from] std::io::Error),
    #[error("pg_dump failed: {0}")]
    PgDump(String),
    #[error("pg_restore failed: {0}")]
    PgRestore(String),
    #[error("backup file integrity check failed: {0}")]
    Integrity(String),
    #[error("timestamp formatting failed: {0}")]
    Format(#[from] time::error::Format),
    #[error(transparent)]
    Json(#[from] serde_json::Error),
}

#[derive(Debug, Clone, Copy)]
pub enum BackupMode {
    Full,
    Period {
        from: OffsetDateTime,
        to: OffsetDateTime,
    },
}

#[derive(Debug, Clone, serde::Serialize)]
pub struct BackupFile {
    pub file_name: String,
    pub sha256: String,
    pub size_bytes: u64,
}

/// Tables included in a period backup and their window column.
const PERIOD_TABLES: &[(&str, &str)] = &[
    ("session", "created_at"),
    ("website_event", "created_at"),
    ("goal_completions", "completed_at"),
];

pub async fn create_backup(
    pool: &PgPool,
    database_url: &str,
    backups_dir: &Path,
    mode: BackupMode,
    kaunta_version: &str,
) -> Result<BackupFile, BackupError> {
    tokio::fs::create_dir_all(backups_dir).await?;
    let temporary = backups_dir.join(format!(".tmp-{}", std::process::id()));
    let result = match mode {
        BackupMode::Full => dump_full(database_url, &temporary).await,
        BackupMode::Period { from, to } => {
            dump_period(pool, &temporary, from, to, kaunta_version).await
        }
    };
    if let Err(error) = result {
        let _ = tokio::fs::remove_file(&temporary).await;
        return Err(error);
    }

    let sha256 = file_sha256(&temporary).await?;
    let size_bytes = tokio::fs::metadata(&temporary).await?.len();
    let timestamp = OffsetDateTime::now_utc()
        .format(&Rfc3339)?
        .replace([':', '-'], "")
        .replace('T', "-");
    let (label, extension) = match mode {
        BackupMode::Full => ("full", "dump"),
        BackupMode::Period { .. } => ("period", "tar.gz"),
    };
    let file_name = format!("kaunta-{label}-{timestamp}-{}.{extension}", &sha256[..16]);
    tokio::fs::rename(&temporary, backups_dir.join(&file_name)).await?;
    Ok(BackupFile {
        file_name,
        sha256,
        size_bytes,
    })
}

/// Restore a full `pg_dump --format=custom` backup into the database.
/// When the filename carries kaunta's sha256 suffix, the file content is
/// verified against it before anything touches the database. The public
/// schema is dropped and recreated instead of using `pg_restore --clean`,
/// which cannot drop the inherited constraints of partitioned tables.
pub async fn restore_full(
    pool: &PgPool,
    database_url: &str,
    file: &Path,
) -> Result<(), BackupError> {
    if let Some(expected) = embedded_sha_prefix(file) {
        let actual = file_sha256(file).await?;
        if !actual.starts_with(&expected) {
            return Err(BackupError::Integrity(format!(
                "file hash {} does not match the {} suffix in the name",
                &actual[..16],
                expected
            )));
        }
    }
    sqlx::query("DROP SCHEMA public CASCADE")
        .execute(pool)
        .await?;
    sqlx::query("CREATE SCHEMA public").execute(pool).await?;
    let output = tokio::process::Command::new("pg_restore")
        .arg("--no-owner")
        .arg("--dbname")
        .arg(database_url)
        .arg(file)
        .output()
        .await
        .map_err(|error| BackupError::PgRestore(format!("failed to run pg_restore: {error}")))?;
    if !output.status.success() {
        return Err(BackupError::PgRestore(
            String::from_utf8_lossy(&output.stderr).trim().to_owned(),
        ));
    }
    Ok(())
}

/// The 16-hex-char sha256 prefix kaunta embeds in backup filenames, when
/// the file still carries one (`kaunta-full-<ts>-<sha16>.dump`).
fn embedded_sha_prefix(file: &Path) -> Option<String> {
    let stem = file.file_stem()?.to_str()?;
    let candidate = stem.rsplit('-').next()?;
    (candidate.len() == 16 && candidate.chars().all(|c| c.is_ascii_hexdigit()))
        .then(|| candidate.to_ascii_lowercase())
}

async fn dump_full(database_url: &str, target: &Path) -> Result<(), BackupError> {
    let output = tokio::process::Command::new("pg_dump")
        .arg("--format=custom")
        .arg("--file")
        .arg(target)
        .arg(database_url)
        .output()
        .await
        .map_err(|error| BackupError::PgDump(format!("failed to run pg_dump: {error}")))?;
    if !output.status.success() {
        return Err(BackupError::PgDump(
            String::from_utf8_lossy(&output.stderr).trim().to_owned(),
        ));
    }
    Ok(())
}

async fn dump_period(
    pool: &PgPool,
    target: &Path,
    from: OffsetDateTime,
    to: OffsetDateTime,
    kaunta_version: &str,
) -> Result<(), BackupError> {
    let staging = target.with_extension("staging");
    tokio::fs::create_dir_all(&staging).await?;

    let mut tables = Vec::new();
    for (table, column) in PERIOD_TABLES {
        let csv_path = staging.join(format!("{table}.csv"));
        let rows = copy_window_csv(pool, table, column, from, to, &csv_path).await?;
        let bytes = tokio::fs::metadata(&csv_path).await?.len();
        tables.push(serde_json::json!({
            "name": table,
            "window_column": column,
            "rows": rows,
            "bytes": bytes,
        }));
    }
    let migration_version: Option<i64> =
        sqlx::query_scalar("SELECT version FROM schema_migrations LIMIT 1")
            .fetch_optional(pool)
            .await?;
    let manifest = serde_json::json!({
        "format_version": 1,
        "mode": "period",
        "from": from.format(&Rfc3339)?,
        "to": to.format(&Rfc3339)?,
        "generated_at": OffsetDateTime::now_utc().format(&Rfc3339)?,
        "kaunta_version": kaunta_version,
        "migration_version": migration_version,
        "tables": tables,
        "restore": "psql: \\copy <table> FROM '<table>.csv' WITH (FORMAT csv, HEADER true)",
    });
    tokio::fs::write(
        staging.join("manifest.json"),
        serde_json::to_vec_pretty(&manifest)?,
    )
    .await?;

    let staging_clone = staging.clone();
    let target_clone = target.to_path_buf();
    let build = tokio::task::spawn_blocking(move || -> Result<(), BackupError> {
        let file = std::fs::File::create(&target_clone)?;
        let encoder = flate2::write::GzEncoder::new(file, flate2::Compression::default());
        let mut archive = tar::Builder::new(encoder);
        for entry in std::fs::read_dir(&staging_clone)? {
            let entry = entry?;
            archive.append_path_with_name(entry.path(), entry.file_name())?;
        }
        archive.into_inner()?.finish()?.sync_all()?;
        Ok(())
    })
    .await
    .map_err(|error| BackupError::PgDump(format!("archive task failed: {error}")))?;
    let _ = tokio::fs::remove_dir_all(&staging).await;
    build
}

async fn copy_window_csv(
    pool: &PgPool,
    table: &str,
    column: &str,
    from: OffsetDateTime,
    to: OffsetDateTime,
    csv_path: &Path,
) -> Result<i64, BackupError> {
    let rows: i64 = sqlx::query_scalar(AssertSqlSafe(format!(
        "SELECT COUNT(*) FROM {table} WHERE {column} >= $1 AND {column} < $2"
    )))
    .bind(from)
    .bind(to)
    .fetch_one(pool)
    .await?;

    let from = from.format(&Rfc3339)?;
    let to = to.format(&Rfc3339)?;
    let statement = format!(
        "COPY (SELECT * FROM {table} WHERE {column} >= '{from}' AND {column} < '{to}' \
         ORDER BY {column}) TO STDOUT (FORMAT csv, HEADER true)"
    );
    let mut connection = pool.acquire().await?;
    let mut stream = connection.copy_out_raw(statement.as_str()).await?;
    let mut file = tokio::fs::File::create(csv_path).await?;
    while let Some(chunk) = stream.try_next().await? {
        file.write_all(&chunk).await?;
    }
    file.flush().await?;
    Ok(rows)
}

async fn file_sha256(path: &Path) -> Result<String, BackupError> {
    let path = path.to_path_buf();
    tokio::task::spawn_blocking(move || -> Result<String, BackupError> {
        use std::io::Read;
        let mut file = std::fs::File::open(&path)?;
        let mut hasher = Sha256::new();
        let mut buffer = [0u8; 64 * 1024];
        loop {
            let read = file.read(&mut buffer)?;
            if read == 0 {
                break;
            }
            hasher.update(&buffer[..read]);
        }
        Ok(hex::encode(hasher.finalize()))
    })
    .await
    .map_err(|error| BackupError::PgDump(format!("hash task failed: {error}")))?
}

/// Remove backup files older than `max_age`; returns how many were removed.
pub async fn sweep_expired(
    backups_dir: &Path,
    max_age: std::time::Duration,
) -> std::io::Result<usize> {
    let mut removed = 0;
    let mut entries = match tokio::fs::read_dir(backups_dir).await {
        Ok(entries) => entries,
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => return Ok(0),
        Err(error) => return Err(error),
    };
    while let Some(entry) = entries.next_entry().await? {
        let metadata = entry.metadata().await?;
        if !metadata.is_file() {
            continue;
        }
        let expired = metadata
            .modified()
            .ok()
            .and_then(|modified| modified.elapsed().ok())
            .is_some_and(|age| age > max_age);
        if expired && tokio::fs::remove_file(entry.path()).await.is_ok() {
            removed += 1;
        }
    }
    Ok(removed)
}

/// Sanitized backup file path for download serving: the name must be a
/// bare generated file name, never a path.
#[must_use]
pub fn download_path(backups_dir: &Path, name: &str) -> Option<PathBuf> {
    let valid = !name.is_empty()
        && name.starts_with("kaunta-")
        && name
            .chars()
            .all(|c| c.is_ascii_alphanumeric() || matches!(c, '.' | '-' | '_'))
        && !name.contains("..");
    if valid {
        Some(backups_dir.join(name))
    } else {
        None
    }
}

#[cfg(test)]
mod tests {
    use super::embedded_sha_prefix;
    use std::path::Path;

    #[test]
    fn sha_prefix_is_read_only_from_kaunta_named_backups() {
        assert_eq!(
            embedded_sha_prefix(Path::new(
                "/b/kaunta-full-20261001-201926.145343Z-e5d287218b9324af.dump"
            ))
            .as_deref(),
            Some("e5d287218b9324af")
        );
        for renamed in [
            "/b/kaunta-full-20261001-201926.145343Z-e5d287218b9324af-copy.dump",
            "/b/nightly.dump",
            "/b/kaunta-full-20261001-zzzzzzzzzzzzzzzz.dump",
        ] {
            assert!(
                embedded_sha_prefix(Path::new(renamed)).is_none(),
                "{renamed}"
            );
        }
    }
}
