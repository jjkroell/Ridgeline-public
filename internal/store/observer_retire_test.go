package store

import (
	"testing"
	"time"
)

// TestRetireObserver verifies that retiring withdraws an observer from the
// observers page while keeping every packet it reported, and that it is
// reversible.
func TestRetireObserver(t *testing.T) {
	st := testStore(t)

	now := time.Now().UTC()
	for _, id := range []string{"gone-obs", "live-obs"} {
		if err := st.Record(Observation{
			Packet:     advertPkt("AABBCCDD"),
			RawHex:     "00",
			ObserverID: id,
			ReceivedAt: now,
		}); err != nil {
			t.Fatalf("record %s: %v", id, err)
		}
	}

	if err := st.RetireObserver("gone-obs", now.Format(time.RFC3339)); err != nil {
		t.Fatalf("RetireObserver: %v", err)
	}

	active, err := st.ListObservers()
	if err != nil {
		t.Fatalf("ListObservers: %v", err)
	}
	if len(active) != 1 || active[0].ID != "live-obs" {
		t.Fatalf("active observers = %v, want [live-obs]", active)
	}

	retired, err := st.ListRetiredObservers()
	if err != nil {
		t.Fatalf("ListRetiredObservers: %v", err)
	}
	if len(retired) != 1 || retired[0].ID != "gone-obs" || retired[0].RetiredAt == "" {
		t.Fatalf("retired observers = %v, want [gone-obs] with a retiredAt", retired)
	}

	// Retiring is presentational: the packets it reported are still there.
	var observations int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM observations WHERE observer_id = 'gone-obs'`).Scan(&observations); err != nil {
		t.Fatalf("count observations: %v", err)
	}
	if observations != 1 {
		t.Fatalf("gone-obs observations = %d, want 1 (kept)", observations)
	}

	if err := st.UnretireObserver("gone-obs"); err != nil {
		t.Fatalf("UnretireObserver: %v", err)
	}
	if active, err = st.ListObservers(); err != nil || len(active) != 2 {
		t.Fatalf("after unretire: observers = %v (err %v), want 2", active, err)
	}
}

// TestRetiredObserverSurvivesStaleSweep verifies the retention sweep leaves
// retired observers alone. Their row is deliberately kept so a replayed retained
// /status can't re-INSERT them; deleting it would undo the retirement and let
// the observer reappear on the next reconnect.
func TestRetiredObserverSurvivesStaleSweep(t *testing.T) {
	st := testStore(t)

	now := time.Now().UTC()
	if err := st.Record(Observation{
		Packet:     advertPkt("AABBCCDD"),
		RawHex:     "00",
		ObserverID: "retired-obs",
		ReceivedAt: now.Add(-2 * time.Hour),
	}); err != nil {
		t.Fatalf("record: %v", err)
	}
	if err := st.RetireObserver("retired-obs", now.Format(time.RFC3339)); err != nil {
		t.Fatalf("RetireObserver: %v", err)
	}

	removed, err := st.DeleteStaleObservers(now.Add(-time.Hour).Format(time.RFC3339Nano))
	if err != nil {
		t.Fatalf("DeleteStaleObservers: %v", err)
	}
	if len(removed) != 0 {
		t.Fatalf("removed = %v, want none (retired observers are skipped)", removed)
	}
	retired, err := st.ListRetiredObservers()
	if err != nil || len(retired) != 1 {
		t.Fatalf("retired = %v (err %v), want the row still present", retired, err)
	}
}

// TestUpdateObserverStatusIfPresentNeverCreates verifies the retained-status
// path cannot conjure an observer row. A retained /status is the broker
// replaying a last-known value on every reconnect, so treating it as a live
// sighting is what used to resurrect decommissioned observers.
func TestUpdateObserverStatusIfPresentNeverCreates(t *testing.T) {
	st := testStore(t)
	now := time.Now().UTC().Format(time.RFC3339)

	found, err := st.UpdateObserverStatusIfPresent("ghost-obs", "R1", "AA", `{"state":"online"}`, "900.0,250,11,5", now)
	if err != nil {
		t.Fatalf("UpdateObserverStatusIfPresent: %v", err)
	}
	if found {
		t.Fatal("reported an update for an observer that does not exist")
	}
	obs, err := st.ListObservers()
	if err != nil {
		t.Fatalf("ListObservers: %v", err)
	}
	if len(obs) != 0 {
		t.Fatalf("observers = %v, want none created from a retained status", obs)
	}

	// An observer we already know is still refreshed by it.
	if err := st.UpsertObserverStatus("real-obs", "R1", "BB", `{"state":"online"}`, "900.0,250,11,5", now); err != nil {
		t.Fatalf("UpsertObserverStatus: %v", err)
	}
	found, err = st.UpdateObserverStatusIfPresent("real-obs", "R1", "BB", `{"state":"offline"}`, "", now)
	if err != nil || !found {
		t.Fatalf("UpdateObserverStatusIfPresent(real-obs) = (%v, %v), want (true, nil)", found, err)
	}
}
