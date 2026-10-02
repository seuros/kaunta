use super::*;

#[test]
fn embedded_migrations_are_contiguous() {
    let migrations = load_migrations().expect("load migrations");
    assert_eq!(migrations.len(), 34);
    for (index, migration) in migrations.iter().enumerate() {
        assert_eq!(migration.version, i64::try_from(index + 1).unwrap());
    }
    assert_eq!(latest_version().unwrap(), 34);
}

#[test]
fn advisory_lock_id_matches_golang_migrate() {
    assert_eq!(advisory_lock_id("public", "kaunta"), 1_861_445_312);
}

#[test]
fn advisory_lock_id_is_never_negative() {
    for (schema, database) in [
        ("public", "kaunta"),
        ("app", "x"),
        ("public", "kaunta_test"),
    ] {
        let id = advisory_lock_id(schema, database);
        assert!((0..=i64::from(u32::MAX)).contains(&id));
    }
}
