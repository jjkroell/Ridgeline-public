# syntax=docker/dockerfile:1

# ── Web build (SvelteKit static SPA) ───────────────────────────────────────
FROM node:22-bookworm-slim AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# ── Go build (pure-Go, CGO disabled — modernc sqlite needs no C toolchain) ──
FROM golang:1.26-bookworm AS go
WORKDIR /src
# Stamp the build with a version (e.g. a git tag/sha). Defaults to "dev".
# Pass with: docker build --build-arg VERSION=$(git describe --tags --always)
# or via compose build.args (RIDGELINE_VERSION).
ARG VERSION=dev
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/ridgelined ./cmd/ridgelined

# ── Runtime (distroless static; nonroot) ───────────────────────────────────
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=go /out/ridgelined /app/ridgelined
COPY --from=web /web/build /app/web/build
EXPOSE 8080
# The distroless image has no shell/curl, so the daemon health-probes itself
# (see the -healthcheck flag). Surfaces as "(healthy)" in `docker ps`.
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
	CMD ["/app/ridgelined", "-healthcheck", "-config", "/config/config.json"]
ENTRYPOINT ["/app/ridgelined", "-config", "/config/config.json"]
