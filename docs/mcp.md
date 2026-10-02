# MCP integration

Kaunta exposes analytics and controlled administration tools over Streamable HTTP at `/mcp`.

## Connect

The endpoint is off by default. Over MCP a CLI-minted key has operator
reach over every website, which is more than the same key can do on the
plain HTTP API, so an instance that does not use agents should not carry
it. Turn it on with `mcp = true` in `kaunta.toml` (or `MCP_ENABLED=true`)
and restart; while it is off, `/mcp` does not exist and returns 404.

The endpoint is served on the same host and port as Kaunta and requires an
API key in a Bearer authorization header. For Claude Code, add a
project `.mcp.json`:

```json
{
  "mcpServers": {
    "kaunta": {
      "type": "http",
      "url": "https://analytics.example.com/mcp",
      "headers": {
        "Authorization": "Bearer YOUR_KAUNTA_API_KEY"
      }
    }
  }
}
```

The server supports Streamable HTTP requests at `/mcp` (POST for JSON-RPC,
GET for SSE, DELETE for session handling).

## Keys and access

Create an API key from the CLI:

```sh
kaunta apikey create example.com --name mcp-operator
```

The CLI prints the secret once; store it securely. CLI-minted keys have
operator scope and can access all websites. A key is the only way in:
there is no transport that accepts database credentials instead, so a
client never needs more access than the key you issued it. API keys created in the dashboard
are associated with their creator and can access that user’s websites and
shared websites. Both kinds authenticate using `Authorization: Bearer <key>`.

## Tools

The server registers 30 tools.

**Read tools**

- `list_websites`
- `website_stats`
- `timeseries`
- `breakdown` (dimensions include `event` for custom event names)
- `props_breakdown` (property keys and values of a custom event)
- `country_stats`
- `period_overview`
- `list_goals`
- `goal_stats`
- `list_exclusions` (addresses whose traffic is discarded)
- `overview_panel`, `realtime_panel`, `map_panel`, `goals_panel` (render the interactive views below)

**Write and operator tools**

- Goals: `create_goal`, `update_goal`, `delete_goal`
- Websites: `create_website`, `update_website`, `delete_website`, `restore_website`
- Exclusions: `add_exclusion`, `remove_exclusion`
- Backup: `create_backup`

Website and goal write operations, restore, and backup require MCP elicitation
confirmation from the client. A client without elicitation support cannot
perform these operations; the server refuses them. HTTP dashboard-minted keys
are scoped to the creator, and management/backup tools are operator-only.

## Interactive views (MCP Apps)

Kaunta implements the [MCP Apps](https://github.com/modelcontextprotocol/ext-apps)
extension. A host that advertises it renders these tools as interactive
panels instead of plain JSON:

| Tool | View | What it shows |
|---|---|---|
| `overview_panel` | `ui://kaunta/overview` | Headline numbers, pageviews over time with a hover readout, top pages and sources |
| `realtime_panel` | `ui://kaunta/realtime` | Visitors online, today's pageviews, and the latest events as they arrive |
| `map_panel` | `ui://kaunta/map` | A world choropleth of visitors by country, with a ranked table |
| `goals_panel` | `ui://kaunta/goals` | Goals, how each is converting, and the controls to add or remove one |

Each view is a single self-contained HTML resource with no external origins,
so the default deny-all CSP applies unchanged. Each pairs with an app-only
tool the view may call but the model may not see: `overview_panel_refresh`
and `map_panel_refresh` for the period buttons, and `realtime_panel_poll`,
which the live view calls on a ten-second timer. So changing period, or watching traffic arrive, costs
no model tokens after the panel opens. The live view stops polling while it
is hidden and on teardown, and its auto-refresh can be switched off.

The goal panel is the one view that writes. Its add and remove buttons call
app-only tools that are still elicitation-gated and operator-only, so a
click in the view asks for the same confirmation a model-issued
`create_goal` would. A client without elicitation support is refused and
nothing is written.

The map carries its country outlines as pre-projected SVG paths, generated
by `scripts/build_map_paths.rb` from the same world atlas the dashboard
uses. Regenerate them if that atlas is ever replaced; nothing else reads
the file.

Hosts without the extension are unaffected: they call `overview_panel` or
`realtime_panel`, receive the same structured JSON, and never see the
app-only tools.

To advertise support, a client sends this during `initialize`:

```json
{"capabilities": {"extensions": {"io.modelcontextprotocol/ui": {"mimeTypes": ["text/html;profile=mcp-app"]}}}}
```

## Website deletion and restoration

Deletion policy is enforced by a PostgreSQL trigger. A website without event
data can be deleted immediately. A website with events enters pending deletion
for 30 days before deletion can complete. `delete_website` can also cancel a
pending deletion, and `restore_website` restores a soft-deleted website.

## Backups

`create_backup` generates either a full database backup or a period archive.
Its result includes a `download_path` such as `/backups/NAME-SHA256...`.
Request that path on the same Kaunta host serving MCP. The SHA-256-suffixed
filename is the capability: the download endpoint does not require
authentication, does not list files, and access ends when the server removes
the temporary file after roughly one hour. Treat the returned path as a
secret.
