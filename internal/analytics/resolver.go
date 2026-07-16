package analytics

import (
	"strings"

	"github.com/jjkroell/ridgeline/internal/store"
)

// newPrefixResolver returns a function mapping a path-hop hex prefix to the
// public key of the unique node it identifies, or "" when no node — or more
// than one — matches at that prefix length. Hops use the originating node's
// hash size, so prefixes vary in length (2/4/6 hex chars); we index every
// node at each length and only resolve unambiguous matches.
func newPrefixResolver(nodes []store.Node) func(hop string) string {
	index := map[int]map[string][]string{2: {}, 4: {}, 6: {}}
	for _, n := range nodes {
		pk := strings.ToUpper(n.PublicKey)
		for _, l := range []int{2, 4, 6} {
			if len(pk) >= l {
				index[l][pk[:l]] = append(index[l][pk[:l]], n.PublicKey)
			}
		}
	}
	return func(hop string) string {
		h := strings.ToUpper(hop)
		m, ok := index[len(h)]
		if !ok {
			return ""
		}
		if matches := m[h]; len(matches) == 1 {
			return matches[0]
		}
		return ""
	}
}

// displayName returns a node's name, falling back to its key when unnamed.
func displayName(n store.Node, key string) string {
	if n.Name != "" {
		return n.Name
	}
	return key
}
