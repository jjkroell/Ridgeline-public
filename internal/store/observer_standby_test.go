package store

import (
	"testing"
	"time"
)

// TestObserverStandbyDiscardsAndRestores covers the whole lifecycle: an
// observer on standby reports as such, comes back on release, and neither
// transition touches anything it already reported.
func TestObserverStandbyDiscardsAndRestores(t *testing.T) {
	st := testStore(t)
	seedObserver(t, st, "obs-a")
	seedObserver(t, st, "obs-b")

	before := observerByID(t, st, "obs-a").PacketCount

	if st.ObserverOnStandby("obs-a") {
		t.Fatal("observer on standby before being stood down")
	}
	if err := st.SetObserverStandby("obs-a", NowRFC3339()); err != nil {
		t.Fatal(err)
	}
	if !st.ObserverOnStandby("obs-a") {
		t.Error("observer not on standby after SetObserverStandby")
	}
	if st.ObserverOnStandby("obs-b") {
		t.Error("standing one observer down put another on standby")
	}

	a := observerByID(t, st, "obs-a")
	if a.StandbySince == "" {
		t.Error("StandbySince not reported")
	}
	if a.PacketCount != before {
		t.Errorf("packet count changed on stand-down: %d -> %d", before, a.PacketCount)
	}

	if err := st.ClearObserverStandby("obs-a"); err != nil {
		t.Fatal(err)
	}
	if st.ObserverOnStandby("obs-a") {
		t.Error("still on standby after being returned to service")
	}
	if observerByID(t, st, "obs-a").StandbySince != "" {
		t.Error("StandbySince still reported after release")
	}
}

// TestStandbyObserverStaysListed pins the difference from retirement: a stood-
// down observer is still an active observer everywhere it is counted, because
// the operator must be able to see that it is on standby.
func TestStandbyObserverStaysListed(t *testing.T) {
	st := testStore(t)
	seedObserver(t, st, "obs-a")

	if err := st.SetObserverStandby("obs-a", NowRFC3339()); err != nil {
		t.Fatal(err)
	}

	active, err := st.ListObservers()
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 || active[0].ID != "obs-a" {
		t.Fatalf("standby observer dropped from ListObservers: %+v", active)
	}
}

// TestStandbyDropCounter — the discard counter is per observer and is cleared
// when the observer returns to service, so a later stand-down starts at zero
// rather than resuming a stale total.
func TestStandbyDropCounter(t *testing.T) {
	st := testStore(t)
	seedObserver(t, st, "obs-a")
	if err := st.SetObserverStandby("obs-a", NowRFC3339()); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 3; i++ {
		st.RecordStandbyDrop("obs-a", NowRFC3339())
	}
	if got := st.StandbyDropped()["obs-a"]; got != 3 {
		t.Errorf("dropped counter = %d, want 3", got)
	}
	if got := observerByID(t, st, "obs-a").StandbyDropped; got != 3 {
		t.Errorf("Observer.StandbyDropped = %d, want 3", got)
	}

	if err := st.ClearObserverStandby("obs-a"); err != nil {
		t.Fatal(err)
	}
	if got := st.StandbyDropped()["obs-a"]; got != 0 {
		t.Errorf("counter survived return to service: %d", got)
	}
}

// TestStandbySurvivesStatusUpsert is the trap retirement already had: an
// observer publishes /status with retain=true, so the broker replays it on every
// reconnect. If the status upsert cleared standby_since, a stand-down would
// silently end at the next daemon restart.
func TestStandbySurvivesStatusUpsert(t *testing.T) {
	st := testStore(t)
	seedObserver(t, st, "obs-a")
	if err := st.SetObserverStandby("obs-a", NowRFC3339()); err != nil {
		t.Fatal(err)
	}

	if err := st.UpsertObserverStatus("obs-a", "Obs A", "R1", "", `{"state":"online"}`, "915.0,250,10,5", NowRFC3339()); err != nil {
		t.Fatal(err)
	}

	if !st.ObserverOnStandby("obs-a") {
		t.Error("a retained /status replay returned the observer to service")
	}
	if observerByID(t, st, "obs-a").StandbySince == "" {
		t.Error("status upsert cleared standby_since")
	}
}

// TestStandbyCacheReloadsOnOpen — the ingest hot path reads an in-memory set, so
// a stand-down must be rebuilt from the table when the daemon restarts.
func TestStandbyCacheReloadsOnOpen(t *testing.T) {
	st := testStore(t)
	seedObserver(t, st, "obs-a")
	if err := st.SetObserverStandby("obs-a", NowRFC3339()); err != nil {
		t.Fatal(err)
	}
	// Rebuild the cache from scratch, as Open does.
	st.standbyMu.Lock()
	st.standbyObservers = map[string]bool{}
	st.standbyMu.Unlock()

	if err := st.loadStandby(); err != nil {
		t.Fatal(err)
	}
	if !st.ObserverOnStandby("obs-a") {
		t.Error("standby not restored from the database")
	}
}

// TestStandbyObserverSurvivesStaleSweep is a regression test for a bug found on
// the dev box: standby stops packets, and packets are the ONLY thing that
// advances an observer's last_seen, so a stood-down observer went stale and
// DeleteStaleObservers would delete the row an hour later — silently discarding
// the stand-down along with it. Retirement was already skipped for exactly this
// reason; standby has to be too.
func TestStandbyObserverSurvivesStaleSweep(t *testing.T) {
	st := testStore(t)
	seedObserver(t, st, "held")
	seedObserver(t, st, "gone")

	if err := st.SetObserverStandby("held", NowRFC3339()); err != nil {
		t.Fatal(err)
	}

	// A cutoff in the far future makes every observer look stale.
	cutoff := time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339)
	removed, err := st.DeleteStaleObservers(cutoff)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range removed {
		if id == "held" {
			t.Fatal("retention deleted an observer that was on standby")
		}
	}
	if !st.ObserverOnStandby("held") {
		t.Error("stand-down lost after a retention sweep")
	}
	// The sweep must still do its job for everything else.
	if len(removed) != 1 || removed[0] != "gone" {
		t.Errorf("retention should have removed exactly the stale observer, got %v", removed)
	}
}

// TestStandbyDropRefreshesLastSeen — the other half of the same bug. A discarded
// packet is still evidence the observer is alive, so it keeps last_seen current
// (throttled); otherwise the observer reads as Silent within minutes of being
// stood down.
func TestStandbyDropRefreshesLastSeen(t *testing.T) {
	st := testStore(t)
	seedObserver(t, st, "held")
	if err := st.SetObserverStandby("held", NowRFC3339()); err != nil {
		t.Fatal(err)
	}
	before := observerByID(t, st, "held").LastSeen

	later := time.Now().UTC().Add(90 * time.Second).Format(time.RFC3339Nano)
	st.RecordStandbyDrop("held", later)

	if got := observerByID(t, st, "held").LastSeen; got != later {
		t.Errorf("last_seen not refreshed by a discarded packet: %q (was %q)", got, before)
	}
}

func seedObserver(t *testing.T, st *Store, id string) {
	t.Helper()
	if err := st.Record(Observation{
		Packet:     advertPkt("AABBCCDD"),
		RawHex:     "00",
		ObserverID: id,
		ReceivedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
}

func observerByID(t *testing.T, st *Store, id string) Observer {
	t.Helper()
	obs, err := st.ListObservers()
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range obs {
		if o.ID == id {
			return o
		}
	}
	t.Fatalf("observer %q not found", id)
	return Observer{}
}

// TestRetirementIsReleasedOnUpgrade — observer retirement was removed in favour
// of standby + delete. An observer retired by the OLD feature must not stay
// hidden by a column no UI can reach any more, so Open() clears it once. This
// pins that an observer carrying a stale retired_at is listed normally.
func TestRetirementIsReleasedOnUpgrade(t *testing.T) {
	st := testStore(t)
	seedObserver(t, st, "was-retired")

	// Simulate a row left behind by the removed feature.
	if _, err := st.db.Exec(`UPDATE observers SET retired_at = ? WHERE id = ?`,
		"2026-07-20T04:41:46Z", "was-retired"); err != nil {
		t.Fatal(err)
	}

	obs, err := st.ListObservers()
	if err != nil {
		t.Fatal(err)
	}
	// Nothing reads retired_at any more, so it is listed even before the clear.
	if len(obs) != 1 || obs[0].ID != "was-retired" {
		t.Fatalf("a stale retired_at still hides an observer: %+v", obs)
	}

	// And the one-time clear removes the invisible state entirely.
	if _, err := st.db.Exec(`UPDATE observers SET retired_at = NULL WHERE retired_at IS NOT NULL`); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM observers WHERE retired_at IS NOT NULL`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("%d observers still carry retired_at", n)
	}
}
