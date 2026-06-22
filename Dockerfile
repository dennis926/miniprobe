# =============================================================================
# MiniProbe Server — Multi-stage Docker Build
# =============================================================================
# Build:   docker build -t miniprobe-server .
# Run:     docker run -d -p 8080:8080 miniprobe-server
# Custom:  docker run -d -p 8080:8080 miniprobe-server -token mysecret
# =============================================================================

# ── Stage 1: build ────────────────────────────────────────────────────────────
FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY server/ .

# Download deps and compile a static binary (no CGO, no libc dependency)
RUN go mod tidy && \
    CGO_ENABLED=0 GOOS=linux go build \
      -ldflags="-s -w" \
      -o /miniprobe-server \
      .

# ── Stage 2: minimal runtime ──────────────────────────────────────────────────
FROM alpine:3.19

RUN apk add --no-cache ca-certificates

COPY --from=builder /miniprobe-server /usr/local/bin/miniprobe-server

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD wget -qO- http://localhost:8080/api/agents || exit 1

ENTRYPOINT ["/usr/local/bin/miniprobe-server"]
CMD ["-port", "8080", "-token", "miniprobe"]
