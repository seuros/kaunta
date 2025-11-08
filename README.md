# Kaunta

Analytics without bloat.

A simple, fast, privacy-focused web analytics engine. Drop-in replacement for Umami.

## Features

- **Privacy-First** - No cookies, no tracking, privacy by design
- **Fast Deployment** - Single binary, ready to run
- **Lightweight** - Minimal memory footprint
- **Umami Compatible** - Same API & database schema
- **Full Analytics** - Visitors, pageviews, referrers, devices, locations, real-time stats
- **Geolocation** - City/region level with automatic MaxMind GeoLite2 download

## Installation

### 1. Configuration

Kaunta requires PostgreSQL 17+. You can configure it using:

**Option 1: Config file** (recommended)

Create `kaunta.toml` in current directory or `~/.kaunta/kaunta.toml`:

```toml
database_url = "postgresql://user:password@localhost:5432/kaunta"
port = "3000"
data_dir = "./data"
```

**Option 2: Environment variables**

```bash
export DATABASE_URL="postgresql://user:password@localhost:5432/kaunta"
export PORT="3000"
export DATA_DIR="./data"
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

### 3. Add Tracker Script

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
```

This will:
- Update the user's password
- Invalidate all existing sessions (user must log in again)

### Access the Dashboard

After creating a user:
1. Navigate to `http://your-server:3000/login`
2. Log in with the username and password
3. You'll be redirected to `/dashboard`

Sessions last 7 days and use HTTP-only cookies for security.

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
- **Real-time** - Live visitor activity (updates every few seconds)

## Umami Compatible

Drop-in replacement for Umami. Works with Umami's JavaScript tracker and seamlessly migrates existing databases:
- Compatible tracking API
- Umami's JS tracker just works
- Auto-migrates existing Umami databases on startup
- Enhanced with bot detection and advanced analytics

## License

MIT - Simple, fast analytics for everyone.
