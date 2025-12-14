# Public Stats API

Kaunta provides two ways to access website statistics programmatically:

1. **API Key (SSR)**: Always available, requires authentication
2. **Public Endpoint (SPA)**: Opt-in per website, no authentication required

## Response Format

Both endpoints return the same JSON structure:

```json
{
  "online": 42,
  "pageviews": 12345,
  "visitors": 5678
}
```

| Field | Description |
|-------|-------------|
| `online` | Active visitors in the last 5 minutes |
| `pageviews` | Total pageviews (all time) |
| `visitors` | Unique visitors (all time) |

---

## API Key Endpoint (SSR)

**Endpoint**: `GET /api/v1/stats/:website_id`

Always available. Requires an API key with the `stats` scope.

### Creating an API Key

```bash
# Create key with stats scope
kaunta apikey create example.com --scope stats --name "Stats Reader"

# Create key with both ingest and stats scopes
kaunta apikey create example.com --scope ingest,stats --name "Full Access"
```

### Usage

```bash
curl -H "Authorization: Bearer kaunta_live_xxx..." \
  https://your-kaunta-host/api/v1/stats/YOUR_WEBSITE_ID
```

### Astro SSR Example

```astro
---
// src/pages/stats.astro
const KAUNTA_API_KEY = import.meta.env.KAUNTA_API_KEY;
const WEBSITE_ID = import.meta.env.PUBLIC_KAUNTA_WEBSITE_ID;

const response = await fetch(
  `${import.meta.env.KAUNTA_URL}/api/v1/stats/${WEBSITE_ID}`,
  {
    headers: {
      Authorization: `Bearer ${KAUNTA_API_KEY}`,
    },
  }
);

const stats = await response.json();
---

<div class="stats-widget">
  <div class="stat">
    <span class="value">{stats.online}</span>
    <span class="label">Online Now</span>
  </div>
  <div class="stat">
    <span class="value">{stats.visitors.toLocaleString()}</span>
    <span class="label">Total Visitors</span>
  </div>
  <div class="stat">
    <span class="value">{stats.pageviews.toLocaleString()}</span>
    <span class="label">Total Pageviews</span>
  </div>
</div>
```

---

## Public Endpoint (SPA)

**Endpoint**: `GET /api/public/stats/:website_id`

No authentication required, but must be explicitly enabled per website.

### Enabling Public Stats

**CLI:**
```bash
kaunta website enable-public-stats example.com
kaunta website disable-public-stats example.com
```

**Dashboard:**
Navigate to Websites → find your website → toggle "Public Stats"

### Usage

```bash
curl https://your-kaunta-host/api/public/stats/YOUR_WEBSITE_ID
```

Returns `404 Not Found` if public stats are disabled for the website.

### Astro SPA Example

```astro
---
// src/components/StatsWidget.astro
const WEBSITE_ID = import.meta.env.PUBLIC_KAUNTA_WEBSITE_ID;
const KAUNTA_URL = import.meta.env.PUBLIC_KAUNTA_URL;
---

<div id="stats-widget" class="stats-widget">
  <div class="stat">
    <span class="value" id="online">-</span>
    <span class="label">Online Now</span>
  </div>
  <div class="stat">
    <span class="value" id="visitors">-</span>
    <span class="label">Total Visitors</span>
  </div>
  <div class="stat">
    <span class="value" id="pageviews">-</span>
    <span class="label">Total Pageviews</span>
  </div>
</div>

<script define:vars={{ WEBSITE_ID, KAUNTA_URL }}>
  async function loadStats() {
    try {
      const response = await fetch(
        `${KAUNTA_URL}/api/public/stats/${WEBSITE_ID}`
      );
      if (!response.ok) {
        throw new Error("Stats not available");
      }
      const stats = await response.json();

      document.getElementById("online").textContent = stats.online;
      document.getElementById("visitors").textContent =
        stats.visitors.toLocaleString();
      document.getElementById("pageviews").textContent =
        stats.pageviews.toLocaleString();
    } catch (err) {
      console.error("Failed to load stats:", err);
    }
  }

  // Load immediately and refresh every 30 seconds
  loadStats();
  setInterval(loadStats, 30000);
</script>

<style>
  .stats-widget {
    display: flex;
    gap: 2rem;
    padding: 1rem;
  }
  .stat {
    display: flex;
    flex-direction: column;
    align-items: center;
  }
  .value {
    font-size: 2rem;
    font-weight: bold;
  }
  .label {
    font-size: 0.875rem;
    color: #666;
  }
</style>
```

---

## CORS

The public endpoint includes CORS headers:

```
Access-Control-Allow-Origin: *
Access-Control-Allow-Methods: GET, OPTIONS
Access-Control-Allow-Headers: Content-Type
```

This allows the endpoint to be called from any origin (client-side JavaScript).

---

## Rate Limiting

Both endpoints respect the website's configured rate limits. The public endpoint may have stricter limits to prevent abuse.

---

## Security Considerations

1. **API Key Endpoint**: Keep your API key secret. Use environment variables, never expose in client-side code.

2. **Public Endpoint**: Only enable if you want your stats visible to anyone. The endpoint exposes aggregate data only (no individual user data).

3. **Website ID**: The website ID is a UUID and is considered semi-public (it's in your tracking script). However, stats are only accessible if:
   - You have an API key with `stats` scope, OR
   - Public stats are explicitly enabled for that website
