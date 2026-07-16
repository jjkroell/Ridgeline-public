package analytics

import (
	"strings"

	"github.com/jjkroell/ridgeline/internal/meshcore"
	"github.com/jjkroell/ridgeline/internal/store"
)

// NodeActivity is a day-of-week × hour-of-day grid of a node's activity over a
// rolling window — the "when is this node alive" heatmap. Counts the node's own
// adverts plus packets it relayed (uniquely-resolved hop), bucketed in UTC.
type NodeActivity struct {
	Grid  [7][24]int `json:"grid"` // [weekday 0=Sun][hour 0-23]
	Max   int        `json:"max"`  // busiest cell (for colour scaling)
	Total int        `json:"total"`
	Days  int        `json:"days"`
}

// NodeHeatmap scans stored observations from sinceISO and bins those attributable
// to pubkey (own adverts + relayed packets) into a weekday×hour grid in UTC.
// Reuses the same unambiguous prefix resolution as the neighbour graph.
func NodeHeatmap(st *store.Store, nodes []store.Node, pubkey, sinceISO string, scanCap, days int) (*NodeActivity, error) {
	if scanCap <= 0 || scanCap > 400000 {
		scanCap = 200000
	}
	pubkey = strings.ToUpper(pubkey)

	raws, err := st.RawWindow(sinceISO, scanCap)
	if err != nil {
		return nil, err
	}
	resolve := newPrefixResolver(nodes)

	out := &NodeActivity{Days: days}
	for _, ro := range raws {
		pkt, err := meshcore.DecodeHex(ro.RawHex)
		if err != nil || pkt == nil {
			continue
		}
		mine := pkt.Advert != nil && strings.ToUpper(pkt.Advert.PublicKey) == pubkey
		if !mine {
			for _, hop := range pkt.Path {
				if strings.ToUpper(resolve(hop)) == pubkey {
					mine = true
					break
				}
			}
		}
		if !mine {
			continue
		}
		t := parseTime(ro.ReceivedAt)
		if t.IsZero() {
			continue
		}
		t = t.UTC()
		out.Grid[int(t.Weekday())][t.Hour()]++
		out.Total++
	}
	for d := 0; d < 7; d++ {
		for h := 0; h < 24; h++ {
			if out.Grid[d][h] > out.Max {
				out.Max = out.Grid[d][h]
			}
		}
	}
	return out, nil
}
