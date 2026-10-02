# kaunta

Self-hosted, privacy-first web analytics. This gem ships the Kaunta
server and CLI as a native binary, so installing it needs no Rust
toolchain:

```sh
gem install kaunta
export DATABASE_URL='postgresql://kaunta:kaunta@localhost:5432/kaunta?sslmode=disable'
kaunta serve
```

Then open `http://localhost:3000/setup`.

The gem is a thin launcher: `exe/kaunta` executes the bundled binary in
`libexec/`. Platform gems are published for Linux and macOS on x86_64 and
arm64; the plain-Ruby gem is a stub that tells you which platform release
to install.

Everything else (configuration, the tracker, MCP, self-hosting) is
documented in the [main repository](https://github.com/seuros/kaunta).

To send events from a Ruby application, use the `kaunta-rails` gem
instead; it talks to a Kaunta instance over HTTP and bundles no binary.
