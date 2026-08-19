package analytics

// Which nodes live BEYOND a sanctioned bridge?
//
// With every observer on one side of a bridge, a node on the far side is only
// ever heard after its traffic crosses. Three properties make that a usable
// signal rather than a guess:
//
//  1. DIRECTION. Relays append their hash as a packet travels, so the path is
//     ordered and index(near) < index(far) IS the near->far direction. Traffic
//     crossing the other way is a different fact and is not counted.
//
//  2. WIDTH. The path hash width is the ORIGINATING node's setting, and every
//     relay appends at that width — so a narrow originator produces narrow hops
//     for the bridge too. A 2-byte hop identifies a bridge end uniquely on a
//     mesh this size; a 1-byte hop usually does not. Rather than discard narrow
//     traffic (which would make a whole class of node invisible — companions
//     skew narrow), a 1-byte crossing is admitted as PROBABLE, and only when the
//     FAR end's single byte is unique among known nodes. The near end may be
//     ambiguous; the far end carries the claim.
//
//  3. NEVER HEARD DIRECTLY. An observer on this side cannot hear the far side
//     directly, so a single zero-hop sighting disqualifies a node outright. This
//     is what separates "lives over there" from "a packet happened to route
//     through the bridge once", and it is the strongest of the three.
//
// The window matters as much as the rule: it must not reach back before the
// bridge existed. Beforehand its two ends were ordinary RF relays and transiting
// them said nothing about segments, so an over-long window blends two different
// topologies and misclassifies nodes that have since moved.

import (
	"strings"

	"github.com/jjkroell/ridgeline/internal/meshcore"
	"github.com/jjkroell/ridgeline/internal/store"
)

// segMinShare is the fraction of a node's path-carrying sightings that must
// have crossed the bridge. Not 100%: a re-flooded copy can arrive by an odd
// route. Combined with the zero-hop veto this is a strict test — every node
// found on the live mesh scored 100%.
const segMinShare = 0.90

// segMinSightings guards against a node with two sightings scoring 100% on
// nothing. A far-side node advertises regularly, so this costs real cases little.
const segMinSightings = 5

// SegmentReport is the outcome of one sweep, for logging and the admin console.
type SegmentReport struct {
	Members   []store.SegmentMember `json:"members"`
	Scanned   int                   `json:"scanned"`
	Crossings int                   `json:"crossings"`
	Reverse   int                   `json:"reverse"`
	// Rejected lists nodes that crossed but failed a test, with the reason —
	// the interesting half of the output when a node is missing from the map.
	Rejected map[string]string `json:"rejected,omitempty"`
}

type segStat struct {
	zeroHop   int
	withPath  int
	confirmed int
	probable  int
}

// DetectSegments finds the nodes reachable only across each sanctioned bridge.
// sinceISO must not predate the bridge being put in place.
func DetectSegments(st *store.Store, nodes []store.Node, links []store.BridgeLink, sinceISO string, scanCap int) (*SegmentReport, error) {
	rep := &SegmentReport{Members: []store.SegmentMember{}, Rejected: map[string]string{}}
	if len(links) == 0 {
		return rep, nil
	}
	if scanCap <= 0 || scanCap > 500000 {
		scanCap = 250000
	}
	raws, err := st.RawWindow(sinceISO, scanCap)
	if err != nil {
		return nil, err
	}

	// A 1-byte far end is only usable when no other known node shares that byte.
	farByteUnique := map[string]bool{}
	for _, l := range links {
		b := l.Far[:2]
		n := 0
		for _, nd := range nodes {
			if strings.HasPrefix(strings.ToUpper(nd.PublicKey), b) {
				n++
			}
		}
		farByteUnique[l.Near] = n <= 1
	}

	// stats[bridgeNear][originKey]
	stats := map[string]map[string]*segStat{}
	for _, l := range links {
		stats[l.Near] = map[string]*segStat{}
	}

	for _, r := range raws {
		rep.Scanned++
		pkt, err := meshcore.DecodeHex(r.RawHex)
		if err != nil || pkt == nil || pkt.Advert == nil || pkt.Advert.PublicKey == "" {
			continue
		}
		origin := strings.ToUpper(pkt.Advert.PublicKey)
		path := pkt.RelayPath()

		for _, l := range links {
			// The bridge's own ends are not beyond it.
			if origin == l.Near || origin == l.Far {
				continue
			}
			s := stats[l.Near][origin]
			if s == nil {
				s = &segStat{}
				stats[l.Near][origin] = s
			}
			if len(path) == 0 {
				s.zeroHop++
				continue
			}
			s.withPath++

			iNearW, iFarW := -1, -1 // >=2-byte positions
			iNear1, iFar1 := -1, -1 // 1-byte positions
			for i, hop := range path {
				h := strings.ToUpper(hop)
				if h == "" {
					continue
				}
				wide := len(h)/2 >= 2
				if strings.HasPrefix(l.Near, h) {
					if wide && iNearW < 0 {
						iNearW = i
					} else if !wide && iNear1 < 0 {
						iNear1 = i
					}
				}
				if strings.HasPrefix(l.Far, h) {
					if wide && iFarW < 0 {
						iFarW = i
					} else if !wide && iFar1 < 0 {
						iFar1 = i
					}
				}
			}
			switch {
			case iNearW >= 0 && iFarW >= 0 && iNearW < iFarW:
				s.confirmed++
				rep.Crossings++
			case iNearW >= 0 && iFarW >= 0 && iFarW < iNearW:
				rep.Reverse++
			case farByteUnique[l.Near] && iNear1 >= 0 && iFar1 >= 0 && iNear1 < iFar1:
				s.probable++
				rep.Crossings++
			}
		}
	}

	names := map[string]string{}
	for _, n := range nodes {
		names[strings.ToUpper(n.PublicKey)] = n.Name
	}
	for _, l := range links {
		for origin, s := range stats[l.Near] {
			via := s.confirmed + s.probable
			if via == 0 {
				continue
			}
			label := names[origin]
			if label == "" {
				label = origin[:min(12, len(origin))]
			}
			// Heard directly by an observer on this side: it is not over there.
			if s.zeroHop > 0 {
				rep.Rejected[label] = "heard directly on this side"
				continue
			}
			if s.withPath < segMinSightings {
				rep.Rejected[label] = "too few sightings to judge"
				continue
			}
			if float64(via)/float64(s.withPath) < segMinShare {
				rep.Rejected[label] = "only some traffic crosses the bridge"
				continue
			}
			conf := "confirmed"
			if s.confirmed == 0 {
				conf = "probable"
			}
			rep.Members = append(rep.Members, store.SegmentMember{
				NodeKey: origin, BridgeNear: l.Near, Confidence: conf,
			})
		}
	}
	return rep, nil
}
