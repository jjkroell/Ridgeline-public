# Changelog

All notable changes to Ridgeline (the public, self-hostable build) are documented
here. The format is based on [Keep a Changelog](https://keepachangelog.com/), and
this project follows [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added
- `setup.sh` now asks whether the install is production or dev/staging. Choosing
  dev/staging writes `environment: "dev"` into `config.json`, enabling the "not
  the live site" banner without editing config by hand.

## [v0.3.0] — 2026-07-17

### Added
- Account deletion now opens a prominent confirmation modal that spells out
  exactly what is removed (account, notes, private locations, shares, sessions)
  versus what remains (nodes you own are released, not deleted — kept public and
  marked "previously owned by …"). The delete button activates only after you
  re-type the account's registered email (case-insensitive) in addition to the
  password.
- Config-gated non-production banner: set `environment` (e.g. `"dev"` or
  `"staging"`) in `config.json` and the UI shows a persistent, obvious "not the
  live site" banner (using your configured site name). It is reported via
  `/api/health`; unset (the default) shows nothing, so production instances are
  unaffected.

### Fixed
- The claimed-node ownership badge now recolours immediately when a claim
  verifies or a node is released, instead of only after a full page reload.
- Long values in confirmation dialogs no longer overflow the modal: dialog
  title/message wrap, and a public key (e.g. in the "scrub node" prompt) is
  shown centred in a dedicated one-line monospace slot sized to fit.

### Changed
- The daemon logs the effective email `baseURL` at startup and warns if it is
  empty while email is enabled, so a misconfigured link origin is obvious.
- Removed the hardcoded default email `baseURL`; each instance must set its own
  public origin in `config.json`. A hardcoded default could silently send an
  instance's verification links to the wrong origin.

## [v0.2.1] — 2026-07-17

Hardening follow-up to v0.2.0.

### Security
- Cap request bodies at 64 KB via a `MaxBytesReader` middleware on all routes
  (except the `/api/live` WebSocket). Endpoint length limits were previously
  enforced only after fully decoding the JSON body, so an unbounded POST could
  buffer arbitrary memory before any check ran; the cap keeps memory bounded and
  the handler returns its usual 400.

## [v0.2.0] — 2026-07-17

Security hardening plus a one-command updater. **Recommended update for anyone
running v0.1.0.**

### Security
- Rate-limit the unauthenticated email endpoints (`POST /api/auth/register`,
  `POST /api/auth/resend-verification`), keyed by client IP and target address, to
  prevent mass verification-email sends, SMTP-quota burn, inbox-bombing, and
  account enumeration.
- Strip CR/LF from outgoing email headers (`To`/`From`/`Subject`) to block email
  header injection.
- Enforce same-origin on the `/api/live` WebSocket (was allow-all) to block
  cross-site WebSocket hijacking.
- Remove the dead `adminToken` config field and its misleading "protects /admin"
  framing. Admin access is the **first registered account** (the protected owner);
  a leftover `adminToken` in an old `config.json` is now simply ignored.

### Added
- `update.sh` — one command to pull the latest, rebuild, restart, and wait for
  health. Preserves your config (`web/.env`, `deploy/config.json`, `deploy/.env`)
  and database, and auto-handles conflicts with installer-personalized files.
  `./update.sh --external` for external-broker installs.
- Documentation: a prominent note that the first registered account becomes the
  owner/admin, and an expanded "Updating" guide.

### Tests
- Added tests for the rate limiter, client-IP extraction, same-origin WebSocket
  check, and email header-injection safety.

## [v0.1.0] — 2026-07-17

Initial public, self-hostable release.

### Added
- Interactive `setup.sh` installer: Q&A for site name, region/map center, MQTT
  broker (self-hosted or external), SMTP, opt-in About page, and domain/TLS.
- Build-time personalization (`web/.env`, `web/src/lib/site.ts`,
  `web/src/lib/site-content.ts`) — nothing is tied to any particular mesh, place,
  or server; no phone-home calls or analytics.
- GDPR/PIPEDA cookie-consent banner and `/privacy` page.
- Node detail: a range-selectable "Heard by" list (`/api/nodes/{pubkey}/observers`);
  removed the redundant "Recent packets" card.
- Docker stack: multi-stage build (Node → Go → distroless), container healthcheck,
  version stamping, and a `docker-compose.external.yml` variant for running
  alongside an existing broker with your own reverse proxy.
- Startup resilience: the daemon serves the API and web UI even if the MQTT broker
  is unreachable at startup, retrying the connection in the background.
- `docs/CONFIG.md` configuration reference.

[v0.2.0]: https://github.com/jjkroell/Ridgeline-public/releases/tag/v0.2.0
[v0.1.0]: https://github.com/jjkroell/Ridgeline-public/releases/tag/v0.1.0
