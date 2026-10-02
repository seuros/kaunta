use super::*;

#[test]
fn reads_json_and_yaml_with_default_names() {
    for (filename, data) in [
        (
            "websites.json",
            r#"{"websites":[{"domain":"example.com"}]}"#,
        ),
        ("websites.yaml", "websites:\n  - domain: example.com\n"),
    ] {
        let import = parse(Path::new(filename), data.as_bytes()).unwrap();
        assert_eq!(import.websites[0].name, "example.com");
    }
}

#[test]
fn rejects_empty_invalid_and_duplicate_imports_before_writes() {
    for json in [
        r#"{"websites":[]}"#,
        r#"{"websites":[{"domain":"https://bad.example"}]}"#,
        r#"{"websites":[{"domain":"example.com"},{"domain":"EXAMPLE.com"}]}"#,
    ] {
        assert!(parse(Path::new("websites.json"), json.as_bytes()).is_err());
    }
    assert!(parse(Path::new("websites.txt"), b"{}").is_err());
}

#[tokio::test]
#[ignore = "requires KAUNTA_TEST_DATABASE_URL pointing to a disposable *_test database"]
async fn database_sync_preserves_ids_and_dry_run_is_read_only() {
    let url = std::env::var("KAUNTA_TEST_DATABASE_URL").expect("test database URL");
    assert!(url::Url::parse(&url).unwrap().path().ends_with("_test"));
    let pool = kaunta::db::connect(&url).await.unwrap();
    kaunta::db::migrations::run(&pool).await.unwrap();
    kaunta::db::startup::synchronize(&pool, &["localhost".to_owned()])
        .await
        .unwrap();
    let directory = tempfile::tempdir().unwrap();
    let path = directory.path().join("websites.json");
    let domain = format!("{}.example", Uuid::new_v4());
    std::fs::write(
        &path,
        format!(r#"{{"websites":[{{"domain":"{domain}","name":"Before"}}]}}"#),
    )
    .unwrap();
    run(&pool, &path, true, false).await.unwrap();
    let count: i64 = sqlx::query_scalar("SELECT COUNT(*) FROM website WHERE domain = $1")
        .bind(&domain)
        .fetch_one(&pool)
        .await
        .unwrap();
    assert_eq!(count, 0);
    run(&pool, &path, false, false).await.unwrap();
    let original: (Uuid, String) =
        sqlx::query_as("SELECT website_id, name FROM website WHERE domain = $1")
            .bind(&domain)
            .fetch_one(&pool)
            .await
            .unwrap();
    std::fs::write(
        &path,
        format!(r#"{{"websites":[{{"domain":"{domain}","name":"After"}}]}}"#),
    )
    .unwrap();
    let before: i64 = sqlx::query_scalar("SELECT COUNT(*) FROM website WHERE deleted_at IS NULL")
        .fetch_one(&pool)
        .await
        .unwrap();
    run(&pool, &path, true, true).await.unwrap();
    let after_dry_run: (Uuid, String) =
        sqlx::query_as("SELECT website_id, name FROM website WHERE domain = $1")
            .bind(&domain)
            .fetch_one(&pool)
            .await
            .unwrap();
    assert_eq!(original, after_dry_run);
    let after: i64 = sqlx::query_scalar("SELECT COUNT(*) FROM website WHERE deleted_at IS NULL")
        .fetch_one(&pool)
        .await
        .unwrap();
    assert_eq!(before, after);
    run(&pool, &path, false, true).await.unwrap();
    let updated: (Uuid, String) =
        sqlx::query_as("SELECT website_id, name FROM website WHERE domain = $1")
            .bind(&domain)
            .fetch_one(&pool)
            .await
            .unwrap();
    assert_eq!(updated, (original.0, "After".to_owned()));
    let remaining: i64 =
        sqlx::query_scalar("SELECT COUNT(*) FROM website WHERE deleted_at IS NULL")
            .fetch_one(&pool)
            .await
            .unwrap();
    assert_eq!(remaining, 2); // imported website + reserved self-tracking website
}
