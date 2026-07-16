package analytics

import (
	"os"
	"testing"
	"time"

	"github.com/jjkroell/ridgeline/internal/store"
)

// TestDetectInjectionIntegration runs the real detector against a Ridgeline-format
// database whose path is given by RIDGELINE_DETECT_DB. Skipped unless that env var
// is set, so it never runs in normal CI. Used to validate detection against real
// captured traffic (e.g. the June-19 .525 bridge injection).
func TestDetectInjectionIntegration(t *testing.T) {
	path := os.Getenv("RIDGELINE_DETECT_DB")
	if path == "" {
		t.Skip("set RIDGELINE_DETECT_DB to a Ridgeline DB to run")
	}
	st, err := store.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()
	nodes, err := st.ListNodes()
	if err != nil {
		t.Fatal(err)
	}
	cutoff := time.Now().Add(-365 * 24 * time.Hour).UTC().Format(time.RFC3339Nano)
	rep, err := DetectInjection(st, nodes, cutoff, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("bridges=%d injectors=%d", len(rep.Bridges), len(rep.Injectors))
	for _, b := range rep.Bridges {
		t.Logf("  BRIDGE %s (%s) captive=%d/%d capFrac=%.2f km=%.0f", b.Name, b.NodeKey[:12], b.CaptiveCount, b.ForeignThrough, b.CaptiveFraction, b.ForeignKm)
	}
	for _, in := range rep.Injectors {
		t.Logf("  INJECTOR %s exclusive=%d", in.Observer, in.ExclusiveCount)
	}
	if len(rep.Bridges) == 0 {
		t.Error("expected at least one bridge candidate")
	}
}
