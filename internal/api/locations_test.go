package api

import (
	"testing"
	"time"

	"github.com/jjkroell/ridgeline/internal/meshcore"
	"github.com/jjkroell/ridgeline/internal/store"
)

func TestPrivateLocationFlow(t *testing.T) {
	st, base, cleanup := newAuthEnv(t)
	defer cleanup()

	pkt, err := meshcore.DecodeHex(claimAdvertHex)
	if err != nil || pkt.Advert == nil {
		t.Fatalf("decode advert: %v", err)
	}
	node := pkt.Advert.PublicKey
	if err := st.Record(store.Observation{Packet: pkt, RawHex: claimAdvertHex, ReceivedAt: time.Now()}); err != nil {
		t.Fatalf("record node: %v", err)
	}

	owner := newClient(t, base) // bootstrap admin
	owner.do("POST", "/api/auth/register",
		map[string]string{"email": "owner@example.com", "password": "hunter2hunter2"}, false)

	member := newClient(t, base)
	_, mb := member.do("POST", "/api/auth/register",
		map[string]string{"email": "member@example.com", "password": "hunter2hunter2", "displayName": "Member"}, false)
	memberID := int64(member.user(mb)["id"].(float64))

	// A non-owner (even logged in) cannot read the private location.
	if resp, _ := member.do("GET", "/api/nodes/"+node+"/private-location", nil, false); resp.StatusCode != 403 {
		t.Errorf("non-owner GET should be 403, got %d", resp.StatusCode)
	}

	// Make the member the verified owner.
	owner.do("POST", "/api/admin/users/flags",
		map[string]any{"id": memberID, "isAdmin": false, "canClaim": true}, true)
	member.do("GET", "/api/auth/me", nil, false)
	_, cb := member.do("POST", "/api/claims", map[string]string{"pubkey": node}, true)
	code := cb["code"].(string)
	if v, err := st.VerifyPendingClaims(node, "MyRepeater "+code); err != nil || len(v) != 1 {
		t.Fatalf("verify claim: n=%d err=%v", len(v), err)
	}

	// Owner reads before setting → set:false.
	_, g0 := member.do("GET", "/api/nodes/"+node+"/private-location", nil, false)
	if g0["set"] != false {
		t.Errorf("expected set:false before any location, got %v", g0)
	}

	// Invalid coordinates rejected.
	if resp, _ := member.do("PUT", "/api/nodes/"+node+"/private-location",
		map[string]any{"latitude": 200.0, "longitude": 0.0}, true); resp.StatusCode != 400 {
		t.Errorf("out-of-range latitude should be 400, got %d", resp.StatusCode)
	}

	// Set a valid location.
	resp, sb := member.do("PUT", "/api/nodes/"+node+"/private-location",
		map[string]any{"latitude": 49.1659, "longitude": -123.9401, "label": "rooftop"}, true)
	if resp.StatusCode != 200 || sb["set"] != true {
		t.Fatalf("set location: %d %v", resp.StatusCode, sb)
	}
	loc := sb["location"].(map[string]any)
	if loc["latitude"].(float64) != 49.1659 || loc["label"] != "rooftop" {
		t.Errorf("stored location mismatch: %v", loc)
	}

	// The private location must NOT leak into the public node detail.
	_, nd := member.do("GET", "/api/nodes/"+node, nil, false)
	ndNode, _ := nd["node"].(map[string]any)
	if ndNode != nil {
		if lat, ok := ndNode["latitude"].(float64); ok && lat == 49.1659 {
			t.Error("private latitude leaked into public node detail")
		}
	}

	// The bootstrap admin (not the owner of THIS node) is forbidden from reading it.
	if resp, _ := owner.do("GET", "/api/nodes/"+node+"/private-location", nil, false); resp.StatusCode != 403 {
		t.Errorf("non-owner admin GET should be 403, got %d", resp.StatusCode)
	}

	// Owner reads it back.
	_, g1 := member.do("GET", "/api/nodes/"+node+"/private-location", nil, false)
	if g1["set"] != true {
		t.Fatalf("owner should read set:true, got %v", g1)
	}

	// Releasing ownership drops the private location. Re-claim + re-verify and it
	// should be gone.
	member.do("DELETE", "/api/claims/"+node, nil, true)
	member.do("POST", "/api/claims", map[string]string{"pubkey": node}, true)
	_, cb2 := member.do("POST", "/api/claims", map[string]string{"pubkey": node}, true)
	code2 := cb2["code"].(string)
	if v, err := st.VerifyPendingClaims(node, "MyRepeater "+code2); err != nil || len(v) != 1 {
		t.Fatalf("re-verify claim: n=%d err=%v", len(v), err)
	}
	_, g2 := member.do("GET", "/api/nodes/"+node+"/private-location", nil, false)
	if g2["set"] != false {
		t.Error("private location should be dropped after releasing ownership")
	}

	// Set again, then explicit delete.
	member.do("PUT", "/api/nodes/"+node+"/private-location",
		map[string]any{"latitude": 48.5, "longitude": -123.0}, true)
	if resp, _ := member.do("DELETE", "/api/nodes/"+node+"/private-location", nil, true); resp.StatusCode != 200 {
		t.Errorf("delete should be 200, got %d", resp.StatusCode)
	}
	_, g3 := member.do("GET", "/api/nodes/"+node+"/private-location", nil, false)
	if g3["set"] != false {
		t.Error("location should be gone after delete")
	}
}
