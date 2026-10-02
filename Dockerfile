FROM rust:1.99.0-alpine AS builder

# musl-dev links the static binary; the rest is what the crypto crates
# need to compile their C shims.
RUN apk add --no-cache musl-dev build-base

WORKDIR /app

COPY Cargo.toml Cargo.lock rust-toolchain.toml ./
COPY crates ./crates
COPY assets ./assets
COPY migrations ./migrations
COPY tracker ./tracker

RUN cargo build --locked --release -p kaunta

FROM alpine:latest

ARG VERSION=dev
LABEL org.opencontainers.image.title="Kaunta" \
      org.opencontainers.image.description="Privacy-focused analytics engine. Analytics without bloat." \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.authors="Abdelkader Boudih" \
      org.opencontainers.image.source="https://github.com/seuros/kaunta" \
      org.opencontainers.image.url="https://github.com/seuros/kaunta" \
      org.opencontainers.image.documentation="https://github.com/seuros/kaunta" \
      org.opencontainers.image.vendor="Seuros" \
      org.opencontainers.image.licenses="MIT"

# postgresql-client supplies the pg_dump and pg_restore that
# `kaunta backup` shells out to; without them backups fail in the image.
RUN apk add --no-cache ca-certificates tzdata postgresql-client \
    && addgroup -S kaunta \
    && adduser -S -G kaunta -h /var/lib/kaunta kaunta \
    && install -d -o kaunta -g kaunta /var/lib/kaunta

COPY --from=builder /app/target/release/kaunta /usr/local/bin/kaunta

ENV DATA_DIR=/var/lib/kaunta \
    PORT=3000

USER kaunta

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD ["kaunta", "healthcheck"]

EXPOSE 3000
ENTRYPOINT ["kaunta"]
