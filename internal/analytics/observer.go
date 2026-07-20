package analytics

import (
	"sort"
	"strings"
	"time"

	"github.com/jjkroell/ridgeline/internal/meshcore"
	"github.com/jjkroell/ridgeline/internal/store"
)

// ObserverAnalytics is everything specific to one observer's feed over a rolling
// window — its throughput, what it hears, and how healthy its clock is. Distinct
// from node analytics: this is about the listening post, not any node.
type ObserverAnalytics struct {
	ID             string         `json:"id"`
	Region         string         `json:"region,omitempty"`
	WindowHours    float64        `json:"windowHours"`
	TotalPackets   int            `json:"totalPackets"`   // receptions in window
	PacketsPerHour float64        `json:"packetsPerHour"` // mean rate over the window
	Activity       []int          `json:"activity"`       // per-hour receptions, oldest bucket first
	PayloadTypes   []NameCount    `json:"payloadTypes"`   // its feed's payload mix
	SNRHist        []HistogramBin `json:"snrHist"`        // SNR distribution of its receptions
	AvgSNR         *float64       `json:"avgSnr,omitempty"`
	DistinctNodes  int            `json:"distinctNodes"` // distinct advert origins it heard
	DirectNodes    int            `json:"directNodes"`   // distinct zero-hop (RF neighbour) nodes
	ClockSkewMs    *float64       `json:"clockSkewMs,omitempty"`
	Neighbors      []DirectLink   `json:"neighbors"` // zero-hop nodes heard, by count
}

// ObserverSummary scans a window of observations once and computes the per-observer
// feed metrics for id. Clock skew still needs cross-observer comparison (consensus
// time on shared packets), so the scan processes every row but only emits id's
// aggregates.
func ObserverSummary(st *store.Store, nodes []store.Node, id, sinceISO string, scanCap int) (*ObserverAnalytics, error) {
	if scanCap <= 0 || scanCap > 300000 {
		scanCap = 120000
	}
	raws, err := st.RawWindow(sinceISO, scanCap)
	if err != nil {
		return nil, err
	}
	byKey := make(map[string]store.Node, len(nodes))
	for _, n := range nodes {
		byKey[n.PublicKey] = n
	}

	since := parseTime(sinceISO)
	windowSec := time.Since(since).Seconds()
	if windowSec <= 0 {
		windowSec = 3600
	}
	buckets := int(windowSec/3600) + 1
	now := time.Now()

	// Neighbours starts as an empty (non-nil) slice so it serialises as [] not null
	// when the observer heard nothing zero-hop — the UI indexes .length on it.
	out := &ObserverAnalytics{ID: id, WindowHours: windowSec / 3600.0, Activity: make([]int, buckets), Neighbors: []DirectLink{}}
	payload := map[string]int{}
	advHeard := map[string]bool{}
	directCount := map[string]int{} // node → zero-hop hear count
	snr := newSNRHist()
	var snrSum float64
	var snrN int
	txObs := map[string][]obsTime{} // messageHash → (observer, time) across ALL observers, for skew

	for _, ro := range raws {
		pkt, err := meshcore.DecodeHex(ro.RawHex)
		if err != nil || pkt == nil {
			continue
		}
		recv := parseTime(ro.ReceivedAt)
		if ro.ObserverID != "" && !recv.IsZero() {
			txObs[pkt.MessageHash] = append(txObs[pkt.MessageHash], obsTime{obs: ro.ObserverID, t: recv})
		}
		if ro.ObserverID != id {
			continue
		}

		out.TotalPackets++
		if !recv.IsZero() {
			// bucket 0 = oldest hour, last = current hour
			b := buckets - 1 - int(now.Sub(recv).Hours())
			if b >= 0 && b < buckets {
				out.Activity[b]++
			}
		}
		payload[pkt.PayloadType.String()]++
		if ro.SNR != nil {
			snrSum += *ro.SNR
			snrN++
			snr.add(*ro.SNR)
		}
		if ro.Region != "" {
			out.Region = ro.Region
		}
		if pkt.Advert != nil && pkt.Advert.PublicKey != "" {
			origin := strings.ToUpper(pkt.Advert.PublicKey)
			advHeard[origin] = true
			if len(pkt.RelayPath()) == 0 {
				directCount[origin]++
			}
		}
	}

	out.PacketsPerHour = float64(out.TotalPackets) / (windowSec / 3600.0)
	out.DistinctNodes = len(advHeard)
	out.DirectNodes = len(directCount)
	if snrN > 0 {
		v := snrSum / float64(snrN)
		out.AvgSNR = &v
	}
	out.PayloadTypes = sortedCounts(payload)
	out.SNRHist = snr.result()

	// Zero-hop RF neighbours, by hear count.
	for nk, c := range directCount {
		n := byKey[nk]
		out.Neighbors = append(out.Neighbors, DirectLink{Observer: id, NodeKey: nk, NodeName: displayName(n, nk), Role: n.Role, Count: c})
	}
	sort.Slice(out.Neighbors, func(i, j int) bool { return out.Neighbors[i].Count > out.Neighbors[j].Count })

	// Clock skew vs consensus on shared packets (same method as the mesh-wide
	// analytics; we only surface this observer's value).
	if v, ok := clockSkew(txObs, 5)[id]; ok {
		out.ClockSkewMs = &v
	}

	return out, nil
}
