package analytics

import (
	"time"

	"github.com/jjkroell/ridgeline/internal/meshcore"
	"github.com/jjkroell/ridgeline/internal/store"
)

// BackfillAdvertTx recomputes advert_tx_count for every node from the full stored
// advert history, using the same advertTxGap transmission grouping as the live
// ingest counter (re-flood / multi-observer copies of one advert within the gap
// are one transmission). Intended to run once, right after the column is added.
// Returns the number of nodes seeded and the total transmissions counted.
func BackfillAdvertTx(st *store.Store) (nodeCount, txTotal int, err error) {
	raws, err := st.AdvertObservationsChrono() // oldest first
	if err != nil {
		return 0, 0, err
	}
	counts := map[string]int{}
	last := map[string]time.Time{}
	for _, o := range raws {
		p, derr := meshcore.DecodeHex(o.RawHex)
		if derr != nil || p.Advert == nil || p.Advert.PublicKey == "" {
			continue
		}
		t, terr := time.Parse(time.RFC3339Nano, o.ReceivedAt)
		if terr != nil {
			continue
		}
		key := p.Advert.PublicKey
		if prev, seen := last[key]; !seen || t.Sub(prev) > advertTxGap {
			counts[key]++
		}
		last[key] = t
	}
	for _, c := range counts {
		txTotal += c
	}
	if err := st.SetAdvertTxCounts(counts); err != nil {
		return 0, 0, err
	}
	return len(counts), txTotal, nil
}
