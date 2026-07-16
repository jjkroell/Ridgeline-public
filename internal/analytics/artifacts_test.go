package analytics

import (
	"strings"
	"testing"

	"github.com/jjkroell/ridgeline/internal/store"
)

func key(prefix string) string {
	return prefix + strings.Repeat("0", 64-len(prefix))
}

func nd(k, name string, adv, hs int) store.Node {
	return store.Node{
		PublicKey:   key(k),
		Name:        name,
		AdvertCount: adv,
		HashSize:    hs,
		LastSeen:    "2026-06-23T00:00:00Z",
	}
}

func TestFindArtifacts(t *testing.T) {
	nodes := []store.Node{
		// 3-byte cohort: corrupted copy sharing 4 leading bytes → HIGH.
		nd("ABCDEF12", "Cougar", 100, 3),  // canonical
		nd("ABCDEF1299", "Cougar?", 1, 3), // shares ABCDEF12 (4 bytes) → high
		// 1-byte cohort: genuine distinct collision (share 1 byte, real names) → NOT flagged.
		nd("AA", "Node Alpha", 50, 1),
		nd("AABB", "Node Beta", 40, 1),
		// 1-byte cohort: identical name, corrupted key (shares <4) → HIGH.
		nd("CC", "Repeater One", 100, 1), // canonical
		nd("CC22", "Repeater One", 1, 1), // identical name → high
		// 1-byte cohort: garbage name next to a dominant node → HIGH.
		nd("DD", "Real Repeater", 50, 1),
		nd("DD22", "=", 1, 1),
		// 1-byte cohort: rarely-heard, shares 2 bytes with dominant → MEDIUM (not auto-scrubbed).
		nd("FFAA", "Busy Node", 100, 1),
		nd("FFAA99", "Other", 1, 1),
		// Cohort isolation: same first byte but different configured length → NOT a collision.
		nd("EE", "One-byte Node", 10, 1),
		nd("EE2222", "Three-byte Node", 1, 3),
	}

	arts := FindArtifacts(nodes)
	if len(arts) != 4 {
		t.Fatalf("expected 4 artifacts (3 high + 1 medium), got %d: %+v", len(arts), arts)
	}

	got := map[string]string{} // key prefix → confidence
	for _, a := range arts {
		got[a.Key[:10]] = a.Confidence
	}
	want := map[string]string{
		key("ABCDEF1299")[:10]: "high",
		key("CC22")[:10]:       "high",
		key("DD22")[:10]:       "high",
		key("FFAA99")[:10]:     "medium",
	}
	for k, conf := range want {
		if got[k] != conf {
			t.Errorf("key %s: want confidence %q, got %q", k, conf, got[k])
		}
	}

	// Genuine distinct nodes and cohort-isolated nodes must never be flagged.
	for _, a := range arts {
		switch a.Key {
		case key("AA"), key("AABB"), key("EE"), key("EE2222"):
			t.Errorf("flagged a genuine/isolated node as artifact: %s (%s)", a.Name, a.Reason)
		}
	}

	// Auto-scrub keys = high confidence only (3), never the medium one.
	scrub := HighConfidenceArtifactKeys(nodes)
	if len(scrub) != 3 {
		t.Fatalf("expected 3 high-confidence scrub keys, got %d: %v", len(scrub), scrub)
	}
	for _, k := range scrub {
		if k == key("FFAA99") {
			t.Error("medium-confidence artifact must not be in the auto-scrub set")
		}
	}
}
