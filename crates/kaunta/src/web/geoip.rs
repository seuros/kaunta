use std::{
    fs::{self, File},
    io,
    net::IpAddr,
    path::{Path, PathBuf},
    sync::Arc,
};

use anyhow::Context as _;
use flate2::read::GzDecoder;
use rama::net::address::ip::geo::{IpGeoDb, MmdbReader, RAMA_IP_GEO_DB_ENV};

pub const DATABASE_FILE: &str = "GeoLite2-City.mmdb";
const DATABASE_URL: &str = "https://cdn.jsdelivr.net/npm/geolite2-city/GeoLite2-City.mmdb.gz";

#[derive(Clone, Default)]
pub struct GeoIp {
    db: Option<Arc<IpGeoDb>>,
}

impl GeoIp {
    pub async fn initialize(data_dir: &Path) -> Self {
        match tokio::task::spawn_blocking(IpGeoDb::from_env).await {
            Ok(Ok(Some(db))) => {
                tracing::info!(
                    sources = ?db.labels().collect::<Vec<_>>(),
                    "loaded GeoIP database from {RAMA_IP_GEO_DB_ENV}"
                );
                return Self::from_db(db);
            }
            Ok(Ok(None)) => {}
            Ok(Err(error)) => {
                tracing::warn!(
                    ?error,
                    "invalid {RAMA_IP_GEO_DB_ENV}; falling back to the bundled GeoLite2 download"
                );
            }
            Err(error) => {
                tracing::warn!(
                    ?error,
                    "GeoIP loader task failed; location enrichment is disabled"
                );
                return Self::default();
            }
        }

        let path = database_path(data_dir);
        if !path.exists()
            && let Err(error) = download_database(&path).await
        {
            tracing::warn!(
                ?error,
                path = %path.display(),
                "GeoIP database is unavailable; location enrichment is disabled"
            );
            return Self::default();
        }

        let open_path = path.clone();
        match tokio::task::spawn_blocking(move || MmdbReader::open(open_path)).await {
            Ok(Ok(reader)) => {
                tracing::info!(path = %path.display(), "loaded GeoIP database");
                Self::from_db(IpGeoDb::builder().reader("geolite2-city", reader).build())
            }
            Ok(Err(error)) => {
                tracing::warn!(
                    ?error,
                    path = %path.display(),
                    "failed to load GeoIP database; location enrichment is disabled"
                );
                Self::default()
            }
            Err(error) => {
                tracing::warn!(
                    ?error,
                    path = %path.display(),
                    "GeoIP loader task failed; location enrichment is disabled"
                );
                Self::default()
            }
        }
    }

    fn from_db(db: IpGeoDb) -> Self {
        for notice in db.attributions() {
            tracing::info!(notice, "GeoIP attribution");
        }
        Self {
            db: Some(Arc::new(db)),
        }
    }

    #[must_use]
    pub fn is_enabled(&self) -> bool {
        self.db.is_some()
    }

    #[must_use]
    pub fn lookup(&self, ip_address: &str) -> (String, String, String) {
        let Some(db) = &self.db else {
            return empty_location();
        };
        let Ok(address) = ip_address.parse::<IpAddr>() else {
            return empty_location();
        };
        let Some(location) = db.lookup(address) else {
            return empty_location();
        };
        let country = location
            .country
            .map(|country| country.code().to_owned())
            .unwrap_or_default();
        let city = location.city.unwrap_or_default().into_string();
        let region = location
            .subdivisions
            .first()
            .and_then(|subdivision| subdivision.name.as_deref())
            .unwrap_or_default()
            .to_owned();
        (country, city, region)
    }
}

#[must_use]
pub fn database_path(data_dir: &Path) -> PathBuf {
    data_dir.join(DATABASE_FILE)
}

async fn download_database(path: &Path) -> anyhow::Result<()> {
    let parent = path
        .parent()
        .context("GeoIP database path has no parent directory")?;
    tokio::fs::create_dir_all(parent)
        .await
        .with_context(|| format!("create GeoIP data directory {}", parent.display()))?;

    tracing::info!(url = DATABASE_URL, "downloading GeoIP database");
    let compressed = reqwest::get(DATABASE_URL)
        .await
        .context("download GeoIP database")?
        .error_for_status()
        .context("GeoIP download returned an error status")?
        .bytes()
        .await
        .context("read GeoIP download")?;

    let target = path.to_owned();
    tokio::task::spawn_blocking(move || decompress_database(&compressed, &target))
        .await
        .context("join GeoIP decompression task")??;
    Ok(())
}

fn decompress_database(compressed: &[u8], target: &Path) -> anyhow::Result<()> {
    let temporary = target.with_extension(format!("mmdb.tmp-{}", std::process::id()));
    let result = (|| {
        let mut decoder = GzDecoder::new(compressed);
        let mut output = File::create(&temporary)
            .with_context(|| format!("create temporary GeoIP database {}", temporary.display()))?;
        io::copy(&mut decoder, &mut output).context("decompress GeoIP database")?;
        output
            .sync_all()
            .context("flush temporary GeoIP database")?;
        fs::rename(&temporary, target).with_context(|| {
            format!(
                "move GeoIP database from {} to {}",
                temporary.display(),
                target.display()
            )
        })
    })();
    if result.is_err() {
        let _ = fs::remove_file(&temporary);
    }
    result
}

fn empty_location() -> (String, String, String) {
    (String::new(), String::new(), String::new())
}

#[cfg(test)]
mod tests;
