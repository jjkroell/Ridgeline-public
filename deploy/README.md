# Deploying Ridgeline

Ridgeline ships as a small Docker Compose stack:

```
observer nodes ──► mosquitto (MQTT/WebSockets) ──► ridgelined ──► caddy ──► the web
```

- **mosquitto** — the broker your MeshCore observer nodes publish packets to.
  (Skip this and point Ridgeline at an existing broker if you already run one.)
- **ridgelined** — the Go daemon: ingests packets, stores them in SQLite, and
  serves the API + built web app.
- **caddy** — reverse proxy and (optionally) automatic HTTPS for your domain.

## Quick start

The easiest path is the interactive installer in the repo root, which asks about
your broker, region, site name, email and domain, then writes every file below
and builds the stack:

```bash
./setup.sh
```

## Manual deploy

If you'd rather configure by hand:

1. **Config** — copy the example and fill it in:

   ```bash
   cp deploy/config.example.json deploy/config.json
   # edit deploy/config.json: set your MQTT broker/topics and (optionally) the
   # email block for account verification. (There's no admin token — the first
   # account you register on the site becomes the owner/admin.)
   ```

   For an **authenticated broker**, set `mqtt.username` / `mqtt.password` (they're
   in the example as empty placeholders; leave them empty for an anonymous broker).

2. **User ids** — so the bind-mounted database directory is writable:

   ```bash
   printf 'RIDGELINE_UID=%s\nRIDGELINE_GID=%s\n' "$(id -u)" "$(id -g)" > deploy/.env
   ```

3. **Domain / TLS** — edit `deploy/Caddyfile`:
   - Public server with a domain: change the first line from `:80` to your
     hostname (e.g. `mesh.example.com`) and Caddy will fetch a certificate
     automatically.
   - Behind Cloudflare Tunnel or another TLS proxy: leave it as `:80`.

4. **Broker** — `deploy/mosquitto.conf` is anonymous by default. Keep it if the
   WebSocket listener is only reachable over TLS; add auth if you expose it.

5. **Launch**:

   ```bash
   cd deploy
   docker compose up -d --build
   docker compose logs -f ridgelined   # watch packets land
   ```

## Running alongside existing infrastructure (external broker / your own proxy)

If you already run a MeshCore broker and a reverse proxy — e.g. you want several
analyzers on one broker — skip the bundled mosquitto + Caddy and run **just the
daemon** with `deploy/docker-compose.external.yml`:

```bash
cd deploy
cp config.example.json config.json     # set mqtt.broker to your broker;
                                       # set mqtt.username/password if it needs auth
printf 'RIDGELINE_UID=%s\nRIDGELINE_GID=%s\n' "$(id -u)" "$(id -g)" > .env
docker compose -f docker-compose.external.yml up -d --build
```

That variant **publishes the app on `:8080`** (the stock stack doesn't — Caddy
fronts it internally), so point your own proxy/TLS at `localhost:8080`. Give each
analyzer sharing a broker a unique `mqtt.clientID`.

## Health & version

The image ships a `HEALTHCHECK` (the daemon self-probes `/api/health`, since the
distroless runtime has no shell/curl), so `docker ps` shows `(healthy)`. Stamp the
build with a version by setting `RIDGELINE_VERSION` (e.g. a git tag) in `deploy/.env`
before `--build`; otherwise it logs `version=dev`. Check the running version with
`docker exec ridgeline-app /app/ridgelined -version`.

## What must never be committed

`deploy/.gitignore` excludes the files that hold secrets or runtime state:
`config.json`, `.env`, `passwd`, and `data/`. Only the `*.example.json`
templates belong in git.

## Updating

```bash
cd deploy
docker compose up -d --build   # rebuilds ridgelined from the current source
```

Your database in `deploy/data/` and Caddy's certificates persist across rebuilds.
