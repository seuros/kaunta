# Tracker reference

Kaunta’s browser tracker records page activity without cookies and supports browser, pixel, and API-based collection.

## Install

Use the website ID shown for the site in Kaunta:

```html
<script async src="https://analytics.example.com/k.js"
  data-website-id="YOUR-WEBSITE-ID"></script>
```

The same script is available at `/kaunta.js` and `/script.js`. `data-api-url`
can point collection requests at a different Kaunta base URL. Configure the
website’s allowed domains to include the page’s origin.

## Script attributes

| Attribute | Required | Default | Behavior |
| --- | --- | --- | --- |
| `data-website-id` | Yes | None | Website UUID used to attribute events. Tracking is disabled without it. |
| `data-api-url` | No | Base URL derived from the script URL | Kaunta server base URL; the tracker appends `/api/send`. |
| `data-auto-track` | No | `true` | Automatically send initial and SPA pageviews and initialize engagement/link tracking. |
| `data-track-outbound` | No | `true` | Track clicks on links whose host differs from the current host. |
| `data-track-downloads` | No | `true` | Track links whose path ends with a configured download extension. |
| `data-download-extensions` | No | `pdf,xlsx,docx,txt,rtf,csv,exe,key,pps,ppt,pptx,7z,pkg,rar,gz,zip,avi,mov,mp4,mpeg,wmv,midi,mp3,wav,ogg,dmg` | Comma-separated, case-insensitive file extensions; whitespace is trimmed. |
| `data-respect-dnt` | No | `true` | Suppress tracking when Do Not Track is enabled. |
| `data-exclude-hash` | No | `false` | Remove URL fragments from tracked URLs. |
| `data-domains` | No | Empty (no domain restriction) | Comma-separated hostname allowlist. When set, tracking runs only on a listed hostname; ports are stripped from entries. |
| `data-debug` | No | `false` | Write tracker diagnostics to the browser console. |

Attributes take effect when equal to the indicated string values: for example,
set boolean options to `"false"` to disable their default-on behavior.

## JavaScript API

When the script initializes, it adds methods to `window.kaunta`:

```js
window.kaunta.track("signup", { plan: "team" });
window.kaunta.trackPageview();
window.kaunta.destroy();
```

`track(name, properties)` sends a named custom event; `properties` is optional
and must be an object to be included. `trackPageview()` sends a pageview using
the current tracker URL state. `destroy()` removes tracker listeners, disconnects
the resize observer, and cancels a pending pageview.

## Automatic behavior

- Pageviews include SPA route changes detected through `pushState`,
  `replaceState`, and `popstate`.
- Outbound clicks send an `Outbound Link: Click` event with the normalized URL.
- Matching file links send a `File Download` event. For a regular same-tab
  click, navigation is delayed up to 500 ms to allow the event to send. New-tab,
  modified, or already-prevented clicks are not intercepted.
- Scroll depth and focused/visible engagement time are attached to pageview
  data and accumulated engagement is sent when the page becomes hidden.
- `utm_source`, `utm_medium`, `utm_campaign`, `utm_term`, and `utm_content`
  are read from the URL and retained in `sessionStorage` for the browser
  session. Kaunta does not set cookies or use `localStorage`.

## Opting a browser out

Opening any tracked page with `?kaunta_ignore=true` stores a flag in that
browser's `localStorage`; the tracker then records nothing from it, on any
site served by that Kaunta instance. `?kaunta_ignore=false` clears it. This
is how an operator excludes their own visits without a fixed IP address;
see [self-hosting](self-hosting.md) for address-based exclusions.

## Do Not Track and request privacy

By default, the tracker respects browser Do Not Track signals (`doNotTrack`,
`navigator.doNotTrack`, and `msDoNotTrack` values recognized as `1` or `yes`).
Set `data-respect-dnt="false"` to opt out of that check. The tracker sends
cross-origin fetches with `credentials: "omit"`; same-origin requests use
same-origin credentials. It uses `sendBeacon` while the document is hidden.

## Pixel tracking

For environments without JavaScript, request the one-pixel GIF:

```html
<img src="https://analytics.example.com/p/YOUR-WEBSITE-ID.gif"
  alt="" width="1" height="1">
```

The server also accepts optional query parameters `url`, `hostname`,
`referrer`, `title`, `name`, `tag`, and the five UTM fields. Without `url`, it
uses the request referrer when available. The response is a GIF even when
tracking cannot be recorded.

## Ingestion API

`POST /api/ingest` accepts one event; `POST /api/ingest/batch` accepts an
`events` array. Both require an API key with the `ingest` scope. A UUID
`event_id` can be supplied for idempotency. Batch requests accept at most 100
events.

```sh
curl -i 'https://analytics.example.com/api/ingest' \
  -H 'Authorization: Bearer YOUR_INGEST_API_KEY' \
  -H 'Content-Type: application/json' \
  -d '{
    "event": "page_view",
    "visitor_id": "visitor-123",
    "event_id": "9dc7e92a-0c9e-4fc3-8eb8-a7bb3b288d04",
    "url": "https://www.example.com/pricing",
    "title": "Pricing",
    "referrer": "https://search.example/",
    "utm_source": "newsletter"
  }'
```

```sh
curl -i 'https://analytics.example.com/api/ingest/batch' \
  -H 'Authorization: Bearer YOUR_INGEST_API_KEY' \
  -H 'Content-Type: application/json' \
  -d '{
    "events": [
      {
        "event": "page_view",
        "visitor_id": "visitor-123",
        "event_id": "9dc7e92a-0c9e-4fc3-8eb8-a7bb3b288d04",
        "url": "https://www.example.com/"
      },
      {
        "event": "signup",
        "visitor_id": "visitor-123",
        "url": "https://www.example.com/signup"
      }
    ]
  }'
```

Use `kaunta apikey create example.com --name collector --scope ingest` to
create an ingestion key. See [self-hosting](self-hosting.md) for operational
and security configuration.
