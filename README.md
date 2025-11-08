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

### 1. Set Up Database

Kaunta requires PostgreSQL 17+:

```bash
export DATABASE_URL="postgresql://user:password@localhost:5432/kaunta"
```

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
