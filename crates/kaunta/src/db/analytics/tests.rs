use super::*;
use crate::domain::goal::{GoalKind, GoalRequest};

#[tokio::test]
#[ignore = "requires KAUNTA_TEST_DATABASE_URL pointing to a disposable *_test database"]
async fn database_analytics_match_embedded_sql_functions() {
    let url = std::env::var("KAUNTA_TEST_DATABASE_URL").expect("test database URL");
    let pool = crate::db::connect(&url).await.unwrap();
    let database: String = sqlx::query_scalar("SELECT current_database()")
        .fetch_one(&pool)
        .await
        .unwrap();
    assert!(database.ends_with("_test"));
    crate::db::migrations::run(&pool).await.unwrap();
    let website = crate::db::websites::create(
        &pool,
        &format!("{}.example", Uuid::new_v4()),
        "Analytics test",
        &[],
        None,
    )
    .await
    .unwrap();
    let id = website.website_id;
    let goal = crate::db::goals::create(
        &pool,
        &GoalRequest {
            website_id: id,
            name: "Thanks".to_owned(),
            kind: GoalKind::PageView,
            value: "/thanks".to_owned(),
        },
    )
    .await
    .unwrap();
    let session = Uuid::new_v4();
    sqlx::query("INSERT INTO session (session_id, website_id, browser, country, device) VALUES ($1, $2, 'Firefox', 'MA', 'desktop')")
        .bind(session).bind(id).execute(&pool).await.unwrap();
    let event: Uuid = sqlx::query_scalar("INSERT INTO website_event (website_id, session_id, visit_id, url_path, goal_id, utm_source) VALUES ($1, $2, $2, '/thanks', $3, 'newsletter') RETURNING event_id")
        .bind(id).bind(session).bind(goal.id).fetch_one(&pool).await.unwrap();
    crate::db::goals::record_completion(&pool, goal.id, session, event, id)
        .await
        .unwrap();
    let filters = AnalyticsFilters::default();
    let stats = dashboard_stats(&pool, id, 7, filters).await.unwrap();
    assert_eq!(stats.today_pageviews, 1);
    assert_eq!(stats.today_visitors, 1);
    assert_eq!(
        timeseries(&pool, id, 7, filters)
            .await
            .unwrap()
            .iter()
            .map(|p| p.value)
            .sum::<i64>(),
        1
    );
    let (pages, total) = top_pages(&pool, id, 7, 10, 0, filters, "views", "desc")
        .await
        .unwrap();
    assert_eq!(total, 1);
    assert_eq!(pages[0].path, "/thanks");
    let (browsers, _) = breakdown(&pool, id, "browser", 7, 10, 0, filters, "count", "desc")
        .await
        .unwrap();
    assert_eq!(browsers[0].name, "Firefox");
    let map = map_data(&pool, id, 7, filters).await.unwrap();
    assert_eq!(map.total_visitors, 1);
    assert_eq!(map.data[0].country, "MA");
    assert_eq!(map.data[0].percentage, 100.0);
    assert_eq!(
        goal_analytics(&pool, goal.id, 7, filters)
            .await
            .unwrap()
            .completions,
        1
    );
    assert_eq!(
        goal_timeseries(&pool, goal.id, 7, filters)
            .await
            .unwrap()
            .iter()
            .map(|p| p.value)
            .sum::<i64>(),
        1
    );
    assert_eq!(
        goal_breakdown(&pool, goal.id, "browser", 7, 10, 0, filters)
            .await
            .unwrap()
            .0[0]
            .name,
        "Firefox"
    );
    assert!(
        goal_converting_pages(&pool, goal.id, 7, 10, 0, filters)
            .await
            .unwrap()
            .0
            .is_empty()
    );
    for statement in [
        "DELETE FROM website_event WHERE website_id = $1",
        "UPDATE website SET deleted_at = NOW() WHERE website_id = $1",
        "DELETE FROM website WHERE website_id = $1",
    ] {
        sqlx::query(statement)
            .bind(id)
            .execute(&pool)
            .await
            .unwrap();
    }
}
