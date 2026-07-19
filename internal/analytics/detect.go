package analytics

import (
	"math"
	"sort"
	"strings"
	"time"

	"github.com/jjkroell/ridgeline/internal/meshcore"
	"github.com/jjkroell/ridgeline/internal/store"
)

// Detection thresholds. Deliberately conservative — these surface *candidates*
// for an admin to confirm, not auto-bans.
const (
	captiveTransit = 0.95 // a foreign node is "captive" to a relay if ≥95% of its
	// individual observed paths transit that relay (no alt route)
	minCaptiveNodes    = 3   // captive foreign nodes needed to flag a bridge
	minCaptiveFraction = 0.6 // captive must be a majority of the relay's foreign set
	minExclusiveNodes  = 3   // nodes sourced by only one observer, to flag an injector

	// minWiredPackets is how much traffic a relay must have carried before a
	// single observed next hop counts as evidence of a wired egress rather than
	// coincidence. RF is broadcast, so a radiating relay accumulates alternative
	// next hops as samples grow — the median relay here reaches 13. Seeing none
	// over this many packets is not something a radio does; the measured bridge
	// sat at 1 over 1,417. It cannot, however, distinguish a wire from a relay
	// with exactly one reachable neighbour, so these are candidates for review
	// (and the console's Dismiss action exists for the latter).
	minWiredPackets = 100

	// minBehindTransit is the share of its traffic an origin must route through a
	// wired relay before being listed as sitting behind it. Without a floor the
	// list fills with nodes that happened to cross it once — 1% transit is a
	// coincidence of flooding, not a topology claim.
	minBehindTransit = 0.25

	// migrationGap is how far a node's last DIRECT reception may lag its most
	// recent RELAYED one before it stops counting as local. A pubkey survives a
	// frequency change, so "is this node local" is a property of a node during an
	// interval, not of the node: a node that moved to the far side still carries
	// direct receptions from before the move, and a window-wide boolean lets that
	// expired evidence mask the move indefinitely. One transmission normally
	// yields a direct reception and its relayed copies within seconds, so a lag
	// this large means the node is transmitting but no longer being heard directly.
	migrationGap = 2 * time.Hour

	// minRelayedAfterMove is how many relayed receptions must arrive after a node
	// stops being heard directly before the change is reported as a move rather
	// than a lull.
	minRelayedAfterMove = 20
)

// Signal names reported on a candidate, so an operator can see which rule fired.
const (
	signalCaptivity = "captivity" // a population of nodes with no alternative route in
	signalWired     = "wired"     // an egress that never varies — a cable, not an antenna
)

// InjectionReport lists detected ingress points for foreign/injected traffic.
type InjectionReport struct {
	WindowHours float64 `json:"windowHours"`
	// AdvertsScanned counts adverts decoded in the window; AdvertsRejected counts
	// those dropped because their Ed25519 signature did not verify. Surfaced so an
	// operator can see how much of the traffic was unusable rather than wondering
	// why a busy window produced few candidates.
	AdvertsScanned  int `json:"advertsScanned"`
	AdvertsRejected int `json:"advertsRejected"`
	// PacketsScanned counts every decoded packet; PathsScanned those carrying at
	// least one hop; UnresolvedHops those whose hash prefix matched no single node
	// (ambiguous 1-byte hops are common). A candidate resting mostly on
	// unresolvable hops deserves less confidence, so the totals are surfaced.
	PacketsScanned int                 `json:"packetsScanned"`
	PathsScanned   int                 `json:"pathsScanned"`
	UnresolvedHops int                 `json:"unresolvedHops"`
	Bridges        []BridgeCandidate   `json:"bridges"`   // RF bridges
	Injectors      []InjectorCandidate `json:"injectors"` // rogue MQTT publishers
	Migrations     []MigrationEvent    `json:"migrations"`
}

// ForeignNode is a node identified as injected (heard only via a bridge/injector).
type ForeignNode struct {
	Key       string   `json:"key"`
	Name      string   `json:"name"`
	Role      string   `json:"role,omitempty"`
	Latitude  *float64 `json:"latitude,omitempty"`
	Longitude *float64 `json:"longitude,omitempty"`
	// TransitPct is the share of this node's observed paths that go through the
	// candidate (bridge candidates only); Captive marks ≥captiveTransit.
	TransitPct float64 `json:"transitPct,omitempty"`
	Captive    bool    `json:"captive,omitempty"`
}

// BridgeCandidate is a node that is the sole ingress for a cluster of
// never-directly-heard nodes — the RF-bridge signature. The discriminating
// signal is captivity: a true bridge is on ~100% of each foreign node's observed
// paths (no alternative route), whereas a legitimate relay serving an
// observer-less area has foreign nodes reachable by many other routes.
type BridgeCandidate struct {
	NodeKey         string  `json:"nodeKey"`
	Name            string  `json:"name"`
	CaptiveCount    int     `json:"captiveCount"`    // foreign nodes ≥95% captive to it
	ForeignThrough  int     `json:"foreignThrough"`  // foreign nodes routed through it at all
	CaptiveFraction float64 `json:"captiveFraction"` // captiveCount / foreignThrough
	// ForeignKm is the geographic displacement of the captive cluster from the
	// mesh centroid. Shown as a corroborator only — NOT used for ranking, so an
	// overlapping (co-located) bridge ranks the same as a distant one.
	ForeignKm float64       `json:"foreignKm"`
	Foreign   []ForeignNode `json:"foreign"` // foreign nodes through it, by transit share desc

	// Path evidence, gathered from EVERY payload type rather than adverts alone.
	// PathVolume is how many packets this relay carried. NextHops is how many
	// distinct relays it was ever observed handing off to, and NextHopTopShare the
	// share taken by its most common one.
	//
	// These describe how the relay behaves physically. RF is broadcast, so which
	// neighbour picks a packet up next varies: on this mesh the median relay hands
	// off to 13 distinct next hops with a 44% top share. A relay whose egress is a
	// WIRE has exactly one possible next hop forever — the measured bridge sat at
	// 1 distinct hop over 1,417 packets. Reported here for review; ranking on it
	// is deliberately left to a later change.
	PathVolume      int     `json:"pathVolume"`
	NextHops        int     `json:"nextHops"`
	NextHopTopShare float64 `json:"nextHopTopShare"`
	// TerminalShare is the share of carried packets where this relay was the last
	// hop — where an observer received its own transmission. Zero over meaningful
	// volume means it transmits where nothing is listening, which is what a
	// bridge's far-side half does. Displayed as a corroborator; nothing ranks on
	// it. On the dev mesh only 2 of 134 relays sit at zero, and only one of those
	// also has a single next hop.
	TerminalShare float64 `json:"terminalShare"`

	// Signals names which rule produced this candidate — "captivity", "wired", or
	// both. They catch different things and neither subsumes the other: captivity
	// finds a LARGE far side (many nodes with no alternative route in), wired
	// finds a SERIAL one (a relay whose egress never varies) no matter how few
	// nodes sit behind it. The bridge that motivated this work has only two
	// adverting far-side nodes and is invisible to captivity entirely.
	Signals []string `json:"signals"`

	// Known marks a bridge the operator has sanctioned. It stays in the report —
	// the bridge is real and worth seeing — but it is not a finding that needs
	// acting on, and it sorts last so genuine news stays at the top.
	Known bool `json:"known,omitempty"`
}

// MigrationEvent records a node that stopped being heard directly and whose
// traffic now arrives through a bridge — it changed sides. The pubkey is
// unchanged, so nothing else in the system notices it moved.
//
// Losing direct reception has several causes: a frequency change, the node
// moving, an antenna or propagation change, or the observer that used to hear it
// going offline. Only the first is a side change, and a bridge picking the node
// up afterwards is what distinguishes it — so events without an attributable
// bridge are not reported here. On the dev mesh that is the difference between
// one event and eight.
type MigrationEvent struct {
	Key          string `json:"key"`
	Name         string `json:"name"`
	Role         string `json:"role,omitempty"`
	LastDirectAt string `json:"lastDirectAt"` // last time an observer heard it at zero hops
	LastRelayAt  string `json:"lastRelayAt"`  // most recent relayed reception
	RelayedAfter int    `json:"relayedAfter"` // relayed receptions since it went quiet directly
	// ViaBridge names a bridge candidate that carries this node's traffic, when
	// one does. That is the difference between "moved behind a bridge" and the
	// far more common "drifted out of every observer's earshot" — both stop being
	// heard directly, and only the first is about bridging.
	ViaBridge string `json:"viaBridge,omitempty"`
}

// InjectorCandidate is an observer that is the sole source of a population of
// nodes — the rogue-MQTT-publisher signature.
type InjectorCandidate struct {
	Observer       string        `json:"observer"`
	ExclusiveCount int           `json:"exclusiveCount"`
	Exclusive      []ForeignNode `json:"exclusive"`
}

// DetectInjection scans a window of observations and flags likely ingress points
// for foreign traffic, by two independent signatures:
//
//   - RF bridge: a node that is the captive ingress for ≥3 never-directly-heard
//     nodes — i.e. ≥95% of each such node's observed paths transit it AND those
//     captive nodes are the majority of its foreign through-traffic. This is a
//     physical consequence of the foreign nodes being on another frequency (no
//     alternative route in), so it holds regardless of geography.
//   - MQTT injector: an observer that is the *only* source of many origins no
//     other observer ever reports.
//
// Origin classification keys on adverts (the only packets whose origin is in the
// clear); that is exactly the foreign-node population an admin wants to remove.
func DetectInjection(st *store.Store, nodes []store.Node, sinceISO string, scanCap int) (*InjectionReport, error) {
	if scanCap <= 0 || scanCap > 300000 {
		scanCap = 120000
	}
	raws, err := st.RawWindow(sinceISO, scanCap)
	if err != nil {
		return nil, err
	}
	byKey := make(map[string]store.Node, len(nodes))
	for _, n := range nodes {
		byKey[strings.ToUpper(n.PublicKey)] = n
	}
	resolve := newPrefixResolver(nodes)

	var packets, pathed, unresolved int // all payload types
	var scanned, rejected int           // adverts seen / dropped as unverifiable
	// Direct vs relayed reception times per origin. Recency, not a boolean: see
	// migrationGap for why a node's history cannot vouch for its present.
	lastDirect := map[string]string{} // origin -> newest zero-hop reception
	lastRelay := map[string]string{}  // origin -> newest relayed reception
	relayedSince := map[string]int{}  // origin -> relayed receptions after lastDirect
	// viaAfter[relay][origin] counts transits that happened AFTER the origin was
	// last heard directly. Attribution needs this rather than the window-wide
	// count: a node that moved carries a whole history of pre-move traffic that
	// never touched the bridge, which dilutes its share below any threshold.
	viaAfter := map[string]map[string]int{}
	reporters := map[string]map[string]bool{} // origin -> set of observer ids
	obsTotal := map[string]int{}              // origin -> # of its observed (pathed) adverts
	// via[relay][origin] = # of origin's observed paths that include relay. The
	// per-observation count (not a union) is what lets us measure captivity.
	via := map[string]map[string]int{}
	// Path facts collected from EVERY payload type, not just adverts. A packet's
	// route is in the clear regardless of whether its payload is; only the
	// *origin* needs an advert to attribute. Restricting path evidence to adverts
	// discarded most of what a bridge reveals about itself — a companion that
	// never adverts contributed nothing at all, despite its messages crossing the
	// bridge with a full path attached.
	adjacency := map[string]map[string]int{} // relay -> next relay -> times observed
	relayVolume := map[string]int{}          // relay -> packets it carried
	// terminalCount[relay] = packets where this relay was the LAST hop, meaning an
	// observer received that relay's own transmission. A relay transmitting on a
	// frequency nobody monitors can never be terminal, however much traffic it
	// carries — its packets only become observable once something else re-sends
	// them. Independent of next-hop entropy: that says the egress never varies,
	// this says the transmission is never heard.
	terminalCount := map[string]int{}

	// RawWindow returns newest-first; walk it in reverse so the scan runs in
	// chronological order. The direct/relayed recency tracking below accumulates
	// forward in time — processed backwards, a node's older direct reception
	// would arrive after its newer relayed ones and reset their count to zero,
	// hiding exactly the migrations this is meant to find.
	for i := len(raws) - 1; i >= 0; i-- {
		ro := raws[i]
		pkt, err := meshcore.DecodeHex(ro.RawHex)
		if err != nil || pkt == nil {
			continue
		}
		packets++

		// --- Path facts: every payload type contributes. ---
		if len(pkt.Path) > 0 {
			pathed++
			prev := ""
			carried := map[string]bool{}
			for _, h := range pkt.Path {
				k := resolve(h)
				if k == "" {
					// An ambiguous hash prefix is UNKNOWN, not absent: it breaks the
					// adjacency chain rather than joining the hops either side of it,
					// which would fabricate a link that was never observed.
					unresolved++
					prev = ""
					continue
				}
				ku := strings.ToUpper(k)
				if !carried[ku] {
					carried[ku] = true
					relayVolume[ku]++
				}
				if prev != "" {
					if adjacency[prev] == nil {
						adjacency[prev] = map[string]int{}
					}
					adjacency[prev][ku]++
				}
				prev = ku
			}
			if last := resolve(pkt.Path[len(pkt.Path)-1]); last != "" {
				terminalCount[strings.ToUpper(last)]++
			}
		}

		// --- Origin facts: verified adverts only. ---
		if pkt.Advert == nil || pkt.Advert.PublicKey == "" {
			continue
		}
		scanned++
		// Only trust adverts whose Ed25519 signature verifies: a corrupt public key
		// invents an origin that never existed, and those phantoms land squarely in
		// the injector rule ("sole source of many origins"). The signature covers
		// the advert payload, so a packet that passes has an authentic key — it
		// says nothing about the path, which is mutable by design.
		if !pkt.Advert.SignatureValid {
			rejected++
			continue
		}
		origin := strings.ToUpper(pkt.Advert.PublicKey)
		if ro.ObserverID != "" {
			if reporters[origin] == nil {
				reporters[origin] = map[string]bool{}
			}
			reporters[origin][ro.ObserverID] = true
		}
		if len(pkt.Path) == 0 {
			if ro.ReceivedAt > lastDirect[origin] {
				lastDirect[origin] = ro.ReceivedAt
				relayedSince[origin] = 0 // heard directly again: it is local now
				for _, m := range viaAfter {
					delete(m, origin)
				}
			}
			continue
		}
		if ro.ReceivedAt > lastRelay[origin] {
			lastRelay[origin] = ro.ReceivedAt
		}
		if ro.ReceivedAt > lastDirect[origin] {
			relayedSince[origin]++
		}
		obsTotal[origin]++
		seen := map[string]bool{} // dedupe relays within this one observation
		for _, h := range pkt.Path {
			k := resolve(h)
			if k == "" {
				continue
			}
			ku := strings.ToUpper(k)
			if seen[ku] {
				continue
			}
			seen[ku] = true
			if via[ku] == nil {
				via[ku] = map[string]int{}
			}
			via[ku][origin]++
			if ro.ReceivedAt > lastDirect[origin] {
				if viaAfter[ku] == nil {
					viaAfter[ku] = map[string]int{}
				}
				viaAfter[ku][origin]++
			}
		}
	}

	report := &InjectionReport{
		WindowHours:     windowHoursFrom(sinceISO),
		PacketsScanned:  packets,
		PathsScanned:    pathed,
		UnresolvedHops:  unresolved,
		AdvertsScanned:  scanned,
		AdvertsRejected: rejected,
		Bridges:         []BridgeCandidate{},
		Injectors:       []InjectorCandidate{},
		Migrations:      []MigrationEvent{},
	}
	// currentlyLocal reports whether an origin is still being heard directly. A
	// node whose last direct reception trails its relayed traffic by more than
	// migrationGap has moved out of direct earshot — the far side of a bridge, or
	// simply away — and must not be excluded from the foreign population by
	// receptions that stopped hours ago.
	currentlyLocal := func(origin string) bool {
		d, r := lastDirect[origin], lastRelay[origin]
		if d == "" {
			return false // never heard directly in this window
		}
		if r == "" || r <= d {
			return true // its newest evidence is a direct reception
		}
		rt, err1 := time.Parse(time.RFC3339Nano, r)
		dt, err2 := time.Parse(time.RFC3339Nano, d)
		if err1 != nil || err2 != nil {
			return true // unparseable: fall back to the conservative answer
		}
		return rt.Sub(dt) <= migrationGap
	}

	meshLat, meshLon, haveMesh := centroid(nodes)
	byRelay := map[string]bool{} // relays already reported, so the two rules merge
	known := st.KnownBridges()

	// Bridge candidates by captivity.
	for relay, origins := range via {
		var foreign []ForeignNode
		captive := 0
		for origin, cnt := range origins {
			if currentlyLocal(origin) {
				continue // still heard directly — not foreign
			}
			frac := float64(cnt) / float64(max(1, obsTotal[origin]))
			n := byKey[origin]
			fn := ForeignNode{
				Key: origin, Name: displayName(n, origin), Role: n.Role,
				Latitude: n.Latitude, Longitude: n.Longitude,
				TransitPct: frac * 100, Captive: frac >= captiveTransit,
			}
			foreign = append(foreign, fn)
			if fn.Captive {
				captive++
			}
		}
		if captive < minCaptiveNodes {
			continue
		}
		capFrac := float64(captive) / float64(max(1, len(foreign)))
		if capFrac < minCaptiveFraction {
			continue // most of its foreign traffic has alternative routes → legit relay
		}
		sort.Slice(foreign, func(i, j int) bool { return foreign[i].TransitPct > foreign[j].TransitPct })
		byRelay[relay] = true
		bc := BridgeCandidate{
			Signals:         []string{signalCaptivity},
			NodeKey:         relay,
			Name:            displayName(byKey[relay], relay),
			CaptiveCount:    captive,
			ForeignThrough:  len(foreign),
			CaptiveFraction: capFrac,
			Foreign:         foreign,
			PathVolume:      relayVolume[relay],
		}
		if next := adjacency[relay]; len(next) > 0 {
			total, top := 0, 0
			for _, v := range next {
				total += v
				if v > top {
					top = v
				}
			}
			bc.NextHops = len(next)
			bc.NextHopTopShare = float64(top) / float64(total)
		}
		if v := relayVolume[relay]; v > 0 {
			bc.TerminalShare = float64(terminalCount[relay]) / float64(v)
		}
		if haveMesh {
			if fLat, fLon, ok := captiveCentroid(foreign); ok {
				bc.ForeignKm = haversineKm(meshLat, meshLon, fLat, fLon)
			}
		}
		report.Bridges = append(report.Bridges, bc)
	}
	// Bridge candidates by wired egress. RF is broadcast, so a relay accumulates
	// alternative next hops as traffic grows; one that never does is handing off
	// over a cable. This is independent of how many nodes sit behind it, which is
	// what captivity measures — a bridge serving two nodes is invisible to that
	// rule but obvious here.
	for relay, next := range adjacency {
		if len(next) != 1 || relayVolume[relay] < minWiredPackets {
			continue
		}
		if byRelay[relay] {
			// Both rules fired on the same relay: label it, don't duplicate it.
			for i := range report.Bridges {
				if report.Bridges[i].NodeKey == relay {
					report.Bridges[i].Signals = append(report.Bridges[i].Signals, signalWired)
					break
				}
			}
			continue
		}
		// Everything routed through it that was never heard directly — the far
		// side, listed even though it is far below the captivity thresholds.
		var foreign []ForeignNode
		captive := 0
		for origin, cnt := range via[relay] {
			if currentlyLocal(origin) {
				continue
			}
			frac := float64(cnt) / float64(max(1, obsTotal[origin]))
			if frac < minBehindTransit {
				continue // crossed it once while flooding; not behind it
			}
			n := byKey[origin]
			fn := ForeignNode{
				Key: origin, Name: displayName(n, origin), Role: n.Role,
				Latitude: n.Latitude, Longitude: n.Longitude,
				TransitPct: frac * 100, Captive: frac >= captiveTransit,
			}
			foreign = append(foreign, fn)
			if fn.Captive {
				captive++
			}
		}
		// A wired egress is the fingerprint; carrying a far side is the function.
		// Without at least one node reaching the mesh through it, this relay is
		// bridging nothing — it is an ordinary repeater with exactly one reachable
		// neighbour, which looks identical in the path data and is far more common.
		// Requiring a far side is what separates the two.
		if len(foreign) == 0 {
			continue
		}
		sort.Slice(foreign, func(i, j int) bool { return foreign[i].TransitPct > foreign[j].TransitPct })
		bc := BridgeCandidate{
			Signals:         []string{signalWired},
			NodeKey:         relay,
			Name:            displayName(byKey[relay], relay),
			CaptiveCount:    captive,
			ForeignThrough:  len(foreign),
			CaptiveFraction: float64(captive) / float64(max(1, len(foreign))),
			Foreign:         foreign,
			PathVolume:      relayVolume[relay],
			NextHops:        1,
			NextHopTopShare: 1,
		}
		if v := relayVolume[relay]; v > 0 {
			bc.TerminalShare = float64(terminalCount[relay]) / float64(v)
		}
		if haveMesh {
			if fLat, fLon, ok := captiveCentroid(foreign); ok {
				bc.ForeignKm = haversineKm(meshLat, meshLon, fLat, fLon)
			}
		}
		report.Bridges = append(report.Bridges, bc)
	}

	// Rank by how many signals fired, then by how much traffic the claim rests on.
	// PathVolume is the evidence base for both rules — ranking on captive count
	// first would push a bridge carrying 1,400 packets below a relay that squeaked
	// past the threshold with 102. Geography is NOT a factor.
	for i := range report.Bridges {
		report.Bridges[i].Known = known[strings.ToUpper(report.Bridges[i].NodeKey)]
	}
	sort.Slice(report.Bridges, func(i, j int) bool {
		a, b := report.Bridges[i], report.Bridges[j]
		// A sanctioned bridge is not news: it sorts last however strong its
		// evidence, so an unexpected one is never buried under the expected one.
		if a.Known != b.Known {
			return !a.Known
		}
		if len(a.Signals) != len(b.Signals) {
			return len(a.Signals) > len(b.Signals)
		}
		if a.PathVolume != b.PathVolume {
			return a.PathVolume > b.PathVolume
		}
		return a.CaptiveCount > b.CaptiveCount
	})

	// Migrations: nodes that stopped being heard directly while their traffic kept
	// arriving relayed. Reported in their own right — the pubkey is unchanged, so
	// nothing else in the system notices a node has moved, and an operator wants
	// to know. A node that was never heard directly in this window is simply
	// distant, not a migration, so it needs a direct reception to have stopped.
	for origin, d := range lastDirect {
		if d == "" || currentlyLocal(origin) {
			continue
		}
		if relayedSince[origin] < minRelayedAfterMove {
			continue // too little evidence since it went quiet to call it a move
		}
		n := byKey[origin]
		ev := MigrationEvent{
			Key:          origin,
			Name:         displayName(n, origin),
			Role:         n.Role,
			LastDirectAt: d,
			LastRelayAt:  lastRelay[origin],
			RelayedAfter: relayedSince[origin],
		}
		// Attribute the move to a bridge when one carries a real share of this
		// node's traffic — otherwise it simply went out of earshot.
		for _, b := range report.Bridges {
			n := viaAfter[b.NodeKey][origin]
			if n == 0 {
				continue
			}
			if float64(n)/float64(max(1, relayedSince[origin])) >= minBehindTransit {
				ev.ViaBridge = b.Name
				break
			}
		}
		if ev.ViaBridge == "" {
			continue // out of earshot, not a side change — see the type comment
		}
		report.Migrations = append(report.Migrations, ev)
	}
	sort.Slice(report.Migrations, func(i, j int) bool {
		return report.Migrations[i].LastDirectAt > report.Migrations[j].LastDirectAt
	})

	// MQTT injector candidates: observers that are the sole source of foreign nodes.
	relayOnly := func(origin string) bool { return !currentlyLocal(origin) }
	exclusive := map[string]map[string]bool{} // observer -> origins only it reports
	for origin, reps := range reporters {
		if len(reps) != 1 || !relayOnly(origin) {
			continue
		}
		var only string
		for o := range reps {
			only = o
		}
		if exclusive[only] == nil {
			exclusive[only] = map[string]bool{}
		}
		exclusive[only][origin] = true
	}
	for obs, set := range exclusive {
		if len(set) < minExclusiveNodes {
			continue
		}
		report.Injectors = append(report.Injectors, InjectorCandidate{
			Observer:       obs,
			ExclusiveCount: len(set),
			Exclusive:      foreignNodes(set, byKey),
		})
	}
	sort.Slice(report.Injectors, func(i, j int) bool {
		return report.Injectors[i].ExclusiveCount > report.Injectors[j].ExclusiveCount
	})

	return report, nil
}

func foreignNodes(set map[string]bool, byKey map[string]store.Node) []ForeignNode {
	out := make([]ForeignNode, 0, len(set))
	for k := range set {
		n := byKey[k]
		fn := ForeignNode{Key: k, Name: displayName(n, k), Role: n.Role}
		fn.Latitude, fn.Longitude = n.Latitude, n.Longitude
		out = append(out, fn)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// centroid returns the median lat/lon of all located nodes (median resists the
// pull of a displaced foreign cluster better than the mean).
func centroid(nodes []store.Node) (lat, lon float64, ok bool) {
	var lats, lons []float64
	for _, n := range nodes {
		if n.Latitude != nil && n.Longitude != nil && (*n.Latitude != 0 || *n.Longitude != 0) {
			lats = append(lats, *n.Latitude)
			lons = append(lons, *n.Longitude)
		}
	}
	if len(lats) == 0 {
		return 0, 0, false
	}
	return median(lats), median(lons), true
}

// captiveCentroid is the median location of a candidate's captive foreign nodes.
func captiveCentroid(fs []ForeignNode) (lat, lon float64, ok bool) {
	var lats, lons []float64
	for _, f := range fs {
		if !f.Captive {
			continue
		}
		if f.Latitude != nil && f.Longitude != nil && (*f.Latitude != 0 || *f.Longitude != 0) {
			lats = append(lats, *f.Latitude)
			lons = append(lons, *f.Longitude)
		}
	}
	if len(lats) == 0 {
		return 0, 0, false
	}
	return median(lats), median(lons), true
}

func median(v []float64) float64 {
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

// haversineKm is the great-circle distance between two lat/lon points in km.
func haversineKm(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371.0
	rad := math.Pi / 180
	dLat := (lat2 - lat1) * rad
	dLon := (lon2 - lon1) * rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return R * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

func windowHoursFrom(sinceISO string) float64 {
	t := parseTime(sinceISO)
	if t.IsZero() {
		return 0
	}
	return time.Since(t).Hours()
}
