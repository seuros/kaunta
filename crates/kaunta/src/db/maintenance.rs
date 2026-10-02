use std::time::Duration;

use sqlx::{AssertSqlSafe, PgPool, Row};
use time::{Date, Duration as TimeDuration, OffsetDateTime, macros::format_description};

const PARTITION_DAYS_AHEAD: i64 = 30;
const IDEMPOTENCY_RETENTION_DAYS: i32 = 7;
const DAILY: Duration = Duration::from_hours(24);

/// Event retention policy derived from `KAUNTA_EVENT_RETENTION_DAYS`.
///
/// `0` disables retention: `website_event_*` and `bot_detection_log_*`
/// partitions are kept indefinitely.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum RetentionPolicy {
    Disabled,
    Days(u32),
}

impl RetentionPolicy {
    #[must_use]
    pub const fn from_days(retention_days: u32) -> Self {
        if retention_days == 0 {
            Self::Disabled
        } else {
            Self::Days(retention_days)
        }
    }

    #[must_use]
    pub const fn is_enabled(self) -> bool {
        matches!(self, Self::Days(_))
    }

    /// Date before which daily partitions may be dropped, or `None` when
    /// retention is disabled.
    #[must_use]
    pub fn cutoff(self, today: Date) -> Option<Date> {
        match self {
            Self::Disabled => None,
            Self::Days(days) => Some(today - TimeDuration::days(i64::from(days))),
        }
    }
}

/// Suffix used for daily partitions, e.g. `2026_09_27`.
#[must_use]
pub fn partition_suffix(date: Date) -> String {
    date.format(format_description!("[year]_[month]_[day]"))
        .expect("static date format is valid")
}

pub async fn create_future_partitions(pool: &PgPool) -> Result<u64, sqlx::Error> {
    let today = OffsetDateTime::now_utc().date();
    let mut created = 0;
    for day in 0..=PARTITION_DAYS_AHEAD {
        created += create_partition(
            pool,
            "website_event",
            "created_at",
            today + TimeDuration::days(day),
        )
        .await?;
        created += create_partition(
            pool,
            "bot_detection_log",
            "detected_at",
            today + TimeDuration::days(day),
        )
        .await?;
    }
    let idempotency =
        crate::db::api_keys::create_idempotency_partitions(pool, IDEMPOTENCY_RETENTION_DAYS)
            .await?;
    Ok(created + u64::try_from(idempotency).unwrap_or_default())
}

async fn create_partition(
    pool: &PgPool,
    parent: &str,
    _timestamp_column: &str,
    date: Date,
) -> Result<u64, sqlx::Error> {
    let suffix = partition_suffix(date);
    let start = date
        .format(format_description!("[year]-[month]-[day]"))
        .expect("static date format is valid");
    let end = (date + TimeDuration::days(1))
        .format(format_description!("[year]-[month]-[day]"))
        .expect("static date format is valid");
    let partition = format!("{parent}_{suffix}");
    let query = format!(
        "CREATE TABLE IF NOT EXISTS {partition}
         PARTITION OF {parent}
         FOR VALUES FROM ('{start}') TO ('{end}')"
    );
    sqlx::raw_sql(AssertSqlSafe(query))
        .execute(pool)
        .await
        .map(|result| result.rows_affected())
}

/// Daily partitions older than this are folded into one partition per month.
/// Kaunta writes a table per day, which is right for recent data (cheap
/// pruning, cheap retention drops) and wasteful for cold months.
const COMPACT_AFTER_DAYS: i32 = 90;
/// Months compacted per run, so a long-neglected database catches up over
/// several days instead of holding locks for one very long maintenance pass.
const COMPACT_MONTHS_PER_RUN: usize = 12;

/// Folds each fully-cold month of daily partitions into a single monthly
/// partition, oldest first. Returns the names of the partitions created.
pub async fn compact_old_partitions(pool: &PgPool) -> Result<Vec<String>, sqlx::Error> {
    let mut compacted = Vec::new();
    for parent in ["website_event", "bot_detection_log"] {
        for _ in 0..COMPACT_MONTHS_PER_RUN {
            let created: Option<String> =
                sqlx::query_scalar("SELECT kaunta_compact_next_partition_month($1::regclass, $2)")
                    .bind(parent)
                    .bind(COMPACT_AFTER_DAYS)
                    .fetch_one(pool)
                    .await?;
            match created {
                Some(name) => compacted.push(name),
                None => break,
            }
        }
    }
    Ok(compacted)
}

/// Drops `website_event` and `bot_detection_log` partitions that end at or
/// before the retention cutoff. Returns `Ok(0)` without touching the database
/// when retention is disabled.
pub async fn cleanup_old_partitions(
    pool: &PgPool,
    policy: RetentionPolicy,
) -> Result<u64, sqlx::Error> {
    let Some(cutoff) = policy.cutoff(OffsetDateTime::now_utc().date()) else {
        return Ok(0);
    };
    let cutoff = cutoff.midnight().assume_utc();
    let mut dropped = drop_partitions_before(pool, "website_event", cutoff).await?;
    dropped += drop_partitions_before(pool, "bot_detection_log", cutoff).await?;
    Ok(dropped)
}

/// Removes expired idempotency partitions (independent of event retention).
pub async fn cleanup_idempotency(pool: &PgPool) -> Result<u64, sqlx::Error> {
    let removed =
        crate::db::api_keys::cleanup_idempotency(pool, IDEMPOTENCY_RETENTION_DAYS).await?;
    Ok(u64::try_from(removed).unwrap_or_default())
}

/// Selects by partition bound rather than by name: once cold months are
/// compacted, `website_event_2026_04` sorts before `website_event_2026_04_15`
/// and a name comparison would drop the rest of that month with it.
async fn drop_partitions_before(
    pool: &PgPool,
    parent: &str,
    cutoff: OffsetDateTime,
) -> Result<u64, sqlx::Error> {
    let rows = sqlx::query(
        "SELECT partition_name AS tablename
         FROM kaunta_partitions_before($1::regclass, $2)",
    )
    .bind(parent)
    .bind(cutoff)
    .fetch_all(pool)
    .await?;

    let mut dropped = 0;
    for row in rows {
        let table: String = row.try_get("tablename")?;
        if table
            .chars()
            .all(|character| character.is_ascii_alphanumeric() || character == '_')
        {
            sqlx::raw_sql(AssertSqlSafe(format!("DROP TABLE IF EXISTS {table}")))
                .execute(pool)
                .await?;
            dropped += 1;
        }
    }
    Ok(dropped)
}

pub async fn refresh_materialized_view(pool: &PgPool, view: &str) -> Result<(), sqlx::Error> {
    if !matches!(
        view,
        "realtime_website_stats"
            | "hourly_website_stats"
            | "daily_website_stats"
            | "bot_stats_by_country"
    ) {
        return Err(sqlx::Error::Protocol(format!(
            "unsupported materialized view: {view}"
        )));
    }
    sqlx::raw_sql(AssertSqlSafe(format!(
        "REFRESH MATERIALIZED VIEW CONCURRENTLY {view}"
    )))
    .execute(pool)
    .await?;
    Ok(())
}

/// Spawns the background maintenance tasks.
///
/// Partition creation and expired session / rate-limit / idempotency cleanup
/// run immediately and then daily. Old event partitions are only dropped when
/// `retention_days > 0`; that job first runs one interval after startup.
#[must_use]
pub fn spawn(pool: PgPool, retention_days: u32) -> Vec<tokio::task::JoinHandle<()>> {
    let policy = RetentionPolicy::from_days(retention_days);
    let mut tasks = vec![
        spawn_job(
            pool.clone(),
            Duration::ZERO,
            DAILY,
            || "partition maintenance",
            |pool| async move {
                create_future_partitions(&pool).await?;
                let compacted = compact_old_partitions(&pool).await?;
                if !compacted.is_empty() {
                    tracing::info!(
                        months = compacted.len(),
                        partitions = ?compacted,
                        "compacted cold daily partitions into monthly partitions"
                    );
                }
                crate::db::auth::cleanup_expired_sessions(&pool).await?;
                crate::db::api_keys::cleanup_rate_limit_storage(&pool).await?;
                cleanup_idempotency(&pool).await?;
                Ok::<_, sqlx::Error>(())
            },
        ),
        spawn_refresh(
            pool.clone(),
            "realtime_website_stats",
            Duration::from_mins(1),
        ),
        spawn_refresh(pool.clone(), "hourly_website_stats", Duration::from_mins(5)),
        spawn_refresh(pool.clone(), "daily_website_stats", Duration::from_hours(1)),
        spawn_refresh(
            pool.clone(),
            "bot_stats_by_country",
            Duration::from_hours(1),
        ),
    ];

    if policy.is_enabled() {
        tracing::info!(
            retention_days,
            "event retention enabled; old partitions are dropped daily"
        );
        tasks.push(spawn_job(
            pool,
            DAILY,
            DAILY,
            || "event retention",
            move |pool| async move {
                let dropped = cleanup_old_partitions(&pool, policy).await?;
                if dropped > 0 {
                    tracing::info!(dropped, "dropped expired event partitions");
                }
                Ok(())
            },
        ));
    } else {
        tracing::info!("event retention disabled; partitions are kept indefinitely");
    }

    tasks
}

fn spawn_refresh(
    pool: PgPool,
    view: &'static str,
    interval: Duration,
) -> tokio::task::JoinHandle<()> {
    spawn_job(
        pool,
        Duration::ZERO,
        interval,
        move || view,
        move |pool| async move { refresh_materialized_view(&pool, view).await },
    )
}

fn spawn_job<F, Fut, Name>(
    pool: PgPool,
    initial_delay: Duration,
    interval: Duration,
    name: Name,
    job: F,
) -> tokio::task::JoinHandle<()>
where
    F: Fn(PgPool) -> Fut + Send + Sync + 'static,
    Fut: Future<Output = Result<(), sqlx::Error>> + Send + 'static,
    Name: Fn() -> &'static str + Send + Sync + 'static,
{
    tokio::spawn(async move {
        if !initial_delay.is_zero() {
            tokio::time::sleep(initial_delay).await;
        }
        loop {
            if let Err(error) = job(pool.clone()).await {
                tracing::warn!(
                    ?error,
                    job = name(),
                    "scheduled database maintenance failed"
                );
            }
            tokio::time::sleep(interval).await;
        }
    })
}

#[cfg(test)]
mod tests;
