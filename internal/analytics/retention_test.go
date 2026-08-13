package analytics

import (
	"sort"
	"testing"

	"github.com/jjkroell/ridgeline/internal/store"
)

func TestStaleNodeKeys(t *testing.T) {
	cutoff := "2026-06-01T00:00:00Z"
	nodes := []store.Node{
		{PublicKey: "AAAA", LastSeen: "2026-05-01T00:00:00Z"}, // old advert → stale
		{PublicKey: "BBBB", LastSeen: "2026-06-15T00:00:00Z"}, // recent advert → keep
		{PublicKey: "CCCC", LastSeen: "2026-05-02T00:00:00Z"}, // old advert but relaying → keep
		{PublicKey: "DDDD", LastSeen: "2026-06-01T00:00:00Z"}, // exactly at cutoff → keep
		{PublicKey: "EEEE", LastSeen: ""},                     // never seen → skip
	}
	keep := map[string]LiveSignal{"CCCC": {RelayCount1h: 3}}

	got := StaleNodeKeys(nodes, keep, cutoff)
	sort.Strings(got)
	want := []string{"AAAA"}
	if len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("StaleNodeKeys = %v, want %v", got, want)
	}
}

func TestFilterRelayedWithin(t *testing.T) {
	// "AB…" and "AC…" share the 1-byte prefix "AB"? No — they share "A" only in
	// hex text; as 2-hex bytes "AB" and "AC" differ. Give two nodes a shared
	// 2-hex prefix "CC" so a 2-hex hop "CC" is ambiguous and credits neither.
	allNodes := []store.Node{
		{PublicKey: "AB11223344556677"}, // uniquely owns 2-hex "AB"
		{PublicKey: "BB11223344556677"}, // uniquely owns 4-hex "BB11"
		{PublicKey: "CC11223344556677"}, // shares 2-hex "CC" with the next node
		{PublicKey: "CC99223344556677"}, // shares 2-hex "CC"
		{PublicKey: "DD11223344556677"}, // never relays
	}
	stale := []string{
		"AB11223344556677", // hop "AB" resolves uniquely → relayed → drop
		"BB11223344556677", // hop "BB11" resolves uniquely → relayed → drop
		"CC11223344556677", // only ambiguous hop "CC" seen → NOT credited → stays
		"DD11223344556677", // no hop → stays stale
	}
	// "CC" is ambiguous (two owners) → credits no one; "AB" and "BB11" are unique.
	relayHops := map[string]bool{"AB": true, "BB11": true, "CC": true}

	got := FilterRelayedWithin(stale, allNodes, relayHops, 1)
	sort.Strings(got)
	want := []string{"CC11223344556677", "DD11223344556677"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("FilterRelayedWithin = %v, want %v", got, want)
	}

	// No hop data → nothing filtered (fail-safe: keep the advert-based decision).
	if got := FilterRelayedWithin(stale, allNodes, nil, 1); len(got) != len(stale) {
		t.Fatalf("empty relayHops should pass stale through unchanged, got %v", got)
	}
}

// The saturation case: a 1-byte hop matches SOME node almost by definition
// (~97% of the 256-value space is observed within a week of real traffic), so
// crediting it made any node with a unique 1-byte prefix immortal. At the
// default minHopBytes=2 a narrow hop is no longer evidence.
func TestFilterRelayedWithin_WidthGate(t *testing.T) {
	dead := "D2AAAAAAAAAA" // unique 1-byte prefix D2, long silent
	live := "7C11223344FF"
	all := []store.Node{{PublicKey: dead}, {PublicKey: live}}
	stale := []string{dead, live}

	t.Run("1-byte hop no longer credits at the default", func(t *testing.T) {
		got := FilterRelayedWithin(stale, all, map[string]bool{"D2": true}, 2)
		if len(got) != 2 {
			t.Fatalf("a 1-byte hop must not count as evidence; still-stale = %v, want both", got)
		}
	})

	t.Run("2-byte hop still credits", func(t *testing.T) {
		got := FilterRelayedWithin(stale, all, map[string]bool{"7C11": true}, 2)
		if len(got) != 1 || got[0] != dead {
			t.Fatalf("2-byte evidence should keep the live node; still-stale = %v, want [%s]", got, dead)
		}
	})

	t.Run("3-byte hop still credits", func(t *testing.T) {
		got := FilterRelayedWithin(stale, all, map[string]bool{"D2AAAA": true}, 2)
		if len(got) != 1 || got[0] != live {
			t.Fatalf("3-byte evidence should keep that node; still-stale = %v", got)
		}
	})

	t.Run("minHopBytes=1 restores the permissive behaviour", func(t *testing.T) {
		got := FilterRelayedWithin(stale, all, map[string]bool{"D2": true}, 1)
		if len(got) != 1 || got[0] != live {
			t.Fatalf("with the gate at 1 the narrow hop should credit again; got %v", got)
		}
	})

	t.Run("ambiguity still wins over width", func(t *testing.T) {
		// Two nodes share 3 bytes: even a wide hop credits nobody.
		a := "AB12345600"
		b := "AB12345611"
		got := FilterRelayedWithin([]string{a, b}, []store.Node{{PublicKey: a}, {PublicKey: b}},
			map[string]bool{"AB1234": true}, 2)
		if len(got) != 2 {
			t.Fatalf("an ambiguous hop must credit nobody regardless of width; got %v", got)
		}
	})
}
