# Kaunta browser acceptance tests

The six tests in `tests/dashboard.rs` are ignored by default. Run them with:

```sh
cargo test -p kaunta-e2e --test dashboard -- --ignored --nocapture
```

They require PostgreSQL with database creation privileges and a local Chrome
installation. Set `KAUNTA_E2E_ADMIN_URL` to override the default
`postgresql://ubuntu:ubuntu@localhost:5432/ubuntu?sslmode=disable`, and `CHROME` to override
`/Applications/Google Chrome.app/Contents/MacOS/Google Chrome`.
Each scenario migrates and seeds its own disposable database, runs the real
server in-process, and drops the database afterward. Failed scenarios write
screenshots under `target/e2e-artifacts/`.
