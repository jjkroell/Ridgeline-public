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
)

// InjectionReport lists detected ingress points for foreign/injected traffic.
type InjectionReport struct {
	WindowHours float64             `json:"windowHours"`
	Bridges     []BridgeCandidate   `json:"bridges"`   // RF bridges
	Injectors   []InjectorCandidate `json:"injectors"` // rogue MQTT publishers
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

	directlyHeard := map[string]bool{}        // origin heard at zero hops
	reporters := map[string]map[string]bool{} // origin -> set of observer ids
	obsTotal := map[string]int{}              // origin -> # of its observed (pathed) adverts
	// via[relay][origin] = # of origin's observed paths that include relay. The
	// per-observation count (not a union) is what lets us measure captivity.
	via := map[string]map[string]int{}

	for _, ro := range raws {
		pkt, err := meshcore.DecodeHex(ro.RawHex)
		if err != nil || pkt == nil || pkt.Advert == nil || pkt.Advert.PublicKey == "" {
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
			directlyHeard[origin] = true
			continue
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
		}
	}

	report := &InjectionReport{
		WindowHours: windowHoursFrom(sinceISO),
		Bridges:     []BridgeCandidate{},
		Injectors:   []InjectorCandidate{},
	}
	meshLat, meshLon, haveMesh := centroid(nodes)

	// Bridge candidates by captivity.
	for relay, origins := range via {
		var foreign []ForeignNode
		captive := 0
		for origin, cnt := range origins {
			if directlyHeard[origin] {
				continue // a local node — not foreign
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
		bc := BridgeCandidate{
			NodeKey:         relay,
			Name:            displayName(byKey[relay], relay),
			CaptiveCount:    captive,
			ForeignThrough:  len(foreign),
			CaptiveFraction: capFrac,
			Foreign:         foreign,
		}
		if haveMesh {
			if fLat, fLon, ok := captiveCentroid(foreign); ok {
				bc.ForeignKm = haversineKm(meshLat, meshLon, fLat, fLon)
			}
		}
		report.Bridges = append(report.Bridges, bc)
	}
	// Rank by captive count, then captive fraction. Geography is NOT a factor.
	sort.Slice(report.Bridges, func(i, j int) bool {
		if report.Bridges[i].CaptiveCount != report.Bridges[j].CaptiveCount {
			return report.Bridges[i].CaptiveCount > report.Bridges[j].CaptiveCount
		}
		return report.Bridges[i].CaptiveFraction > report.Bridges[j].CaptiveFraction
	})

	// MQTT injector candidates: observers that are the sole source of foreign nodes.
	relayOnly := func(origin string) bool { return !directlyHeard[origin] }
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
