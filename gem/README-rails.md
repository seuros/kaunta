# kaunta-rails

Send events from a Rails application to [Kaunta](https://github.com/seuros/kaunta),
self-hosted privacy-first web analytics.

**This version reserves the name. The client is not implemented yet.**

It will wrap Kaunta's ingestion API (`/api/ingest` and
`/api/ingest/batch`) for the cases the browser tracker cannot cover:
background jobs, API-only endpoints, and server-rendered flows.

To run a Kaunta server, install the `kaunta` gem instead; it ships the
native binary and bundles no Ruby client.
