package store

import "testing"

const locNode = "AABBCCDDEEFF00112233445566778899AABBCCDDEEFF00112233445566778899"

func TestPrivateLocationRoundTrip(t *testing.T) {
	st := testStore(t)
	u, _ := st.CreateUser("owner@example.com", "h", "Owner")

	// Nothing set yet.
	if _, ok, err := st.GetPrivateLocation(locNode); err != nil || ok {
		t.Fatalf("expected no location, got ok=%v err=%v", ok, err)
	}

	// Set it.
	loc, err := st.SetPrivateLocation(locNode, u.ID, 49.1659, -123.9401, "rooftop")
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if loc.Latitude != 49.1659 || loc.Longitude != -123.9401 || loc.Label != "rooftop" {
		t.Fatalf("returned loc mismatch: %+v", loc)
	}

	got, ok, err := st.GetPrivateLocation(locNode)
	if err != nil || !ok {
		t.Fatalf("get after set: ok=%v err=%v", ok, err)
	}
	if got.Latitude != 49.1659 || got.Longitude != -123.9401 || got.UserID != u.ID {
		t.Fatalf("get mismatch: %+v", got)
	}

	// Upsert replaces in place (one row per node).
	if _, err := st.SetPrivateLocation(locNode, u.ID, 48.5, -123.0, ""); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _, _ = st.GetPrivateLocation(locNode)
	if got.Latitude != 48.5 || got.Label != "" {
		t.Fatalf("update not applied: %+v", got)
	}

	// Delete.
	removed, err := st.DeletePrivateLocation(locNode)
	if err != nil || !removed {
		t.Fatalf("delete: removed=%v err=%v", removed, err)
	}
	if _, ok, _ := st.GetPrivateLocation(locNode); ok {
		t.Fatal("location should be gone after delete")
	}
}

// Case-insensitive pubkey handling: a location set under any case is retrievable
// with any case (all normalised to upper).
func TestPrivateLocationPubkeyCase(t *testing.T) {
	st := testStore(t)
	u, _ := st.CreateUser("owner@example.com", "h", "Owner")
	lower := "aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899"
	if _, err := st.SetPrivateLocation(lower, u.ID, 1, 2, ""); err != nil {
		t.Fatalf("set: %v", err)
	}
	if _, ok, err := st.GetPrivateLocation(locNode); err != nil || !ok {
		t.Fatalf("expected upper lookup to hit, ok=%v err=%v", ok, err)
	}
}
