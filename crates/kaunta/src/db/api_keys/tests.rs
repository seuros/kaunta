use super::hash_api_key;

#[test]
fn hashes_full_key_with_sha256() {
    assert_eq!(
        hash_api_key("kaunta_live_test"),
        "9f8fb3b7e5a73796593fcd33b686f14f543ad530f89974a8ae990cf5ad73eaf4"
    );
}
