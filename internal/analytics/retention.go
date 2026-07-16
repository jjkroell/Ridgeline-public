package analytics

import (
	"strings"

	"github.com/jjkroell/ridgeline/internal/store"
)

// StaleNodeKeys returns the public keys of nodes that have gone silent past the
// retention cutoff and should be pruned. A node is stale when its last advert is
// older than cutoffISO (an RFC3339Nano UTC timestamp) AND it isn't in keep — the
// set of nodes seen relaying within the recent liveness window.
//
// last_seen on a node row tracks only its own adverts, and a healthy MeshCore
// node re-adverts every few hours, so an advert silence measured in weeks is a
// reliable "gone" signal. keep guards the rare node whose advert is stale but
// which is still forwarding traffic (so still part of the mesh) from being cut.
func StaleNodeKeys(nodes []store.Node, keep map[string]LiveSignal, cutoffISO string) []string {
	var stale []string
	for _, n := range nodes {
		if n.LastSeen == "" || n.LastSeen >= cutoffISO {
			continue // never seen (shouldn't happen) or adverted recently enough
		}
		if _, alive := keep[n.PublicKey]; alive {
			continue // stale advert, but still relaying — keep it
		}
		stale = append(stale, n.PublicKey)
	}
	return stale
}

// FilterRelayedWithin removes from stale any node that relayed traffic within the
// retention window, extending the liveness guard in StaleNodeKeys (which only
// reaches the short analytics window) to the whole window. relayHops is the set
// of relay-hop identifiers seen in packet paths across the window (see
// store.RelayHopPrefixesSince); allNodes is every known node, needed to resolve
// those hops to nodes.
//
// Resolution is UNIQUE-match, mirroring the analytics relay resolver: a hop
// credits a node only when exactly one known node's public key carries that hash
// prefix. Generous prefix matching would be useless here — 1-byte hops share a
// 256-value space that saturates under real traffic, so almost every node would
// match some hop and nothing could ever be pruned. Requiring a unique owner means
// an ambiguous short hop credits no one (correct: it isn't evidence THIS node
// relayed), while a node with a distinctive multi-byte presence is reliably kept.
func FilterRelayedWithin(stale []string, allNodes []store.Node, relayHops map[string]bool) []string {
	if len(relayHops) == 0 || len(stale) == 0 {
		return stale
	}
	pubkeys := make([]string, 0, len(allNodes))
	for _, n := range allNodes {
		pubkeys = append(pubkeys, strings.ToUpper(n.PublicKey))
	}
	relayed := make(map[string]bool)
	for hop := range relayHops {
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
