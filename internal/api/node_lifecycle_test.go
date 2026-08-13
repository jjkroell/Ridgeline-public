package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jjkroell/ridgeline/internal/meshcore"
	"github.com/jjkroell/ridgeline/internal/store"
)

// seedClaimedNode records a real advert, registers two users, and gives the
// second one a VERIFIED claim on the node. Returns the node pubkey, its owner,
// and a signed-in non-owner.
func seedClaimedNode(t *testing.T) (st *store.Store, base, node string, owner, other *authTestClient, cleanup func()) {
	t.Helper()
	st, base, cleanup = newAuthEnv(t)

	pkt, err := meshcore.DecodeHex(claimAdvertHex)
	if err != nil || pkt.Advert == nil {
		t.Fatalf("decode advert: %v", err)
	}
	node = pkt.Advert.PublicKey
	if err := st.Record(store.Observation{Packet: pkt, RawHex: claimAdvertHex, ReceivedAt: time.Now()}); err != nil {
		t.Fatalf("record node: %v", err)
	}

	// First registration becomes admin; keep it as the NON-owner so the test
	// proves ownership is what grants access, not privilege.
	other = newClient(t, base)
	other.do("POST", "/api/auth/register",
		map[string]string{"email": "other@example.com", "password": "hunter2hunter2"}, false)

	owner = newClient(t, base)
	owner.do("POST", "/api/auth/register",
		map[string]string{"email": "owner@example.com", "password": "hunter2hunter2", "displayName": "Owner"}, false)

	resp, cb := owner.do("POST", "/api/claims", map[string]string{"pubkey": node}, true)
	if resp.StatusCode != 200 {
		t.Fatalf("open claim: %d %v", resp.StatusCode, cb)
	}
	code, _ := cb["code"].(string)
	if v, err := st.VerifyPendingClaims(node, "MyRepeater "+code); err != nil || len(v) != 1 {
		t.Fatalf("verify claim: n=%d err=%v", len(v), err)
	}
	return st, base, node, owner, other, cleanup
}

// The whole point of the feature: you must have CLAIMED the node before you can
// retire or scrub it. A signed-in stranger — even the admin account — is
// refused, and gets a response that reveals nothing about the node.
func TestNodeLifecycle_RequiresVerifiedOwnership(t *testing.T) {
	_, base, node, _, other, cleanup := seedClaimedNode(t)
	defer cleanup()

	for _, path := range []string{"/retire", "/unretire", "/scrub"} {
		resp, _ := other.do("POST", "/api/nodes/"+node+path,
			map[string]any{"deleteHistory": true}, true)
		if resp.StatusCode != 403 {
			t.Errorf("%s by a non-owner should be 403, got %d", path, resp.StatusCode)
		}
	}
	// And an anonymous caller never gets past the session gate.
	anon := newClient(t, base)
	if resp, _ := anon.do("POST", "/api/nodes/"+node+"/scrub", map[string]any{"deleteHistory": true}, true); resp.StatusCode != 401 {
		t.Errorf("anonymous scrub should be 401, got %d", resp.StatusCode)
	}
}

// Retire hides the node from the public list but keeps the row, the claim and
// everything it sent — and survives a re-advert, which is what makes it useful
// for a node that is briefly still on air.
func TestNodeLifecycle_RetireIsReversibleAndKeepsHistory(t *testing.T) {
	st, base, node, owner, _, cleanup := seedClaimedNode(t)
	defer cleanup()

	if resp, _ := owner.do("POST", "/api/nodes/"+node+"/retire", nil, true); resp.StatusCode != 200 {
		t.Fatalf("retire should succeed for the owner, got %d", resp.StatusCode)
	}
	if retired, err := st.IsNodeRetired(node); err != nil || !retired {
		t.Fatalf("node should be retired: %v %v", retired, err)
	}
	if inList(t, base, node) {
		t.Error("a retired node must not appear in /api/nodes")
	}
	// Still owned, and its history is intact.
	if _, cs := owner.do("GET", "/api/nodes/"+node+"/claim", nil, false); cs["ownedByMe"] != true {
		t.Error("retire must NOT release the claim")
	}

	// A re-advert must not resurrect it (the upsert leaves retired_at alone).
	pkt, _ := meshcore.DecodeHex(claimAdvertHex)
	if err := st.Record(store.Observation{Packet: pkt, RawHex: claimAdvertHex, ReceivedAt: time.Now()}); err != nil {
		t.Fatalf("re-record: %v", err)
	}
	if retired, _ := st.IsNodeRetired(node); !retired {
		t.Error("a re-advert must not un-retire the node")
	}

	if resp, _ := owner.do("POST", "/api/nodes/"+node+"/unretire", nil, true); resp.StatusCode != 200 {
		t.Fatalf("unretire should succeed, got %d", resp.StatusCode)
	}
	if !inList(t, base, node) {
		t.Error("an un-retired node should be listed again")
	}
}

// Scrub is the destructive path: it must refuse without explicit confirmation,
// and when it does run it releases the claim (deliberately — an orphaned claim
// would block the node from ever being re-claimed).
func TestNodeLifecycle_ScrubNeedsConfirmationAndReleasesClaim(t *testing.T) {
	st, _, node, owner, _, cleanup := seedClaimedNode(t)
	defer cleanup()

	if resp, _ := owner.do("POST", "/api/nodes/"+node+"/scrub", map[string]any{}, true); resp.StatusCode != 400 {
		t.Errorf("scrub without deleteHistory must be refused, got %d", resp.StatusCode)
	}
	if retired, _ := st.IsNodeRetired(node); retired {
		t.Error("a refused scrub must not have changed anything")
	}

	if resp, _ := owner.do("POST", "/api/nodes/"+node+"/scrub", map[string]any{"deleteHistory": true}, true); resp.StatusCode != 200 {
		t.Fatalf("confirmed scrub should succeed, got %d", resp.StatusCode)
	}
	nodes, err := st.ListNodes()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, n := range nodes {
		if n.PublicKey == node {
			t.Fatal("scrubbed node should be gone from the store entirely")
		}
	}
	// Claim released: ownedByMe is false and the node is re-claimable.
	if _, cs := owner.do("GET", "/api/nodes/"+node+"/claim", nil, false); cs["ownedByMe"] == true {
		t.Error("scrub must release the claim")
	}
}

// The audit row is the only trace that survives a scrub, since the scrub
// deletes the claim that would otherwise say who owned the node.
func TestNodeLifecycle_ScrubIsAudited(t *testing.T) {
	st, _, node, owner, _, cleanup := seedClaimedNode(t)
	defer cleanup()

	owner.do("POST", "/api/nodes/"+node+"/scrub", map[string]any{"deleteHistory": true}, true)

	rows, err := st.ListAudit(node)
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	var found bool
	for _, a := range rows {
		if a.Action == "node_scrub" && a.ActorEmail == "owner@example.com" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a node_scrub audit row naming the actor, got %+v", rows)
	}
}

// /api/nodes returns an ARRAY, which authTestClient.do can't decode, so fetch it
// directly.
func inList(t *testing.T, base, node string) bool {
	t.Helper()
	resp, err := http.Get(base + "/api/nodes")
	if err != nil {
		t.Fatalf("/api/nodes: %v", err)
	}
	defer resp.Body.Close()
	var out []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode nodes: %v", err)
	}
	for _, n := range out {
		if n["publicKey"] == node {
			return true
		}
	}
	return false
}

// Retention prunes a silent node's row (PurgeTargets) but deliberately KEEPS the
// claim, so the owner reconnects if the node returns. Those claims render as
// "Dormant" on the account page — and until now had no release control anywhere,
// because the only one lived on a node page that no longer exists. Releasing
// must therefore work with the node row gone: claimDelete never touches the
// nodes table, and this pins that.
func TestClaimRelease_WorksWhenNodeRowIsGone(t *testing.T) {
	st, _, node, owner, _, cleanup := seedClaimedNode(t)
	defer cleanup()

	// Simulate the retention sweep: node row removed, claim left behind.
	if _, err := st.PurgeTargets(nil, nil, []string{node}); err != nil {
		t.Fatalf("purge: %v", err)
	}
	nodes, _ := st.ListNodes()
	for _, n := range nodes {
		if strings.EqualFold(n.PublicKey, node) {
			t.Fatal("precondition: node row should be gone")
		}
	}
	// The claim is now orphaned — exactly the "Dormant" state.
	if _, cs := owner.do("GET", "/api/nodes/"+node+"/claim", nil, false); cs["ownedByMe"] != true {
		t.Fatal("precondition: the claim should have survived the purge")
	}

	resp, _ := owner.do("DELETE", "/api/claims/"+node, nil, true)
	if resp.StatusCode != 200 {
		t.Fatalf("releasing a dormant claim should succeed, got %d", resp.StatusCode)
	}
	if _, cs := owner.do("GET", "/api/nodes/"+node+"/claim", nil, false); cs["ownedByMe"] == true {
		t.Error("claim should be gone after release")
	}
}

// Scrubbing a DORMANT node (row already pruned by retention, claim surviving)
// must still cascade the user-authored data. This is what makes "delete
// everything" reachable for a node that no longer has a detail page.
func TestScrubDormantNode_CascadesUserData(t *testing.T) {
	st, _, node, owner, _, cleanup := seedClaimedNode(t)
	defer cleanup()

	owner.do("POST", "/api/nodes/"+node+"/notes", map[string]any{"body": "my repeater", "visibility": "public"}, true)
	owner.do("PUT", "/api/nodes/"+node+"/location", map[string]any{"latitude": 49.1, "longitude": -123.9, "label": "home"}, true)

	// Retention prunes the row; the claim (and the notes) survive.
	if _, err := st.PurgeTargets(nil, nil, []string{node}); err != nil {
		t.Fatalf("purge: %v", err)
	}
	if n, _ := st.NotesForNode(node, 0, false); len(n) != 1 {
		t.Fatalf("precondition: expected the note to survive the purge, got %d", len(n))
	}

	resp, _ := owner.do("POST", "/api/nodes/"+node+"/scrub", map[string]any{"deleteHistory": true}, true)
	if resp.StatusCode != 200 {
		t.Fatalf("scrubbing a dormant node should succeed, got %d", resp.StatusCode)
	}
	if n, _ := st.NotesForNode(node, 0, false); len(n) != 0 {
		t.Errorf("scrub must cascade notes, %d left", len(n))
	}
	if _, cs := owner.do("GET", "/api/nodes/"+node+"/claim", nil, false); cs["ownedByMe"] == true {
		t.Error("scrub must release the claim")
	}
}
