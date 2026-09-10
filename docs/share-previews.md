# Public link previews

Ridgeline's Go daemon renders route metadata into the **initial HTML response**
for nodes and public section roots. It also serves a deterministic 1200×630 PNG
at `/share/card.png?path=/nodes/PUBLIC_KEY`. Svelte stays a static SPA: this is
server-rendered metadata, not full Svelte SSR, and needs no Node runtime or browser.

## Build and configuration

Run `cd web && npm ci && npm run build`, then rebuild/restart the daemon as usual.
The postbuild step exports the existing `VITE_SITE_NAME` and `VITE_SITE_URL` from
Vite's production environment into `build/share-site.json`. Set `VITE_SITE_URL`
to the public HTTP(S) **origin** for absolute canonical and image URLs. Never use
request Host or forwarded headers as the canonical origin. A blank origin leaves
relative URLs for local development; configure an origin for reliable unfurling.
A malformed configured origin disables enhanced previews and logs a warning.

Branding uses the normal production build (`npm run build`); if building with a
custom Vite mode, run the postbuild with `NODE_ENV` set to that same mode so it
reads the matching environment file. No credentials go into this manifest.

## Scope and data contract

- `/nodes/{64-hex-key}`: current public name, role, public-key fingerprint,
  first observation and last **advert** observation (not a connectivity claim).
- Public section roots: overview, nodes, feed, channels, both maps, analytics,
  topology, identity planner and observers. `/m` equivalents canonicalize to
  the desktop route. Existing prerendered pages and static assets are preserved.
- Observer detail cards, arbitrary entity types and full Svelte SSR are follow-ups.
- Missing, retired and quarantined nodes have no public card. The original page
  fallback still works; image requests return 404. No notes, precise coordinates,
  ownership, session data or inferred radio configuration are read.
- The identicon is derived from the identity; it is not a coverage/activity plot.
- Go's bundled regular/bold fonts keep rendering offline and deterministic.
  Unsupported glyphs use a visible box in the image; full Unicode remains in HTML.

## Freshness and caching

A fresh request reads the current public node row using an indexed lookup. The
image URL includes a content revision; it changes when the displayed record or
branding changes. Both HTML and PNG responses use ETags and a five-minute HTTP
cache lifetime with revalidation. Images are rendered on demand, with at most
four concurrent renders and no unbounded in-process image cache. Overload returns
503 and Retry-After. Errors and missing images use `no-store`.

The image endpoint always returns the latest public record, including for an old
`v` query. It is deliberately **not immutable** and does not store historical
copies. Sharing services may cache a card longer than the origin requests and
cannot be forced to update already-sent messages. Absolute UTC observation dates
and “open for current details” keep those copies interpretable. There is no
“online now”, “2 minutes ago”, or undated rolling counter that will become false.

The initial head contains one authoritative metadata set. On client mount,
`Seo.svelte` takes ownership and removes server metadata, then updates image URLs
on navigation so a previous node's card cannot linger in the DOM.

## Example

![Synthetic node card; example data, not a live capture](images/share-card-example.png)

## Verification / AI development handoff

These implementation notes and the contribution were generated with Codex.
They apply equally to Claude or Codex continuing development.

```sh
go test ./internal/api -run 'TestShare' -count=1
go test ./...
go build ./...
cd web && npm run check && npm run build
```

For a synthetic visual fixture, set `RIDGELINE_SHARE_PREVIEW_SAMPLE` to an absolute
PNG output path while running `TestShareEscapingAndCard`. The fixture is example
data, not a captured live node. Test the built app through **the Go server**:
Vite preview alone has no Go metadata/image handler. Fetch a node document without
JavaScript, follow its `og:image`, check PNG MIME/dimensions, then test browser
navigation from one node to another and to a prerendered page. Real iMessage /
Discord / WhatsApp unfurl validation requires deployment; local HTTP tests do not
prove those services have refreshed their caches.
