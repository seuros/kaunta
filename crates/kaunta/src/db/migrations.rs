use include_dir::{Dir, include_dir};
use sqlx::{Acquire, PgPool, Postgres, Row, pool::PoolConnection};
use thiserror::Error;

static MIGRATIONS: Dir<'_> = include_dir!("$CARGO_MANIFEST_DIR/../../migrations");
const MIGRATIONS_TABLE: &str = "schema_migrations";
/// Salt used by golang-migrate's `database.GenerateAdvisoryLockId`.
const ADVISORY_LOCK_SALT: u32 = 1_486_364_155;

#[derive(Debug, Error)]
pub enum MigrationError {
    #[error("database error: {0}")]
    Database(#[from] sqlx::Error),
    #[error("database migration state is dirty at version {0}")]
    Dirty(i64),
    #[error("invalid migration filename {0}")]
    InvalidFilename(String),
    #[error("migration {version} is not valid UTF-8: {name}")]
    InvalidUtf8 { version: i64, name: String },
    #[error("database migration version {database} is newer than this binary ({binary})")]
    NewerDatabase { database: i64, binary: i64 },
}

#[derive(Debug, Clone, Copy)]
struct Migration {
    version: i64,
    name: &'static str,
    sql: &'static str,
}

pub async fn current_version(pool: &PgPool) -> Result<Option<(i64, bool)>, MigrationError> {
    let mut connection = pool.acquire().await?;
    current_version_on(&mut connection).await
}

async fn current_version_on(
    connection: &mut PoolConnection<Postgres>,
) -> Result<Option<(i64, bool)>, MigrationError> {
    let exists: bool = sqlx::query_scalar(
        "SELECT EXISTS (
            SELECT 1
            FROM information_schema.tables
            WHERE table_schema = 'public' AND table_name = 'schema_migrations'
        )",
    )
    .fetch_one(&mut **connection)
    .await?;

    if !exists {
        return Ok(None);
    }

    let row = sqlx::query("SELECT version, dirty FROM schema_migrations LIMIT 1")
        .fetch_optional(&mut **connection)
        .await?;

    row.map(|row| Ok((row.try_get("version")?, row.try_get("dirty")?)))
        .transpose()
}

pub async fn verify_clean(pool: &PgPool) -> Result<Option<i64>, MigrationError> {
    match current_version(pool).await? {
        Some((version, true)) => Err(MigrationError::Dirty(version)),
        Some((version, false)) => Ok(Some(version)),
        None => Ok(None),
    }
}

pub fn latest_version() -> Result<i64, MigrationError> {
    Ok(load_migrations()?.last().map_or(0, |item| item.version))
}

pub async fn run(pool: &PgPool) -> Result<i64, MigrationError> {
    let migrations = load_migrations()?;
    let latest = migrations.last().map_or(0, |item| item.version);
    let mut connection = pool.acquire().await?;
    let lock_id = advisory_lock_id_on(&mut connection).await?;

    sqlx::query("SELECT pg_advisory_lock($1)")
        .bind(lock_id)
        .execute(&mut *connection)
        .await?;

    let result = run_locked(&mut connection, &migrations, latest).await;
    if let Err(error) = sqlx::query("SELECT pg_advisory_unlock($1)")
        .bind(lock_id)
        .execute(&mut *connection)
        .await
    {
        tracing::warn!(?error, "failed to release migration advisory lock");
    }
    result
}

async fn run_locked(
    connection: &mut PoolConnection<Postgres>,
    migrations: &[Migration],
    latest: i64,
) -> Result<i64, MigrationError> {
    ensure_version_table(connection).await?;
    let current = match current_version_on(connection).await? {
        Some((version, true)) => return Err(MigrationError::Dirty(version)),
        Some((version, false)) => version,
        None => 0,
    };

    if current > latest {
        return Err(MigrationError::NewerDatabase {
            database: current,
            binary: latest,
        });
    }

    let mut applied = current;
    for migration in migrations.iter().filter(|item| item.version > current) {
        tracing::info!(
            version = migration.version,
            migration = migration.name,
            "applying database migration"
        );
        set_version(connection, migration.version, true).await?;

        if let Err(error) = sqlx::raw_sql(migration.sql)
            .execute(&mut **connection)
            .await
        {
            return Err(MigrationError::Database(error));
        }

        set_version(connection, migration.version, false).await?;
        applied = migration.version;
    }

    Ok(applied)
}

/// Computes the same advisory lock key golang-migrate (v4) uses for its
/// postgres driver, so a Go and a Rust process never migrate concurrently.
///
/// golang-migrate joins `[current_schema, "schema_migrations", url.Path]` with
/// NUL bytes (the URL path keeps its leading slash), takes the CRC32 IEEE of
/// that string, multiplies by a fixed salt with wrapping `uint32` arithmetic
/// and widens the result to `bigint`.
async fn advisory_lock_id_on(
    connection: &mut PoolConnection<Postgres>,
) -> Result<i64, MigrationError> {
    let (schema, database): (String, String) =
        sqlx::query_as("SELECT current_schema(), current_database()")
            .fetch_one(&mut **connection)
            .await?;
    Ok(advisory_lock_id(&schema, &database))
}

#[must_use]
fn advisory_lock_id(schema: &str, database: &str) -> i64 {
    let name = format!("{schema}\0{MIGRATIONS_TABLE}\0/{database}");
    let id = crc32fast::hash(name.as_bytes()).wrapping_mul(ADVISORY_LOCK_SALT);
    i64::from(id)
}

/// Creates the version table with golang-migrate's exact shape so either
/// implementation can pick up where the other left off.
async fn ensure_version_table(
    connection: &mut PoolConnection<Postgres>,
) -> Result<(), MigrationError> {
    sqlx::query(
        "CREATE TABLE IF NOT EXISTS schema_migrations (
            version bigint not null primary key,
            dirty boolean not null
        )",
    )
    .execute(&mut **connection)
    .await?;
    Ok(())
}

async fn set_version(
    connection: &mut PoolConnection<Postgres>,
    version: i64,
    dirty: bool,
) -> Result<(), MigrationError> {
    let mut transaction = connection.begin().await?;
    sqlx::query("TRUNCATE schema_migrations")
        .execute(&mut *transaction)
        .await?;
    sqlx::query("INSERT INTO schema_migrations (version, dirty) VALUES ($1, $2)")
        .bind(version)
        .bind(dirty)
        .execute(&mut *transaction)
        .await?;
    transaction.commit().await?;
    Ok(())
}

fn load_migrations() -> Result<Vec<Migration>, MigrationError> {
    let mut migrations = Vec::new();
    for file in MIGRATIONS.files() {
        let Some(name) = file.path().file_name().and_then(|name| name.to_str()) else {
            continue;
        };
        if !name.ends_with(".up.sql") {
            continue;
        }
        let Some((version, _)) = name.split_once('_') else {
            return Err(MigrationError::InvalidFilename(name.to_owned()));
        };
        let version = version
            .parse::<i64>()
            .map_err(|_| MigrationError::InvalidFilename(name.to_owned()))?;
        let sql = file
            .contents_utf8()
            .ok_or_else(|| MigrationError::InvalidUtf8 {
                version,
                name: name.to_owned(),
            })?;
        migrations.push(Migration { version, name, sql });
    }
    migrations.sort_unstable_by_key(|item| item.version);
    Ok(migrations)
}

#[cfg(test)]
mod tests;
