package api

import "testing"

// Admins can retire a stale observer via POST /api/admin/delete {observers:[...]};
// it removes the observers row (drops off the list) with no blocklist entry.
func TestAdminDeleteObserver(t *testing.T) {
	st, base, cleanup := newAuthEnv(t)
	defer cleanup()

	// Seed an observer row (id has spaces, like the real "Test Observer One").
	const obsID = "Test Observer One"
	if err := st.UpsertObserverStatus(obsID, "R2", "", "", "", "2026-07-06T20:39:32Z"); err != nil {
		t.Fatalf("seed observer: %v", err)
	}
	if obs, _ := st.ListObservers(); len(obs) != 1 {
		t.Fatalf("expected 1 seeded observer, got %d", len(obs))
	}

	admin := newClient(t, base) // first account = admin
	admin.do("POST", "/api/auth/register",
		map[string]string{"email": "admin@example.com", "password": "hunter2hunter2"}, false)
	member := newClient(t, base)
	member.do("POST", "/api/auth/register",
		map[string]string{"email": "member@example.com", "password": "hunter2hunter2"}, false)

	// A plain member cannot delete.
	if resp, _ := member.do("POST", "/api/admin/delete",
		map[string]any{"observers": []string{obsID}}, true); resp.StatusCode != 403 {
		t.Errorf("member delete should be 403, got %d", resp.StatusCode)
	}
	// Empty request is a 400.
	if resp, _ := admin.do("POST", "/api/admin/delete", map[string]any{}, true); resp.StatusCode != 400 {
		t.Errorf("empty delete should be 400, got %d", resp.StatusCode)
	}
	// Admin deletes the observer.
	if resp, _ := admin.do("POST", "/api/admin/delete",
		map[string]any{"observers": []string{obsID}}, true); resp.StatusCode != 200 {
		t.Fatalf("admin delete observer should be 200, got %d", resp.StatusCode)
	}
	if obs, _ := st.ListObservers(); len(obs) != 0 {
		t.Errorf("observer should be gone after delete, still have %d", len(obs))
	}
}

// The injection-detection / quarantine console is now gated by the is_admin
// session (any admin account), not a static token.
func TestAdminConsoleSessionGated(t *testing.T) {
	_, base, cleanup := newAuthEnv(t)
	defer cleanup()

	anon := newClient(t, base)
	// Unauthenticated → 401.
	for _, p := range []string{"/api/admin/blocklist", "/api/admin/detect"} {
		if resp, _ := anon.do("GET", p, nil, false); resp.StatusCode != 401 {
			t.Errorf("anon %s should be 401, got %d", p, resp.StatusCode)
		}
	}

	admin := newClient(t, base) // first account = admin
	admin.do("POST", "/api/auth/register",
		map[string]string{"email": "admin@example.com", "password": "hunter2hunter2"}, false)

	member := newClient(t, base)
	member.do("POST", "/api/auth/register",
		map[string]string{"email": "member@example.com", "password": "hunter2hunter2"}, false)

	// A plain member is forbidden from the console.
	if resp, _ := member.do("GET", "/api/admin/blocklist", nil, false); resp.StatusCode != 403 {
		t.Errorf("member blocklist should be 403, got %d", resp.StatusCode)
	}
	if resp, _ := member.do("POST", "/api/admin/block",
		map[string]string{"kind": "node", "key": "AA"}, true); resp.StatusCode != 403 {
		t.Errorf("member block should be 403, got %d", resp.StatusCode)
	}

	// The admin can read the blocklist + run detection.
	if resp, _ := admin.do("GET", "/api/admin/blocklist", nil, false); resp.StatusCode != 200 {
		t.Errorf("admin blocklist should be 200, got %d", resp.StatusCode)
	}
	if resp, _ := admin.do("GET", "/api/admin/detect", nil, false); resp.StatusCode != 200 {
		t.Errorf("admin detect should be 200, got %d", resp.StatusCode)
	}
	// Admin mutations still require the CSRF token (they run through requireUser).
	if resp, _ := admin.do("POST", "/api/admin/block",
		map[string]string{"kind": "node", "key": "AA"}, false); resp.StatusCode != 403 {
		t.Errorf("admin block without CSRF should be 403, got %d", resp.StatusCode)
	}
	if resp, _ := admin.do("POST", "/api/admin/block",
		map[string]string{"kind": "node", "key": "AABBCC"}, true); resp.StatusCode != 200 {
		t.Errorf("admin block with CSRF should be 200, got %d", resp.StatusCode)
	}
}
