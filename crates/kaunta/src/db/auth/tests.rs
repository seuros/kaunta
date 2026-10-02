use super::hash_token;

#[test]
fn token_hash_matches_sha256_hex_contract() {
    assert_eq!(
        hash_token("kaunta"),
        "91d879639fd84a0d659d9008e212522e58ce14e5bacdb2246df3861521d00f10"
    );
}
