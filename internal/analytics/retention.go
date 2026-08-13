package analytics

import (
	"strings"

	"github.com/jjkroell/ridgeline/internal/store"
)

// StaleNodeKeys returns the public keys of nodes whose own adverts have gone
// silent past the retention cutoff (an RFC3339Nano UTC timestamp). These are
// only CANDIDATES: FilterRelayedWithin then decides which of them are still
// demonstrably relaying and must be kept.
//
// last_seen on a node row tracks only its own adverts, and a healthy MeshCore
// node re-adverts every few hours, so an advert silence measured in weeks is a
// reliable "gone" signal.
//
// This deliberately does NOT consult the analytics liveness snapshot any more.
// That snapshot's relay counts come from the prefix resolver, which credits a
// hop to whichever node uniquely owns that prefix AT ANY WIDTH — including one
// byte, where the space is ~97% saturated by real traffic. A node with a unique
// 1-byte prefix therefore looked permanently "live" off other nodes' packets and
// was skipped here before FilterRelayedWithin's width gate could ever see it.
// Confirmed in the field: a repeater its owner had taken off the mesh 34 days
// earlier survived two sweeps that way. Relay evidence is now judged in exactly
// one place, with the width gate, over the full retention window (which is wider
// than the liveness window, so nothing legitimate is lost). The clock signals
// carried alongside it are derived from adverts, so they cannot testify that an
// advert-stale node is alive either.
func StaleNodeKeys(nodes []store.Node, cutoffISO string) []string {
	var stale []string
	for _, n := range nodes {
		if n.LastSeen == "" || n.LastSeen >= cutoffISO {
			continue // never seen (shouldn't happen) or adverted recently enough
		}
		stale = append(stale, n.PublicKey)
	}
	return stale
}

// FilterRelayedWithin removes from stale any node that relayed traffic within the
// retention window. This is the ONLY place relay evidence is judged for
// retention — see StaleNodeKeys for why the analytics liveness snapshot is not
// consulted. relayHops is the set
// of relay-hop identifiers seen in packet paths across the window (see
// store.RelayHopPrefixesSince); allNodes is every known node, needed to resolve
// those hops to nodes.
//
// Two gates apply. First WIDTH: a hop narrower than minHopBytes is discarded
// outright — see NodeRetentionMinHopBytes. Then UNIQUE-match, mirroring the
// analytics relay resolver: a hop credits a node only when exactly one known
// node's public key carries that hash prefix. Generous prefix matching would be useless here — 1-byte hops share a
// 256-value space that saturates under real traffic, so almost every node would
// match some hop and nothing could ever be pruned. Requiring a unique owner means
// an ambiguous short hop credits no one (correct: it isn't evidence THIS node
// relayed), while a node with a distinctive multi-byte presence is reliably kept.
func FilterRelayedWithin(stale []string, allNodes []store.Node, relayHops map[string]bool, minHopBytes int) []string {
	if minHopBytes < 1 {
		minHopBytes = 1
	}
	if len(relayHops) == 0 || len(stale) == 0 {
		return stale
	}
	pubkeys := make([]string, 0, len(allNodes))
	for _, n := range allNodes {
		pubkeys = append(pubkeys, strings.ToUpper(n.PublicKey))
	}
	relayed := make(map[string]bool)
	for hop := range relayHops {
		// Width gate. A hop narrower than minHopBytes is not attribution: the
		// 1-byte space saturates under real traffic (measured at ~97% of all 256
		// values inside a week), so such a hop matches SOME node almost by
		// definition and says nothing about which one relayed.
		if len(hop)/2 < minHopBytes {
			continue
		}
		owner, count := "", 0
		for _, pk := range pubkeys {
			if strings.HasPrefix(pk, hop) {
				owner, count = pk, count+1
				if count > 1 {
					break // ambiguous — this hop credits no single node
				}
			}
		}
		if count == 1 {
			relayed[owner] = true
		}
	}
	kept := make([]string, 0, len(stale))
	for _, pk := range stale {
		if relayed[strings.ToUpper(pk)] {
			continue // relayed within the window → not stale, keep the node
		}
		kept = append(kept, pk)
	}
	return kept
}
