FROM oven/bun:alpine AS frontend-builder
WORKDIR /app

COPY package.json bun.lock ./
RUN bun install --frozen-lockfile

COPY tracker/ ./tracker/
COPY frontend/ ./frontend/
RUN bun run build

FROM golang:1.25-alpine AS backend-builder
WORKDIR /app

RUN apk add --no-cache git ca-certificates tzdata

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=frontend-builder /app/cmd/kaunta/assets ./cmd/kaunta/assets

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build \
    -tags=docker \
    -ldflags="-w -s" \
    -o kaunta \
    ./cmd/kaunta

FROM alpine:latest

ARG VERSION=0.6.1
LABEL org.opencontainers.image.title="Kaunta" \
      org.opencontainers.image.description="Privacy-focused analytics engine. Analytics without bloat." \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.authors="Abdelkader Boudih" \
      org.opencontainers.image.source="https://github.com/seuros/kaunta" \
      org.opencontainers.image.url="https://github.com/seuros/kaunta" \
      org.opencontainers.image.documentation="https://github.com/seuros/kaunta" \
      org.opencontainers.image.vendor="Seuros" \
      org.opencontainers.image.licenses="MIT"

RUN apk add --no-cache ca-certificates tzdata

COPY --from=backend-builder /app/kaunta /kaunta

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD ["/kaunta", "healthcheck"]

EXPOSE 3000
ENTRYPOINT ["/kaunta"]
