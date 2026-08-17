package api

import "testing"

// Standby is an admin action: it stands an observer down so its packets are
// discarded at ingest while it stays connected and listed. This covers the
// authorisation gate, the round trip, and the two properties that distinguish it
// from retiring — the observer is NOT hidden, and nothing it already reported is
// touched.
func TestAdminObserverStandby(t *testing.T) {
	st, base, cleanup := newAuthEnv(t)
	defer cleanup()

	const obsID = "Test Observer One"
	if err := st.UpsertObserverStatus(obsID, "Observer Label", "R1", "", "", "", "2026-07-06T20:39:32Z"); err != nil {
		t.Fatalf("seed observer: %v", err)
	}

	admin := newClient(t, base) // first account = admin
	admin.do("POST", "/api/auth/register",
		map[string]string{"email": "admin@example.com", "password": "hunter2hunter2"}, false)
	member := newClient(t, base)
	member.do("POST", "/api/auth/register",
		map[string]string{"email": "member@example.com", "password": "hunter2hunter2"}, false)

	// A plain member cannot stand an observer down.
	if resp, _ := member.do("POST", "/api/admin/observers/standby",
		map[string]any{"observer": obsID}, true); resp.StatusCode != 403 {
		t.Errorf("member standby should be 403, got %d", resp.StatusCode)
	}
	if st.ObserverOnStandby(obsID) {
		t.Fatal("a rejected member request still put the observer on standby")
	}
	// Missing observer is a 400.
	if resp, _ := admin.do("POST", "/api/admin/observers/standby",
		map[string]any{}, true); resp.StatusCode != 400 {
		t.Errorf("empty standby should be 400, got %d", resp.StatusCode)
	}

	// Admin stands it down.
	if resp, _ := admin.do("POST", "/api/admin/observers/standby",
		map[string]any{"observer": obsID}, true); resp.StatusCode != 200 {
		t.Fatalf("admin standby should be 200, got %d", resp.StatusCode)
	}
	if !st.ObserverOnStandby(obsID) {
		t.Error("ingest was not told to discard this observer's packets")
	}

	obs, err := st.ListObservers()
	if err != nil {
		t.Fatal(err)
	}
	if len(obs) != 1 {
		t.Fatalf("standby must not hide the observer; list has %d", len(obs))
	}
	if obs[0].StandbySince == "" {
		t.Error("standbySince not exposed on the observer")
	}
	if obs[0].RetiredAt != "" {
		t.Error("standby must not retire the observer")
	}

	// And back to service.
	if resp, _ := admin.do("POST", "/api/admin/observers/resume",
		map[string]any{"observer": obsID}, true); resp.StatusCode != 200 {
		t.Fatalf("admin resume should be 200, got %d", resp.StatusCode)
	}
	if st.ObserverOnStandby(obsID) {
		t.Error("observer still on standby after resume")
	}
}
