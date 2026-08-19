package store

import (
	"testing"
	"time"
)

// TestUpdateObserverStatusIfPresentNeverCreates verifies the retained-status
// path cannot conjure an observer row. A retained /status is the broker
// replaying a last-known value on every reconnect, so treating it as a live
// sighting is what used to resurrect decommissioned observers.
func TestUpdateObserverStatusIfPresentNeverCreates(t *testing.T) {
	st := testStore(t)
	now := time.Now().UTC().Format(time.RFC3339)

	found, err := st.UpdateObserverStatusIfPresent("ghost-obs", "Ghost Label", "R1", "AA", `{"state":"online"}`, "900.0,250,11,5", now)
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
	if err := st.UpsertObserverStatus("real-obs", "Real Label", "R1", "BB", `{"state":"online"}`, "900.0,250,11,5", now); err != nil {
		t.Fatalf("UpsertObserverStatus: %v", err)
	}
	found, err = st.UpdateObserverStatusIfPresent("real-obs", "Real Label", "R1", "BB", `{"state":"offline"}`, "", now)
	if err != nil || !found {
		t.Fatalf("UpdateObserverStatusIfPresent(real-obs) = (%v, %v), want (true, nil)", found, err)
	}
}
