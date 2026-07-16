package api

import (
	"testing"
	"time"

	"github.com/jjkroell/ridgeline/internal/meshcore"
	"github.com/jjkroell/ridgeline/internal/store"
)

// A real captured advert; Record it so the claimed node exists in the store.
const claimAdvertHex = "10B76000008A654F144C4A43D07F024E8E0A59120F9A3C2E825453F88F861C48C2F2BA245CD672C56C5BE3F52CB4470337904147543724983C2978DCCE234CE41898674714704C6A642E674B47350417E441B17CF5CC4BF444792266EDC3F4FE7970F4DF977CA9344CE16A45FEA7FCD25A85E9653FD2F1DB63666B0A792290A37F398341E70B61099279FFED021081B5F8F09F8D924368657272792048696C6C20F09F8D92"

func TestClaimFlow(t *testing.T) {
	st, base, cleanup := newAuthEnv(t)
	defer cleanup()

	// Seed the node by recording a real advert.
	pkt, err := meshcore.DecodeHex(claimAdvertHex)
	if err != nil || pkt.Advert == nil {
		t.Fatalf("decode advert: %v", err)
	}
	node := pkt.Advert.PublicKey
	if err := st.Record(store.Observation{Packet: pkt, RawHex: claimAdvertHex, ReceivedAt: time.Now()}); err != nil {
		t.Fatalf("record node: %v", err)
	}

	owner := newClient(t, base)
	owner.do("POST", "/api/auth/register",
		map[string]string{"email": "owner@example.com", "password": "hunter2hunter2"}, false)

	member := newClient(t, base)
	member.do("POST", "/api/auth/register",
		map[string]string{"email": "member@example.com", "password": "hunter2hunter2", "displayName": "Member"}, false)

	// Claiming is universal — any signed-in member can claim right away.
	// Invalid + unknown pubkeys.
	if resp, _ := member.do("POST", "/api/claims", map[string]string{"pubkey": "xyz"}, true); resp.StatusCode != 400 {
		t.Errorf("bad pubkey should be 400, got %d", resp.StatusCode)
	}
	unknown := "00000000000000000000000000000000000000000000000000000000000000AA"
	if resp, _ := member.do("POST", "/api/claims", map[string]string{"pubkey": unknown}, true); resp.StatusCode != 404 {
		t.Errorf("unknown node should be 404, got %d", resp.StatusCode)
	}

	// Member opens a claim → gets a code.
	resp, cb := member.do("POST", "/api/claims", map[string]string{"pubkey": node}, true)
	if resp.StatusCode != 200 {
		t.Fatalf("claim create: %d body %v", resp.StatusCode, cb)
	}
	code, _ := cb["code"].(string)
	if code == "" || cb["status"] != "pending" {
		t.Fatalf("expected a pending claim with a code, got %v", cb)
	}

	// Status endpoint shows the caller's pending claim, no owner yet.
	_, cs := member.do("GET", "/api/nodes/"+node+"/claim", nil, false)
	if cs["owner"] != nil || cs["ownedByMe"] != false {
		t.Errorf("node should be unowned pre-verification, got %v", cs)
	}
	if mine, _ := cs["mine"].(map[string]any); mine == nil || mine["status"] != "pending" {
		t.Errorf("expected caller's pending claim in status, got %v", cs["mine"])
	}

	// Simulate the ingest verifier seeing a signature-valid advert whose name
	// carries the code (the ingest hook calls exactly this after the sig check).
	if v, err := st.VerifyPendingClaims(node, "MyRepeater "+code); err != nil || len(v) != 1 {
		t.Fatalf("verify claim: n=%d err=%v", len(v), err)
	}

	// Now the member owns it; a public status call shows the owner's name.
	anon := newClient(t, base)
	_, ps := anon.do("GET", "/api/nodes/"+node+"/claim", nil, false)
	owner2, _ := ps["owner"].(map[string]any)
	if owner2 == nil || owner2["displayName"] != "Member" {
		t.Errorf("public status should show owner display name, got %v", ps["owner"])
	}
	_, ms := member.do("GET", "/api/nodes/"+node+"/claim", nil, false)
	if ms["ownedByMe"] != true {
		t.Error("member should see ownedByMe=true")
	}

	// Someone else can't claim an owned node.
	if resp, _ := owner.do("POST", "/api/claims", map[string]string{"pubkey": node}, true); resp.StatusCode != 409 {
		t.Errorf("claiming an owned node should be 409, got %d", resp.StatusCode)
	}

	// Owner list works, and release frees the node.
	if resp, _ := member.do("GET", "/api/claims/mine", nil, false); resp.StatusCode != 200 {
		t.Errorf("claims/mine should be 200, got %d", resp.StatusCode)
	}
	if resp, _ := member.do("DELETE", "/api/claims/"+node, nil, true); resp.StatusCode != 200 {
		t.Errorf("release should be 200, got %d", resp.StatusCode)
	}
	_, after := anon.do("GET", "/api/nodes/"+node+"/claim", nil, false)
	if after["owner"] != nil {
		t.Error("node should be unowned after release")
	}
}

func TestAccountDeleteReleasesNodes(t *testing.T) {
	st, base, cleanup := newAuthEnv(t)
	defer cleanup()

	// Seed the node.
	pkt, err := meshcore.DecodeHex(claimAdvertHex)
	if err != nil || pkt.Advert == nil {
		t.Fatalf("decode advert: %v", err)
	}
	node := pkt.Advert.PublicKey
	if err := st.Record(store.Observation{Packet: pkt, RawHex: claimAdvertHex, ReceivedAt: time.Now()}); err != nil {
		t.Fatalf("record node: %v", err)
	}

	// First account = protected owner; second is a normal member.
	owner := newClient(t, base)
	owner.do("POST", "/api/auth/register",
		map[string]string{"email": "owner@example.com", "password": "hunter2hunter2", "displayName": "Owner"}, false)
	member := newClient(t, base)
	member.do("POST", "/api/auth/register",
		map[string]string{"email": "member@example.com", "password": "hunter2hunter2", "displayName": "Member"}, false)

	// Give the member verified ownership of the node (bypass the advert dance).
	mu, _, _ := st.GetUserByEmail("member@example.com")
	if _, err := st.CreateVerifiedClaim(node, mu.ID); err != nil {
		t.Fatalf("verified claim: %v", err)
	}

	// Wrong password → 403, account untouched.
	if resp, _ := member.do("POST", "/api/account/delete", map[string]string{"password": "nope"}, true); resp.StatusCode != 403 {
		t.Fatalf("wrong password should be 403, got %d", resp.StatusCode)
	}
	if _, ok, _ := st.GetUserByEmail("member@example.com"); !ok {
		t.Fatal("member should still exist after a failed delete")
	}

	// The protected owner cannot delete their own account.
	if resp, _ := owner.do("POST", "/api/account/delete", map[string]string{"password": "hunter2hunter2"}, true); resp.StatusCode != 403 {
		t.Fatalf("owner self-delete should be 403, got %d", resp.StatusCode)
	}

	// Member deletes their account.
	if resp, _ := member.do("POST", "/api/account/delete", map[string]string{"password": "hunter2hunter2"}, true); resp.StatusCode != 200 {
		t.Fatalf("member delete should be 200, got %d", resp.StatusCode)
	}
	if _, ok, _ := st.GetUserByEmail("member@example.com"); ok {
		t.Error("member account should be gone")
	}
	// Node released and stamped with the former owner's name.
	if _, ok, _ := st.NodeOwner(node); ok {
		t.Error("node should have no current owner")
	}
	if prev, _ := st.NodePrevOwner(node); prev != "Member" {
		t.Errorf("prev owner = %q, want Member", prev)
	}
	// Session cleared — /me now returns a null user.
	if _, body := member.do("GET", "/api/auth/me", nil, false); body["user"] != nil {
		t.Errorf("deleted member's session should be void, got user %v", body["user"])
	}

	// The public claim status surfaces the previous owner to a fresh visitor.
	visitor := newClient(t, base)
	_, cs := visitor.do("GET", "/api/nodes/"+node+"/claim", nil, false)
	if cs["previousOwner"] != "Member" {
		t.Errorf("claim status previousOwner = %v, want Member", cs["previousOwner"])
	}
}
