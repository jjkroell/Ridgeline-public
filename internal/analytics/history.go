package analytics

import (
	"sort"
	"strings"

	"github.com/jjkroell/ridgeline/internal/meshcore"
	"github.com/jjkroell/ridgeline/internal/store"
)

// HistoryEntry is one stored observation attributable to a specific node: either
// the node's own advert, or a packet where the node is a uniquely-resolved relay
// hop in the path. Unlike the analytics snapshot (a fixed rolling window), this
// is an on-demand query that can span any time range the store retains.
type HistoryEntry struct {
	MessageHash string   `json:"messageHash"`
	PayloadType string   `json:"payloadType"`
	RouteType   string   `json:"routeType"`
	Kind        string   `json:"kind"` // "advert" (sent by the node) | "relay" (forwarded by it)
	ReceivedAt  string   `json:"receivedAt"`
	ObserverID  string   `json:"observerId,omitempty"`
	Region      string   `json:"region,omitempty"`
	SNR         *float64 `json:"snr,omitempty"`
	RSSI        *float64 `json:"rssi,omitempty"`
	PathHops    int      `json:"pathHops"`
	HopIndex    int      `json:"hopIndex"` // relay only: 0-based position of the node in the path
}

// NodeHistory scans stored observations received at or after sinceISO (newest
// first, decoding up to scanCap rows) and returns those attributable to pubkey —
// the node's own adverts and packets where it is a uniquely-resolved relay hop —
// newest first, capped at limit. Relay attribution uses the same unambiguous
// prefix resolution as the analytics neighbour graph, so a hop that several
// nodes could match is not counted for any of them.
func NodeHistory(st *store.Store, nodes []store.Node, pubkey, sinceISO string, scanCap, limit int) ([]HistoryEntry, error) {
	if scanCap <= 0 || scanCap > 200000 {
		scanCap = 80000
	}
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	pubkey = strings.ToUpper(pubkey)

	raws, err := st.RawWindow(sinceISO, scanCap)
	if err != nil {
		return nil, err
	}
	resolve := newPrefixResolver(nodes)

	out := []HistoryEntry{}
	for _, ro := range raws {
		if len(out) >= limit {
			break
		}
		pkt, err := meshcore.DecodeHex(ro.RawHex)
		if err != nil || pkt == nil {
			continue
		}
		if pkt.Advert != nil && strings.ToUpper(pkt.Advert.PublicKey) == pubkey {
			out = append(out, historyEntry(pkt, ro, "advert", 0))
			continue
		}
		for i, hop := range pkt.Path {
			if strings.ToUpper(resolve(hop)) == pubkey {
				out = append(out, historyEntry(pkt, ro, "relay", i))
				break
			}
		}
	}
	return out, nil
}

// NodeObservers aggregates, per observer, the reception of a node's own adverts
// over an arbitrary time range (received at or after sinceISO). Unlike the
// analytics snapshot's fixed advert window, this queries the store on demand so
// the UI can widen the range to span a node's advert cadence (often ~30h),
// which the short snapshot window rarely captures. Returned newest-heard-first
// isn't meaningful here; results are sorted by reception count, descending.
func NodeObservers(st *store.Store, nodes []store.Node, pubkey, sinceISO string, scanCap int) ([]ObserverStat, error) {
	if scanCap <= 0 || scanCap > 200000 {
		scanCap = 80000
	}
	pubkey = strings.ToUpper(pubkey)

	raws, err := st.RawWindow(sinceISO, scanCap)
	if err != nil {
		return nil, err
	}

	type acc struct {
		region          string
		count           int
		snrSum, rssiSum float64
		snrN, rssiN     int
	}
	byObs := map[string]*acc{}
	for _, ro := range raws {
		pkt, err := meshcore.DecodeHex(ro.RawHex)
		if err != nil || pkt == nil || pkt.Advert == nil {
			continue
		}
		if strings.ToUpper(pkt.Advert.PublicKey) != pubkey {
			continue
		}
		a := byObs[ro.ObserverID]
		if a == nil {
			a = &acc{region: ro.Region}
			byObs[ro.ObserverID] = a
		}
		a.count++
		if ro.SNR != nil {
			a.snrSum += *ro.SNR
			a.snrN++
		}
		if ro.RSSI != nil {
			a.rssiSum += *ro.RSSI
			a.rssiN++
		}
	}

	out := make([]ObserverStat, 0, len(byObs))
	for id, a := range byObs {
		os := ObserverStat{ID: id, Region: a.region, Count: a.count}
		if a.snrN > 0 {
			v := a.snrSum / float64(a.snrN)
			os.AvgSNR = &v
		}
		if a.rssiN > 0 {
			v := a.rssiSum / float64(a.rssiN)
			os.AvgRSSI = &v
		}
		out = append(out, os)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Count > out[j].Count })
	return out, nil
}

func historyEntry(pkt *meshcore.Packet, ro store.RawObservation, kind string, hopIndex int) HistoryEntry {
	return HistoryEntry{
		MessageHash: pkt.MessageHash,
		PayloadType: pkt.PayloadType.String(),
		RouteType:   pkt.RouteType.String(),
		Kind:        kind,
		ReceivedAt:  ro.ReceivedAt,
		ObserverID:  ro.ObserverID,
		Region:      ro.Region,
		SNR:         ro.SNR,
		RSSI:        ro.RSSI,
		PathHops:    pkt.PathHopCount,
		HopIndex:    hopIndex,
	}
}
