// Package analytics computes per-node aggregates (health, observer breakdown,
// neighbours, relay activity, traffic-share and bridge/betweenness scores) from
// a rolling window of stored observations. A background recompute builds an
// in-memory snapshot the API serves instantly, so node-detail requests don't
// each re-decode history.
package analytics

import (
	"sort"
	"sync"
	"time"

	"github.com/jjkroell/ridgeline/internal/meshcore"
	"github.com/jjkroell/ridgeline/internal/store"
)

// ObserverStat is one observer's reception of a node's adverts.
type ObserverStat struct {
	// ID is the observer's stable identity (its public key); Name is the label to
	// show. Name is empty when no observer row resolves the id.
	ID      string   `json:"id"`
	Name    string   `json:"name,omitempty"`
	Region  string   `json:"region,omitempty"`
	Count   int      `json:"count"`
	AvgSNR  *float64 `json:"avgSnr,omitempty"`
	AvgRSSI *float64 `json:"avgRssi,omitempty"`
}

// NeighborStat is a node adjacent to this one in observed packet paths.
type NeighborStat struct {
	PublicKey string `json:"publicKey"`
	Name      string `json:"name"`
	Role      string `json:"role"`
	Count     int    `json:"count"`
}

// PacketRef is a lightweight reference to one of a node's advert transmissions.
type PacketRef struct {
	MessageHash string   `json:"messageHash"`
	PayloadType string   `json:"payloadType"`
	ReceivedAt  string   `json:"receivedAt"`
	ObserverID  string   `json:"observerId,omitempty"`
	SNR         *float64 `json:"snr,omitempty"`
	RSSI        *float64 `json:"rssi,omitempty"`
	PathHops    int      `json:"pathHops"`
}

// RelayStat summarizes how often a node appeared as a relay hop.
type RelayStat struct {
	LastRelayed string `json:"lastRelayed,omitempty"`
	Count1h     int    `json:"count1h"`
	Count24h    int    `json:"count24h"`
	Active      bool   `json:"active"`
}

// NodeDetail is the computed analytics for one node over the window.
type NodeDetail struct {
	PublicKey         string         `json:"publicKey"`
	WindowHours       int            `json:"windowHours"`
	TotalPackets      int            `json:"totalPackets"`      // advert transmissions in window
	TotalObservations int            `json:"totalObservations"` // advert observations in window
	PacketsToday      int            `json:"packetsToday"`
	AvgSNR            *float64       `json:"avgSnr,omitempty"`
	AvgHops           *float64       `json:"avgHops,omitempty"`
	FirstHeard        string         `json:"firstHeard,omitempty"`
	LastHeard         string         `json:"lastHeard,omitempty"`
	Observers         []ObserverStat `json:"observers"`
	RecentPackets     []PacketRef    `json:"recentPackets"`
	Neighbors         []NeighborStat `json:"neighbors"`
	Relay             RelayStat      `json:"relay"`
	TrafficShare      float64        `json:"trafficShare"`
	Bridge            float64        `json:"bridge"`
	// AdvertIntervalSec is the median seconds between the node's advert
	// transmissions in the window — its heartbeat cadence. Nil with <2 adverts.
	AdvertIntervalSec *float64 `json:"advertIntervalSec,omitempty"`
	// Activity is per-hour advert-transmission counts over the window, oldest
	// bucket first, newest last (length == WindowHours).
	Activity []int `json:"activity"`
}

// Engine holds the latest analytics snapshot.
type Engine struct {
	mu          sync.RWMutex
	details     map[string]*NodeDetail
	generatedAt time.Time
	windowHours int
}

// New creates an Engine computing over the last windowHours (default 6).
func New(windowHours int) *Engine {
	if windowHours <= 0 {
		windowHours = 6
	}
	return &Engine{details: map[string]*NodeDetail{}, windowHours: windowHours}
}

// Get returns the snapshot for a node (nil if it has no activity in the window)
// and when the snapshot was generated.
func (e *Engine) Get(pubkey string) (*NodeDetail, time.Time) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.details[pubkey], e.generatedAt
}

// LiveSignal is the recent-activity summary used to compute a node's liveness
// (alongside its last advert). A relay within the window is strong proof the
// node is alive and working in the mesh, even when its advert is stale.
type LiveSignal struct {
	LastRelayed  string `json:"lastRelayed,omitempty"`
	RelayCount1h int    `json:"relayCount1h,omitempty"`
}

// Liveness returns the relay-activity signal for every node in the current
// snapshot, keyed by pubkey. Cheap copy so callers don't hold the lock.
func (e *Engine) Liveness() map[string]LiveSignal {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make(map[string]LiveSignal, len(e.details))
	for pk, d := range e.details {
		if d.Relay.LastRelayed == "" && d.Relay.Count1h == 0 {
			continue
		}
		out[pk] = LiveSignal{LastRelayed: d.Relay.LastRelayed, RelayCount1h: d.Relay.Count1h}
	}
	return out
}

// relayWindowHours is the lookback for relay activity / liveness. It's wider
// than the advert window (which drives the cadence sparkline) so relay counts
// like Count24h and lastRelayed reflect a full day, not just the advert window.
const relayWindowHours = 24

// Recompute fetches the window of observations and rebuilds the snapshot. Relay
// activity is scanned over relayWindowHours; advert stats are kept to the
// (narrower) advert window via advertCutoff so the sparkline/cadence are unchanged.
func (e *Engine) Recompute(st *store.Store, nodes []store.Node) error {
	lookback := e.windowHours
	if relayWindowHours > lookback {
		lookback = relayWindowHours
	}
	now := time.Now()
	cutoff := now.Add(-time.Duration(lookback) * time.Hour).UTC().Format(time.RFC3339Nano)
	advertCutoff := now.Add(-time.Duration(e.windowHours) * time.Hour).UTC().Format(time.RFC3339Nano)
	raws, err := st.RawWindow(cutoff, 0)
	if err != nil {
		return err
	}
	details := build(raws, nodes, e.windowHours, advertCutoff)
	// Observations carry the observer's stable id (its public key); attach the
	// friendly label so callers don't each have to resolve it.
	if names, err := st.ObserverNames(); err == nil {
		for _, d := range details {
			for i := range d.Observers {
				if n := names[d.Observers[i].ID]; n != "" {
					d.Observers[i].Name = n
				}
			}
		}
	}
	e.mu.Lock()
	e.details = details
	e.generatedAt = time.Now()
	e.mu.Unlock()
	return nil
}

// --- aggregation ---

type observerAcc struct {
	region  string
	count   int
	snrSum  float64
	snrN    int
	rssiSum float64
	rssiN   int
}

// advObs is one observation of a node's advert. MeshCore re-floods the same
// advert payload (identical messageHash), so transmissions are recovered by
// time-gap grouping per hash rather than by hash alone.
type advObs struct {
	hash       string
	t          time.Time
	receivedAt string
	observerID string
	snr, rssi  *float64
	hops       int
}

type nodeAcc struct {
	obsCount    int
	snrSum      float64
	snrN        int
	hopsSum     int
	hopsN       int
	first, last string
	observers   map[string]*observerAcc
	advs        []advObs
}

func newNodeAcc() *nodeAcc {
	return &nodeAcc{observers: map[string]*observerAcc{}}
}

// build decodes the window once and produces per-node analytics.
// build computes the snapshot. raws span the relay window; advertCutoff (an
// RFC3339Nano timestamp) bounds advert stats to the narrower advert window so
// the cadence sparkline and per-node advert counts ignore older transmissions.
func build(raws []store.RawObservation, nodes []store.Node, windowHours int, advertCutoff string) map[string]*NodeDetail {
	now := time.Now()
	today := now.UTC().Format("2006-01-02")

	byKey := make(map[string]store.Node, len(nodes))
	for _, n := range nodes {
		byKey[n.PublicKey] = n
	}
	resolve := newPrefixResolver(nodes)

	accs := map[string]*nodeAcc{}
	accFor := func(pk string) *nodeAcc {
		a := accs[pk]
		if a == nil {
			a = newNodeAcc()
			accs[pk] = a
		}
		return a
	}

	// Representative path per transmission (first/newest observation wins).
	type txPath struct {
		hops       []string // resolved pubkeys (unique matches only)
		receivedAt string
		advert     bool
	}
	txPaths := map[string]*txPath{}

	for _, ro := range raws {
		pkt, err := meshcore.DecodeHex(ro.RawHex)
		if err != nil || pkt == nil {
			continue
		}
		hash := pkt.MessageHash

		// Per-transmission representative path (newest seen, since rows are DESC).
		if _, ok := txPaths[hash]; !ok {
			resolved := make([]string, 0, len(pkt.RelayPath()))
			for _, hop := range pkt.RelayPath() {
				if pk := resolve(hop); pk != "" {
					resolved = append(resolved, pk)
				}
			}
			txPaths[hash] = &txPath{hops: resolved, receivedAt: ro.ReceivedAt, advert: pkt.Advert != nil}
		}

		// Advert attribution → the advertising node's own stats. Bounded to the
		// advert window so the wider relay scan doesn't inflate advert counts.
		if pkt.Advert != nil && ro.ReceivedAt >= advertCutoff {
			a := accFor(pkt.Advert.PublicKey)
			a.obsCount++
			if ro.SNR != nil {
				a.snrSum += *ro.SNR
				a.snrN++
			}
			a.hopsSum += pkt.PathHopCount
			a.hopsN++
			if a.first == "" || ro.ReceivedAt < a.first {
				a.first = ro.ReceivedAt
			}
			if ro.ReceivedAt > a.last {
				a.last = ro.ReceivedAt
			}
			oa := a.observers[ro.ObserverID]
			if oa == nil {
				oa = &observerAcc{region: ro.Region}
				a.observers[ro.ObserverID] = oa
			}
			oa.count++
			if ro.SNR != nil {
				oa.snrSum += *ro.SNR
				oa.snrN++
			}
			if ro.RSSI != nil {
				oa.rssiSum += *ro.RSSI
				oa.rssiN++
			}
			a.advs = append(a.advs, advObs{
				hash: hash, t: parseTime(ro.ReceivedAt), receivedAt: ro.ReceivedAt,
				observerID: ro.ObserverID, snr: ro.SNR, rssi: ro.RSSI, hops: pkt.PathHopCount,
			})
		}
	}

	// Relay / neighbour / traffic-share over non-advert transmissions with paths.
	relayCount := map[string]int{}
	relay1h := map[string]int{}
	relay24h := map[string]int{}
	relayLast := map[string]string{}
	edges := map[string]map[string]int{}
	addEdge := func(a, b string) {
		if a == b {
			return
		}
		if edges[a] == nil {
			edges[a] = map[string]int{}
		}
		if edges[b] == nil {
			edges[b] = map[string]int{}
		}
		edges[a][b]++
		edges[b][a]++
	}
	totalRelayTx := 0
	for _, tp := range txPaths {
		if tp.advert || len(tp.hops) == 0 {
			continue
		}
		totalRelayTx++
		t := parseTime(tp.receivedAt)
		for i, pk := range tp.hops {
			relayCount[pk]++
			if relayLast[pk] == "" || tp.receivedAt > relayLast[pk] {
				relayLast[pk] = tp.receivedAt
			}
			if !t.IsZero() {
				if now.Sub(t) <= time.Hour {
					relay1h[pk]++
				}
				if now.Sub(t) <= 24*time.Hour {
					relay24h[pk]++
				}
			}
			if i+1 < len(tp.hops) {
				addEdge(pk, tp.hops[i+1])
			}
		}
	}

	bridge := betweenness(edges)

	// Assemble per-node details for any node with advert or relay activity.
	details := map[string]*NodeDetail{}
	ensure := func(pk string) *NodeDetail {
		d := details[pk]
		if d == nil {
			d = &NodeDetail{PublicKey: pk, WindowHours: windowHours, Observers: []ObserverStat{}, RecentPackets: []PacketRef{}, Neighbors: []NeighborStat{}, Activity: []int{}}
			details[pk] = d
		}
		return d
	}

	for pk, a := range accs {
		d := ensure(pk)
		d.TotalObservations = a.obsCount
		cnt, todayCnt, recent, txTimes := summarizeAdverts(a.advs, today)
		d.TotalPackets = cnt
		d.PacketsToday = todayCnt
		d.RecentPackets = recent
		d.AdvertIntervalSec = medianInterval(txTimes)
		d.Activity = activityBuckets(txTimes, now, windowHours)
		d.FirstHeard = a.first
		d.LastHeard = a.last
		if a.snrN > 0 {
			v := a.snrSum / float64(a.snrN)
			d.AvgSNR = &v
		}
		if a.hopsN > 0 {
			v := float64(a.hopsSum) / float64(a.hopsN)
			d.AvgHops = &v
		}
		for id, oa := range a.observers {
			os := ObserverStat{ID: id, Region: oa.region, Count: oa.count}
			if oa.snrN > 0 {
				v := oa.snrSum / float64(oa.snrN)
				os.AvgSNR = &v
			}
			if oa.rssiN > 0 {
				v := oa.rssiSum / float64(oa.rssiN)
				os.AvgRSSI = &v
			}
			d.Observers = append(d.Observers, os)
		}
		sort.Slice(d.Observers, func(i, j int) bool { return d.Observers[i].Count > d.Observers[j].Count })
	}

	for pk, c := range relayCount {
		d := ensure(pk)
		d.Relay = RelayStat{
			LastRelayed: relayLast[pk],
			Count1h:     relay1h[pk],
			Count24h:    relay24h[pk],
			Active:      relay1h[pk] > 0,
		}
		if totalRelayTx > 0 {
			d.TrafficShare = float64(c) / float64(totalRelayTx)
		}
	}

	for pk, nbrs := range edges {
		d := ensure(pk)
		for nb, cnt := range nbrs {
			n := byKey[nb]
			name := n.Name
			if name == "" {
				name = nb[:min(12, len(nb))]
			}
			d.Neighbors = append(d.Neighbors, NeighborStat{PublicKey: nb, Name: name, Role: n.Role, Count: cnt})
		}
		sort.Slice(d.Neighbors, func(i, j int) bool { return d.Neighbors[i].Count > d.Neighbors[j].Count })
		if len(d.Neighbors) > 12 {
			d.Neighbors = d.Neighbors[:12]
		}
	}

	for pk, b := range bridge {
		ensure(pk).Bridge = b
	}

	return details
}

// advertTxGap is how far apart two adverts must land to count as separate
// transmissions; closer ones are re-flood / multi-observer copies of one
// broadcast. Keep in sync with store.advertTxGap (the live ingest counter).
const advertTxGap = 90 * time.Second

// summarizeAdverts groups a node's advert observations into transmissions.
// Because re-floods share a messageHash, observations of one hash are split
// into separate transmissions wherever they're more than advertTxGap apart. Returns the
// transmission count, how many were today, up to 20 newest as PacketRefs
// (best-SNR observation per transmission), and the transmission times (newest
// first) for cadence/activity analysis.
func summarizeAdverts(advs []advObs, today string) (count, todayCount int, recent []PacketRef, txTimes []time.Time) {
	recent = []PacketRef{}
	if len(advs) == 0 {
		return 0, 0, recent, nil
	}
	byHash := map[string][]advObs{}
	for _, o := range advs {
		byHash[o.hash] = append(byHash[o.hash], o)
	}
	type tx struct {
		rep advObs
		t   time.Time
	}
	var txs []tx
	for _, group := range byHash {
		sort.Slice(group, func(i, j int) bool { return group[i].t.Before(group[j].t) })
		var cur []advObs
		flush := func() {
			if len(cur) == 0 {
				return
			}
			rep := cur[0]
			for _, o := range cur {
				if o.snr != nil && (rep.snr == nil || *o.snr > *rep.snr) {
					rep = o
				}
			}
			txs = append(txs, tx{rep: rep, t: cur[0].t})
			cur = nil
		}
		var prev time.Time
		for _, o := range group {
			if len(cur) > 0 && !o.t.IsZero() && !prev.IsZero() && o.t.Sub(prev) > advertTxGap {
				flush()
			}
			cur = append(cur, o)
			prev = o.t
		}
		flush()
	}
	sort.Slice(txs, func(i, j int) bool { return txs[i].t.After(txs[j].t) })
	count = len(txs)
	for _, x := range txs {
		if len(x.rep.receivedAt) >= 10 && x.rep.receivedAt[:10] == today {
			todayCount++
		}
		if !x.t.IsZero() {
			txTimes = append(txTimes, x.t)
		}
		if len(recent) < 20 {
			recent = append(recent, PacketRef{
				MessageHash: x.rep.hash,
				PayloadType: "Advert",
				ReceivedAt:  x.rep.receivedAt,
				ObserverID:  x.rep.observerID,
				SNR:         x.rep.snr,
				RSSI:        x.rep.rssi,
				PathHops:    x.rep.hops,
			})
		}
	}
	return count, todayCount, recent, txTimes
}

// medianInterval returns the median gap, in seconds, between consecutive
// transmission times (a node's advert cadence). Nil with fewer than two times.
func medianInterval(times []time.Time) *float64 {
	if len(times) < 2 {
		return nil
	}
	ts := append([]time.Time(nil), times...)
	sort.Slice(ts, func(i, j int) bool { return ts[i].Before(ts[j]) })
	diffs := make([]float64, 0, len(ts)-1)
	for i := 1; i < len(ts); i++ {
		diffs = append(diffs, ts[i].Sub(ts[i-1]).Seconds())
	}
	med := medianFloat(diffs)
	return &med
}

// activityBuckets bins transmission times into windowHours one-hour buckets,
// oldest first and most-recent last, so the UI can draw a recency sparkline.
func activityBuckets(times []time.Time, now time.Time, windowHours int) []int {
	buckets := make([]int, windowHours)
	for _, t := range times {
		if t.IsZero() {
			continue
		}
		fromEnd := int(now.Sub(t).Hours()) // 0 = current hour
		idx := windowHours - 1 - fromEnd
		if idx >= 0 && idx < windowHours {
			buckets[idx]++
		}
	}
	return buckets
}

func parseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// betweenness runs Brandes' algorithm on the undirected neighbour graph and
// normalizes scores to [0,1] by the maximum (1.0 = most central node).
func betweenness(adj map[string]map[string]int) map[string]float64 {
	nodes := make([]string, 0, len(adj))
	for n := range adj {
		nodes = append(nodes, n)
	}
	bc := make(map[string]float64, len(nodes))
	for _, n := range nodes {
		bc[n] = 0
	}
	for _, s := range nodes {
		stack := []string{}
		pred := map[string][]string{}
		sigma := map[string]float64{}
		dist := map[string]int{}
		for _, v := range nodes {
			sigma[v] = 0
			dist[v] = -1
		}
		sigma[s] = 1
		dist[s] = 0
		queue := []string{s}
		for len(queue) > 0 {
			v := queue[0]
			queue = queue[1:]
			stack = append(stack, v)
			for w := range adj[v] {
				if dist[w] < 0 {
					dist[w] = dist[v] + 1
					queue = append(queue, w)
				}
				if dist[w] == dist[v]+1 {
					sigma[w] += sigma[v]
					pred[w] = append(pred[w], v)
				}
			}
		}
		delta := map[string]float64{}
		for i := len(stack) - 1; i >= 0; i-- {
			w := stack[i]
			for _, v := range pred[w] {
				delta[v] += (sigma[v] / sigma[w]) * (1 + delta[w])
			}
			if w != s {
				bc[w] += delta[w]
			}
		}
	}
	maxv := 0.0
	for _, v := range bc {
		if v > maxv {
			maxv = v
		}
	}
	if maxv > 0 {
		for k := range bc {
			bc[k] /= maxv
		}
	}
	return bc
}
