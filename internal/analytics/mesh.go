package analytics

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jjkroell/ridgeline/internal/meshcore"
	"github.com/jjkroell/ridgeline/internal/store"
)

// MeshAnalytics is a mesh-wide aggregate over a rolling window of observations —
// the data behind the dedicated analytics page. Unlike the per-node snapshot it
// answers "how is the whole network doing": traffic mix, RF/link health, channel
// busy-ness and the busiest relays. Computed on demand from stored raw_hex so the
// window is caller-selectable.
type MeshAnalytics struct {
	GeneratedAt   string             `json:"generatedAt"`
	WindowHours   float64            `json:"windowHours"`
	Radio         RadioParams        `json:"radio"`
	KPIs          MeshKPIs           `json:"kpis"`
	PayloadTypes  []NameCount        `json:"payloadTypes"`
	RouteTypes    []NameCount        `json:"routeTypes"`
	LinkScoreHist []HistogramBin     `json:"linkScoreHist"`
	SNRHist       []HistogramBin     `json:"snrHist"`
	Airtime       []AirtimeBucket    `json:"airtime"`
	TopRelays     []RelayRank        `json:"topRelays"`
	Observers     []ObserverCoverage `json:"observers"`
	DirectLinks   []DirectLink       `json:"directLinks"`
	DirectReach   []HistogramBin     `json:"directReach"`
	HashSizes     []NameCount        `json:"hashSizes"`
	Topology      Topology           `json:"topology"`
}

// Topology is the mesh relay graph: nodes that forwarded traffic and the
// node-to-node edges where one relayed a packet immediately after another in an
// observed flood path. Unlike the observer↔node RF graph (direct, zero-hop
// links), this is the inferred repeater backbone — who hands off to whom.
type Topology struct {
	Nodes []TopologyNode `json:"nodes"`
	Edges []TopologyEdge `json:"edges"`
}

// TopologyNode is one relaying node in the mesh graph. Relayed is the number of
// distinct transmissions it forwarded in the window (its weight/size in the viz).
type TopologyNode struct {
	PublicKey string `json:"publicKey"`
	Name      string `json:"name"`
	Role      string `json:"role"`
	Relayed   int    `json:"relayed"`
}

// TopologyEdge is an undirected adjacency between two relays, weighted by how
// many flood paths placed them consecutively. A and B are pubkeys (A < B).
type TopologyEdge struct {
	A      string `json:"a"`
	B      string `json:"b"`
	Weight int    `json:"weight"`
}

// ObserverCoverage summarises one observer's RF reach: how much it heard, how many
// distinct nodes' adverts reached it, and how many of those it heard directly
// (zero-hop) — i.e. its true RF neighbours.
type ObserverCoverage struct {
	// ID is the observer's stable identity (its public key); Name is the label.
	ID            string `json:"id"`
	Name          string `json:"name,omitempty"`
	Region        string `json:"region,omitempty"`
	Observations  int    `json:"observations"`
	DistinctNodes int    `json:"distinctNodes"`
	DirectNodes   int    `json:"directNodes"`
	// ClockSkewMs is the observer's median receive-time deviation from consensus
	// across packets it heard alongside other observers — a clock-drift / data-
	// quality signal. Nil when it shared too few packets to estimate.
	ClockSkewMs *float64 `json:"clockSkewMs,omitempty"`
}

// DirectLink is a true RF edge: an observer that heard a node's advert at zero
// hops, so the two are within direct radio range. Count is how many times.
type DirectLink struct {
	// Observer is the observer's stable identity; ObserverName is the label.
	Observer     string `json:"observer"`
	ObserverName string `json:"observerName,omitempty"`
	NodeKey      string `json:"nodeKey"`
	NodeName     string `json:"nodeName"`
	Role         string `json:"role"`
	Count        int    `json:"count"`
}

// MeshKPIs are the single-number headline tiles.
type MeshKPIs struct {
	ActiveNodes     int      `json:"activeNodes"`   // distinct nodes that originated or relayed
	Transmissions   int      `json:"transmissions"` // unique logical packets (by messageHash)
	Observations    int      `json:"observations"`  // raw reception rows
	AvgLinkScore    *float64 `json:"avgLinkScore,omitempty"`
	FloodRedundancy *float64 `json:"floodRedundancy,omitempty"` // observations / transmission
	ChannelUtilPct  float64  `json:"channelUtilPct"`            // est. logical airtime / window
	CongestionTier  string   `json:"congestionTier"`            // Quiet | Normal | Busy | Congested
}

// NameCount is one labelled tally (payload-type / route-type breakdowns).
type NameCount struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

// HistogramBin is one bucket of a distribution (link score, SNR).
type HistogramBin struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

// AirtimeBucket is one time slice of estimated channel airtime, with the
// relay-health signals (forwarded transmissions and mean link score) over the
// same slice so the timeline doubles as a trend of how the mesh is performing.
type AirtimeBucket struct {
	Timestamp     string   `json:"timestamp"` // bucket start, RFC3339
	AirtimeMs     float64  `json:"airtimeMs"`
	UtilPct       float64  `json:"utilPct"`
	Transmissions int      `json:"transmissions"`
	RelayTx       int      `json:"relayTx"`                // transmissions that were relayed (≥1 hop)
	AvgLinkScore  *float64 `json:"avgLinkScore,omitempty"` // mean per-reception link score in slice
}

// RelayRank is one node on the busiest-relays leaderboard.
type RelayRank struct {
	PublicKey string  `json:"publicKey"`
	Name      string  `json:"name"`
	Role      string  `json:"role"`
	Relayed   int     `json:"relayed"`   // distinct transmissions this node forwarded
	AirtimeMs float64 `json:"airtimeMs"` // est. airtime of those transmissions (channel load)
}

// txAgg collapses every observation of one logical packet (messageHash) into a
// single transmission, so distributions and airtime count it once regardless of
// how many observers heard it.
type txAgg struct {
	payloadType string
	routeType   string
	lengthBytes int
	firstSeen   time.Time
	relayed     bool // any observation of it carried ≥1 relay hop
}

// obsTime is one observer's reception time of a transmission, for clock-skew.
type obsTime struct {
	obs string
	t   time.Time
}

// MeshSummary scans stored observations received at or after sinceISO (decoding
// up to scanCap rows) and aggregates them into a mesh-wide snapshot. Channel
// utilisation counts each logical packet's airtime once (a lower bound — it does
// not multiply by physical flood retransmissions we can't disambiguate), so the
// percentage is a stable congestion trend rather than an exact duty figure.
func MeshSummary(st *store.Store, nodes []store.Node, sinceISO string, scanCap int, radio RadioParams, bucketMinutes int) (*MeshAnalytics, error) {
	if scanCap <= 0 || scanCap > 300000 {
		scanCap = 120000
	}
	if bucketMinutes <= 0 {
		bucketMinutes = 10
	}
	raws, err := st.RawWindow(sinceISO, scanCap)
	if err != nil {
		return nil, err
	}
	resolve := newPrefixResolver(nodes)
	byKey := make(map[string]store.Node, len(nodes))
	for _, n := range nodes {
		byKey[n.PublicKey] = n
	}

	txs := map[string]*txAgg{}                // messageHash → collapsed transmission
	active := map[string]bool{}               // distinct participating pubkeys
	relayHits := map[string]map[string]bool{} // pubkey → set of messageHashes relayed
	relayPath := map[string][]string{}        // messageHash → resolved consecutive hop sequence
	var scoreSum float64
	var scoreN int
	linkBins := make([]int, 5) // [0-.2,.2-.4,.4-.6,.6-.8,.8-1]
	snr := newSNRHist()
	observations := 0

	// Per-bucket link-score accumulator (filled per observation by receive time),
	// merged into the airtime buckets below for the relay-health trend.
	bucketSec := int64(bucketMinutes * 60)
	type linkAcc struct {
		sum float64
		n   int
	}
	bucketLink := map[int64]*linkAcc{}

	// Per-observer RF coverage + direct (zero-hop) adjacency.
	obsCount := map[string]int{}              // observer → total receptions
	obsRegion := map[string]string{}          // observer → region
	advHeard := map[string]map[string]bool{}  // observer → distinct advert nodes heard
	directLink := map[string]map[string]int{} // observer → node → zero-hop hear count
	txObs := map[string][]obsTime{}           // messageHash → (observer, time) for clock skew

	for _, ro := range raws {
		pkt, err := meshcore.DecodeHex(ro.RawHex)
		if err != nil || pkt == nil {
			continue
		}
		observations++
		lengthBytes := len(ro.RawHex) / 2
		recv := parseTime(ro.ReceivedAt)

		if ro.ObserverID != "" {
			obsCount[ro.ObserverID]++
			if ro.Region != "" {
				obsRegion[ro.ObserverID] = ro.Region
			}
			if !recv.IsZero() {
				txObs[pkt.MessageHash] = append(txObs[pkt.MessageHash], obsTime{obs: ro.ObserverID, t: recv})
			}
		}

		// Per-observation RF/link health.
		if ro.SNR != nil {
			sc := LinkScore(*ro.SNR, lengthBytes, radio.SpreadingFactor)
			scoreSum += sc
			scoreN++
			linkBins[linkBinIndex(sc)]++
			snr.add(*ro.SNR)
			if !recv.IsZero() {
				b := (recv.Unix() / bucketSec) * bucketSec
				la := bucketLink[b]
				if la == nil {
					la = &linkAcc{}
					bucketLink[b] = la
				}
				la.sum += sc
				la.n++
			}
		}

		// Collapse to a transmission (first sighting wins for type/length).
		if _, ok := txs[pkt.MessageHash]; !ok {
			txs[pkt.MessageHash] = &txAgg{
				payloadType: pkt.PayloadType.String(),
				routeType:   pkt.RouteType.String(),
				lengthBytes: lengthBytes,
				firstSeen:   recv,
			}
		} else if t := txs[pkt.MessageHash]; !recv.IsZero() && (t.firstSeen.IsZero() || recv.Before(t.firstSeen)) {
			t.firstSeen = recv
		}
		if len(pkt.RelayPath()) > 0 {
			txs[pkt.MessageHash].relayed = true
		}

		// Participation: advert originator + uniquely-resolved relay hops.
		if pkt.Advert != nil && pkt.Advert.PublicKey != "" {
			origin := strings.ToUpper(pkt.Advert.PublicKey)
			active[origin] = true
			// RF coverage: this observer heard this node's advert (at any hop count).
			if ro.ObserverID != "" {
				if advHeard[ro.ObserverID] == nil {
					advHeard[ro.ObserverID] = map[string]bool{}
				}
				advHeard[ro.ObserverID][origin] = true
				// Zero-hop = direct RF range between observer and node.
				if len(pkt.RelayPath()) == 0 {
					if directLink[ro.ObserverID] == nil {
						directLink[ro.ObserverID] = map[string]int{}
					}
					directLink[ro.ObserverID][origin]++
				}
			}
		}
		var seq []string
		for _, hop := range pkt.RelayPath() {
			rk := strings.ToUpper(resolve(hop))
			if rk == "" {
				continue
			}
			active[rk] = true
			if relayHits[rk] == nil {
				relayHits[rk] = map[string]bool{}
			}
			relayHits[rk][pkt.MessageHash] = true
			seq = append(seq, rk)
		}
		// Record one representative resolved hop sequence per transmission, for the
		// node-to-node topology graph (avoids inflating edges by observer count).
		if len(seq) > 1 {
			if _, ok := relayPath[pkt.MessageHash]; !ok {
				relayPath[pkt.MessageHash] = seq
			}
		}
	}

	out := &MeshAnalytics{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Radio:       radio,
	}

	// Window length from the oldest retained row (or the requested since).
	since := parseTime(sinceISO)
	windowSec := time.Since(since).Seconds()
	if windowSec <= 0 {
		windowSec = float64(len(raws)) // fallback; avoids div-by-zero
	}
	out.WindowHours = windowSec / 3600.0

	// KPIs.
	out.KPIs.ActiveNodes = len(active)
	out.KPIs.Transmissions = len(txs)
	out.KPIs.Observations = observations
	if scoreN > 0 {
		v := scoreSum / float64(scoreN)
		out.KPIs.AvgLinkScore = &v
	}
	if len(txs) > 0 {
		v := float64(observations) / float64(len(txs))
		out.KPIs.FloodRedundancy = &v
	}

	// Distributions + airtime (per transmission).
	payload := map[string]int{}
	route := map[string]int{}
	buckets := map[int64]*AirtimeBucket{}
	var totalAirtime float64
	for _, t := range txs {
		payload[t.payloadType]++
		route[t.routeType]++
		ms := Airtime(t.lengthBytes, radio)
		totalAirtime += ms
		if !t.firstSeen.IsZero() {
			b := (t.firstSeen.Unix() / bucketSec) * bucketSec
			bk := buckets[b]
			if bk == nil {
				bk = &AirtimeBucket{Timestamp: time.Unix(b, 0).UTC().Format(time.RFC3339)}
				buckets[b] = bk
			}
			bk.AirtimeMs += ms
			bk.Transmissions++
			if t.relayed {
				bk.RelayTx++
			}
		}
	}
	out.PayloadTypes = sortedCounts(payload)
	out.RouteTypes = sortedCounts(route)
	if windowSec > 0 {
		out.KPIs.ChannelUtilPct = totalAirtime / (windowSec * 1000.0) * 100.0
	}

	// Airtime time series, chronological, with per-bucket utilisation.
	bk := make([]AirtimeBucket, 0, len(buckets))
	for ts, v := range buckets {
		v.UtilPct = v.AirtimeMs / (float64(bucketSec) * 1000.0) * 100.0
		if la := bucketLink[ts]; la != nil && la.n > 0 {
			avg := la.sum / float64(la.n)
			v.AvgLinkScore = &avg
		}
		bk = append(bk, *v)
	}
	sort.Slice(bk, func(i, j int) bool { return bk[i].Timestamp < bk[j].Timestamp })
	out.Airtime = bk

	// Histograms.
	linkLabels := []string{"0–0.2", "0.2–0.4", "0.4–0.6", "0.6–0.8", "0.8–1.0"}
	for i, c := range linkBins {
		out.LinkScoreHist = append(out.LinkScoreHist, HistogramBin{Label: linkLabels[i], Count: c})
	}
	out.SNRHist = snr.result()

	// Top relays.
	ranks := make([]RelayRank, 0, len(relayHits))
	for pk, hashes := range relayHits {
		n := byKey[pk]
		name := displayName(n, pk)
		var air float64
		for h := range hashes {
			if t := txs[h]; t != nil {
				air += Airtime(t.lengthBytes, radio)
			}
		}
		ranks = append(ranks, RelayRank{PublicKey: pk, Name: name, Role: n.Role, Relayed: len(hashes), AirtimeMs: air})
	}
	sort.Slice(ranks, func(i, j int) bool { return ranks[i].Relayed > ranks[j].Relayed })
	if len(ranks) > 15 {
		ranks = ranks[:15]
	}
	out.TopRelays = ranks

	// Observer coverage.
	obsNames, _ := st.ObserverNames()
	cov := make([]ObserverCoverage, 0, len(obsCount))
	for id, n := range obsCount {
		cov = append(cov, ObserverCoverage{
			ID:            id,
			Name:          obsNames[id],
			Region:        obsRegion[id],
			Observations:  n,
			DistinctNodes: len(advHeard[id]),
			DirectNodes:   len(directLink[id]),
		})
	}
	sort.Slice(cov, func(i, j int) bool { return cov[i].Observations > cov[j].Observations })
	out.Observers = cov

	// Direct RF links (zero-hop adjacency) + how many observers reach each node.
	links := []DirectLink{}
	reachByNode := map[string]int{} // node → distinct observers hearing it directly
	for obsID, nodes := range directLink {
		for nk, c := range nodes {
			reachByNode[nk]++
			n := byKey[nk]
			links = append(links, DirectLink{Observer: obsID, ObserverName: obsNames[obsID], NodeKey: nk, NodeName: displayName(n, nk), Role: n.Role, Count: c})
		}
	}
	sort.Slice(links, func(i, j int) bool { return links[i].Count > links[j].Count })
	if len(links) > 600 {
		links = links[:600]
	}
	out.DirectLinks = links

	// Direct-reach redundancy: nodes heard directly by 1 / 2 / 3 / 4+ observers.
	reachBins := make([]int, 4)
	for _, r := range reachByNode {
		switch {
		case r <= 1:
			reachBins[0]++
		case r == 2:
			reachBins[1]++
		case r == 3:
			reachBins[2]++
		default:
			reachBins[3]++
		}
	}
	reachLabels := []string{"1 observer", "2 observers", "3 observers", "4+ observers"}
	for i, c := range reachBins {
		out.DirectReach = append(out.DirectReach, HistogramBin{Label: reachLabels[i], Count: c})
	}

	// Clock skew: per-observer median deviation from each transmission's consensus
	// time, surfacing drifting observer clocks.
	skew := clockSkew(txObs, 5)
	for i := range out.Observers {
		if v, ok := skew[out.Observers[i].ID]; ok {
			vv := v
			out.Observers[i].ClockSkewMs = &vv
		}
	}

	// Hash-size (identity) distribution across the known node set.
	hashCounts := map[string]int{}
	for _, n := range nodes {
		switch n.HashSize {
		case 1, 2, 3:
			hashCounts[strconv.Itoa(n.HashSize)+"-byte"]++
		default:
			hashCounts["unknown"]++
		}
	}
	for _, lbl := range []string{"1-byte", "2-byte", "3-byte", "unknown"} {
		if c := hashCounts[lbl]; c > 0 {
			out.HashSizes = append(out.HashSizes, NameCount{Label: lbl, Count: c})
		}
	}

	// Congestion tier from estimated channel utilisation (lower-bound, so the
	// thresholds are tuned to this mesh's observed range).
	out.KPIs.CongestionTier = congestionTier(out.KPIs.ChannelUtilPct)

	// Relay backbone (node-to-node topology) from the representative flood paths.
	out.Topology = buildTopology(relayPath, relayHits, byKey, 60)

	return out, nil
}

// buildTopology turns the per-transmission resolved hop sequences into an
// undirected relay graph: an edge for each consecutive pair of hops, weighted by
// how many paths used it. Nodes are ranked by distinct transmissions forwarded
// and capped at maxNodes; edges are kept only between surviving nodes so the
// payload (and the viz) stays legible on a busy mesh.
func buildTopology(relayPath map[string][]string, relayHits map[string]map[string]bool, byKey map[string]store.Node, maxNodes int) Topology {
	// Aggregate undirected edge weights from consecutive hops.
	edgeW := map[[2]string]int{}
	for _, seq := range relayPath {
		for i := 0; i+1 < len(seq); i++ {
			a, b := seq[i], seq[i+1]
			if a == b {
				continue
			}
			if a > b {
				a, b = b, a
			}
			edgeW[[2]string{a, b}]++
		}
	}

	// Rank relaying nodes by distinct transmissions forwarded; keep the top set.
	type nr struct {
		key string
		n   int
	}
	ranked := make([]nr, 0, len(relayHits))
	for pk, hashes := range relayHits {
		ranked = append(ranked, nr{key: pk, n: len(hashes)})
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].n != ranked[j].n {
			return ranked[i].n > ranked[j].n
		}
		return ranked[i].key < ranked[j].key
	})
	if maxNodes > 0 && len(ranked) > maxNodes {
		ranked = ranked[:maxNodes]
	}
	keep := make(map[string]bool, len(ranked))
	out := Topology{Nodes: []TopologyNode{}, Edges: []TopologyEdge{}}
	for _, r := range ranked {
		keep[r.key] = true
		n := byKey[r.key]
		out.Nodes = append(out.Nodes, TopologyNode{
			PublicKey: r.key, Name: displayName(n, r.key), Role: n.Role, Relayed: r.n,
		})
	}
	for e, w := range edgeW {
		if keep[e[0]] && keep[e[1]] {
			out.Edges = append(out.Edges, TopologyEdge{A: e[0], B: e[1], Weight: w})
		}
	}
	sort.Slice(out.Edges, func(i, j int) bool { return out.Edges[i].Weight > out.Edges[j].Weight })
	return out
}

// congestionTier buckets estimated channel utilisation into a coarse mesh-load
// label. Thresholds are calibrated to the lower-bound utilisation figure.
func congestionTier(utilPct float64) string {
	switch {
	case utilPct >= 5:
		return "Congested"
	case utilPct >= 2:
		return "Busy"
	case utilPct >= 0.5:
		return "Normal"
	default:
		return "Quiet"
	}
}

func medianFloat(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

// clockSkew computes each observer's median receive-time deviation (ms) from the
// per-transmission consensus (median) time, over transmissions heard by at least
// two observers. Observers with fewer than minSamples deviations are omitted, as
// too few shared packets give an unreliable estimate. Shared by the mesh-wide and
// per-observer analytics.
func clockSkew(txObs map[string][]obsTime, minSamples int) map[string]float64 {
	dev := map[string][]float64{}
	for _, grp := range txObs {
		seen := map[string]bool{}
		for _, e := range grp {
			seen[e.obs] = true
		}
		if len(seen) < 2 {
			continue
		}
		ms := make([]float64, len(grp))
		for i, e := range grp {
			ms[i] = float64(e.t.UnixNano()) / 1e6
		}
		consensus := medianFloat(ms)
		for i, e := range grp {
			dev[e.obs] = append(dev[e.obs], ms[i]-consensus)
		}
	}
	out := map[string]float64{}
	for obs, ds := range dev {
		if len(ds) >= minSamples {
			out[obs] = medianFloat(ds)
		}
	}
	return out
}

// snrHist accumulates SNR observations into a fixed set of dB buckets shared by
// the mesh-wide and per-observer SNR distributions.
type snrHist struct{ bins []int }

var snrHistEdges = []float64{-20, -15, -10, -5, 0, 5, 10}

func newSNRHist() *snrHist { return &snrHist{bins: make([]int, len(snrHistEdges)+1)} }

func (h *snrHist) add(v float64) { h.bins[snrBinIndex(v, snrHistEdges)]++ }

func (h *snrHist) result() []HistogramBin {
	out := make([]HistogramBin, len(h.bins))
	for i, c := range h.bins {
		out[i] = HistogramBin{Label: snrBinLabel(i, snrHistEdges), Count: c}
	}
	return out
}

// linkBinIndex maps a 0..1 link score to one of 5 equal bins; 1.0 lands in the
// top bin.
func linkBinIndex(score float64) int {
	i := int(score * 5)
	if i > 4 {
		i = 4
	}
	if i < 0 {
		i = 0
	}
	return i
}

func snrBinIndex(snr float64, edges []float64) int {
	for i, e := range edges {
		if snr < e {
			return i
		}
	}
	return len(edges)
}

func snrBinLabel(i int, edges []float64) string {
	if i == 0 {
		return "< " + ftoa(edges[0])
	}
	if i == len(edges) {
		return "≥ " + ftoa(edges[len(edges)-1])
	}
	return ftoa(edges[i-1]) + "–" + ftoa(edges[i])
}

func ftoa(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

func sortedCounts(m map[string]int) []NameCount {
	out := make([]NameCount, 0, len(m))
	for k, v := range m {
		out = append(out, NameCount{Label: k, Count: v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Label < out[j].Label
	})
	return out
}
