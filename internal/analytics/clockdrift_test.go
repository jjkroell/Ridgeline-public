package analytics

import (
	"testing"
	"time"
)

func base() time.Time { return time.Unix(1_700_000_000, 0).UTC() }

func TestDriftNeedsTwoSamples(t *testing.T) {
	d := newDriftAcc()
	if got, _ := d.drift(); got != nil {
		t.Error("no samples should yield nil")
	}
	b := base()
	d.observe("h1", uint32(b.Unix()+30), b)
	if got, _ := d.drift(); got != nil {
		t.Error("a single advert should not be enough to claim a drift")
	}
	d.observe("h2", uint32(b.Unix()+30), b)
	got, _ := d.drift()
	if got == nil || *got != 30 {
		t.Fatalf("drift = %v, want 30", got)
	}
}

// TestDriftIgnoresRefloods is the important one. MeshCore re-floods an advert
// payload unchanged, so the same message hash keeps arriving for a long time
// carrying its original timestamp. Only the earliest reception may count, or a
// perfectly healthy node reads as drifting further behind the longer it runs.
func TestDriftIgnoresRefloods(t *testing.T) {
	b := base()
	d := newDriftAcc()

	// Two adverts, each in sync, each re-flooded much later.
	d.observe("h1", uint32(b.Unix()), b)
	d.observe("h1", uint32(b.Unix()), b.Add(20*time.Minute))
	d.observe("h1", uint32(b.Unix()), b.Add(55*time.Minute))
	d.observe("h2", uint32(b.Add(time.Hour).Unix()), b.Add(time.Hour))
	d.observe("h2", uint32(b.Add(time.Hour).Unix()), b.Add(2*time.Hour))

	got, _ := d.drift()
	if got == nil {
		t.Fatal("expected a drift figure")
	}
	if *got != 0 {
		t.Errorf("drift = %.0fs, want 0 — re-floods must not register as lag", *got)
	}
	if d.samples() != 2 {
		t.Errorf("samples = %d, want 2 distinct adverts", d.samples())
	}
}

// An out-of-order arrival must still lower the earliest, not just the first seen.
func TestDriftKeepsEarliestArrival(t *testing.T) {
	b := base()
	d := newDriftAcc()
	d.observe("h1", uint32(b.Unix()), b.Add(10*time.Minute)) // late arrival first
	d.observe("h1", uint32(b.Unix()), b)                     // true first reception
	d.observe("h2", uint32(b.Unix()), b)
	got, _ := d.drift()
	if got == nil || *got != 0 {
		t.Fatalf("drift = %v, want 0", got)
	}
}

func TestDriftMedianResistsOutlier(t *testing.T) {
	b := base()
	d := newDriftAcc()
	// Three readings at +60s and one wild one; the median must hold.
	d.observe("h1", uint32(b.Unix()+60), b)
	d.observe("h2", uint32(b.Unix()+60), b)
	d.observe("h3", uint32(b.Unix()+60), b)
	d.observe("h4", uint32(b.Unix()+9000), b)
	got, _ := d.drift()
	if got == nil || *got != 60 {
		t.Fatalf("drift = %v, want 60 (median, not mean)", got)
	}
}

func TestDriftRejectsUnsetAndAbsurdClocks(t *testing.T) {
	b := base()
	d := newDriftAcc()
	d.observe("h1", 0, b)    // never set
	d.observe("h2", 1000, b) // epoch-era garbage
	d.observe("h3", uint32(b.AddDate(20, 0, 0).Unix()), b) // two decades ahead
	if d.samples() != 0 {
		t.Errorf("unset/absurd clocks must not be recorded, samples = %d", d.samples())
	}
	if got, _ := d.drift(); got != nil {
		t.Errorf("drift = %v, want nil", got)
	}
}

// A clock stuck years back is MeshCore falling back to the firmware build date
// — the node's clock was never set. That is a distinct fault from drifting, so
// it is reported as unset rather than as a nonsense "-812 days behind".
func TestDriftFlagsNeverSetClock(t *testing.T) {
	b := base()
	d := newDriftAcc()
	stuck := uint32(b.AddDate(-2, 0, 0).Unix()) // a real date, but two years stale
	d.observe("h1", stuck, b)
	d.observe("h2", stuck, b)
	got, unset := d.drift()
	if !unset {
		t.Error("a clock two years stale should be flagged as never set")
	}
	if got != nil {
		t.Errorf("offset = %v, want nil — an unset clock has no meaningful drift", got)
	}
}

// Some nodes emit a few correctly-stamped adverts among many build-date ones.
// A plain median over both populations flips between them and reports a wild
// figure; the majority must decide the verdict instead.
func TestDriftMixedPopulationsTakeTheMajority(t *testing.T) {
	b := base()
	stuck := uint32(b.AddDate(-2, 0, 0).Unix())

	mostlyUnset := newDriftAcc()
	mostlyUnset.observe("h1", stuck, b)
	mostlyUnset.observe("h2", stuck, b)
	mostlyUnset.observe("h3", stuck, b)
	mostlyUnset.observe("h4", uint32(b.Unix()+45), b)
	if got, unset := mostlyUnset.drift(); !unset || got != nil {
		t.Errorf("mostly-unset node: got (%v, %v), want (nil, true)", got, unset)
	}

	mostlyReal := newDriftAcc()
	mostlyReal.observe("h1", uint32(b.Unix()+45), b)
	mostlyReal.observe("h2", uint32(b.Unix()+45), b)
	mostlyReal.observe("h3", uint32(b.Unix()+45), b)
	mostlyReal.observe("h4", stuck, b)
	got, unset := mostlyReal.drift()
	if unset {
		t.Error("mostly-real node should not be flagged unset")
	}
	if got == nil || *got != 45 {
		t.Errorf("offset = %v, want 45 — the stale outlier must not drag the median", got)
	}
}

func TestDriftNegativeWhenNodeLags(t *testing.T) {
	b := base()
	d := newDriftAcc()
	d.observe("h1", uint32(b.Unix()-125), b)
	d.observe("h2", uint32(b.Unix()-125), b)
	got, _ := d.drift()
	if got == nil || *got != -125 {
		t.Fatalf("drift = %v, want -125 (node behind the server)", got)
	}
}
