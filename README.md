# Kaunta

Self-hosted web analytics. The browser tracker sets no cookies, respects
Do Not Track by default, and reports only to the instance you run. One
binary serves the collector, the dashboard, and an MCP endpoint, so there
is nothing else to deploy and nothing leaves your server.

## What it does

- **Collects** pageviews including SPA navigation, outbound clicks, file
  downloads, custom events, scroll depth, engagement time, and UTM
  attribution. A 1×1 GIF pixel covers clients without JavaScript, and an
  API-key ingestion endpoint covers servers and background jobs.
- **Reports** visitors, pageviews, bounce rate, a pageview series, and
  breakdowns by page, entry and exit page, source, acquisition channel,
  referrer, country, city, region, browser, OS, device, custom event, and
  every UTM field. Click any row to filter the whole dashboard. Campaigns,
  goals, a world map, a live feed, and CSV export are built in.
- **Answers agents** over MCP: 30 tools on the same port, plus four
  interactive panels that an MCP Apps host renders inline rather than
  printing JSON.

## Install

You need PostgreSQL 18 or newer and an empty database. The released gem
carries a prebuilt binary, so there is nothing to compile:

```sh
gem install kaunta
export DATABASE_URL='postgresql://kaunta:kaunta@localhost:5432/kaunta?sslmode=disable'
kaunta serve
```

Open `http://localhost:3000/setup`. With no users configured Kaunta
serves a setup wizard that checks the database, applies its migrations,
creates the administrator, writes `kaunta.toml` with mode `0600`, and
continues into the main server in the same process.

## Add the tracker

Create a website in the dashboard, then put its ID in the snippet:

```html
<script async src="https://analytics.example.com/k.js"
  data-website-id="YOUR-WEBSITE-ID"></script>
```

Every option is in the [tracker reference](docs/tracker.md). To keep your
own visits out of the data, open any tracked page with
`?kaunta_ignore=true`.

## Ask an agent

Set `mcp = true` and point an MCP client at `/mcp` with an API key, and it
can read the same
analytics the dashboard shows, manage websites and goals, and take
backups. Writes require human confirmation through MCP elicitation, so an
agent cannot delete a website on its own. Hosts that implement MCP Apps
render the overview, live, map, and goal panels as interactive views.
See [MCP integration](docs/mcp.md).

## Configuration

Later sources win: built-in defaults, then `./kaunta.toml` (or
`$XDG_CONFIG_HOME/kaunta/kaunta.toml`), then environment variables, then
the `--database-url`, `--port`, and `--data-dir` flags.

```toml
database_url = "postgresql://kaunta:kaunta@localhost:5432/kaunta?sslmode=disable"
port = "3000"
data_dir = "./data"
secure_cookies = true
trusted_origins = ["localhost"]
# Keep proxy_mode above any TOML table header.
proxy_mode = "none"        # none, xforwarded, cloudflare
excluded_ips = []          # addresses or CIDR blocks that are never recorded
mcp = false                # serve the MCP endpoint at /mcp
event_retention_days = 0   # 0 keeps events indefinitely
```

`proxy_mode` has to sit above a `[trusted_origins]` table: in TOML, a key
written after a table header belongs to that table, and the top-level
setting is then silently absent. `trusted_origins` takes an array or a
comma-separated string.

The environment equivalents are `DATABASE_URL`, `PORT`, `DATA_DIR`,
`SECURE_COOKIES`, `TRUSTED_ORIGINS`, `PROXY_MODE`, `EXCLUDED_IPS`,
`MCP_ENABLED`, and `EVENT_RETENTION_DAYS`; an empty value counts as unset, and an invalid
proxy mode or retention value stops startup. Logging reads
`KAUNTA_LOG_LEVEL`, `KAUNTA_LOG_FORMAT` (`json` for JSON lines), and
`KAUNTA_LOG_SOURCE`.

## CLI

```sh
kaunta serve
kaunta migrate up                    # also runs at startup
kaunta migration-status
kaunta healthcheck
kaunta user create admin --name "Administrator"
kaunta website create example.com --name "Example"
kaunta website tracking-code example.com
kaunta apikey create example.com --name collector --scope ingest
kaunta backup create --days 7 --output ./backups
kaunta backup restore ./backups/kaunta-full-...dump
```

Global flags work before or after a subcommand, and
`kaunta <command> --help` prints the full syntax. Migrations only roll
forward: there are no down migrations, and partial steps are unsupported.

## Docker

```sh
docker compose up --build
```

The stack runs PostgreSQL 18 and publishes Kaunta on
`http://localhost:3010`, with named volumes for database and application
data. The runtime image runs as an unprivileged user and ships a health
check. [Self-hosting](docs/self-hosting.md) covers reverse proxies, systemd,
backups, upgrades, retention, and partition compaction.

## Development

Kaunta is written in Rust and builds with the toolchain pinned in
`rust-toolchain.toml`:

```sh
cargo run -p kaunta -- serve                 # run it
cargo test --workspace                       # unit and integration tests
cargo test -p kaunta-e2e -- --ignored        # browser and API scenarios
```

The e2e scenarios are opt-in because they drive a real Chrome against a
real database. `crates/kaunta-e2e/README.md` explains the harness,
including the screenshot tour used for design review.

## License

MIT. See [LICENSE](LICENSE).
