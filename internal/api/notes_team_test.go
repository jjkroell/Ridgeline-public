package api

import (
	"testing"
	"time"

	"github.com/jjkroell/ridgeline/internal/meshcore"
	"github.com/jjkroell/ridgeline/internal/store"
)

// TestTeamNotesAndUserSearch covers the "team" note tier (owner + shared-with
// users only) and the share-autocomplete user search endpoint.
func TestTeamNotesAndUserSearch(t *testing.T) {
	st, base, cleanup := newAuthEnv(t)
	defer cleanup()

	pkt, _ := meshcore.DecodeHex(claimAdvertHex)
	node := pkt.Advert.PublicKey
	if err := st.Record(store.Observation{Packet: pkt, RawHex: claimAdvertHex, ReceivedAt: time.Now()}); err != nil {
		t.Fatalf("record node: %v", err)
	}

	admin := newClient(t, base)
	admin.do("POST", "/api/auth/register",
		map[string]string{"email": "admin@example.com", "password": "hunter2hunter2"}, false)

	owner := newClient(t, base)
	_, ob := owner.do("POST", "/api/auth/register",
		map[string]string{"email": "owner@example.com", "password": "hunter2hunter2", "displayName": "OP-OWNER"}, false)
	ownerID := int64(owner.user(ob)["id"].(float64))

	friend := newClient(t, base)
	friend.do("POST", "/api/auth/register",
		map[string]string{"email": "friend@example.com", "password": "hunter2hunter2", "displayName": "OP-FRIEND"}, false)

	stranger := newClient(t, base)
	stranger.do("POST", "/api/auth/register",
		map[string]string{"email": "stranger@example.com", "password": "hunter2hunter2", "displayName": "OP-STRANGER"}, false)

	// Owner claims the node.
	admin.do("POST", "/api/admin/users/flags",
		map[string]any{"id": ownerID, "isAdmin": false, "canClaim": true}, true)
	owner.do("GET", "/api/auth/me", nil, false)
	_, cb := owner.do("POST", "/api/claims", map[string]string{"pubkey": node}, true)
	code := cb["code"].(string)
	if v, err := st.VerifyPendingClaims(node, "Repeater "+code); err != nil || len(v) != 1 {
		t.Fatalf("verify: %v", err)
	}

	// User search: the endpoint is auth-gated + returns 200. (The `do` helper only
	// decodes JSON objects, so the bare-array body is verified via the store.)
	if resp, _ := owner.do("GET", "/api/users/search?q=OP-FR", nil, false); resp.StatusCode != 200 {
		t.Errorf("user search should be 200, got %d", resp.StatusCode)
	}
	if got, _ := st.SearchUsersByName("OP-FR", ownerID, 8); len(got) != 1 || got[0].DisplayName != "OP-FRIEND" {
		t.Fatalf("search should find OP-FRIEND, got %v", got)
	}
	// A too-short query returns an empty list (200, not an error).
	if resp, _ := owner.do("GET", "/api/users/search?q=x", nil, false); resp.StatusCode != 200 {
		t.Errorf("short query should be 200, got %d", resp.StatusCode)
	}

	// Stranger (not in the circle) can't post a team note.
	if resp, _ := stranger.do("POST", "/api/nodes/"+node+"/notes",
		map[string]string{"body": "sneaky", "visibility": "team"}, true); resp.StatusCode != 403 {
		t.Errorf("stranger team note should be 403, got %d", resp.StatusCode)
	}
	// ...but can still post a public note.
	if resp, _ := stranger.do("POST", "/api/nodes/"+node+"/notes",
		map[string]string{"body": "hello all", "visibility": "public"}, true); resp.StatusCode != 200 {
		t.Errorf("stranger public note should be 200, got %d", resp.StatusCode)
	}

	// Owner posts a team note (in circle as owner).
	if resp, _ := owner.do("POST", "/api/nodes/"+node+"/notes",
		map[string]string{"body": "gate code 1234", "visibility": "team"}, true); resp.StatusCode != 200 {
		t.Fatalf("owner team note should be 200, got %d", resp.StatusCode)
	}

	// Friend can't see the team note or post one until shared with.
	_, fn := friend.do("GET", "/api/nodes/"+node+"/notes", nil, false)
	if fn["canTeam"] != false {
		t.Errorf("friend should not have canTeam before sharing, got %v", fn["canTeam"])
	}
	if resp, _ := friend.do("POST", "/api/nodes/"+node+"/notes",
		map[string]string{"body": "me too", "visibility": "team"}, true); resp.StatusCode != 403 {
		t.Errorf("un-shared friend team note should be 403, got %d", resp.StatusCode)
	}

	// Owner shares the location with the friend → friend joins the circle.
	owner.do("POST", "/api/nodes/"+node+"/location-shares",
		map[string]string{"email": "friend@example.com"}, true)

	_, fn2 := friend.do("GET", "/api/nodes/"+node+"/notes", nil, false)
	if fn2["canTeam"] != true {
		t.Errorf("shared friend should have canTeam, got %v", fn2["canTeam"])
	}
	notes, _ := fn2["notes"].([]any)
	sawTeam := false
	for _, n := range notes {
		if m, _ := n.(map[string]any); m["visibility"] == "team" {
			sawTeam = true
		}
	}
	if !sawTeam {
		t.Error("shared friend should see the owner's team note")
	}
	if resp, _ := friend.do("POST", "/api/nodes/"+node+"/notes",
		map[string]string{"body": "spare fuse in the box", "visibility": "team"}, true); resp.StatusCode != 200 {
		t.Errorf("shared friend team note should be 200, got %d", resp.StatusCode)
	}

	// Stranger still can't see any team note.
	_, sn := stranger.do("GET", "/api/nodes/"+node+"/notes", nil, false)
	snotes, _ := sn["notes"].([]any)
	for _, n := range snotes {
		if m, _ := n.(map[string]any); m["visibility"] == "team" {
			t.Error("stranger must not see team notes")
		}
	}
}
