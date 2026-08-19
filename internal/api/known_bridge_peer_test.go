package api

import (
	"encoding/json"
	"testing"

	"github.com/jjkroell/ridgeline/internal/store"
)

// Marking a bridge as sanctioned carries the far side of the link, so the
// console can render it as "near -> far" rather than naming one end.
func TestAdminBlockCarriesKnownBridgePeer(t *testing.T) {
	st, base, cleanup := newAuthEnv(t)
	defer cleanup()

	const near = "AA11BB22CC33DD44EE55FF6600112233"
	const far = "99887766554433221100AABBCCDDEEFF"

	admin := newClient(t, base) // first account = admin
	admin.do("POST", "/api/auth/register",
		map[string]string{"email": "admin@example.com", "password": "hunter2hunter2"}, false)
	member := newClient(t, base)
	member.do("POST", "/api/auth/register",
		map[string]string{"email": "member@example.com", "password": "hunter2hunter2"}, false)

	body := map[string]any{"kind": "known", "key": near, "name": "Near End", "reason": "known bridge", "peer": far}

	// Still admin-only.
	if resp, _ := member.do("POST", "/api/admin/block", body, true); resp.StatusCode != 403 {
		t.Errorf("member block should be 403, got %d", resp.StatusCode)
	}
	// A peer is meaningless on anything but a sanctioned bridge.
	if resp, _ := admin.do("POST", "/api/admin/block",
		map[string]any{"kind": "bridge", "key": near, "peer": far}, true); resp.StatusCode != 400 {
		t.Errorf("peer on kind=bridge should be 400, got %d", resp.StatusCode)
	}

	if resp, _ := admin.do("POST", "/api/admin/block", body, true); resp.StatusCode != 200 {
		t.Fatalf("admin block should be 200, got %d", resp.StatusCode)
	}

	e := findBlock(t, st, store.BlockKnown, near)
	if e.Peer != far {
		t.Errorf("stored peer = %q, want %q", e.Peer, far)
	}
	// Sanctioning must never block traffic — that is the whole point of "known".
	if st.IsNodeBlocked(near) || st.IsNodeBlocked(far) {
		t.Error("marking a bridge known blocked a node")
	}

	// And it comes back over the wire for the console to render. The shared test
	// client decodes into a map, so read the array directly.
	resp, err := admin.http.Get(base + "/api/admin/blocklist")
	if err != nil {
		t.Fatalf("GET blocklist: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("blocklist should be 200, got %d", resp.StatusCode)
	}
	var listed []store.BlockEntry
	if err := json.NewDecoder(resp.Body).Decode(&listed); err != nil {
		t.Fatalf("decode blocklist: %v", err)
	}
	var found bool
	for _, b := range listed {
		if b.Kind == store.BlockKnown && b.Key == near {
			found = true
			if b.Peer != far {
				t.Errorf("API peer = %q, want %q", b.Peer, far)
			}
		}
	}
	if !found {
		t.Errorf("known bridge missing from /api/admin/blocklist: %+v", listed)
	}

	// clearPeer forgets the link without unmarking the bridge.
	if resp, _ := admin.do("POST", "/api/admin/block",
		map[string]any{"kind": "known", "key": near, "clearPeer": true}, true); resp.StatusCode != 200 {
		t.Fatalf("clearPeer should be 200, got %d", resp.StatusCode)
	}
	if e := findBlock(t, st, store.BlockKnown, near); e.Peer != "" {
		t.Errorf("peer = %q after clearPeer, want empty", e.Peer)
	}
}

func findBlock(t *testing.T, st *store.Store, kind, key string) store.BlockEntry {
	t.Helper()
	blocks, err := st.ListBlocks()
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range blocks {
		if b.Kind == kind && b.Key == key {
			return b
		}
	}
	t.Fatalf("block %s/%s not found in %+v", kind, key, blocks)
	return store.BlockEntry{}
}
