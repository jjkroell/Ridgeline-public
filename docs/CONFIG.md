# Configuration reference

Everything region-specific in Ridgeline is supplied by **you**. The interactive
installer (`./setup.sh`) collects it and writes the files below; you can also edit
them by hand and rebuild. Nothing points back at any particular mesh or server.

## Where settings live

| File | Committed? | What it holds |
|------|-----------|---------------|
| `deploy/config.json` | **No** (gitignored) | Runtime secrets: admin token, MQTT broker + credentials, SMTP block |
| `deploy/.env` | **No** (gitignored) | Host `RIDGELINE_UID`/`GID` for the container |
| `deploy/Caddyfile` | Yes | Site address / TLS (a domain, or `:80` behind a proxy) |
| `deploy/mosquitto.conf` | Yes | Bundled broker (anonymous by default) |
| `web/.env` | **No** (gitignored) | Build-time `VITE_*` values (site name, region, map center) |
| `web/src/lib/site.ts` | Yes | Reads the `VITE_*` values, with defaults |
| `web/src/lib/site-content.ts` | Yes | Radio settings table + About page content |

> The frontend values are **build-time** constants — Vite inlines them when the
> web app is built. After changing `web/.env` or `web/src/lib/site-content.ts`,
> rebuild (`cd deploy && docker compose up -d --build`, or `cd web && npm run build`).

## Frontend (`web/.env`)

| Variable | Default | Purpose |
|----------|---------|---------|
| `VITE_SITE_NAME` | `Ridgeline` | Name in the header, tab title, SEO |
| `VITE_SITE_TAGLINE` | `MeshCore Observatory` | Line under the wordmark |
| `VITE_SITE_URL` | *(empty)* | Absolute origin for canonical/social URLs |
| `VITE_SITE_DESCRIPTION` | generic | SEO description fallback |
| `VITE_PRIVACY_CONTACT` | *(empty)* | Data-controller email shown on `/privacy` |
| `VITE_MAP_CENTER_LAT` | `20` | Default map latitude |
| `VITE_MAP_CENTER_LON` | `0` | Default map longitude |
| `VITE_MAP_ZOOM` | `3` | Default map zoom (1 world → 12 city) |

`VITE_ALLOWED_HOSTS` (comma-separated) can also be set for the dev server when
reaching it through a reverse proxy.

## About page (`web/src/lib/site-content.ts`)

- `ABOUT.enabled` — set `false` to remove the `/about` page and its nav link.
- `ABOUT.kicker` / `title` / `intro` / `sections[]` / `footer` — your words only.
- `ABOUT.showRadio` + `RADIO_PARAMS` — the LoRa settings table (frequency,
  bandwidth, spreading factor, coding rate). Empty `RADIO_PARAMS` hides it.

## Privacy & cookie consent

The app ships a GDPR/PIPEDA cookie-consent banner and a `/privacy` page out of the
box. By default it sets only strictly-necessary cookies (`rl_session`, `rl_csrf`)
and functional `localStorage`, and loads **no analytics or third-party trackers** —
so the banner is a simple "essential cookies only" notice. Set `VITE_PRIVACY_CONTACT`
to surface a data-controller email on the policy page.

To add privacy-respecting, opt-in analytics for your own deployment, edit
`web/src/lib/analytics.ts` (set `ANALYTICS_ENABLED = true`, name your provider, and
inject its script only when `consent.analytics` is true). The banner then
automatically shows a granular analytics opt-in and the policy lists it.

## Backend (`deploy/config.json`)

```jsonc
{
  "listenAddr": "0.0.0.0:8080",
  "dbPath": "/data/ridgeline.db",
  "webDir": "/app/web/build",
  "mqtt": {
    "broker": "tcp://mosquitto:1883",      // bundled broker, or tcp://your-host:1883
    "clientID": "ridgelined",
    "topics": ["meshcore/+/+/packets", "meshcore/+/+/status"],
    "username": "...", "password": "..."   // omit for an anonymous broker
  },
  "adminToken": "a-long-random-secret",     // protects /admin
  "email": {                                 // optional; omit to disable email
    "host": "smtp-relay.example.com", "port": 587,
    "username": "...", "password": "...",
    "from": "noreply@example.com", "fromName": "Ridgeline",
    "baseURL": "https://mesh.example.com"    // used to build links in emails
  }
}
```

Email is **fully optional**. With no `email` block, accounts still work —
registration just auto-verifies instead of sending a confirmation link.

## Serving & TLS (`deploy/Caddyfile`)

- **Public host with a domain:** first line = your hostname (e.g.
  `mesh.example.com`) → Caddy fetches a real certificate automatically.
- **Behind Cloudflare Tunnel / another proxy:** leave it as `:80` (plain HTTP;
  the proxy terminates TLS).

## Never commit

`deploy/config.json`, `deploy/.env`, `web/.env`, `deploy/passwd`, and `deploy/data/`
are gitignored because they hold secrets or per-deploy state. Only the
`*.example.json` templates belong in git.
