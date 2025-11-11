FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata bash curl libstdc++ libgcc
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ENV BUN_INSTALL=/root/.bun
ENV PATH="/root/.bun/bin:$PATH"

RUN curl -fsSL https://bun.sh/install | bash
RUN bun install --frozen-lockfile
RUN bun run build

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

COPY --from=builder /app/kaunta /kaunta
# All assets and templates are embedded in the binary via //go:embed directives
# No need to copy templates/ or assets/ directories

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD ["/kaunta", "healthcheck"]

EXPOSE 3000
ENTRYPOINT ["/kaunta"]
