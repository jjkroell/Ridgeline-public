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

	got := FilterRelayedWithin(stale, allNodes, relayHops)
	sort.Strings(got)
	want := []string{"CC11223344556677", "DD11223344556677"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("FilterRelayedWithin = %v, want %v", got, want)
	}

	// No hop data → nothing filtered (fail-safe: keep the advert-based decision).
	if got := FilterRelayedWithin(stale, allNodes, nil); len(got) != len(stale) {
		t.Fatalf("empty relayHops should pass stale through unchanged, got %v", got)
	}
}
