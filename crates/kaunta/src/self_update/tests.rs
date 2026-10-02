use super::*;
use flate2::{Compression, write::GzEncoder};

#[test]
fn parses_release_versions() {
    assert_eq!(
        parse_version("v1.2.3").expect("parse version"),
        Version::new(1, 2, 3)
    );
}

#[test]
fn selects_primary_and_legacy_assets() {
    let release = Release {
        version: Version::new(1, 2, 3),
        assets: vec![
            Asset {
                name: "ore_linux_amd64.tar.gz".to_owned(),
                browser_download_url: "legacy".to_owned(),
            },
            Asset {
                name: "kaunta_linux_amd64.tar.gz".to_owned(),
                browser_download_url: "primary".to_owned(),
            },
        ],
    };

    assert_eq!(
        release
            .find_asset("linux", "amd64")
            .expect("find primary asset")
            .browser_download_url,
        "primary"
    );
}

#[test]
fn extracts_kaunta_binary_from_archive() {
    let mut encoder = GzEncoder::new(Vec::new(), Compression::default());
    {
        let mut builder = tar::Builder::new(&mut encoder);
        let payload = b"kaunta-test-binary";
        let mut header = tar::Header::new_gnu();
        header
            .set_path("release/kaunta-linux-amd64")
            .expect("set path");
        header.set_size(payload.len() as u64);
        header.set_mode(0o755);
        header.set_cksum();
        builder
            .append(&header, payload.as_slice())
            .expect("append binary");
        builder.finish().expect("finish archive");
    }
    let archive = encoder.finish().expect("finish gzip");

    assert_eq!(
        extract_binary(&archive, "kaunta").expect("extract binary"),
        b"kaunta-test-binary"
    );
}

#[cfg(unix)]
#[test]
fn replaces_binary_atomically_and_preserves_mode() {
    use std::os::unix::fs::PermissionsExt;

    let directory = env::temp_dir().join(format!(
        "kaunta-self-update-test-{}-{}",
        std::process::id(),
        SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .expect("clock after epoch")
            .as_nanos()
    ));
    fs::create_dir_all(&directory).expect("create test directory");
    let target = directory.join("kaunta");
    fs::write(&target, b"old").expect("write old binary");
    fs::set_permissions(&target, fs::Permissions::from_mode(0o751)).expect("set binary mode");

    replace_binary(&target, b"new").expect("replace binary");

    assert_eq!(fs::read(&target).expect("read new binary"), b"new");
    assert_eq!(
        fs::metadata(&target)
            .expect("read new metadata")
            .permissions()
            .mode()
            & 0o777,
        0o751
    );
    fs::remove_dir_all(directory).expect("remove test directory");
}

fn asset(name: &str) -> Asset {
    Asset {
        name: name.to_owned(),
        browser_download_url: format!("https://example.invalid/{name}"),
    }
}

fn checksum_line(archive: &[u8], name: &str) -> String {
    format!("{}  {name}\n", hex::encode(Sha256::digest(archive)))
}

#[test]
fn finds_checksum_asset_next_to_archive() {
    let release = Release {
        version: Version::new(1, 2, 3),
        assets: vec![
            asset("kaunta_linux_amd64.tar.gz"),
            asset("kaunta_linux_amd64.tar.gz.sha256"),
        ],
    };
    let archive = release.find_asset("linux", "amd64").expect("find asset");
    assert_eq!(
        release
            .find_checksum_asset(archive)
            .expect("find checksum")
            .name,
        "kaunta_linux_amd64.tar.gz.sha256"
    );
}

#[test]
fn refuses_release_without_checksum_asset() {
    let release = Release {
        version: Version::new(1, 2, 3),
        assets: vec![asset("kaunta_linux_amd64.tar.gz")],
    };
    let archive = release.find_asset("linux", "amd64").expect("find asset");
    let error = release
        .find_checksum_asset(archive)
        .expect_err("missing checksum must be refused")
        .to_string();
    assert!(
        error.contains("kaunta_linux_amd64.tar.gz.sha256"),
        "{error}"
    );
    assert!(error.contains("refusing"), "{error}");
}

#[test]
fn accepts_matching_checksum() {
    let archive = b"release-bytes";
    let name = "kaunta_linux_amd64.tar.gz";
    verify_checksum(archive, &checksum_line(archive, name), name).expect("matching checksum");
    let digest_only = hex::encode(Sha256::digest(archive));
    verify_checksum(archive, &digest_only, name).expect("digest-only checksum");
    let starred = format!("{digest_only} *{name}");
    verify_checksum(archive, &starred, name).expect("starred checksum");
}

#[test]
fn rejects_mismatched_checksum() {
    let name = "kaunta_linux_amd64.tar.gz";
    let error = verify_checksum(b"tampered", &checksum_line(b"original", name), name)
        .expect_err("mismatch must fail")
        .to_string();
    assert!(error.contains("SHA-256 mismatch"), "{error}");
}

#[test]
fn rejects_checksum_for_a_different_file_or_malformed_digest() {
    let archive = b"release-bytes";
    let name = "kaunta_linux_amd64.tar.gz";
    assert!(verify_checksum(archive, &checksum_line(archive, "other.tar.gz"), name).is_err());
    assert!(verify_checksum(archive, "not-hex  kaunta_linux_amd64.tar.gz", name).is_err());
    assert!(verify_checksum(archive, "abcd  kaunta_linux_amd64.tar.gz", name).is_err());
    assert!(verify_checksum(archive, "\n  \n", name).is_err());
}

#[cfg(unix)]
#[test]
fn install_archive_writes_nothing_when_checksum_mismatches() {
    let directory = env::temp_dir().join(format!(
        "kaunta-self-update-checksum-{}-{}",
        std::process::id(),
        SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .expect("clock after epoch")
            .as_nanos()
    ));
    fs::create_dir_all(&directory).expect("create test directory");
    let target = directory.join("kaunta");
    fs::write(&target, b"old").expect("write old binary");

    let error = install_archive(b"new", "00  kaunta", "kaunta", &target)
        .expect_err("mismatch must fail")
        .to_string();
    assert!(error.contains("checksum verification failed"), "{error}");
    assert_eq!(fs::read(&target).expect("read binary"), b"old");

    install_archive(b"new", &checksum_line(b"new", "kaunta"), "kaunta", &target)
        .expect("install verified raw binary");
    assert_eq!(fs::read(&target).expect("read binary"), b"new");
    fs::remove_dir_all(directory).expect("remove test directory");
}
