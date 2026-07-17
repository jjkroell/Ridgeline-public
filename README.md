# Ridgeline

**A live observatory for [MeshCore](https://meshcore.co.uk) LoRa mesh networks.**

Ridgeline listens to your mesh through receive-only *observer* nodes, decodes
every packet, and turns it into a real-time picture of the network: who's on the
air, which repeaters are carrying traffic, how far each signal reaches, and how
the whole mesh is stitched together. A mesh is mostly invisible while it runs —
Ridgeline gives it a screen.

It's built to be **self-hosted for any region**. Nothing here is tied to a
particular mesh, place, or server: an interactive installer asks about *your*
network and personalizes everything — name, map, radio settings, About page,
broker, email, domain — before it builds. There are no phone-home calls, no
analytics, and no hardcoded locations.

> ## ⚠️ Register your account FIRST — the first account is the owner
>
> **The very first account registered on your site automatically becomes the
> site owner / master admin.** That account is:
> - made an **administrator** (gets the `/admin` console),
> - **auto-verified** — it works even before you've set up email, and
> - **protected** — it can't be demoted, locked out, or deleted by anyone.
>
> So the moment your site is live, **go to `/login`, choose "Create account", and
> register your own account before sharing the URL with anyone.** Whoever
> registers first owns the deployment; everyone who signs up after that is an
> ordinary member.

---

## Table of contents

- [What you get](#what-you-get)
- [How it works](#how-it-works)
- [Requirements](#requirements)
- [Quick start](#quick-start)
- [The installer, step by step](#the-installer-step-by-step)
- [What the installer writes](#what-the-installer-writes)
- [Accounts & node ownership](#accounts--node-ownership)
- [Updating & re-running setup](#updating--re-running-setup)
- [Manual install](#manual-install)
- [Development](#development)
- [Configuration reference](#configuration-reference)
- [Project layout](#project-layout)
- [Status & credits](#status--credits)

---

## What you get

A single web app with, among other things:

- **Overview** — a customizable dashboard (drag/show/hide cards) summarizing the mesh.
- **Nodes** — searchable directory of every node and repeater, with per-node
  detail: role, location, radio config, neighbours, hop stats and history.
- **Live feed** — packets streaming in as they're heard (adverts, messages, traces…).
- **Coverage map** — node/repeater locations with estimated reach.
- **Live signal map** — watch packets and links light up in real time.
- **Analytics** — network health, link quality, relay activity, betweenness/bridge scores.
- **Topology** — the mesh as a graph of who-hears-whom.
- **Channels** — public channel activity.
- **Observers** — the receive-only stations feeding Ridgeline.
- **About** — an optional, fully customizable page describing your mesh.
- **Accounts** — optional email-verified logins, node claiming, notes, private
  locations, and sharing (see [below](#accounts--node-ownership)).
- **Mobile PWA** — an installable phone app served at `/m` with its own chrome.
- **Admin console** — token-protected management at `/admin`.
- **Light/dark theme**, and a WebGL-free fallback map for browsers without it.

## How it works

```
MeshCore observers ──MQTT──▶ ridgelined ──▶ SQLite
                                  │
                        REST + WebSocket (live)
                                  │
                             web app (Svelte)
```

- **`cmd/ridgelined`** — one Go daemon: subscribes to your MQTT broker, decodes
  MeshCore packets, stores them in SQLite, and serves a REST API + a WebSocket
  live feed. Pure Go (CGO off), so it runs as a tiny static binary.
- **`web/`** — a SvelteKit (Svelte 5) single-page app, built to static files and
  served by the daemon. MapLibre GL maps, Tailwind CSS v4.
- **SQLite** — a single file in WAL mode; the daemon is the only writer.

The production stack (via Docker Compose) is three containers: **mosquitto**
(the broker your observers publish to — optional), **ridgelined**, and **caddy**
(reverse proxy + automatic HTTPS).

## Requirements

- **[Docker](https://docs.docker.com/get-docker/)** with the Compose plugin
  (`docker compose`) — the recommended way to build and run.
- **`python3`** — used by the installer to write config files safely.
- **Bash** — to run `setup.sh`.
- One or more **MeshCore observer nodes** publishing to an MQTT broker (Ridgeline
  can host that broker for you).

For a from-source / development build without Docker you'll also want **Go 1.26+**
and **Node 22+**.

## Quick start

```sh
git clone https://github.com/jjkroell/Ridgeline-public.git ridgeline
cd ridgeline
./setup.sh
```

The installer walks you through everything, writes your configuration, and can
build and launch the whole stack at the end. Prefer to do it by hand? See
[Manual install](#manual-install).

**➡️ As soon as it's up, open `/login` → "Create account" and register your own
account first — it becomes the protected owner/admin (see the ⚠️ owner note at the top of this README).**

## The installer, step by step

`./setup.sh` is interactive. Every question shows a `[default]` in dim text —
**press Enter to accept it**. You can re-run the script any time to change your
answers; it just overwrites the same files. Passwords (e.g. SMTP) are never
echoed to the screen.

It asks in seven short sections:

### 1. Your site
| Prompt | Default | What it sets |
|--------|---------|--------------|
| Site name | `Ridgeline` | Header wordmark, browser tab title, SEO |
| Tagline | `MeshCore Observatory` | Line under the wordmark |
| Public site URL | *(blank)* | Canonical/social links, sitemap, email links |
| One-line description | generic | Search-engine description |

### 2. Region
The default map view — where every map opens before it has data. Tip: right-click
a spot in Google Maps to copy its latitude/longitude.

| Prompt | Default | What it sets |
|--------|---------|--------------|
| Center latitude / longitude | *(blank → world view)* | Where maps center |
| Default zoom | `8` | 1 = world, 8 = regional, 12 = city |

### 3. MQTT broker
Where your observer nodes publish packets.

- **Host one for me (recommended)** — a `mosquitto` broker runs alongside
  Ridgeline; observers connect on port **9001** (MQTT-over-WebSockets).
- **Use an existing broker** — you'll be asked for the broker URL (e.g.
  `tcp://broker.example.com:1883`) and, if it's not anonymous, a username/password.

You then set the MQTT **client id** and the **topics** to subscribe to (default
`meshcore/+/+/packets,meshcore/+/+/status`).

### 4. Email *(optional)*
Configure an SMTP relay to enable **account email verification** and
**owner note notifications**. Works with any provider (Brevo, Postmark, SendGrid,
Amazon SES, your own server). Skip it and accounts still work — registration
simply auto-verifies without sending mail.

### 5. About page
Choose one:
1. **Keep the generic default** page (a neutral primer on MeshCore + Ridgeline).
2. **Write your own** — you're prompted for a heading, intro, and as many
   heading + text sections as you like. **Only the text you type appears** —
   nothing is assumed about your mesh.
3. **No About page at all** — the `/about` route and its nav link are removed.

If you keep or customize the page, you can optionally include a **LoRa radio
settings table** (frequency, bandwidth, spreading factor, coding rate) so newcomers
know how to configure a radio for your mesh.

### 6. Web serving (TLS)
How the site is reached from the internet:
1. **Caddy gets HTTPS automatically** for a domain you point DNS at — enter the
   hostname (e.g. `mesh.example.com`) and Caddy fetches a real certificate.
2. **Behind Cloudflare Tunnel / another proxy** that terminates TLS — Caddy stays
   on plain HTTP (`:80`).

### 7. Admin access
There's no admin token to set. The **first account registered** on the running
site becomes the protected owner/admin (see the ⚠️ note at the top). The installer
just reminds you to create yours first, before sharing the URL.

Finally it prints a **review** of your answers and asks for confirmation before
writing anything. If Docker is available, it offers to **build and launch** the
stack right then (the first build compiles the web app and Go daemon — a few
minutes); otherwise it tells you the one command to run when you're ready.

## What the installer writes

| File | Committed to git? | Contents |
|------|-------------------|----------|
| `web/.env` | No (gitignored) | Build-time site name, region, map center |
| `web/src/lib/site-content.ts` | Yes | About-page content + radio settings table |
| `deploy/config.json` | No (gitignored) | MQTT + email settings |
| `deploy/.env` | No (gitignored) | Host UID/GID for the container |
| `deploy/Caddyfile` | Yes | Your domain / TLS mode |
| `web/src/app.html`, `web/static/{sitemap.xml,robots.txt}` | Yes | Your name/URL in crawl-time SEO |

Files holding secrets or per-deploy state (`deploy/config.json`, `deploy/.env`,
`web/.env`, `deploy/data/`) are **gitignored** and never committed.

## Accounts & node ownership

Accounts are optional but unlock a lot. Once email is configured (or left off,
in which case registration auto-verifies), users can:

- **Claim their nodes** two ways — briefly rename the node to a one-time code, or
  prove ownership instantly by signing a challenge with the node's private key
  (the key never leaves the browser).
- **Get notified** by email when someone leaves a note on a node they own.
- **Leave notes** on nodes (public, team, or private).
- **Set a private, precise location** for a node, visible only to them and people
  they share it with.
- **Manage their profile** — display name, email, password — and delete their
  account, which releases their claimed nodes.

## Updating & re-running setup

Change any answer by re-running the installer:

```sh
./setup.sh
```

To pull new code and rebuild:

```sh
git pull
cd deploy && docker compose up -d --build
```

Your database in `deploy/data/` and Caddy's certificates persist across rebuilds.

## Manual install

Prefer not to use the installer? The full manual path — copying
`deploy/config.example.json`, setting UID/GID, choosing a TLS mode, and launching
Compose — is documented in **[`deploy/README.md`](deploy/README.md)**.

## Development

```sh
# backend (reads a local config.json — see config.example.json)
go run ./cmd/ridgelined -config config.json

# frontend (Vite dev server proxies /api to the daemon)
cd web && npm install && npm run dev
```

Useful checks:

```sh
go build ./... && go test ./...     # backend
cd web && npm run check && npm run build   # frontend types + build
```

## Configuration reference

Every setting — and which file it lives in — is documented in
**[`docs/CONFIG.md`](docs/CONFIG.md)**. In short: frontend look/branding is
build-time (`web/.env` + `web/src/lib/site.ts`, inlined by Vite), while runtime
secrets and the broker/email config live in `deploy/config.json`.

## Project layout

```
cmd/ridgelined      the daemon (MQTT ingest, decode, API, live feed)
internal/           store, analytics, mail, MeshCore decoder, HTTP API
web/                SvelteKit single-page app (built static)
deploy/             Docker Compose stack: mosquitto + ridgelined + caddy
docs/CONFIG.md      full settings reference
setup.sh            interactive installer
```

## Status & credits

Actively developed. The MeshCore protocol behavior is referenced from the
MIT-licensed [MeshCore firmware](https://github.com/meshcore-dev/MeshCore);
Ridgeline itself is an independent, from-scratch implementation.

## License

MIT — see [LICENSE](LICENSE).
