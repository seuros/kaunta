use super::*;

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
async fn shared_websites_are_visible_to_every_user() {
    let pool = test_pool().await;
    let suffix = Uuid::new_v4();
    let owner = crate::db::auth::create_user(&pool, &format!("owner-{suffix}"), "pw", "")
        .await
        .unwrap();
    let other = crate::db::auth::create_user(&pool, &format!("other-{suffix}"), "pw", "")
        .await
        .unwrap();

    let shared_site = create(&pool, &format!("shared-{suffix}.example"), "", &[], None)
        .await
        .unwrap();
    let private_site = create(
        &pool,
        &format!("private-{suffix}.example"),
        "",
        &[],
        Some(owner.user_id),
    )
    .await
    .unwrap();

    assert!(
        is_accessible_by(&pool, shared_site.website_id, owner.user_id)
            .await
            .unwrap()
    );
    assert!(
        is_accessible_by(&pool, private_site.website_id, owner.user_id)
            .await
            .unwrap()
    );
    assert!(
        is_accessible_by(&pool, shared_site.website_id, other.user_id)
            .await
            .unwrap()
    );
    assert!(
        !is_accessible_by(&pool, private_site.website_id, other.user_id)
            .await
            .unwrap()
    );

    let ids = |sites: Vec<Website>| sites.into_iter().map(|w| w.website_id).collect::<Vec<_>>();
    let owner_sites = ids(list_for_user(&pool, owner.user_id).await.unwrap());
    assert!(owner_sites.contains(&shared_site.website_id));
    assert!(owner_sites.contains(&private_site.website_id));
    let other_sites = ids(list_for_user(&pool, other.user_id).await.unwrap());
    assert!(other_sites.contains(&shared_site.website_id));
    assert!(!other_sites.contains(&private_site.website_id));

    let (page, total) = list_page_for_user(&pool, other.user_id, 1000, 0)
        .await
        .unwrap();
    assert_eq!(page.len(), usize::try_from(total).unwrap());
    assert!(page.iter().any(|w| w.website_id == shared_site.website_id));
    assert!(page.iter().all(|w| w.website_id != private_site.website_id));

    assert!(exists_active(&pool, shared_site.website_id).await.unwrap());
    soft_delete(&pool, shared_site.website_id).await.unwrap();
    assert!(!exists_active(&pool, shared_site.website_id).await.unwrap());
    assert!(
        !is_accessible_by(&pool, shared_site.website_id, other.user_id)
            .await
            .unwrap()
    );
    assert!(!exists_active(&pool, Uuid::new_v4()).await.unwrap());
}
