use std::path::PathBuf;

use serde::{Deserialize, Deserializer, Serialize};

use crate::domain::website::ProxyMode;

/// Runtime configuration shared by the server and the CLI.
///
/// Deserialization accepts both the Rust layout and the layout written by the
/// Go setup wizard:
///
/// * `trusted_origins` may be an array, a comma-joined string, or a
///   `[trusted_origins]` table with an `origins` array.
/// * `port` may be absent, in which case `[server] port` is used.
///
/// Serialization always emits the Rust layout (array origins, top-level port).
#[derive(Debug, Clone, Serialize)]
pub struct Config {
    pub database_url: String,
    pub port: String,
    pub data_dir: PathBuf,
    pub secure_cookies: bool,
    pub trusted_origins: Vec<String>,
    pub proxy_mode: ProxyMode,
    pub event_retention_days: u32,
    /// Addresses whose traffic is never recorded: your own office or VPN,
    /// given as plain IPs or CIDR blocks (`203.0.113.4`, `10.0.0.0/8`).
    pub excluded_ips: Vec<String>,
    /// Whether to serve the MCP endpoint at `/mcp`. Off by default: it
    /// gives a CLI-minted API key operator reach over every website, which
    /// is far more than the same key can do on the plain HTTP API.
    pub mcp: bool,
    pub security: SecurityConfig,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub server: Option<ServerConfig>,
}

/// Whether `address` falls inside `rule`, which is either a plain IP or a
/// CIDR block. Unparsable rules never match.
#[must_use]
pub fn ip_matches(rule: &str, address: &std::net::IpAddr) -> bool {
    let rule = rule.trim();
    let Some((network, prefix)) = rule.split_once('/') else {
        return rule
            .parse::<std::net::IpAddr>()
            .is_ok_and(|ip| ip == *address);
    };
    let (Ok(network), Ok(prefix)) = (network.parse::<std::net::IpAddr>(), prefix.parse::<u32>())
    else {
        return false;
    };
    let (network, address) = match (network, address) {
        (std::net::IpAddr::V4(network), std::net::IpAddr::V4(address)) => {
            (network.octets().to_vec(), address.octets().to_vec())
        }
        (std::net::IpAddr::V6(network), std::net::IpAddr::V6(address)) => {
            (network.octets().to_vec(), address.octets().to_vec())
        }
        _ => return false,
    };
    if prefix > u32::try_from(network.len() * 8).unwrap_or(0) {
        return false;
    }
    let whole = (prefix / 8) as usize;
    let remainder = prefix % 8;
    if network[..whole] != address[..whole] {
        return false;
    }
    if remainder == 0 {
        return true;
    }
    let mask = 0xffu8 << (8 - remainder);
    network[whole] & mask == address[whole] & mask
}

/// Whether `rule` is a usable exclusion rule (plain IP or CIDR block).
#[must_use]
pub fn is_ip_rule(rule: &str) -> bool {
    let rule = rule.trim();
    match rule.split_once('/') {
        None => rule.parse::<std::net::IpAddr>().is_ok(),
        Some((network, prefix)) => {
            let Ok(network) = network.parse::<std::net::IpAddr>() else {
                return false;
            };
            let bits = if network.is_ipv4() { 32 } else { 128 };
            prefix.parse::<u32>().is_ok_and(|prefix| prefix <= bits)
        }
    }
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
#[serde(default)]
pub struct SecurityConfig {
    pub install_lock: bool,
}

/// `[server]` table written by the Go setup wizard.
#[derive(Debug, Clone, Default, PartialEq, Eq, Serialize, Deserialize)]
#[serde(default)]
pub struct ServerConfig {
    pub host: Option<String>,
    pub port: Option<String>,
}

impl Default for Config {
    fn default() -> Self {
        Self {
            database_url: String::new(),
            port: "3000".to_owned(),
            data_dir: PathBuf::from("./data"),
            secure_cookies: true,
            trusted_origins: vec!["localhost".to_owned()],
            excluded_ips: Vec::new(),
            mcp: false,
            proxy_mode: ProxyMode::None,
            event_retention_days: 0,
            security: SecurityConfig::default(),
            server: None,
        }
    }
}

/// Split a comma-joined origin list the same way the Go loader does.
#[must_use]
pub fn parse_trusted_origins(value: &str) -> Vec<String> {
    value
        .split(',')
        .map(str::trim)
        .filter(|origin| !origin.is_empty())
        .map(str::to_lowercase)
        .collect()
}

#[derive(Deserialize)]
#[serde(untagged)]
enum TrustedOrigins {
    List(Vec<String>),
    Joined(String),
    /// `[trusted_origins]` table with an `origins` key, as written by the Go
    /// setup wizard.
    Table {
        origins: Vec<String>,
    },
}

impl From<TrustedOrigins> for Vec<String> {
    fn from(value: TrustedOrigins) -> Self {
        match value {
            TrustedOrigins::List(origins) | TrustedOrigins::Table { origins } => origins
                .iter()
                .map(|origin| origin.trim())
                .filter(|origin| !origin.is_empty())
                .map(str::to_lowercase)
                .collect(),
            TrustedOrigins::Joined(joined) => parse_trusted_origins(&joined),
        }
    }
}

#[derive(Deserialize)]
#[serde(default)]
struct RawConfig {
    database_url: String,
    port: Option<String>,
    data_dir: PathBuf,
    secure_cookies: bool,
    trusted_origins: Option<TrustedOrigins>,
    proxy_mode: ProxyMode,
    event_retention_days: u32,
    excluded_ips: Option<TrustedOrigins>,
    mcp: bool,
    security: SecurityConfig,
    server: Option<ServerConfig>,
}

impl Default for RawConfig {
    fn default() -> Self {
        let defaults = Config::default();
        Self {
            database_url: defaults.database_url,
            port: None,
            data_dir: defaults.data_dir,
            secure_cookies: defaults.secure_cookies,
            trusted_origins: None,
            proxy_mode: defaults.proxy_mode,
            event_retention_days: defaults.event_retention_days,
            excluded_ips: None,
            mcp: defaults.mcp,
            security: defaults.security,
            server: None,
        }
    }
}

impl<'de> Deserialize<'de> for Config {
    fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
    where
        D: Deserializer<'de>,
    {
        let raw = RawConfig::deserialize(deserializer)?;
        let defaults = Self::default();
        let port = raw
            .port
            .filter(|port| !port.trim().is_empty())
            .or_else(|| {
                raw.server
                    .as_ref()
                    .and_then(|server| server.port.clone())
                    .filter(|port| !port.trim().is_empty())
            })
            .unwrap_or(defaults.port);
        let trusted_origins = raw
            .trusted_origins
            .map_or(defaults.trusted_origins, Vec::from);
        Ok(Self {
            database_url: raw.database_url,
            port,
            data_dir: raw.data_dir,
            secure_cookies: raw.secure_cookies,
            trusted_origins,
            proxy_mode: raw.proxy_mode,
            event_retention_days: raw.event_retention_days,
            excluded_ips: raw.excluded_ips.map_or(defaults.excluded_ips, Vec::from),
            mcp: raw.mcp,
            security: raw.security,
            server: raw.server,
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[test]
    fn trusted_origins_accepts_joined_string() {
        let config: Config =
            serde_json::from_value(json!({ "trusted_origins": " A.example.com, ,b.example.com " }))
                .expect("deserialize");
        assert_eq!(
            config.trusted_origins,
            vec!["a.example.com", "b.example.com"]
        );
    }

    #[test]
    fn trusted_origins_accepts_array() {
        let config: Config =
            serde_json::from_value(json!({ "trusted_origins": ["A.example.com", "", " b "] }))
                .expect("deserialize");
        assert_eq!(config.trusted_origins, vec!["a.example.com", "b"]);
    }

    #[test]
    fn trusted_origins_accepts_go_wizard_table() {
        let config: Config = toml::from_str(
            r#"
database_url = "postgresql://kaunta:kaunta@localhost:5432/kauntadb?sslmode=disable"
port = "3002"
data_dir = "/opt/kaunta/data"
secure_cookies = false

[trusted_origins]
origins = ["https://census.example.com", "https://dag.ma"]
"#,
        )
        .expect("deserialize Go wizard layout");
        assert_eq!(
            config.trusted_origins,
            vec!["https://census.example.com", "https://dag.ma"]
        );
    }

    #[test]
    fn trusted_origins_empty_string_yields_no_origins() {
        let config: Config =
            serde_json::from_value(json!({ "trusted_origins": "" })).expect("deserialize");
        assert!(config.trusted_origins.is_empty());
    }

    #[test]
    fn trusted_origins_defaults_when_absent() {
        let config: Config = serde_json::from_value(json!({})).expect("deserialize");
        assert_eq!(config.trusted_origins, vec!["localhost"]);
    }

    #[test]
    fn server_port_is_used_when_top_level_port_is_absent() {
        let config: Config = serde_json::from_value(json!({
            "server": { "host": "0.0.0.0", "port": "8080" }
        }))
        .expect("deserialize");
        assert_eq!(config.port, "8080");
        assert_eq!(
            config.server,
            Some(ServerConfig {
                host: Some("0.0.0.0".to_owned()),
                port: Some("8080".to_owned()),
            })
        );
    }

    #[test]
    fn top_level_port_wins_over_server_port() {
        let config: Config = serde_json::from_value(json!({
            "port": "9000",
            "server": { "port": "8080" }
        }))
        .expect("deserialize");
        assert_eq!(config.port, "9000");
    }

    #[test]
    fn missing_port_falls_back_to_default() {
        let config: Config = serde_json::from_value(json!({ "server": { "host": "0.0.0.0" } }))
            .expect("deserialize");
        assert_eq!(config.port, "3000");
    }

    #[test]
    fn proxy_mode_and_retention_default_and_parse() {
        let config: Config = serde_json::from_value(json!({})).expect("deserialize");
        assert_eq!(config.proxy_mode, ProxyMode::None);
        assert_eq!(config.event_retention_days, 0);

        let config: Config = serde_json::from_value(json!({
            "proxy_mode": "cloudflare",
            "event_retention_days": 90
        }))
        .expect("deserialize");
        assert_eq!(config.proxy_mode, ProxyMode::Cloudflare);
        assert_eq!(config.event_retention_days, 90);

        assert!(serde_json::from_value::<Config>(json!({ "proxy_mode": "bogus" })).is_err());
    }

    #[test]
    fn excluded_ips_match_plain_addresses_and_cidr_blocks() {
        let v4 = |s: &str| s.parse::<std::net::IpAddr>().unwrap();
        assert!(ip_matches("203.0.113.4", &v4("203.0.113.4")));
        assert!(!ip_matches("203.0.113.4", &v4("203.0.113.5")));
        assert!(ip_matches("10.0.0.0/8", &v4("10.42.7.1")));
        assert!(!ip_matches("10.0.0.0/8", &v4("11.0.0.1")));
        assert!(ip_matches("192.168.1.0/24", &v4("192.168.1.255")));
        assert!(!ip_matches("192.168.1.0/24", &v4("192.168.2.1")));
        assert!(ip_matches("192.168.0.0/22", &v4("192.168.3.9")));
        assert!(!ip_matches("192.168.0.0/22", &v4("192.168.4.9")));
        assert!(ip_matches("2001:db8::/32", &v4("2001:db8:1::9")));
        assert!(!ip_matches("2001:db8::/32", &v4("2001:db9::9")));
        assert!(!ip_matches("10.0.0.0/8", &v4("::1")));
        assert!(!ip_matches("not-an-ip", &v4("10.0.0.1")));
        assert!(!ip_matches("10.0.0.0/99", &v4("10.0.0.1")));
    }

    #[test]
    fn excluded_ips_accept_joined_or_array_form() {
        let config: Config =
            serde_json::from_value(json!({ "excluded_ips": " 10.0.0.0/8 , ,203.0.113.4 " }))
                .expect("deserialize");
        assert_eq!(config.excluded_ips, vec!["10.0.0.0/8", "203.0.113.4"]);
        let config: Config = serde_json::from_value(json!({})).expect("deserialize");
        assert!(config.excluded_ips.is_empty());
    }

    #[test]
    fn mcp_endpoint_is_off_unless_asked_for() {
        let config: Config = serde_json::from_value(json!({})).expect("deserialize");
        assert!(!config.mcp);
        let config: Config = toml::from_str("mcp = true\n").expect("deserialize");
        assert!(config.mcp);
    }
}
