# Ridgeline

**Mesh network observability for [MeshCore](https://meshcore.co.uk) LoRa networks.**

Ridgeline ingests packets from MeshCore observer nodes over MQTT, decodes them,
and turns them into a live picture of your mesh: nodes, links, hops, channels,
and the terrain-shaped RF reality in between.

Designed to be self-hosted for any MeshCore region — an interactive installer
personalizes it to your mesh (see [Quick start](#quick-start)).

## Quick start

Clone the repo and run the installer. It asks about your mesh — broker, region,
site name, email, domain, and whether you want an About page — then writes your
config, builds the app, and (optionally) launches the Docker stack:

```sh
git clone https://github.com/jjkroell/Ridgeline-public.git ridgeline
cd ridgeline
./setup.sh
```

Nothing about the project is tied to any particular mesh or region: every
region-specific value (name, map center, radio settings, About text, broker,
SMTP, domain) is supplied by you during setup. See [`deploy/README.md`](deploy/README.md)
for the manual path and [`docs/CONFIG.md`](docs/CONFIG.md) for what every setting does.

## Architecture

- **`cmd/ridgelined`** — single Go daemon: MQTT ingest → MeshCore packet
  decoder → SQLite → REST API + WebSocket live feed
- **`web/`** — SvelteKit (Svelte 5) single-page app, built static and served
  by the daemon; MapLibre GL maps, Tailwind CSS v4
- **SQLite** — one file, WAL mode, single writer (the daemon)

```
MeshCore observers ──MQTT──▶ ridgelined ──▶ SQLite
                                  │
                        REST + WebSocket
                                  │
                              web (Svelte)
```

## Development

```sh
# backend
go run ./cmd/ridgelined -config config.json

# frontend (dev server proxies /api to the daemon)
cd web && npm run dev
```

## Status

Early development. Written from scratch — protocol behavior referenced from
the MIT-licensed [MeshCore firmware](https://github.com/meshcore-dev/MeshCore).

## License

MIT — see [LICENSE](LICENSE).
