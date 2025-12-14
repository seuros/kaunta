# Kaunta

[![Go Version](https://img.shields.io/badge/go-1.25-00ADD8?style=flat-square&logo=go)](https://go.dev/)
[![PostgreSQL](https://img.shields.io/badge/postgresql-17+-336791?style=flat-square&logo=postgresql&logoColor=white)](https://www.postgresql.org/)
[![License](https://img.shields.io/badge/license-MIT-green?style=flat-square)](LICENSE)
[![Website](https://img.shields.io/badge/website-live-success?style=flat-square&logo=github)](https://seuros.github.io/kaunta/)
[![GitHub Release](https://img.shields.io/github/v/release/seuros/kaunta?style=flat-square)](https://github.com/seuros/kaunta/releases)

Analytics without bloat.

A simple, fast, privacy-focused web analytics engine. Drop-in replacement for Umami.

🌐 **[Visit Website](https://seuros.github.io/kaunta/)**

## Features

- **Privacy-First** - No cookies, no tracking, privacy by design
- **Fast Deployment** - Single binary (~17MB), ready to run
- **Lightweight** - Docker image only 28MB, minimal memory footprint
- **Umami Compatible** - Same API & database schema
- **Full Analytics** - Visitors, pageviews, referrers, devices, locations, real-time stats
- **Geolocation** - City/region level with automatic MaxMind GeoLite2 download
- **Multi-Domain Support** - Custom domains via CNAME with shared authentication
- **Cross-Platform** - Linux, macOS, Windows, and FreeBSD binaries available

## Supported Platforms

Pre-built binaries are available for:
- **Linux** (amd64, arm64)
- **macOS** (amd64, arm64)
- **Windows** (amd64, arm64)
- **FreeBSD** (amd64)

Single-board computers:
- **Raspberry Pi / Orange Pi (64-bit)** - use the published `linux/arm64` Docker image or build on-device with `docker buildx build --platform linux/arm64 .`; see the step-by-step guide at https://dockerplaybooks.dpdns.org/kauntaonpi/kaunta.html

Download the latest release from [GitHub Releases](https://github.com/seuros/kaunta/releases). The `--self-upgrade` command works on all platforms to update to the latest version.

### NAS Devices

Kaunta runs on NAS devices via Docker:
- **Synology** - [Installation Guide](https://mariushosting.com/how-to-install-kaunta-on-your-synology-nas/)
- **Ugreen** - [Installation Guide](https://dockerplaybooks.dpdns.org/kauntaonugreen/kaunta.html)
- **QNAP**, **TrueNAS**, **Unraid** - Use the standard Docker image

## Installation

### Quick Install (Linux/macOS/FreeBSD)

```bash
# Install to ~/.local/bin (default)
curl -fsSL https://raw.githubusercontent.com/seuros/kaunta/master/scripts/install.sh | bash

# Install to /usr/local/bin (system-wide, requires sudo)
curl -fsSL https://raw.githubusercontent.com/seuros/kaunta/master/scripts/install.sh | bash -s -- --system

# Install specific version
curl -fsSL https://raw.githubusercontent.com/seuros/kaunta/master/scripts/install.sh | bash -s -- --version v0.21.1

# Custom install location
curl -fsSL https://raw.githubusercontent.com/seuros/kaunta/master/scripts/install.sh | bash -s -- --prefix /custom/path
```

The install script supports:
- Auto-detection of platform (Linux/macOS/FreeBSD) and architecture (amd64/arm64)
- Automatic latest release download from GitHub
- Optional sudo for system-wide installation
- Custom install directories via `--prefix`
- Version pinning via `--version`

**Environment Variables:**
- `KAUNTA_INSTALL_PREFIX` - Install directory (e.g., `~/.local/bin`)
- `KAUNTA_INSTALL_VERSION` - Version to install (e.g., `v0.21.1`)
- `KAUNTA_INSTALL_ASSUME_SUDO` - One of: `always`, `never`, `prompt` (default)

For Windows, download the `.exe` binary from [GitHub Releases](https://github.com/seuros/kaunta/releases).

### 1. Configuration

Kaunta requires PostgreSQL 17+. You can configure it using:

**Option 1: Config file** (recommended)

Create `kaunta.toml` in current directory or `~/.kaunta/kaunta.toml`:

```toml
database_url = "postgresql://user:password@localhost:5432/kaunta"
port = "3000"
data_dir = "./data"
secure_cookies = false  # Set to true when serving Kaunta behind HTTPS
```

**Option 2: Environment variables**

```bash
export DATABASE_URL="postgresql://user:password@localhost:5432/kaunta"
export PORT="3000"
export DATA_DIR="./data"
export SECURE_COOKIES="true"  # Enable for HTTPS deployments (default: false)
```

**Option 3: Command flags**

```bash
kaunta --database-url="postgresql://..." --port=3000 --data-dir=./data
```

**Priority order:** Flags > Config file > Environment variables

### 2. Run the Server

```bash
# Docker
docker run -e DATABASE_URL="postgresql://..." -p 3000:3000 kaunta

# Or standalone binary
./kaunta
```

The server will:
- Auto-run database migrations on startup
- Download GeoIP database if missing
- Start on port 3000 (configurable with `PORT` env var)

Health check endpoint: `GET /up`

**HTTPS / TLS Termination**

Kaunta only listens for plain HTTP traffic (no built-in TLS). For HTTPS you should:
- Put Kaunta behind a reverse proxy that handles TLS (nginx, Cloudflare, Caddy, etc.)
- Terminate HTTPS at the proxy and forward requests to Kaunta over HTTP
- Set `secure_cookies = true` in `kaunta.toml` (or `SECURE_COOKIES=true`) when your proxy serves HTTPS so CSRF/session cookies are marked `Secure`
- See `docs/examples/nginx.md` for a sample nginx config and `docs/examples/systemd.md` to run Kaunta as a systemd service

### 3. Create a Website

```bash
kaunta website create example.com
```

Or use the dashboard UI at `/dashboard/websites` to create and manage websites with allowed domains.

This generates the website ID needed for tracking.

### 4. Add Tracker Script

Add this to your website (works like Google Analytics):

```html
<script src="https://your-kaunta-server.com/k.js"
        data-website-id="your-website-uuid"
        data-debug="true"
        async defer></script>
```

Set `data-debug="true"` to log tracker activity to the browser console while testing. Remove it in production to keep the script silent.

That's it! Analytics start collecting.

## User Management

Kaunta uses CLI-based user management. There is no web registration - all users must be created via the command line.

If you have a config file (`kaunta.toml`), the CLI commands will automatically use those settings. Otherwise, use the `--database-url` flag or set `DATABASE_URL` environment variable.

### Create a User

```bash
kaunta user create <username>
# You'll be prompted for name (optional) and password
# Password must be at least 8 characters

# Or with database URL flag:
kaunta --database-url="postgresql://..." user create admin
```

Example:
```bash
$ kaunta user create admin
Full name (optional): Admin User
Password: ********
Confirm password: ********

✓ User created successfully
  ID:       550e8400-e29b-41d4-a716-446655440000
  Username: admin
  Name:     Admin User
  Created:  2025-01-08 14:30:00
```

You can also provide the name via flag:
```bash
kaunta user create admin --name "Admin User"
```

### List Users

```bash
kaunta user list
```

This shows all users with their ID, username, name, and creation date.

### Delete a User

```bash
kaunta user delete <username>
# You'll be asked to confirm (use --force to skip)
```

When you delete a user:
- All their sessions are invalidated
- Websites owned by the user become unassigned (user_id set to NULL)

### Reset Password

```bash
kaunta user reset-password <username>
# You'll be prompted for the new password

# Or provide password via flag (useful for Docker/automation)
kaunta user reset-password <username> --password "new-password"
```

This will:
- Update the user's password
- Invalidate all existing sessions (user must log in again)

### Docker User Management

When running in Docker, use `sh` instead of `bash` (Alpine Linux doesn't include bash):

```bash
# Create a user interactively
docker exec -it kaunta-container sh -c "kaunta user create admin"

# Or use the --password flag for non-interactive mode
docker exec kaunta-container kaunta user create admin --password "your-password-here"

# Using docker-compose
docker-compose exec kaunta sh -c "kaunta user create admin"
docker-compose exec kaunta kaunta user create admin --password "your-password-here"

# List users
docker exec kaunta-container kaunta user list

# Reset password (non-interactive)
docker exec kaunta-container kaunta user reset-password admin --password "new-password"
```

**Note:** The `--password` flag allows non-interactive password setup, useful for Docker environments and automation scripts.

### Access the Dashboard

After creating a user:
1. Navigate to `http://your-server:3000/login`
2. Log in with the username and password
3. You'll be redirected to `/dashboard`

Sessions last 7 days and use HTTP-only cookies for security.

## Domain Management

Kaunta supports multiple custom domains for dashboard access (e.g., `analytics.yourdomain.com`, `stats.client.com`) using CNAME records. This allows you to provide white-label analytics dashboards while maintaining a single Kaunta instance with shared authentication.

Trusted domains are stored in the database and managed via CLI commands. Changes take effect within 5 minutes (cache TTL).

### Add a Trusted Domain

```bash
kaunta domain add <domain>

# With description
kaunta domain add analytics.example.com --description "Main analytics dashboard"
```

**Important:** Provide the domain without protocol (no `http://` or `https://`). Port numbers are handled automatically.

Example:
```bash
$ kaunta domain add analytics.example.com --description "Client dashboard"

✓ Trusted domain added successfully
  ID:     1
  Domain: analytics.example.com
  Desc:   Client dashboard
  Active: true
  Added:  2025-01-10 10:30:00

Note: Changes take effect within 5 minutes (cache TTL)
```

### List Trusted Domains

```bash
kaunta domain list           # Show all domains
kaunta domain list --active  # Show only active domains
```

Example output:
```
Total domains: 3

ID    Active  Domain                      Description            Created
--------------------------------------------------------------------------------
1     ✓       analytics.example.com       Main dashboard         2025-01-10 10:30:00
2     ✓       stats.client.com            Client analytics       2025-01-10 11:45:00
3     ✗       old.domain.com              Deprecated             2025-01-05 09:15:00
```

### Remove a Trusted Domain

```bash
kaunta domain remove <domain>
# Or use domain ID:
kaunta domain remove 1

# Skip confirmation:
kaunta domain remove analytics.example.com --force
```

### Toggle Domain Status

Temporarily disable a domain without deleting it:

```bash
kaunta domain toggle <domain>
# Or use ID:
kaunta domain toggle 1
```

Example:
```bash
$ kaunta domain toggle analytics.example.com
✓ Domain 'analytics.example.com' disabled successfully
Note: Changes take effect within 5 minutes (cache TTL)

$ kaunta domain toggle analytics.example.com
✓ Domain 'analytics.example.com' enabled successfully
```

### Verify a Domain

Test if an origin URL is trusted (useful for debugging CSRF issues):

```bash
kaunta domain verify <origin-url>
```

Example:
```bash
$ kaunta domain verify https://analytics.example.com
✓ Origin 'https://analytics.example.com' is TRUSTED

$ kaunta domain verify https://unknown.com
✗ Origin 'https://unknown.com' is NOT TRUSTED

Add the domain with: kaunta domain add <domain>
```

### Setting Up Custom Domains

1. **Add domain to Kaunta:**
   ```bash
   kaunta domain add analytics.yourdomain.com
   ```

2. **Configure DNS CNAME:**
   Point your custom domain to your Kaunta server:
   ```
   analytics.yourdomain.com  CNAME  kaunta.yourserver.com
   ```

3. **Configure reverse proxy** (if using nginx/Cloudflare):
   - Ensure the proxy forwards the `Origin` header
   - Enable HTTPS (required for `SameSite=None` cookies)

4. **Test the configuration:**
   ```bash
   kaunta domain verify https://analytics.yourdomain.com
   ```

Users can now log in at `https://analytics.yourdomain.com/login` and access the dashboard. Sessions work across all trusted domains.

## Upgrading Kaunta

When running Kaunta as a standalone binary, you can update it in place without re-downloading releases manually:

```bash
kaunta --self-upgrade           # download and install the latest release
kaunta --self-upgrade-yes       # skip the confirmation prompt
kaunta --self-upgrade-check     # only check if a newer version exists
```

The `--self-upgrade` flag is omitted from Docker builds, since containers should be upgraded by replacing the image (`docker pull`).

## Dashboard

Visit `http://your-server:3000/dashboard` to see:
- **Overview** - Total visitors, pageviews, bounce rate, session duration
- **Pages** - Which pages get the most traffic
- **Referrers** - Where your visitors come from
- **Browsers/Devices** - What devices people use
- **Locations** - Map showing visitor countries and cities
- **Campaigns** - UTM campaign parameter analytics
- **Real-time** - Live visitor activity (updates every few seconds)

## UTM Campaign Tracking

Kaunta automatically tracks UTM campaign parameters from your URLs. When visitors arrive via links with UTM parameters, Kaunta captures and stores:

- `utm_source` - Traffic source (e.g., google, newsletter)
- `utm_medium` - Marketing medium (e.g., cpc, email, social)
- `utm_campaign` - Campaign name (e.g., spring_sale, product_launch)
- `utm_term` - Paid search keywords
- `utm_content` - A/B test or ad variant identifier

### How It Works

1. **Automatic extraction** - The tracker script extracts UTM parameters from the URL
2. **Session persistence** - UTM values are stored in sessionStorage and persist across page navigation
3. **Dashboard view** - Visit `/dashboard/campaigns` to see breakdowns by each UTM dimension

### Example URLs

```
https://yoursite.com?utm_source=google&utm_medium=cpc&utm_campaign=spring_sale
https://yoursite.com?utm_source=newsletter&utm_medium=email&utm_campaign=weekly_digest
https://yoursite.com?utm_source=twitter&utm_medium=social&utm_campaign=product_launch
```

No additional configuration needed - just use standard UTM parameters in your marketing links.

## Pixel Tracking (No JavaScript Required)

For environments where JavaScript doesn't run (emails, RSS feeds, bots), use the pixel tracking endpoint:

```html
<!-- Basic pixel -->
<img src="https://analytics.example.com/p/YOUR-WEBSITE-ID.gif"
     width="1" height="1" alt="" />

<!-- With UTM parameters for email campaigns -->
<img src="https://analytics.example.com/p/YOUR-WEBSITE-ID.gif?name=email_open&utm_source=mailchimp&utm_campaign=welcome"
     width="1" height="1" style="display:none" />

<!-- RSS feed tracking -->
<img src="https://analytics.example.com/p/YOUR-WEBSITE-ID.gif?url=https://blog.example.com/post-title"
     width="1" height="1" />

<!-- No-JS fallback -->
<noscript>
  <img src="https://analytics.example.com/p/YOUR-WEBSITE-ID.gif"
       width="1" height="1" alt="" />
</noscript>
```

### Query Parameters

All parameters are optional except the website ID (in the URL path):

- `url` - Page URL being tracked (falls back to Referer header)
- `title` - Page title
- `name` - Custom event name (default: pageview)
- `utm_source`, `utm_medium`, `utm_campaign`, `utm_term`, `utm_content` - Campaign tracking

### What Gets Tracked

The pixel automatically captures:
- **IP address** - For GeoIP lookup (country, city, region)
- **User-Agent** - Browser, OS, device detection
- **Referrer** - From HTTP Referer header
- **Language** - From Accept-Language header
- **Bot detection** - Same PostgreSQL-based detection as JS tracking

### Use Cases

- **Email campaigns** - Track email opens and click-throughs
- **RSS feeds** - Monitor RSS reader engagement
- **AMP pages** - Google AMP compatible tracking
- **Server-side events** - Track backend processes, API calls, webhooks
- **Bot/crawler analytics** - Track search engine crawlers and bots

The pixel tracking shares the same session ID algorithm as JavaScript tracking, so visitors are tracked consistently across both methods.

## Server-Side Ingest API

For backend applications (Rails, Node, Python, etc.) that need to push analytics events programmatically, Kaunta provides a server-side ingestion API. This is useful for:

- **E-commerce** - Track purchases, cart events from your backend
- **SaaS applications** - Track user actions server-side
- **Mobile apps** - Send events from your API server
- **IoT devices** - Track events from any HTTP-capable device

### Create an API Key

```bash
kaunta apikey create example.com --name "Rails Backend"

# Output:
# API Key: kaunta_live_284ded8452e244a23a9ea9a33e9432986fc350a8792c34a1caf2dff1cb742473
# IMPORTANT: Save this key now. It will NOT be shown again.
```

### Send Events

```bash
# Single event
curl -X POST https://your-kaunta-server/api/ingest \
  -H "Authorization: Bearer kaunta_live_..." \
  -H "Content-Type: application/json" \
  -d '{
    "event": "page_view",
    "visitor_id": "user_123",
    "url": "/products"
  }'

# Custom event with properties
curl -X POST https://your-kaunta-server/api/ingest \
  -H "Authorization: Bearer kaunta_live_..." \
  -H "Content-Type: application/json" \
  -d '{
    "event": "purchase",
    "visitor_id": "user_123",
    "properties": {
      "amount": 99.99,
      "product_id": "SKU-123"
    }
  }'

# Batch (up to 100 events)
curl -X POST https://your-kaunta-server/api/ingest/batch \
  -H "Authorization: Bearer kaunta_live_..." \
  -H "Content-Type: application/json" \
  -d '{
    "events": [
      {"event": "page_view", "visitor_id": "v1", "url": "/page1"},
      {"event": "click", "visitor_id": "v1", "properties": {"button": "buy"}}
    ]
  }'
```

### Payload Schema

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `event` | string | Yes | Event name (e.g., `page_view`, `purchase`) |
| `visitor_id` | string | Yes | Unique visitor identifier |
| `url` | string | For `page_view` | Page URL |
| `properties` | object | No | Custom event properties (max 100KB) |
| `user_id` | string | No | Authenticated user ID |
| `session_id` | string | No | Custom session ID |
| `event_id` | string | No | UUID for idempotency |
| `timestamp` | integer | No | Unix timestamp (within ±30 days) |
| `utm_source`, `utm_medium`, `utm_campaign`, `utm_term`, `utm_content` | string | No | UTM parameters |
| `context.locale` | string | No | User locale (e.g., `en-US`) |
| `context.screen` | string | No | Screen resolution |

### API Key Management

```bash
kaunta apikey list example.com      # List keys for a website
kaunta apikey show kaunta_live_...  # Show key details
kaunta apikey revoke kaunta_live_...  # Revoke a key
```

### Security Notes

- API keys use SHA256 hashing (secure for high-entropy tokens)
- IP/User-Agent resolved from request headers (GDPR compliant)
- Rate limited per key (default 1000 req/min) and per website (default 5000 req/min)
- Idempotency support via `event_id` (7-day deduplication window)

## Public Stats API

Expose real-time stats (online users, pageviews, visitors) via API for widgets and dashboards.

| Endpoint | Auth | Description |
|----------|------|-------------|
| `GET /api/v1/stats/:website_id` | API key (`stats` scope) | Always available |
| `GET /api/public/stats/:website_id` | None | Opt-in per website |

```bash
# Create API key with stats scope
kaunta apikey create example.com --scope stats --name "Stats Widget"

# Enable public stats (no auth required)
kaunta website enable-public-stats example.com
```

See [PUBLIC_STATS.md](PUBLIC_STATS.md) for full documentation and Astro integration examples.

## Umami Compatible

Drop-in replacement for Umami. Works with Umami's JavaScript tracker and seamlessly migrates existing databases:
- Compatible tracking API
- Umami's JS tracker just works
- Auto-migrates existing Umami databases on startup
- Enhanced with bot detection and advanced analytics

## License

MIT - Simple, fast analytics for everyone.
