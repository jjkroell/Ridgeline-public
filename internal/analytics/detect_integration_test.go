package analytics

import (
	"os"
	"strconv"
	"strings"
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
	// Window matters: a long one lets every node be heard directly at some point,
	// which erases the very signal being measured. RIDGELINE_DETECT_HOURS selects it.
	hours := 365 * 24
	if h := os.Getenv("RIDGELINE_DETECT_HOURS"); h != "" {
		if v, err := strconv.Atoi(h); err == nil && v > 0 {
			hours = v
		}
	}
	cutoff := time.Now().Add(-time.Duration(hours) * time.Hour).UTC().Format(time.RFC3339Nano)
	t.Logf("window=%dh", hours)
	rep, err := DetectInjection(st, nodes, cutoff, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("packets=%d paths=%d unresolvedHops=%d | adverts=%d rejected=%d | bridges=%d injectors=%d",
		rep.PacketsScanned, rep.PathsScanned, rep.UnresolvedHops,
		rep.AdvertsScanned, rep.AdvertsRejected, len(rep.Bridges), len(rep.Injectors))
	for _, b := range rep.Bridges {
		t.Logf("  [%s] %s (%s) captive=%d/%d | pathVol=%d nextHops=%d topShare=%.0f%% terminal=%.1f%%",
			strings.Join(b.Signals, "+"), b.Name, b.NodeKey[:12], b.CaptiveCount, b.ForeignThrough,
			b.PathVolume, b.NextHops, b.NextHopTopShare*100, b.TerminalShare*100)
		for _, f := range b.Foreign {
			t.Logf("        behind: %s (%s) transit=%.0f%%", f.Name, f.Key[:10], f.TransitPct)
		}
	}
	for _, m := range rep.Migrations {
		via := ""
		if m.ViaBridge != "" {
			via = "  -> now behind " + m.ViaBridge
		}
		t.Logf("  MIGRATED %s (%s) lastDirect=%s relayedAfter=%d%s",
			m.Name, m.Key[:10], m.LastDirectAt[:19], m.RelayedAfter, via)
	}
	for _, in := range rep.Injectors {
		t.Logf("  INJECTOR %s exclusive=%d", in.Observer, in.ExclusiveCount)
	}
	// No assertion on count: this harness is for comparing detector behaviour
	// across windows and revisions against known ground truth, not a pass/fail gate.
}
