package analytics

import (
	"testing"

	"github.com/jjkroell/ridgeline/internal/store"
)

func TestBuildTopology(t *testing.T) {
	// Two transmissions; path A→B→C once, A→B twice (one as its own tx).
	relayPath := map[string][]string{
		"h1": {"A", "B", "C"},
		"h2": {"A", "B"},
	}
	relayHits := map[string]map[string]bool{
		"A": {"h1": true, "h2": true},
		"B": {"h1": true, "h2": true},
		"C": {"h1": true},
	}
	byKey := map[string]store.Node{
		"A": {PublicKey: "A", Name: "Alpha", Role: "Repeater"},
		"B": {PublicKey: "B", Name: "Bravo", Role: "Repeater"},
		"C": {PublicKey: "C", Name: "Charlie", Role: "Repeater"},
	}

	topo := buildTopology(relayPath, relayHits, byKey, 60)

	if len(topo.Nodes) != 3 {
		t.Fatalf("nodes = %d, want 3", len(topo.Nodes))
	}
	// A and B forwarded 2 transmissions each → ranked above C (1).
	if topo.Nodes[0].Relayed != 2 || topo.Nodes[len(topo.Nodes)-1].Relayed != 1 {
		t.Errorf("ranking off: %+v", topo.Nodes)
	}

	wantW := map[[2]string]int{{"A", "B"}: 2, {"B", "C"}: 1}
	if len(topo.Edges) != len(wantW) {
		t.Fatalf("edges = %d, want %d (%+v)", len(topo.Edges), len(wantW), topo.Edges)
	}
	for _, e := range topo.Edges {
		if e.A > e.B {
			t.Errorf("edge not canonical-ordered: %+v", e)
		}
		if wantW[[2]string{e.A, e.B}] != e.Weight {
			t.Errorf("edge %s-%s weight = %d, want %d", e.A, e.B, e.Weight, wantW[[2]string{e.A, e.B}])
		}
	}

	// Capping keeps only the busiest nodes and drops edges to dropped nodes.
	capped := buildTopology(relayPath, relayHits, byKey, 2)
	if len(capped.Nodes) != 2 {
		t.Fatalf("capped nodes = %d, want 2", len(capped.Nodes))
	}
	for _, e := range capped.Edges {
		if e.A == "C" || e.B == "C" {
			t.Errorf("edge to dropped node C survived: %+v", e)
		}
	}
}
