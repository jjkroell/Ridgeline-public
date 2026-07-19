# RF bridge detection — design

How Ridgeline finds RF bridges: nodes that carry traffic between the mesh it
observes and a mesh on another frequency. Written 2026-07-19 after the original
detector failed to find a live, known bridge on the dev instance.

A bridge is not inherently hostile — the one this work was validated against is
the operator's own, deliberate and useful. The goal is to *identify* ingress
points so an operator can decide, not to auto-ban.

## Evidence base

Measured over 14 days on the dev instance (243 nodes, 16 observers, ~900k
observations). Known ground truth:

| Case | Truth |
|---|---|
| `bridge-far` → `bridge-near` | **bridge** — two bands, serial-linked pair |
| `a distant repeater` | **not a bridge** — the original detector's top candidate |
| `a remote relay` | **not a bridge** — a relay serving an observer-less pocket; the captivity rule's core false-positive class |
| `a repeater that moved` | **migration** — moved one band to another, same pubkey |
| `a renamed repeater` | **migration** — renamed, same pubkey, now far side |
| `another operator's node` | **migration** — another operator, temporary another band test |

## Why the original detector failed

`DetectInjection` classified an origin as foreign when it was never heard at zero
hops, called it *captive* to a relay when ≥95% of its paths transited that relay,
and flagged a relay with ≥3 captive nodes forming ≥60% of its foreign traffic.

Four independent failures, each sufficient alone:

**1. "Never heard directly" carries almost no information.** 263 of 316 origins
(83%) qualify, and 90+ active relays with zero direct receptions are plainly
legitimate. With 16 observers over this
footprint, being out of direct earshot is the normal condition, not a signature.

**2. Adverts only.** The scan skips every packet where `pkt.Advert == nil`. A
companion that never adverts is structurally invisible — one far-side companion
was observed only through channel messages, with 13 observer copies carrying full
path evidence that the detector discarded. Most of a bridge's evidence is thrown
away before scoring.

**3. `minCaptiveNodes ≥ 3` fails on small far sides.** Only two far-side nodes
adverted in the window, so the bridge was below threshold by construction.

**4. Unverified adverts create phantom origins.** A corrupt public key invents a
node that never existed. These are rare — measured over 24h on the dev mesh, 8 of
7,857 adverts fail Ed25519 verification (0.1%) — but they concentrate in the
injector rule, where a handful of one-off phantom keys reported by a single
observer is exactly the "sole source of many origins" signature. Removing them
eliminated a standing injector false positive. `Advert.SignatureValid` is computed
by the decoder and was never consulted.

Note the signature covers the advert *payload*, not the path: the path is mutable
by design, since relays append to it. So signature validation cannot vouch for
route data, only for the originator's identity and payload.

## The signal that works: downstream determinism

Across 138 relays, the median hands off to **13 distinct next hops** with a **44%
top share**. RF is broadcast — whoever hears you first varies, so downstream
choice is high-entropy.

`bridge-far` handed off **1,417 times to exactly one node, and never to
anything else** (entropy 0.00).

That is the physical signature of a bridge: its egress is a wire, not an antenna.

Score each relay on the entropy of its next-hop distribution, built from **all
packet types**, weighted by sample count:

    H(relay) = -Σ p(next) log₂ p(next)

Confidence scales with the number of samples observed without any alternative
appearing. 1,417 samples with zero alternatives is overwhelming; 100 is weak.

**Determinism alone is not proof.** A relay with exactly one physical neighbour
looks identical to a wire. Three other nodes on this mesh reach 100% determinism
(535, 178 and 154 samples) and a fourth
sits at 98%. They are an order of magnitude below bridge-far in sample count, which
is what confidence weighting is for. The output is a ranked shortlist for review,
not a verdict.

### Corroborator: never terminal

How often a relay is the LAST hop in a path — where an observer received that
relay's own transmission. A relay transmitting on a frequency nobody monitors can
never be terminal, however much traffic it carries: its packets only become
observable once something else re-sends them.

This is independent of next-hop entropy. Entropy says the egress never varies;
this says the transmission is never heard. Measured over 14 days, only 2 of 134
relays with ≥200 path appearances sit at zero — the bridge (1,417 packets, 0
terminal) and one node simply out of every observer's range. Only the bridge also
has a single next hop.

It does not separate "on another frequency" from "out of everyone's earshot", and
at low volume it means little — a candidate with 102 packets reads 0% too.
Displayed as corroborating evidence; nothing ranks on it.

### Corroborator: asymmetric pair

A bridge joins a zero-entropy node to a high-entropy one — the wire feeds a radio
that broadcasts normally. `bridge-far (H=0) → bridge-near (radiates into the observed band)` fits.
Two isolated relays chained by RF would not.

## Time-aware side classification

A pubkey survives a frequency change, so "is this node local?" is a property of a
node *during an interval*, not of the node. that repeater's last direct the observed band reception
was 18:13:21 and its first arrival via the bridge 21:21:40 — a clean transition
with no overlap. Under a window-wide boolean it reads "heard directly" and is
excluded from the far-side population by evidence that expired hours earlier.

Track **last direct reception** and **last bridge-entry reception** per node and
compare them. that repeater then reads "currently far side" at any window size, with no
tuning. This is what the manual 1h/6h/24h/3d window selector was working around.

Surface the **changepoint** as a first-class event — an operator wants to know a
node moved. A clean changepoint means migration; continuous interleaving of both
modes means dual-homing or a spoofed key. Same statistic, different verdict.

## Hygiene

- Gate advert-derived facts on `SignatureValid` (phase 1).
- Treat unresolvable hops as **unknown**, not absent. 6,012 one-byte hops were
  unresolvable over 7 days; a node whose evidence is mostly 1-byte hops deserves
  lower confidence, not silent undercounting.
- Never rank on geography. An overlapping bridge is as real as a distant one.

## Registry

An operator can mark a bridge as sanctioned (blocklist kind `known`). This is the
opposite assertion to `allow`/Dismiss, which says a candidate is NOT a bridge and
hides it: `known` says it IS one and is wanted, so it stays visible and labelled.
Nothing is blocked or hidden — a sanctioned bridge's traffic is exactly the
traffic the operator wants to keep.

Known bridges sort last however strong their evidence, so an unexpected bridge is
never buried beneath the expected one. Without this the operator's own bridge is
the top finding on every scan forever, which is the alert fatigue this work set
out to avoid.

## Validation set

Any change is measured against the table at the top before shipping. Previously
there was no way to tell whether a tweak helped.

## Phasing

1. ✅ **Signature gate** — reject adverts failing Ed25519 verification.
2. ✅ **All-payload path extraction** — path evidence from every payload type,
   origin attribution still from verified adverts only.
3. ✅ **Wired detector** — runs alongside captivity; each candidate is labelled
   with the signal(s) that produced it.
4. ✅ **Recency + changepoint** classification — side membership from recent
   evidence, and nodes that stop being heard directly reported in their own right.
5. ✅ **Known-bridge registry** — mark a bridge as sanctioned; it stays reported
   and labelled, sorted below anything unexpected.

Findings surface in the admin console; no automatic notifications.

## Results after phase 3

Both rules run and neither subsumes the other:

- **captivity** finds a bridge with a LARGE far side — many nodes with no
  alternative route in.
- **wired** finds one with a SERIAL egress, however few nodes sit behind it.

Measured on the dev mesh, the bridge is now the top candidate at every window:

    [wired]     bridge-far      pathVol=1417  nextHops=1  topShare=100%
                  behind: a far-side companion (74% transit)
    [captivity] a remote relay  pathVol=248   nextHops=7  topShare=42%

Captivity still produces its false positive, but the path evidence now sits
beside it and contradicts it at a glance: seven next hops is a radio.

Candidates are ranked by number of signals, then by packets carried — ranking on
captive count first would push a bridge carrying 1,400 packets below a relay that
squeaked past the threshold with 102.

### Phase 4

Side membership now goes on recency: a node whose last direct reception trails
its relayed traffic by more than `migrationGap` (2h) is no longer local. One
transmission normally yields a direct reception and its relayed copies within
seconds, so a lag that large means the node is transmitting and no longer being
heard directly.

Nodes crossing that line are reported as their own list. The known migration is
caught with the right timestamps and attributed:

    a repeater that moved  lastDirect 2026-07-18T18:13:21  relayedAfter=70
                       -> now behind bridge-far

Attribution counts transits AFTER the node went quiet, not across the window: a
node that moved carries a history of pre-move traffic that never touched the
bridge, which dilutes its share below any threshold. Without a bridge named, the
node simply drifted out of every observer's earshot — a real event worth showing,
but not a bridging one. On the dev mesh a 48h window yields 8 such nodes, exactly
one of which is attributed to a bridge.

**This pass depends on chronological iteration.** `RawWindow` returns newest
first; the scan walks it in reverse. Processed backwards, a node's older direct
reception arrives after its newer relayed ones and resets their count to zero,
hiding precisely the migrations being looked for.

`minWiredPackets = 100` is the bar for a single observed next hop to count as
evidence. That fingerprint alone is not enough: an ordinary repeater with exactly
one reachable neighbour looks identical in the path data and is far more common.
What separates them is FUNCTION — a bridge carries a far side, a chained repeater
carries nothing — so a wired candidate must have at least one node reaching the
mesh through it. Requiring that removed every ordinary repeater from the console
while keeping the bridge.

The cost is real: a bridge whose far side is silent in the selected window no
longer appears in it. The bridge drops out of a 6h window for that reason and is
present at 24h and 7d. A bridge carrying nothing is arguably not something to
act on, but the shorter windows are now less sensitive. Nodes listed as "behind" a wired relay must route ≥25%
of their traffic through it, or the list fills with nodes that crossed it once
while flooding.
