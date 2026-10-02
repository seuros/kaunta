use super::*;
use time::macros::date;

#[test]
fn zero_days_disables_retention() {
    let policy = RetentionPolicy::from_days(0);
    assert_eq!(policy, RetentionPolicy::Disabled);
    assert!(!policy.is_enabled());
    assert_eq!(policy.cutoff(date!(2026 - 09 - 27)), None);
}

#[test]
fn positive_days_enable_retention_with_cutoff() {
    let policy = RetentionPolicy::from_days(90);
    assert_eq!(policy, RetentionPolicy::Days(90));
    assert!(policy.is_enabled());
    assert_eq!(
        policy.cutoff(date!(2026 - 09 - 27)),
        Some(date!(2026 - 06 - 29))
    );
    assert_eq!(
        RetentionPolicy::from_days(1).cutoff(date!(2026 - 03 - 01)),
        Some(date!(2026 - 02 - 28))
    );
}

#[test]
fn partition_suffix_is_zero_padded_and_sorts_lexically() {
    assert_eq!(partition_suffix(date!(2026 - 01 - 05)), "2026_01_05");
    assert_eq!(partition_suffix(date!(2026 - 12 - 31)), "2026_12_31");
    assert!(partition_suffix(date!(2026 - 09 - 30)) < partition_suffix(date!(2026 - 10 - 01)));
}

#[test]
fn cutoff_suffix_matches_partition_naming() {
    let cutoff = RetentionPolicy::from_days(30)
        .cutoff(date!(2026 - 09 - 27))
        .unwrap();
    assert_eq!(
        format!("website_event_{}", partition_suffix(cutoff)),
        "website_event_2026_08_28"
    );
}

async fn test_pool() -> PgPool {
    let url = std::env::var("KAUNTA_TEST_DATABASE_URL").expect("test database URL");
    let pool = crate::db::connect(&url).await.unwrap();
    let database: String = sqlx::query_scalar("SELECT current_database()")
        .fetch_one(&pool)
        .await
        .unwrap();
    assert!(database.ends_with("_test"));
    crate::db::migrations::run(&pool).await.unwrap();
    pool
}

#[tokio::test]
#[ignore = "requires KAUNTA_TEST_DATABASE_URL pointing to a disposable *_test database"]
async fn database_compacts_cold_months_and_keeps_retention_within_the_month() {
    let pool = test_pool().await;
    let cold = OffsetDateTime::now_utc().date() - TimeDuration::days(240);
    let month_start = cold.replace_day(1).unwrap();
    for day in 0..28 {
        let date = month_start + TimeDuration::days(day);
        create_partition(&pool, "website_event", "created_at", date)
            .await
            .unwrap();
    }

    let compacted = compact_old_partitions(&pool).await.unwrap();
    let monthly = format!(
        "website_event_{}",
        month_start
            .format(time::macros::format_description!("[year]_[month]"))
            .unwrap()
    );
    assert!(compacted.contains(&monthly), "{compacted:?}");

    let dailies: i64 = sqlx::query_scalar(
        "SELECT count(*) FROM pg_inherits i JOIN pg_class c ON c.oid = i.inhrelid
         WHERE i.inhparent = 'website_event'::regclass AND c.relkind = 'r'
           AND c.relname ~ ($1 || '_[0-9]{2}$')",
    )
    .bind(&monthly)
    .fetch_one(&pool)
    .await
    .unwrap();
    assert_eq!(dailies, 0, "daily partitions survived compaction");

    let mid_month = (month_start + TimeDuration::days(14))
        .midnight()
        .assume_utc();
    let selected: Vec<String> =
        sqlx::query_scalar("SELECT partition_name FROM kaunta_partitions_before($1::regclass, $2)")
            .bind("website_event")
            .bind(mid_month)
            .fetch_all(&pool)
            .await
            .unwrap();
    assert!(!selected.contains(&monthly), "{selected:?}");

    let after_month = (month_start + TimeDuration::days(31))
        .midnight()
        .assume_utc();
    let selected: Vec<String> =
        sqlx::query_scalar("SELECT partition_name FROM kaunta_partitions_before($1::regclass, $2)")
            .bind("website_event")
            .bind(after_month)
            .fetch_all(&pool)
            .await
            .unwrap();
    assert!(selected.contains(&monthly), "{selected:?}");

    sqlx::raw_sql(AssertSqlSafe(format!("DROP TABLE IF EXISTS {monthly}")))
        .execute(&pool)
        .await
        .unwrap();
}
