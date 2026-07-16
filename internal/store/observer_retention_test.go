package store

import (
	"testing"
	"time"
)

// TestDeleteStaleObservers verifies the sweep removes only observers whose
// last_seen predates the cutoff, keeps active ones, and leaves the observations
// (packets) they reported untouched.
func TestDeleteStaleObservers(t *testing.T) {
	st := testStore(t)

	now := time.Now().UTC()
	rec := func(observer string, at time.Time) {
		if err := st.Record(Observation{
			Packet:     advertPkt("AABBCCDD"),
			RawHex:     "00",
			ObserverID: observer,
			ReceivedAt: at,
		}); err != nil {
			t.Fatalf("record %s: %v", observer, err)
		}
	}
	rec("stale-obs", now.Add(-2*time.Hour))
	rec("fresh-obs", now)

	cutoff := now.Add(-1 * time.Hour).Format(time.RFC3339Nano)
	removed, err := st.DeleteStaleObservers(cutoff)
	if err != nil {
		t.Fatalf("DeleteStaleObservers: %v", err)
	}
	if len(removed) != 1 || removed[0] != "stale-obs" {
		t.Fatalf("removed = %v, want [stale-obs]", removed)
	}

	obs, err := st.ListObservers()
	if err != nil {
		t.Fatalf("ListObservers: %v", err)
	}
	if len(obs) != 1 || obs[0].ID != "fresh-obs" {
		t.Fatalf("remaining observers = %v, want [fresh-obs]", obs)
	}

	// The stale observer's reported packet must survive.
	var observations int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM observations`).Scan(&observations); err != nil {
		t.Fatalf("count observations: %v", err)
	}
	if observations != 2 {
		t.Fatalf("observations = %d, want 2 (both kept)", observations)
	}

	// A second sweep with nothing stale is a no-op.
	if removed, err := st.DeleteStaleObservers(cutoff); err != nil || removed != nil {
		t.Fatalf("second sweep = (%v, %v), want (nil, nil)", removed, err)
	}
}
