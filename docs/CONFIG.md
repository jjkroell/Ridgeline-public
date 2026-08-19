# Configuration reference

Everything region-specific in Ridgeline is supplied by **you**. The interactive
installer (`./setup.sh`) collects it and writes the files below; you can also edit
them by hand and rebuild. Nothing points back at any particular mesh or server.

## Where settings live

| File | Committed? | What it holds |
|------|-----------|---------------|
| `deploy/config.json` | **No** (gitignored) | Runtime config: MQTT broker + credentials, SMTP block |
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

## "Add an observer" guide (`web/src/lib/site-content.ts`)

The Observers page can carry a guide telling other people how to point a station
at your mesh — flashing, radio settings, WiFi and the broker details, each with a
copy button. It is **hidden entirely** until you set a broker, since without one
there is nothing for a newcomer to connect to.

- `MQTT.broker` — the URL observers connect to, with a scheme, e.g.
  `wss://mqtt.example.com:443`. Empty hides the guide and its button.
- `MQTT.audience` — the JWT audience if your broker uses Ed25519 observer tokens
  (see below). It must equal the broker's hostname **exactly**, or tokens are
  refused as having been minted for a different broker. Leave empty for a broker
  using username/password.
- `MQTT.defaultRegion` — the region code the guide suggests. Conventionally the
  nearest airport's IATA code; it becomes the second segment of the observer's
  MQTT topic (`meshcore/{REGION}/{PUBKEY}/packets`).
- `MQTT.showIataPicker` — offers a searchable list of airports to pick that code
  from. The bundled list is **Canada-only**, so this is off by default.
- `RADIO_CLI` — the numeric `freq,bw,sf,cr` form of `RADIO_PARAMS`, pasted into
  the guide's `set radio` command. Empty omits that step.

## Observer authentication (Ed25519 tokens)

Optionally, observers can authenticate instead of publishing anonymously. Each
one signs a token with its own node key, so it can only publish under its own
identity and nobody can report traffic as somebody else. There is no password to
issue and nothing to register.

`deploy/mosquitto-jwt.conf` and `deploy/mosquitto-jwt/` build a second broker that
delegates every decision to ridgelined over the compose network. Run it **beside**
your existing broker rather than replacing it, so stations can be moved one at a
time — both feed the same database, and a half-migrated network is a normal state.

- `mqttAuth.audience` in `deploy/config.json` — enables the `/api/mqtt-auth/*`
  endpoints. Empty leaves them disabled (404).
- `mqttAuth.consumerUsername` / `consumerPassword` — ridgelined's own login on the
  authenticated broker, which allows anonymous connections from nobody. Must match
  the matching entry in `extraBrokers`.
- `extraBrokers[]` — additional brokers to ingest from alongside `mqtt`. An
  observer must publish to exactly **one** of them; a station publishing to two is
  counted twice.

⚠ The auth endpoints must not be reachable from the internet — the bundled
`Caddyfile` returns 404 for `/api/mqtt-auth/*` at the edge, and the broker reaches
ridgelined over the compose network instead.

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
    "username": "", "password": ""         // set for an authenticated broker; empty = anonymous
  },
  "email": {                                 // optional; omit to disable email
    "host": "smtp-relay.example.com", "port": 587,
    "username": "...", "password": "...",
    "from": "noreply@example.com", "fromName": "Ridgeline",
    "baseURL": "https://mesh.example.com"    // used to build links in emails
  },
  "environment": "dev"                        // optional; "dev"/"staging" shows a
                                              // "not the live site" banner. Omit
                                              // (the default) for production.
}
```

Email is **fully optional**. With no `email` block, accounts still work —
registration just auto-verifies instead of sending a confirmation link.

`environment` is **optional**. Set it to `"dev"` or `"staging"` on a
non-production instance and every page shows a prominent "not the live site"
banner (using your site name); omit it for a normal production site. `setup.sh`
asks about this and sets it for you. It is reported on `/api/health`.

There is **no admin token/password** in the config. Admin access is by account:
the **first account registered** on a fresh deployment becomes the protected
owner/admin (gets `/admin`, auto-verified, can't be demoted or deleted). Register
your own account first, before sharing the URL. A legacy `adminToken` key in an
old config is ignored.

## Serving & TLS (`deploy/Caddyfile`)

- **Public host with a domain:** first line = your hostname (e.g.
  `mesh.example.com`) → Caddy fetches a real certificate automatically.
- **Behind Cloudflare Tunnel / another proxy:** leave it as `:80` (plain HTTP;
  the proxy terminates TLS).

## Never commit

`deploy/config.json`, `deploy/.env`, `web/.env`, `deploy/passwd`, and `deploy/data/`
are gitignored because they hold secrets or per-deploy state. Only the
`*.example.json` templates belong in git.
