package analytics

import (
	"fmt"
	"sort"
	"strings"

	"github.com/jjkroell/ridgeline/internal/store"
)

// Corruption-artifact detection. Packet corruption can flip bytes in an advert —
// including the public key — producing a phantom node record that shares a real
// node's hash-ID prefix and shows up as a false "collision". This is the
// server-side twin of web/src/lib/hash-ids.ts, used by the periodic auto-scrub.
//
// Collisions are scoped to a node's *configured* hash-ID length (HashSize): a
// node only collides with other nodes set to the same length, compared at that
// length. Within a colliding set the most-adverted key is the true node; lesser
// records are checked against it for the corruption signals below.

// sharedDefinite: two distinct 32-byte keys sharing this many leading bytes is
// statistically impossible by chance (~5e-6 across a whole mesh), so it can only
// be the same node with a corrupted key.
const sharedDefinite = 4

// Artifact is a phantom record judged to be a packet-corruption copy of a real node.
type Artifact struct {
	Key           string `json:"key"`           // the corrupt record's public key
	Name          string `json:"name"`          // its (often mangled) name
	CanonicalKey  string `json:"canonicalKey"`  // the real node it duplicates
	CanonicalName string `json:"canonicalName"` // the real node's name
	Reason        string `json:"reason"`
	Confidence    string `json:"confidence"` // "high" | "medium"
	SharedBytes   int    `json:"sharedBytes"`
	HashSize      int    `json:"hashSize"`
}

func normName(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

// isGarbageName: empty, a single character, or no ASCII letter/digit at all —
// e.g. "=" or a string of stray symbols. Mirrors the TS /[a-z0-9]/i test so
// emoji- and accent-bearing real names ("Mt Sicker ⛰️") are NOT garbage.
func isGarbageName(s string) bool {
	t := strings.TrimSpace(s)
	if len([]rune(t)) <= 1 {
		return true
	}
	for _, r := range t {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

// sharedPrefixBytes counts the leading whole bytes (hex pairs) two keys share.
func sharedPrefixBytes(a, b string) int {
	a = strings.ToUpper(a)
	b = strings.ToUpper(b)
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	n /= 2
	for i := 0; i < n; i++ {
		if a[i*2:i*2+2] != b[i*2:i*2+2] {
			return i
		}
	}
	return n
}

// moreCanonical reports whether a is the more plausible "true" node than b:
// most adverts wins, then a located record, then most-recently seen, then key.
func moreCanonical(a, b store.Node) bool {
	if a.AdvertCount != b.AdvertCount {
		return a.AdvertCount > b.AdvertCount
	}
	if a.HasLocation != b.HasLocation {
		return a.HasLocation
	}
	if a.LastSeen != b.LastSeen {
		return a.LastSeen > b.LastSeen
	}
	return a.PublicKey < b.PublicKey
}

// bestParent picks, among busier candidate sources, the most plausible original
// of m — the one whose key it most resembles (exact name match breaks ties).
func bestParent(m store.Node, cands []store.Node) store.Node {
	best := cands[0]
	for _, c := range cands[1:] {
		cs := sharedPrefixBytes(m.PublicKey, c.PublicKey)
		bs := sharedPrefixBytes(m.PublicKey, best.PublicKey)
		if cs != bs {
			if cs > bs {
				best = c
			}
			continue
		}
		if boolToInt(normName(c.Name) == normName(m.Name)) > boolToInt(normName(best.Name) == normName(m.Name)) {
			best = c
		}
	}
	return best
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nameOr(s string) string {
	if strings.TrimSpace(s) == "" {
		return "a real node"
	}
	return s
}

// classifyMember decides whether m is a corruption artifact of canonical node a,
// at the given hash-ID length. Returns nil if m looks like a distinct real node.
func classifyMember(m, a store.Node, byteLen int) *Artifact {
	if m.PublicKey == a.PublicKey {
		return nil
	}
	shared := sharedPrefixBytes(m.PublicKey, a.PublicKey)
	mk := func(conf, reason string) *Artifact {
		return &Artifact{
			Key: m.PublicKey, Name: m.Name,
			CanonicalKey: a.PublicKey, CanonicalName: a.Name,
			Reason: reason, Confidence: conf, SharedBytes: shared, HashSize: byteLen,
		}
	}

	// 1. Structural near-identity — impossible by chance, so a corrupted copy.
	if shared >= sharedDefinite {
		return mk("high", fmt.Sprintf("key matches %s in %d of 32 bytes — corrupted copy", nameOr(a.Name), shared))
	}
	// 2. Same name → same node with a corrupted key.
	if normName(m.Name) != "" && normName(m.Name) == normName(a.Name) && !isGarbageName(m.Name) {
		return mk("high", "identical name to "+a.Name)
	}
	// 3. Unreadable name on a barely-heard record next to a dominant real node.
	if isGarbageName(m.Name) && a.AdvertCount >= 5 && a.AdvertCount >= m.AdvertCount*4 {
		return mk("high", fmt.Sprintf("unreadable name, heard %d× vs %s's %d×", m.AdvertCount, nameOr(a.Name), a.AdvertCount))
	}
	// 4. Residual: rarely heard, shares more than the hash length with a strongly
	//    dominant node. Medium — never auto-scrubbed.
	if m.AdvertCount <= 2 && shared > byteLen && a.AdvertCount >= 10 && a.AdvertCount >= m.AdvertCount*8 {
		return mk("medium", fmt.Sprintf("rarely heard (%d×), shares %d bytes with %s", m.AdvertCount, shared, nameOr(a.Name)))
	}
	return nil
}

// classifyGroup walks a colliding set best-first, classifying each member
// against the busier members above it.
func classifyGroup(members []store.Node, byteLen int) []Artifact {
	ordered := append([]store.Node(nil), members...)
	sort.SliceStable(ordered, func(i, j int) bool { return moreCanonical(ordered[i], ordered[j]) })
	var arts []Artifact
	for i := 1; i < len(ordered); i++ {
		if a := classifyMember(ordered[i], bestParent(ordered[i], ordered[:i]), byteLen); a != nil {
			arts = append(arts, *a)
		}
	}
	return arts
}

// FindArtifacts returns every record judged to be a packet-corruption copy of a
// real node, scoped per configured hash-ID length cohort.
func FindArtifacts(nodes []store.Node) []Artifact {
	var out []Artifact
	for _, byteLen := range []int{1, 2, 3} {
		buckets := map[string][]store.Node{}
		for _, n := range nodes {
			if n.HashSize != byteLen || len(n.PublicKey) < byteLen*2 {
				continue
			}
			p := strings.ToUpper(n.PublicKey[:byteLen*2])
			buckets[p] = append(buckets[p], n)
		}
		for _, ms := range buckets {
			if len(ms) >= 2 {
				out = append(out, classifyGroup(ms, byteLen)...)
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].SharedBytes > out[j].SharedBytes })
	return out
}

// HighConfidenceArtifactKeys returns the public keys of artifacts safe to remove
// automatically — only the high-confidence (provably corrupt) ones.
func HighConfidenceArtifactKeys(nodes []store.Node) []string {
	var keys []string
	for _, a := range FindArtifacts(nodes) {
		if a.Confidence == "high" {
			keys = append(keys, a.Key)
		}
	}
	return keys
}
