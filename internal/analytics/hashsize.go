package analytics

import (
	"sort"
	"time"

	"github.com/jjkroell/ridgeline/internal/meshcore"
	"github.com/jjkroell/ridgeline/internal/store"
)

// ConsensusHashSizes scans the advert window and returns each node's hash size
// by majority vote of its FLOOD adverts' path-length bytes, keyed by pubkey.
// Only nodes with a clear winner are included.
//
// Only flood adverts carry the configured size: a node sends its periodic flood
// advert (default every ~47h) with path_hash_mode+1 in the path-length byte, but
// its far more frequent zero-hop adverts go out with path_len=0 (always size 1).
// Counting zero-hop adverts would bury the few genuine flood transmissions under
// many bogus size-1 votes, so they're excluded (here and in the store query).
//
// A node's true hash size is fixed, but a corrupt advert (a flipped path-length
// byte) reports a wrong size, and ingest takes only the latest advert — so one
// corrupt packet can flip the stored value. The vote counts ONE vote per
// transmission, not per received copy: re-flood and multi-observer copies of a
// single broadcast all carry the originator's size, so counting them raw would
// let one heavily-heard (possibly corrupt-at-origin) transmission dominate.
// Grouping copies into transmissions and voting across them judges a node by
// independent broadcasts over time, so the window must span several advert
// cadences for a confident result (see hashSizeConsensusWindow).
func ConsensusHashSizes(st *store.Store, cutoffISO string) (map[string]int, error) {
	advs, err := st.AdvertObservationsSince(cutoffISO)
	if err != nil {
		return nil, err
	}

	type sizedCopy struct {
		t  time.Time
		hs int
	}
	perNode := map[string][]sizedCopy{}
	for _, ro := range advs {
		pkt, err := meshcore.DecodeHex(ro.RawHex)
		if err != nil || pkt == nil || pkt.Advert == nil || pkt.Advert.PublicKey == "" {
			continue
		}
		// Only flood adverts carry the configured hash size. A zero-hop (direct)
		// advert is sent with path_len=0, so it always decodes as size 1 — and
		// because each lands minutes apart it would otherwise form its own vote,
		// flooding the tally with bogus size-1 votes that drown out the (bursty,
		// hence few) genuine flood transmissions. Skip them entirely.
		if !pkt.RouteType.IsFlood() {
			continue
		}
		hs := pkt.PathHashSize
		// Valid sizes are 1..3 (path-length byte top-2-bits + 1); anything else is
		// a corrupt byte and not a usable vote.
		if hs < 1 || hs > 3 {
			continue
		}
		perNode[pkt.Advert.PublicKey] = append(perNode[pkt.Advert.PublicKey],
			sizedCopy{t: parseTime(ro.ReceivedAt), hs: hs})
	}

	out := make(map[string]int, len(perNode))
	for pk, copies := range perNode {
		// Collapse copies into per-transmission votes (modal size of each
		// transmission's copies), then take the confident majority across them.
		sort.Slice(copies, func(i, j int) bool { return copies[i].t.Before(copies[j].t) })
		votes := map[int]int{}
		group := map[int]int{}
		var prev time.Time
		flush := func() {
			if len(group) == 0 {
				return
			}
			votes[mode(group)]++
			group = map[int]int{}
		}
		for _, c := range copies {
			if !prev.IsZero() && c.t.Sub(prev) > advertTxGap {
				flush()
			}
			group[c.hs]++
			prev = c.t
		}
		flush()

		if size, ok := confidentMode(votes); ok {
			out[pk] = size
		}
	}
	return out, nil
}

// mode returns the most-counted key, breaking ties toward the smaller size so a
// single corrupt copy can't tip a transmission's vote.
func mode(counts map[int]int) int {
	best, bestN := 0, -1
	for size, n := range counts {
		if n > bestN || (n == bestN && size < best) {
			best, bestN = size, n
		}
	}
	return best
}

// confidentMode returns the size with the most votes and whether it's a clear
// winner: at least two votes and a strict majority of the total. This keeps a
// lone corrupt transmission from being treated as the verdict while a node with
// too few or evenly-split transmissions is left untouched.
func confidentMode(votes map[int]int) (int, bool) {
	best, bestN, total := 0, 0, 0
	for size, n := range votes {
		total += n
		if n > bestN || (n == bestN && size < best) {
			best, bestN = size, n
		}
	}
	if bestN < 2 || bestN*2 <= total {
		return 0, false
	}
	return best, true
}
