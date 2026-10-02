use std::{
    env, fs,
    io::{self, Cursor, Read, Write},
    path::{Path, PathBuf},
    time::{Duration, SystemTime, UNIX_EPOCH},
};

use anyhow::{Context, bail};
use flate2::read::GzDecoder;
use semver::Version;
use serde::Deserialize;
use sha2::{Digest, Sha256};

const LATEST_RELEASE_URL: &str = "https://api.github.com/repos/seuros/kaunta/releases/latest";
const USER_AGENT: &str = "kaunta-selfupdate";

#[derive(Clone, Debug, Deserialize)]
struct Asset {
    name: String,
    browser_download_url: String,
}

#[derive(Debug, Deserialize)]
struct GithubRelease {
    tag_name: String,
    assets: Vec<Asset>,
}

#[derive(Debug)]
struct Release {
    version: Version,
    assets: Vec<Asset>,
}

impl Release {
    fn find_asset(&self, os: &str, arch: &str) -> anyhow::Result<&Asset> {
        let expected = format!("kaunta_{os}_{arch}.tar.gz");
        let alternatives = [
            expected.clone(),
            format!("kaunta-{os}-{arch}.tar.gz"),
            format!("kaunta_v{}_{os}_{arch}.tar.gz", self.version),
            format!("ore_{os}_{arch}.tar.gz"),
            format!("ore-{os}-{arch}.tar.gz"),
            format!("ore_v{}_{os}_{arch}.tar.gz", self.version),
        ];

        alternatives
            .iter()
            .find_map(|candidate| self.assets.iter().find(|asset| asset.name == *candidate))
            .with_context(|| format!("no asset found for {os}/{arch} (expected {expected})"))
    }

    /// Locate the `<asset>.sha256` companion published by the release workflow.
    ///
    /// Releases without a checksum are refused: a self-upgrade replaces the
    /// running executable, so an unverifiable download is not installed.
    fn find_checksum_asset(&self, asset: &Asset) -> anyhow::Result<&Asset> {
        let expected = format!("{}.sha256", asset.name);
        self.assets
            .iter()
            .find(|candidate| candidate.name == expected)
            .with_context(|| {
                format!(
                    "release v{} has no checksum asset {expected}; refusing to install an unverified binary",
                    self.version
                )
            })
    }
}

pub async fn run(current_version: &str, check_only: bool, auto_yes: bool) -> anyhow::Result<()> {
    let current = parse_version(current_version)
        .with_context(|| format!("invalid current version {current_version:?}"))?;

    println!("Checking current version... v{current}");
    print!("Checking latest released version... ");
    io::stdout().flush().context("flush version check output")?;

    let latest = match detect_latest().await {
        Ok(release) => release,
        Err(error) => {
            println!();
            return Err(error).context("failed to check for updates");
        }
    };
    println!("v{}", latest.version);

    if latest.version <= current {
        println!("Kaunta is already up to date");
        return Ok(());
    }

    println!("New release found! v{} --> v{}", current, latest.version);
    if check_only {
        return Ok(());
    }

    let (os, arch) = release_platform()?;
    let executable = env::current_exe().context("determine executable path")?;
    let executable = fs::canonicalize(&executable).unwrap_or(executable);
    let asset = latest.find_asset(os, arch)?;
    let checksum_asset = latest.find_checksum_asset(asset)?;

    println!();
    println!("Kaunta release status:");
    println!("  * Current exe: {}", executable.display());
    println!("  * Target OS/Arch: {os}/{arch}");
    println!("  * Download URL: {}", asset.browser_download_url);
    println!("  * Checksum URL: {}", checksum_asset.browser_download_url);
    println!();

    if !auto_yes {
        println!("The new release will download and replace the current binary.");
        print!("Do you want to continue? [Y/n] ");
        io::stdout().flush().context("flush upgrade prompt")?;

        let mut response = String::new();
        io::stdin()
            .read_line(&mut response)
            .context("read upgrade confirmation")?;
        let response = response.trim().to_ascii_lowercase();
        if !response.is_empty() && response != "y" && response != "yes" {
            println!("Update cancelled.");
            return Ok(());
        }
    }

    println!("Downloading release...");
    update_to(asset, checksum_asset, &executable)
        .await
        .context("self-upgrade failed")?;
    println!("Updated Kaunta to v{}", latest.version);
    Ok(())
}

fn parse_version(value: &str) -> anyhow::Result<Version> {
    let value = value.trim().strip_prefix('v').unwrap_or(value.trim());
    if value.is_empty() {
        bail!("self-upgrade is only available for release builds");
    }
    Version::parse(value).context("parse semantic version")
}

async fn detect_latest() -> anyhow::Result<Release> {
    let client = reqwest::Client::builder()
        .timeout(Duration::from_secs(30))
        .user_agent(USER_AGENT)
        .build()
        .context("create GitHub client")?;
    let response = client
        .get(LATEST_RELEASE_URL)
        .header("Accept", "application/vnd.github+json")
        .header("X-GitHub-Api-Version", "2022-11-28")
        .send()
        .await
        .context("fetch latest GitHub release")?;

    if response.status() == reqwest::StatusCode::NOT_FOUND {
        bail!("no releases found for seuros/kaunta");
    }
    let response = response
        .error_for_status()
        .context("GitHub releases API returned an error")?;
    let release = response
        .json::<GithubRelease>()
        .await
        .context("decode GitHub release")?;
    let version = parse_version(&release.tag_name)
        .with_context(|| format!("invalid version tag {:?}", release.tag_name))?;

    Ok(Release {
        version,
        assets: release.assets,
    })
}

fn release_platform() -> anyhow::Result<(&'static str, &'static str)> {
    let os = match env::consts::OS {
        "linux" => "linux",
        "macos" => "darwin",
        "freebsd" => "freebsd",
        unsupported => bail!("self-upgrade is not supported on {unsupported}"),
    };
    let arch = match env::consts::ARCH {
        "x86_64" => "amd64",
        "aarch64" => "arm64",
        unsupported => bail!("self-upgrade is not supported on architecture {unsupported}"),
    };
    Ok((os, arch))
}

async fn update_to(asset: &Asset, checksum_asset: &Asset, target: &Path) -> anyhow::Result<()> {
    let client = reqwest::Client::builder()
        .timeout(Duration::from_mins(5))
        .user_agent(USER_AGENT)
        .build()
        .context("create download client")?;
    let archive = download(&client, &asset.browser_download_url)
        .await
        .context("download release")?;
    let checksum = download(&client, &checksum_asset.browser_download_url)
        .await
        .context("download release checksum")?;
    let checksum = String::from_utf8(checksum).context("release checksum is not UTF-8")?;

    install_archive(&archive, &checksum, &asset.name, target)
}

async fn download(client: &reqwest::Client, url: &str) -> anyhow::Result<Vec<u8>> {
    let response = client
        .get(url)
        .send()
        .await
        .context("send download request")?
        .error_for_status()
        .context("download returned an error")?;
    Ok(response
        .bytes()
        .await
        .context("read download body")?
        .to_vec())
}

/// Verify the archive against its published checksum, then install the
/// binary it contains. Nothing is written to disk before verification passes.
fn install_archive(
    archive: &[u8],
    checksum: &str,
    asset_name: &str,
    target: &Path,
) -> anyhow::Result<()> {
    verify_checksum(archive, checksum, asset_name)
        .context("release checksum verification failed")?;
    let binary = if looks_like_gzip(archive) || asset_name.to_ascii_lowercase().ends_with(".tar.gz")
    {
        extract_binary(archive, "kaunta").context("extract Kaunta from release archive")?
    } else {
        archive.to_vec()
    };
    replace_binary(target, &binary)
}

/// Parse a `sha256sum`-style line (`<hex>  <file>`) and compare it with the
/// SHA-256 of `archive`. The file name, when present, must match `asset_name`.
fn verify_checksum(archive: &[u8], checksum: &str, asset_name: &str) -> anyhow::Result<()> {
    let expected = parse_checksum(checksum, asset_name)?;
    let actual = Sha256::digest(archive);
    if actual.as_slice() != expected.as_slice() {
        bail!(
            "SHA-256 mismatch for {asset_name}: expected {}, got {}",
            hex::encode(expected),
            hex::encode(actual)
        );
    }
    Ok(())
}

fn parse_checksum(checksum: &str, asset_name: &str) -> anyhow::Result<Vec<u8>> {
    let line = checksum
        .lines()
        .map(str::trim)
        .find(|line| !line.is_empty())
        .context("checksum file is empty")?;
    let mut fields = line.split_whitespace();
    let digest = fields.next().context("checksum file has no digest")?;
    if let Some(name) = fields.next() {
        let name = name.trim_start_matches('*');
        if name != asset_name {
            bail!("checksum file is for {name:?}, expected {asset_name:?}");
        }
    }
    let expected = hex::decode(digest).context("checksum digest is not valid hex")?;
    if expected.len() != 32 {
        bail!(
            "checksum digest has {} bytes, expected 32 for SHA-256",
            expected.len()
        );
    }
    Ok(expected)
}

fn looks_like_gzip(bytes: &[u8]) -> bool {
    bytes.starts_with(&[0x1f, 0x8b])
}

fn extract_binary(archive: &[u8], binary_name: &str) -> anyhow::Result<Vec<u8>> {
    let decoder = GzDecoder::new(Cursor::new(archive));
    let mut archive = tar::Archive::new(decoder);
    for entry in archive.entries().context("read tar archive")? {
        let mut entry = entry.context("read tar entry")?;
        if !entry.header().entry_type().is_file() {
            continue;
        }
        let path = entry.path().context("read tar entry path")?;
        let Some(name) = path
            .file_name()
            .and_then(|name| name.to_str())
            .map(str::to_owned)
        else {
            continue;
        };
        let extension = Path::new(&name)
            .extension()
            .and_then(|value| value.to_str());
        if !name.starts_with(binary_name) || extension.is_some_and(|extension| extension != "exe") {
            continue;
        }

        let mut binary = Vec::new();
        entry
            .read_to_end(&mut binary)
            .context("read binary from release archive")?;
        if binary.is_empty() {
            bail!("binary {name:?} in release archive is empty");
        }
        return Ok(binary);
    }

    bail!("binary {binary_name:?} not found in release archive")
}

#[cfg(unix)]
fn replace_binary(target: &Path, binary: &[u8]) -> anyhow::Result<()> {
    use std::os::unix::fs::{OpenOptionsExt, PermissionsExt};

    if binary.is_empty() {
        bail!("refusing to install an empty executable");
    }
    let metadata = fs::metadata(target)
        .with_context(|| format!("read executable metadata {}", target.display()))?;
    if !metadata.is_file() {
        bail!("executable target is not a file: {}", target.display());
    }

    let parent = target
        .parent()
        .with_context(|| format!("executable has no parent directory: {}", target.display()))?;
    let file_name = target
        .file_name()
        .and_then(|name| name.to_str())
        .context("executable path is not valid UTF-8")?;
    let temporary = temporary_path(parent, file_name);
    let mut options = fs::OpenOptions::new();
    options.write(true).create_new(true).mode(0o700);
    let mut file = options
        .open(&temporary)
        .with_context(|| format!("create temporary executable {}", temporary.display()))?;

    let write_result = (|| -> anyhow::Result<()> {
        file.write_all(binary)
            .context("write temporary executable")?;
        let mode = metadata.permissions().mode();
        file.set_permissions(fs::Permissions::from_mode(mode))
            .context("preserve executable permissions")?;
        file.sync_all().context("sync temporary executable")?;
        Ok(())
    })();
    drop(file);
    if let Err(error) = write_result {
        let _ = fs::remove_file(&temporary);
        return Err(error);
    }

    if let Err(error) = fs::rename(&temporary, target) {
        let _ = fs::remove_file(&temporary);
        return Err(error).with_context(|| format!("replace executable {}", target.display()));
    }
    if let Ok(directory) = fs::File::open(parent) {
        let _ = directory.sync_all();
    }
    Ok(())
}

#[cfg(not(unix))]
fn replace_binary(_target: &Path, _binary: &[u8]) -> anyhow::Result<()> {
    bail!("self-upgrade executable replacement is only supported on Unix")
}

fn temporary_path(parent: &Path, file_name: &str) -> PathBuf {
    let nonce = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_nanos();
    parent.join(format!(".{file_name}.new-{}-{nonce}", std::process::id()))
}

#[cfg(test)]
mod tests;
