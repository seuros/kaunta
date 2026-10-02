# Self-hosting

This guide covers installing, exposing, backing up, and upgrading a Kaunta instance.

## Requirements and installation

Kaunta requires PostgreSQL 18 or newer. The release installer supports Linux
(glibc 2.35 or newer) and macOS on amd64/arm64, plus FreeBSD 15.1+ on amd64. It
installs to `~/.local/bin` by default:

```sh
curl -fsSL https://raw.githubusercontent.com/seuros/kaunta/master/scripts/install.sh \
  | bash
```

To choose a release and install prefix:

```sh
curl -fsSL https://raw.githubusercontent.com/seuros/kaunta/master/scripts/install.sh \
  | bash -s -- --version v0.112.0 --prefix "$HOME/bin"
```

Create an empty database, set `DATABASE_URL`, and start the service:

```sh
export DATABASE_URL='postgresql://kaunta:change-me@127.0.0.1:5432/kaunta?sslmode=disable'
kaunta serve
```

When initial setup is required, browse to `http://localhost:3000/setup` and
complete the wizard. It verifies PostgreSQL 18+, applies migrations, creates
the first administrator and session, reconciles the reserved dashboard
website, writes `kaunta.toml` with mode `0600`, and starts the main server.

## Docker Compose

The repository’s Compose file starts PostgreSQL 18 and Kaunta:

```sh
docker compose up -d
```

It publishes Kaunta at `http://localhost:3010` and stores database and app
data in named volumes. The sample Compose configuration sets
`SECURE_COOKIES=false` and trusts `localhost`; change these for deployment
behind HTTPS and configure the public origin. The container listens on port
3100 internally; Compose maps it to 3010.

## Reverse proxy

Terminate TLS at a trusted reverse proxy and forward requests to Kaunta’s
listening port. For nginx:

```nginx
server {
    listen 443 ssl;
    server_name analytics.example.com;

    location / {
        proxy_pass http://127.0.0.1:3000;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_read_timeout 3600s;
    }
}
```

Set `secure_cookies = true` (the default) when serving over HTTPS, and add
the public hostname to `trusted_origins`. The `proxy_mode` setting controls
how Kaunta derives client IP addresses:

- `none`: direct peer address; use when clients connect directly.
- `xforwarded`: read `X-Forwarded-For` / `X-Real-IP`; use behind a proxy that
  overwrites these headers and cannot be bypassed by untrusted clients.
- `cloudflare`: use `CF-Connecting-IP` when requests arrive through
  Cloudflare.

For Cloudflare, set `proxy_mode = "cloudflare"` and restrict origin access to
Cloudflare’s published network ranges at the firewall or proxy layer. Do not
trust forwarded IP headers from arbitrary clients. Configure `trusted_origins`
for the public host as well.

TOML parsing detail: `proxy_mode` must be a top-level key before any table
header such as `[trusted_origins]`. A key written after a table header belongs
to that table and is silently ignored as a top-level proxy setting.

## Excluding your own traffic

`excluded_ips` drops requests from addresses you do not want measured,
such as your office, your VPN, or a monitoring probe, before anything is
written:

```toml
excluded_ips = ["203.0.113.4", "10.0.0.0/8", "2001:db8::/32"]
```

Plain addresses and CIDR blocks are both accepted, as is the
`EXCLUDED_IPS` environment variable with a comma-separated list. Excluded
requests receive a normal `202 Accepted` with `{"excluded": true}`, so the
tracker does not retry or report an error on the page. The address is the
one Kaunta resolves for the request, so set `proxy_mode` correctly or the
rules will be compared against your proxy's address instead of the
visitor's.

Config entries require a restart to change, which is awkward on a
connection whose address moves. Two alternatives avoid that:

**Exclude a browser instead of an address.** Open any tracked page with
`?kaunta_ignore=true`. The tracker stores a flag in that browser's
`localStorage` and records nothing from it afterwards, whatever the
address. `?kaunta_ignore=false` undoes it. This is the simplest option for
excluding yourself.

**Exclude addresses at runtime.** The Websites page lists stored
exclusions along with the address Kaunta currently resolves for you, so a
new address is one click away. The same list is available over MCP
(`list_exclusions`, `add_exclusion`, `remove_exclusion`), which is useful
when an agent manages the instance. Stored rules take effect within a
minute and need no restart; rules from the config file remain in force and
cannot be removed this way.

## Running under systemd

Kaunta reads `kaunta.toml` from the working directory or the XDG config
path, so the unit points its working directory at the data root rather
than naming a config file:

```ini
[Unit]
Description=Kaunta Analytics
After=network-online.target postgresql.service
Wants=network-online.target

[Service]
User=kaunta
Group=kaunta
WorkingDirectory=/var/lib/kaunta
ExecStart=/usr/local/bin/kaunta serve
Restart=on-failure
RestartSec=5s
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
```

Put the configuration at `/var/lib/kaunta/kaunta.toml`, or pass the few
settings that have flags (`--database-url`, `--port`, `--data-dir`) or
their environment equivalents.

## Backups

Create a full PostgreSQL custom-format backup:

```sh
kaunta backup create --output /var/backups/kaunta
```

Create an archive of the last 7 days instead:

```sh
kaunta backup create --days 7 --output /var/backups/kaunta
```

Full backups use `pg_dump -Fc`; period backups are tar.gz archives of CSV
data. Schedule the CLI command with an external scheduler and copy the output
to storage outside the Kaunta host. The MCP `create_backup` tool also creates
temporary downloads under `/backups/{name}`; those files expire after roughly
one hour and are not a substitute for a retained backup routine.

Restore a full backup into the configured database:

```sh
kaunta backup restore /var/backups/kaunta/kaunta-full-20261001-201926.145343Z-e5d287218b9324af.dump
```

Stop the server first: the restore drops and recreates the `public` schema
before loading the dump (`pg_restore --clean` cannot drop the inherited
constraints of Kaunta's partitioned tables). When the filename still carries
the sha256 suffix Kaunta wrote, the file is hashed and checked against it
before the database is touched; a mismatch aborts the restore. `--yes` skips
the confirmation prompt. Period archives are data exports, not restorable
dumps: load their CSVs with `psql \copy` as described in the archive's
`manifest.json`. Both `pg_dump` and `pg_restore` must be on `PATH`.

## Upgrading

Install or copy the replacement release binary, then apply migrations before
or during the next service start:

```sh
kaunta migrate up
```

The server also runs embedded migrations at startup. Keep a verified backup
before upgrading. Release binaries provide self-update options:
`kaunta --self-upgrade-check`, `kaunta --self-upgrade`, and
`kaunta --self-upgrade --self-upgrade-yes`.

## Retention and data directory

`DATA_DIR` defaults to `./data`; it stores GeoIP data and temporary MCP backup
files. The server loads `GeoLite2-City.mmdb` there and downloads it when
missing; unavailable or invalid data disables GeoIP enrichment with a warning.

Set `EVENT_RETENTION_DAYS` to a non-negative integer to remove older event
partitions. Its default is `0`, which retains events indefinitely. Set the
retention policy according to the data-minimization requirements of your
deployment.

Events are stored in daily partitions. Daily granularity is useful while data
is queried and dropped often; for cold data it means thousands of small tables
and indexes. Daily maintenance therefore folds every month whose days are all
older than 90 days into a single monthly partition (`website_event_2026_04`),
oldest first, up to twelve months per run. Each month is compacted in one
transaction, so queries see either the daily partitions or the monthly one.
Retention still applies to compacted months, but only once the whole month is
past the cutoff.
