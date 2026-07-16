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
   # edit deploy/config.json: set a strong adminToken, your MQTT broker/topics,
   # and (optionally) the email block for account verification.
   ```

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
