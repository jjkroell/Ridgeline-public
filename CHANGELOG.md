# Changelog

All notable changes to Ridgeline (the public, self-hostable build) are documented
here. The format is based on [Keep a Changelog](https://keepachangelog.com/), and
this project follows [Semantic Versioning](https://semver.org/).

## [v0.9.9] — 2026-08-19

### Removed
- **Retiring an observer is gone.** An observer is now in service, on standby, or
  deleted. Retiring only hid a receiver from the observers page — it never
  stopped ingesting anything that receiver reported, which is the opposite of
  what it kept being reached for. Standby is the control that actually stops the
  feed, so the ambiguous one has been removed along with the Retired Observers
  panel. An observer retired before this release is released automatically on
  upgrade; nothing it reported is affected.

### Added
- **A known bridge now records which node it is bridged to.** A bridge is a link,
  but detection only ever names the near end — the relay whose behaviour gave it
  away. Marking one as known now asks which node sits on the far side, offering
  that node's observed neighbours busiest-first with shared-packet counts, and
  the Known Bridges list reads **`near → far`** instead of naming one end. You
  can still mark a bridge known without naming a peer, and rename either end
  without breaking the record.
- **The members list collapses**, and starts collapsed. It grows without bound on
  a busy site and was pushing the detection and bridge sections — the reason the
  page gets opened — below the fold. Your choice is remembered per browser.

### Changed
- **The Quarantine list is now two lists: Blocked and Dismissed.** They do
  opposite things. Blocked traffic is dropped at ingest and hidden from the site;
  a dismissed node is one you've confirmed is *not* a bridge, so it's excluded
  from detection and nothing about it is blocked. Filing both under "Quarantine"
  was wrong for every dismissed row. The actions now read *unblock* and *restore*
  rather than "release" for both.

### Fixed
- **On mobile, the Purged section and the members list were invisible unless
  something was quarantined.** The quarantine block wrapped both, so an admin
  with an empty quarantine list could not reach either.

## [v0.9.8] — 2026-08-16

### Added
- **Put an observer on standby.** A stood-down observer stays connected and stays
  on the observers page, but every packet it publishes is discarded at ingest
  instead of being stored. It is the reversible middle ground between the two
  things that already existed — blocking a publisher (permanent, and says the
  publisher is rogue) and retiring a receiver (hides it, but keeps ingesting
  everything it reports). Intended for a receiver you don't want influencing the
  data right now: one being moved or re-sited, sitting on the bench, or running a
  firmware build you don't yet trust. Its `/status` messages are still processed
  on purpose, so it keeps reporting online with live battery and noise telemetry
  — you can watch the device while nothing it hears reaches the database. Admins
  get a **Put on standby** / **Return to duty** control on the observer page
  (desktop and mobile); the observer is badged amber wherever it appears, with a
  banner on its page explaining that the statistics below have stopped moving and
  how many packets have been discarded. Nothing already recorded is affected, and
  nothing discarded during a stand-down is backfilled when it returns to service.
  New endpoints: `POST /api/admin/observers/standby` and `.../resume`.

### Changed
- **The node-detail location map now uses the Hillshade basemap**, matching the
  full maps' default, on the node modal, the desktop node page, the mobile node
  page and the WebGL-free fallback. On a small locked thumbnail the terrain
  shading is what carries the information — it's the difference between a dot on
  a flat grey field and a dot visibly sitting on a ridge.

### Fixed
- **An observer on standby could be deleted by retention.** Packets are the only
  thing that advances an observer's last-seen time, so standing one down made it
  look silent within minutes, and the hourly sweep that removes genuinely dead
  observers would delete the row an hour later — quietly taking the stand-down
  with it. A discarded packet is still evidence the observer is alive, so it now
  keeps the last-seen time current, and retention skips stood-down observers the
  same way it already skips retired ones.

## [v0.9.7] — 2026-08-16

### Fixed
- **Nodes in a remote part of the mesh could vanish from every map.** The
  corrupt-GPS detector tested latitude and longitude independently against the
  3×IQR whiskers of every located node. If your mesh has one dense cluster and a
  smaller group some distance away, that spread is small enough that the remote
  group fails both axes at once and is dropped from the static map, the live
  map, the node-detail inset and the dashboard mini-map — silently, because
  those views hide suspect nodes rather than marking them. Coordinates are now
  judged on distance from the mesh centroid, a single measurement, so a node
  that is moderately north *and* moderately west of the main cluster no longer
  fails twice over; and a node within 500 km of the centroid is never flagged,
  however tightly the rest of the mesh is clustered. Genuinely distant outliers
  are still caught.
- **Null island is now detected as the error it is.** A node that has never had a
  GPS fix reports 0,0. Those were only being caught incidentally by the same
  statistical test, and they skewed it: they dragged both the centre and the
  spread, so whether a real remote node was visible depended on how many nodes
  with broken GPS happened to be on air. 0,0 and out-of-range coordinates are
  now rejected directly and excluded from the statistics.

## [v0.9.6] — 2026-08-13

### Fixed
Five more places where a fact was attributed to the wrong node — the same class
of error as v0.9.5, found by re-reading the identity page and the node panel
that links into it with "whose setting is this?" in mind.

- **"In use by another *N*-byte node" named a width the occupier may not use —
  and then named nobody.** Whether a hash ID is taken is correctly judged over
  every routing node, but the occupant list was filtered to nodes whose own
  adverts use the selected width, so the "Used by …" line usually had nothing to
  show: on the mesh this was measured against, 180 of 197 occupied 2-byte
  prefixes reported "in use" with no name attached. The status now reads "In use
  by another routing node" and always names the holder.
- **A node could be marked "unique" while being ambiguous in a narrower path.**
  Uniqueness is only ever measured at the node's own advertised width, but the
  badge claimed it flatly — and 96 of 197 routing nodes were unique at their own
  width and ambiguous in a 1-byte path, a width carrying 15.8% of recent
  traffic. The badge now reads "unique at 3 bytes" and says in a tooltip what a
  narrower sender does to that claim.
- **A collision tooltip called the other nodes "*N*-byte nodes".** They share the
  prefix at the width being examined, whatever their own adverts use.
- **The collision panel exonerated every node it listed.** It said the listed
  nodes "are not misconfigured", contradicting the callout directly below it,
  which correctly points out that a node advertising at the selected width
  originates narrow paths itself and its operator can fix that. The intro now
  agrees with the callout.
- **The width buttons counted adverts but read as "nodes affected".** Each button
  showed "*N* nodes" from the advert cohort, on a control that selects a packet
  width — the same conflation the v0.9.5 copy fix was about. They now say "N
  advertising".

## [v0.9.5] — 2026-08-13

### Fixed
- **The hash-ID planner denied that the path width is a node setting.** The
  length selector explained itself with "this is the width of the packet, not a
  per-node setting", which contradicted the guide page it links to: the width
  *is* a per-node setting (`path.hash.mode`) — it just belongs to the node that
  **sends** the packet, and it then governs every hop that packet takes. The
  copy now says that, so the panel and the guide agree and the reader is not
  told the setting they are about to change does not exist.

## [v0.9.4] — 2026-08-13

### Added
- **Links in channel messages are now clickable.** A URL posted to a channel
  renders as a link on both the desktop and mobile readers, opening in a new
  tab. `http://`, `https://` and bare `www.` addresses are recognised; trailing
  sentence punctuation stays out of the link, and a closing bracket is kept
  only when the URL opened it, so both `…/Foo_(bar)` and `(see https://x.com)`
  come out right. Message text is decrypted from the air and is untrusted, so
  it is still rendered as text — the links are built as elements, never by
  injecting HTML, and only http(s) addresses can become one.

## [v0.9.3] — 2026-08-13

### Changed
- **The hash-ID guide is no longer in the mobile navigation menu.** It is still
  linked from the identity page itself, where it is relevant, and remains
  reachable by URL — it just no longer takes a slot in the More sheet.

### Fixed
- **The collision panel had two "how to" links in one sentence.** The first
  pointed at the repeater instructions and the second at the companion ones,
  which read as a stutter rather than a choice. Only the sender link remains,
  which is the one that matters: the sender chooses the path width.

## [v0.9.2] — 2026-08-13

### Changed
- **Picking an ID and generating a key pair are now desktop-only.** Both panels
  are gone from the phone layout, replaced by a short note saying where to find
  them. A 3-byte vanity search is a brute-force loop over millions of keypairs —
  minutes of sustained CPU, which on a phone means heat and battery for
  something you do once per node — and the panels needed more width than a
  handset has. The collision analysis, which is the genuinely useful part on a
  phone, is unchanged.

### Fixed
- **The identity page no longer scrolls sideways on a phone.** It was the only
  page that did. The picker and key-generator sat in a grid whose items default
  to `min-width: auto`, so a child with wide content pushed the column past the
  viewport instead of shrinking. Removing those panels from the phone layout
  removes the cause; the grid also gained the missing `min-width: 0` for the
  desktop layout.

## [v0.9.1] — 2026-08-13

### Fixed
- **Destructive actions now show they registered.** Deleting a claimed node, or
  clearing a dormant claim, gave no sign anything had happened: the buttons kept
  their idle labels while the request was in flight, and on the node page the
  dialog closed the instant the delete succeeded — throwing away the summary of
  what had been removed and leaving the deleted node still on screen. Buttons
  now report their own progress and disable their siblings, and the node dialog
  stays open to show what was deleted with a way back to the node list.
- **Tooltips on the claim actions** use the app's own tooltip rather than raw
  browser title text, so they match everything else.
- **Node names are readable in collision groups on a phone.** On the identity
  page every badge in a row is fixed-width, so the name — the only thing able to
  give — was squeezed to about 60px and became unreadable. On narrow screens the
  row now wraps, giving the name its own line, and the public key is dropped
  since it is noise on a phone and remains one tap away on the node itself.

## [v0.9.0] — 2026-08-13

### Added
- **Owners can retire or delete their own nodes.** A verified owner no longer
  needs an admin to remove a node they control. Ownership is proved
  cryptographically, so this is a signature check rather than a flag.
  **Retire** withdraws the node from the map and node lists while keeping every
  packet it sent and your claim; it survives a re-advert, so it is the right
  action for a decommissioned node that is briefly still on air, and it is
  reversible. **Delete** is permanent, requires typing the node's name to
  confirm, and says plainly what it does: it releases your claim (an orphaned
  claim would block the node from ever being re-claimed) and it removes
  observations that other operators' receivers recorded. Users get no access to
  the blocklist — blocking plus a released claim would strand a node that can
  neither reappear nor be re-claimed. Destructive actions are recorded in a new
  audit log, written before the deletion, because the claim that names the owner
  is removed by the same operation.
- **Dormant claims can be cleared.** A claim outlives its node on purpose: the
  retention sweep prunes a silent node's row but keeps ownership so it
  reconnects if the node returns. Those entries showed on the account page as
  greyed-out "Dormant" rows with no way to act on them — the only release
  control lived on a node page that no longer existed. Both account pages now
  offer **Release** (give up ownership, keep your notes) and **Delete
  everything** (also removes your notes and any private location, which release
  leaves behind).

### Fixed
- **Weak hop attributions are no longer treated as certain.** A path hop is
  matched to a node by unique prefix at whatever width the sender chose. That
  guards against two known nodes sharing a prefix, but not against saturation:
  a busy mesh exercises almost the whole 1-byte space within a week, so a
  1-byte hop matching one known node may have been written by a node never
  seen. Injection detection — which ends in an admin quarantining a node — now
  requires 2-byte evidence, matching node retention. Topology edges seen only
  via 1-byte hops are marked as inferences and drawn dashed, with a note
  explaining why; a single 2-byte sighting promotes an edge to a measurement.
  Aggregate relay counts and heatmaps deliberately keep using the permissive
  match, where the effect measured about 5% and dropping real signal would cost
  more than the noise.

## [v0.8.1] — 2026-08-13

### Fixed
- **Closed the second path that kept dead nodes alive.** v0.8.0 stopped 1-byte
  relay hops counting as evidence, but only in one of the two gates the
  retention sweep runs. The first gate skipped any node present in the analytics
  liveness snapshot, and that snapshot's relay counts come from the prefix
  resolver, which credits a hop to whichever node uniquely owns it *at any
  width* — including one byte. A node with a unique 1-byte prefix therefore
  still looked permanently live off other nodes' packets, and was skipped before
  the width gate could see it. Confirmed in the field: a repeater whose owner
  had taken it off the mesh 34 days earlier survived both sweeps this way.

  Relay evidence for retention is now judged in exactly one place, with the
  width gate, over the full retention window — which is wider than the liveness
  window, so nothing legitimately active is lost. The clock signals carried
  alongside relay counts are derived from adverts and so cannot testify that an
  advert-stale node is alive either.

## [v0.8.0] — 2026-08-13

### Changed
- **Node retention no longer accepts 1-byte relay hops as proof of life.** A node
  that stops advertising is kept if something credits it as still relaying, and
  hops are recorded at whatever width the *sender* chose. The 1-byte space turns
  out to be heavily saturated — on a live ~300-node mesh, 248 of the 256
  possible values were observed within a week — so a 1-byte hop matching a node
  is background traffic from whoever actually relayed, not evidence about that
  node. The effect was that any node with a unique 1-byte prefix could never be
  swept; on that mesh 25 of 30 silent nodes were being held alive this way, one
  of them 33 days without an advert.

  Retention now requires evidence at **2 bytes or wider**, where the space is
  99.3% unsaturated and a unique match is real attribution. The new
  `nodeRetentionMinHopBytes` option (default 2) restores the previous behaviour
  if set to 1. Nodes that are genuinely relaying keep their reprieve — they show
  up at 2 or 3 bytes as soon as they carry anyone's wider packet — and anything
  removed reappears the moment it advertises again.

  Considered and rejected: matching the evidence width to each node's *own*
  configured hash size. A node's setting governs what it originates, not how it
  is written down when relaying someone else's packet, so that rule would have
  re-admitted saturated evidence for exactly the nodes most affected, and set a
  weaker liveness bar for operators who hadn't upgraded.

## [v0.7.4] — 2026-08-12

### Fixed
- **The planner no longer tells 1-byte node operators there is nothing to fix.**
  v0.7.3's "who fixes this" note said flatly that the listed nodes were not at
  fault — true for a node advertising wider than the selected width, which
  appears only because someone else sent a narrow packet through it, but wrong
  for one whose own adverts already use that width. That node originates narrow
  paths itself and its operator has a direct fix in `set path.hash.mode`, so the
  note was steering exactly the people who could act away from acting. The note
  is now conditional: it counts the nodes advertising at the selected width and
  points their operators at the repeater instructions, then explains the
  remainder separately. The per-node badge highlights those actionable nodes
  rather than the bystanders, which was the wrong way round.

## [v0.7.3] — 2026-08-12

### Changed
- **Hash-ID planner presents width ambiguity as conditional, not as a node
  defect.** v0.7.2 corrected the maths — a relay stamps its prefix at the width
  the *sender* chose, so any routing node can be ambiguous inside a narrow path
  — but kept calling the result a "collision". Listing a node that advertises
  at 3 bytes under that heading reads as an accusation that its configuration
  is broken, when its own adverts are perfectly unambiguous. The panel is now
  "Ambiguous in an N-byte path", each node carries a badge showing the width
  its own adverts use, and a note names who can actually fix it: the senders
  still emitting narrow paths, not the nodes listed.
- **The planner now measures rather than warns.** It shows the share of traffic
  observed at the selected width over the last 24 hours, so a reader can tell a
  live problem from a theoretical one. Below 1% it says so explicitly.

## [v0.7.2] — 2026-08-12

Three issues raised on the public repo, all confirmed and fixed.

### Fixed
- **One metadata set per route.** Prerendered pages emitted six duplicated tags
  — `title`, `description`, `og:title`, `og:description`, `twitter:title`,
  `twitter:description` — with the generic shell value first, so link previews
  in Discord, iMessage and similar could show a generic card instead of the
  page being shared. The generic block now sits after `%sveltekit.head%`, and a
  postbuild step strips it from prerendered HTML entirely, leaving one
  authoritative set. The SPA fallback shell keeps the generic card. Fifteen
  routes that had no metadata at all gained it, including per-node and
  per-observer titles on the detail pages — the case that matters most when
  someone shares a link to a specific node.
- **Narrow-screen header no longer overflows the page.** The mobile header
  rendered all eleven navigation items in one non-wrapping row, making the
  document far wider than the viewport at every size below the desktop
  breakpoint and letting the whole page pan sideways. It is now a menu button
  and panel, with a 44px tap target; collapsed links are absent from the DOM so
  keyboard focus cannot land off-screen. Verified from 320px to 1024px.
- **Hash-ID guide corrected against MeshCore's own documentation.** The page
  shipped in v0.7.1 without a compatibility warning: repeaters older than
  firmware 1.14 silently drop 2- and 3-byte packets, so recommending two bytes
  unconditionally was advice that could black-hole traffic. The recommendation
  is now conditional on firmware and regional coordination. The planner warning
  was also backwards — prefixes nest, so a prefix unique at one byte is
  necessarily unique at two and three, and the risk runs from longer prefixes
  to shorter packet widths. Collision consequences are no longer overstated:
  MeshCore's FAQ says packets continue to pass and duplicates mainly cost path
  analysis, so the page now separates the certain analysis cost from the
  possible forwarding effect. Address-space figures corrected to 254 / 65,024 /
  16,646,144, since `00`/`FF` are reserved on the first byte only.
- **Planner measured the wrong population.** Collision analysis compared only
  nodes whose own advert width matched the selected width, but a relay writes
  its prefix at the width the *sender* chose. On a mesh where most repeaters
  advertise at three bytes this understated one-byte exposure by a factor of
  48. It now compares every path-participating node at the selected packet
  width.

## [v0.7.1] — 2026-08-09

### Added
- **Multi-byte hash ID guide (`/hash-ids`).** A linkable explainer for why a
  mesh benefits from moving off 1-byte path IDs, and how to change a repeater,
  room server or companion. Built as a prerendered route rather than a modal so
  individual answers can be shared directly — `#why`, `#repeaters`,
  `#companions` — and so it is indexable and readable without JavaScript. The
  collision odds are computed from the birthday problem over the 254 usable
  1-byte IDs (`00`/`FF` reserved): a coin flip at 20 routing nodes and
  effectively certain at 50. The cost is stated too — every hop carries that
  many bytes — so two bytes is the usual compromise rather than three by
  default. The point the guide leads with is the one operators most often miss:
  the path width is chosen by whoever *originates* a packet, not by the
  repeaters carrying it, so a companion's setting governs the whole route even
  though a companion never appears in a path and cannot itself collide.

## [v0.7.0] — 2026-08-06

### Added
- **Per-node clock health.** Every advert carries the node's own clock, so
  comparing it against when that advert was first heard gives the node's offset
  from the server. Shown on node detail (desktop and mobile) and in a new CLOCK
  HEALTH panel on Analytics listing the worst offenders. Only the earliest
  reception of each advert counts, because MeshCore re-floods an advert payload
  unchanged and later copies still carry the original timestamp — counting those
  would make a healthy node look progressively further behind. Only
  signature-verified adverts are trusted, and the median across adverts is used
  so one queued or corrupt reading cannot move the figure. A node whose adverts
  are stamped years out is reported separately as **never set** — MeshCore
  falling back to the firmware build date — rather than as an absurd drift,
  because that is a different fault with a different fix.
- **Per-packet route map.** A transmission's detail now draws the route(s) it
  took: one coloured path per observer, earliest reception first, with an
  all-paths overlay and click-to-isolate. Observers routinely report different
  paths for the same flood, so drawing them separately shows that spread instead
  of collapsing it into a single "best" path that never existed. Drilling into a
  single repeat shows just that observer's route. A hop whose key prefix matches
  several located nodes is drawn dashed with hollow nodes — it is an inference,
  not a measurement. A Trace's header path is never drawn, since those bytes are
  per-hop SNR rather than relay hops.
- **Shareable map links.** The map and live map (and their mobile screens) now
  carry centre, zoom, basemap and role filter in the URL, with a Copy link
  button. Panning updates the URL in place, so a reload keeps your view.
  Following someone else's link never overwrites your own saved basemap
  preference — it applies for that visit only.
- **Flood scoping.** Per node, a count of the plain (unscoped) FLOOD
  transmissions it forwarded, plus a FLOOD SCOPING panel on Analytics showing
  what share of the mesh's floods carry a region scope. On a mesh using scoping,
  a repeater running `flood.max.unscoped 0` should forward none, so a non-zero
  count points at that node's configuration. The panel checks adoption first and
  presents the counts as reference until scoping is actually in use, so a mesh
  that has not adopted regions is not shown a screen full of false faults.
- **Route flag on the feed.** Every row carries its routing mode — `FLOOD`
  (unscoped), `T·FLOOD` (region-scoped), `DIRECT`, `T·DIRECT` — so an unscoped
  flood is visible as it arrives rather than only in aggregate. A toolbar toggle
  narrows the feed to unscoped floods alone, on desktop and mobile.

### Changed
- **Static Map moves directly under Live Map in the desktop navigation.**
- **Relay traffic share is now weighted by time-on-air, not packet count.** A
  long advert occupies the channel far longer than a short ack, but the old
  ratio counted them equally, understating relays that carry bulk traffic and
  overstating ones that carry chatter. The share keeps its meaning (the fraction
  of relayed traffic transiting the node) and its scale; only the weighting
  changes. Node detail also reports the absolute airtime a node relayed.

## [v0.6.1] — 2026-08-02

### Changed
- **Channels and Map swapped places in the navigation.** Channels moves up to
  sit directly under Feed, and the static Map drops to where Channels was,
  between Topology and Identity. Applied to both the desktop sidebar (and its
  narrow-screen header nav, which renders the same list) and the mobile More
  sheet, so the two layouts stay in the same order.

## [v0.6.0] — 2026-07-28

### Added
- **Five colour themes, replacing the light/dark toggle.** Signal (the default
  dark), Slate (deep blueprint navy, cyan accent), Graphite (warm near-black
  brown, brass accent), Paper (warm topographic light) and Mist (cool blue-grey
  light, indigo accent). A picker in the sidebar — and in the mobile More sheet —
  previews each theme with its own ground and accent colour. Every palette
  clears WCAG AA for body and dimmed text, and 3:1 for the faint tier and
  accents, in both light and dark.
- **Ago/Clock timestamp toggle on the live feed.** The feed showed only elapsed
  time ("2m"); the wall-clock time a packet was heard was reachable only by
  opening a row. A segmented control in the toolbar now switches every row
  between the two, on desktop and mobile, and the choice is remembered.

### Fixed
- **Relative timestamps no longer go stale.** The elapsed label was computed
  during render, so it only refreshed when something else re-rendered the feed —
  which arriving packets happened to do. Pausing the feed, or a quiet mesh, left
  every "2m" frozen at whatever it said when it was last drawn. The labels now
  tick on their own.
- **Tooltips are anchored by their measured width, not their maximum.** The
  bubble was clamped against half of its 250px max width regardless of how wide
  it actually rendered, so any tooltip whose trigger sat near a viewport edge was
  pushed inward to a fixed position. In the sidebar that collapsed a whole row of
  controls onto one shared spot. The clamp now uses the real width and only moves
  a tooltip that would genuinely overflow.
- **The feed's Time column widens for clock timestamps.** It was sized for "2m",
  so a wall-clock time overflowed into the Type column.

### Notes for self-hosters
- Themes are selected by a `data-theme` attribute on `<html>`; the `theme-light`
  class now only marks a theme as having a light base (which is what the map
  layers read to pick a basemap). Adding your own theme is one entry in
  `web/src/lib/theme.svelte.ts` plus one token block in `web/src/app.css`.
- Existing visitors keep their saved preference: the previous `dark` and `light`
  values are still valid theme ids.

## [v0.5.5] — 2026-07-20

### Fixed
- **A trace's header path is no longer counted as relay hops.** Trace packets
  are not shaped like the rest: their header path carries one signed SNR
  reading per hop rather than a list of relay hashes, and the route being
  traced lives in the payload, sized independently. Those SNR bytes are
  indistinguishable from 1-byte hop hashes, so every path walker was reading
  them as relays — on a live mesh, 40% of them coincidentally matched a known
  node's prefix, inventing a relay that never carried the packet and adjacency
  between nodes that were never neighbours. This fed relay counts, neighbour
  resolution, the activity heatmap, mesh topology, node history, observer
  direct-link detection and bridge detection; it also let a phantom hop keep a
  silent node alive through the retention sweep, and could drop a packet that
  never crossed a quarantined bridge. Trace is a small share of traffic, so
  mesh-wide conclusions are unchanged — the correction is per-node: a node no
  longer shows a relay it never performed.

## [v0.5.4] — 2026-07-20

### Fixed
- **Observers are shown by name again.** Keying observers by public key in
  v0.5.3 made the key their id, and the UI renders ids — so the observers list,
  both detail pages, the heard-by lists on node detail, the retired panel, and
  the analytics coverage list and direct-link graph all displayed 64 hex
  characters where a name belongs. The key is the right identity and the wrong
  label. Every observer surface now shows the name, falling back to the id only
  for an observer that never carried a key (where the id *is* its name). The
  detail pages keep the key visible but quiet beneath the heading — it is what
  the observer's MQTT topic is keyed by.

## [v0.5.3] — 2026-07-19

Observers are now identified by their public key, not their name.

### Changed
- **An observer's identity is its public key; the name is a label.** The name is
  something the operator changes at will, so using it as the identity meant a
  rename started a whole new observer: every packet and telemetry sample stayed
  under the old name and the renamed one began from nothing. The name is not
  reliably distinct either — a device publishing `"Foo "` and `"Foo"` was two
  observers. The MQTT topic carries the public key on every message, so it is
  always available and survives any number of renames.
- **Existing data is re-keyed once, on upgrade.** History recorded under each old
  name is repointed at that observer's key, rows that were separate identities
  only because of a rename are merged, and blocklist entries follow so a
  quarantined observer does not silently come off the list. Merging takes the
  observer's current identity — label, status, region, radio, and whether it is
  retired — from its most recent row, so a receiver retired under an old name and
  since returned to the air is not left hidden. Observers whose key is unknown
  stay keyed by name; there is nothing better, and dropping them would lose
  their history.
- **Names are resolved server-side** for the live feed, node history, per-node
  observer lists and mesh analytics, so a name renders even for an observer that
  has since been retired.

### Upgrade note
The re-key rewrites the observer id on every stored observation. It runs
automatically on first start and is idempotent, but **back up your database
first** — there is no down-migration.

### Known issue
- Telemetry stranded by a rename whose old observer row was already deleted
  cannot be re-attached: nothing records which key that name belonged to. Renames
  from here on strand nothing.

## [v0.5.2] — 2026-07-19

### Fixed
- **Deleting an observer now removes its device telemetry too.** The
  battery/noise series is keyed by observer id and nothing else references it,
  so deleting the observer stranded every sample it had ever recorded — rows no
  page could reach and no sweep collected, for every observer ever deleted. The
  delete dialog said "all of its stored packets"; it now says what it does, and
  reports the telemetry rows removed.

### Known issue
- Observer identity is the friendly name, so **renaming an observer strands its
  telemetry** under the old name and starts a fresh series. This is separate
  from the fix above and is not addressed here — a rename is not a deletion, and
  the stranded samples are real measurements worth keeping until observers are
  keyed by something stable.

## [v0.5.1] — 2026-07-19

Decommissioned observers no longer come back from the dead.

### Fixed
- **A retired observer stays retired.** Observers publish their `/status` with
  the MQTT retain flag, so the broker keeps that message and replays it to the
  daemon on *every* reconnect — for as long as it exists, whether or not the
  device is still on the air. The daemon treated a replay as a live sighting and
  re-created the observer, which is why one removed from the observers page
  reappeared after the next restart or redeploy. A retained status is a stale
  last-known value: it may now refresh an observer that already exists, but it
  can never create one.
- **No more invented telemetry.** The same replay appended a battery/noise
  sample stamped with the reconnect time — a reading that was never taken, one
  per reconnect, for as long as the retained message lived.
- **The observer count on the dashboard** now agrees with the observers page.

### Added
- **Retire an observer** instead of deleting it. Retiring withdraws a
  decommissioned receiver from the observers page and keeps every packet it
  reported, still attributed to it in history. Deleting an observer removes its
  packets too, which quietly rewrites the record — retiring is the right action
  for a receiver that has simply left the network. Reversible from the admin
  console, on desktop and mobile.

## [v0.5.0] — 2026-07-19

RF bridge detection, rebuilt. The previous detector could not find a live bridge
on the mesh it was written for; this release finds it, explains why it was
missed, and keeps a bridge you run on purpose from being reported as news
forever. Design notes and the evidence behind it are in
[docs/bridge-detection.md](docs/bridge-detection.md).

### Added
- **A second detection signal: wired egress.** RF is broadcast, so which
  neighbour relays a packet next varies — a typical relay hands off to ~13
  different nodes. A relay whose next hop *never* varies is handing off over a
  cable. This finds a bridge however few nodes sit behind it; the old rule needed
  three and could not see a small far side at all. Both rules now run and every
  candidate is labelled with the signal(s) that produced it.
- **Moved behind a bridge.** Nodes that stopped being heard directly and now
  arrive through a bridge are reported in their own right. A node keeps its
  public key across a frequency change, so nothing else notices it moved.
- **Known bridges.** Mark a bridge you run on purpose: it moves to its own list
  and stops appearing as a candidate. Nothing is blocked or hidden — the
  opposite of Dismiss, which says a candidate is not a bridge.
- **Scan summary and per-candidate evidence** in the admin console: packets
  scanned, paths, unresolved hops, adverts rejected, and for each candidate the
  traffic it carried, how many distinct next hops it had, and whether an observer
  ever received its own transmission.

### Fixed
- **Path evidence now comes from every packet type, not just adverts.** A route
  is in the clear whatever the payload; only the *origin* needs an advert. A
  companion that never adverts previously contributed nothing at all despite its
  messages crossing a bridge with a full path attached — on the reference mesh
  this raised the evidence base from 1,699 adverts to 5,667 packets per window.
- **Side membership is judged on recent evidence.** A single direct reception
  anywhere in the window used to mark a node local for the whole window, so a
  node that moved kept being excused by evidence that had expired hours earlier.
- **Adverts whose signature does not verify are rejected.** A corrupt public key
  invents a node that never existed; those phantoms were surfacing as injector
  candidates.
- **Ordinary repeaters no longer flagged as bridges.** A single unvarying next
  hop is also what a repeater with exactly one reachable neighbour looks like;
  a candidate must now actually carry a far side.
- **Admin console** only renders sections that hold something, and the Known
  action is no longer adjacent to the show/hide toggle.

## [v0.4.0] — 2026-07-19

### Added
- **Claimed filter on the Nodes list**, showing your own nodes first and other
  operators' after. It joins the role and favorites filters in a new filter
  modal, replacing the row of pills that competed with the table: the header is
  now a search field and one control that names the active filters
  ("Repeaters · Claimed") so the constraint is legible at a glance.
- **Member management on mobile.** `/m/admin` gained the MEMBERS panel — promote
  or demote admins, block, unblock and remove — matching the desktop console.
  Both now share one component.
- **Dormant claims and shares.** A claim or location share outlives its node when
  the retention sweep prunes a node that has gone quiet. Those rows now render
  un-linked with a "Dormant" pill explaining the claim is kept and reconnects if
  the node advertises again, instead of linking to a "Node not found" page.

### Fixed
- **Scrubbing a node now removes the data attached to it** — the ownership claim,
  notes, private location and location shares. Previously they were orphaned: the
  claim still showed in "Claimed Nodes" pointing at a node that no longer existed,
  and it would have blocked the node from ever being re-claimed. Re-scrubbing a
  key cleans up leftovers from earlier scrubs.
- **The automatic retention sweep no longer deletes that data.** A node pruned for
  going silent is expected to come back, so an operator who takes a repeater down
  for a week keeps their claim and private location.
- **Heuristic sweeps skip claimed nodes.** Neither the corruption-artifact scrub nor
  the detector-driven bridge purge will delete a node someone has claimed — a claim
  means the heuristic misfired. Purge still blocks (reversible); only the delete
  holds back, and the console reports what it skipped.
- **The live feed backfills after a reconnect.** A dropped WebSocket (redeploy,
  tunnel blip, laptop sleep, backgrounded PWA) left a permanent hole: the page
  looked connected while silently omitting everything from the outage. Channel
  conversations would simply stop updating until reload.
- **Nodes lists sort by the time they display.** Ordering used the last advert
  while the "Heard" column showed the most recent advert *or* relay, so rows could
  appear out of order.
- **The mobile "Companions" filter matched nothing** — it filtered on a role value
  no node reports.

## [v0.3.2] — 2026-07-17

### Added
- **Password reset.** A "Forgot password?" flow: request a reset link by email
  (`POST /api/auth/forgot`, always responds 200 so it never reveals whether an
  address has an account), then set a new password from the emailed single-use
  link (`/reset-password`, 1-hour expiry). Completing a reset revokes the
  account's other sessions, confirms the email address, and signs you in.
  Available on desktop and mobile.

### Security
- **Login brute-force protection.** `POST /api/auth/login` is now rate-limited per
  client IP and per target account (429 when exceeded, returned before the account
  lookup so it reveals nothing). Bursts stay generous enough for a mistyped
  password but bound sustained guessing. The reset endpoint is IP-limited too.

## [v0.3.1] — 2026-07-17

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
