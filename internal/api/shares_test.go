package api

import (
	"strconv"
	"testing"
	"time"

	"github.com/jjkroell/ridgeline/internal/meshcore"
	"github.com/jjkroell/ridgeline/internal/store"
)

func TestLocationSharingFlow(t *testing.T) {
	st, base, cleanup := newAuthEnv(t)
	defer cleanup()

	pkt, _ := meshcore.DecodeHex(claimAdvertHex)
	node := pkt.Advert.PublicKey
	if err := st.Record(store.Observation{Packet: pkt, RawHex: claimAdvertHex, ReceivedAt: time.Now()}); err != nil {
		t.Fatalf("record node: %v", err)
	}

	admin := newClient(t, base) // bootstrap admin
	admin.do("POST", "/api/auth/register",
		map[string]string{"email": "admin@example.com", "password": "hunter2hunter2"}, false)

	member := newClient(t, base)
	_, mb := member.do("POST", "/api/auth/register",
		map[string]string{"email": "member@example.com", "password": "hunter2hunter2", "displayName": "Member"}, false)
	memberID := int64(member.user(mb)["id"].(float64))

	friend := newClient(t, base)
	_, fb := friend.do("POST", "/api/auth/register",
		map[string]string{"email": "friend@example.com", "password": "hunter2hunter2", "displayName": "Friend"}, false)
	friendID := int64(friend.user(fb)["id"].(float64))

	stranger := newClient(t, base)
	stranger.do("POST", "/api/auth/register",
		map[string]string{"email": "stranger@example.com", "password": "hunter2hunter2"}, false)

	// Make member the verified owner and set a location.
	admin.do("POST", "/api/admin/users/flags",
		map[string]any{"id": memberID, "isAdmin": false, "canClaim": true}, true)
	member.do("GET", "/api/auth/me", nil, false)
	_, cb := member.do("POST", "/api/claims", map[string]string{"pubkey": node}, true)
	code := cb["code"].(string)
	if v, err := st.VerifyPendingClaims(node, "Repeater "+code); err != nil || len(v) != 1 {
		t.Fatalf("verify: %v", err)
	}
	member.do("PUT", "/api/nodes/"+node+"/private-location",
		map[string]any{"latitude": 49.1, "longitude": -123.9, "label": "site"}, true)

	// Friend can't see it before being shared with; non-owners can't manage sharing.
	if resp, _ := friend.do("GET", "/api/nodes/"+node+"/private-location", nil, false); resp.StatusCode != 403 {
		t.Errorf("un-shared friend GET should be 403, got %d", resp.StatusCode)
	}
	if resp, _ := friend.do("GET", "/api/nodes/"+node+"/location-shares", nil, false); resp.StatusCode != 403 {
		t.Errorf("non-owner shares list should be 403, got %d", resp.StatusCode)
	}

	// Owner shares with an unknown email → 404; with a real email → 200.
	if resp, _ := member.do("POST", "/api/nodes/"+node+"/location-shares",
		map[string]string{"email": "nobody@example.com"}, true); resp.StatusCode != 404 {
		t.Errorf("share to unknown email should be 404, got %d", resp.StatusCode)
	}
	if resp, _ := member.do("POST", "/api/nodes/"+node+"/location-shares",
		map[string]string{"email": "friend@example.com"}, true); resp.StatusCode != 200 {
		t.Fatalf("share to friend should be 200, got %d", resp.StatusCode)
	}
	if shares, _ := st.ListLocationShares(node); len(shares) != 1 || shares[0].GranteeUserID != friendID {
		t.Fatalf("store should record friend's share, got %v", shares)
	}

	// Friend can now read it, read-only (canEdit=false, sharedBy names the owner).
	_, fg := friend.do("GET", "/api/nodes/"+node+"/private-location", nil, false)
	if fg["set"] != true || fg["canEdit"] != false {
		t.Errorf("friend should read set:true canEdit:false, got %v", fg)
	}
	if sb, _ := fg["sharedBy"].(map[string]any); sb == nil || sb["displayName"] != "Member" {
		t.Errorf("sharedBy should name the owner, got %v", fg["sharedBy"])
	}

	// Friend still can't EDIT, re-share, or list; stranger sees nothing.
	if resp, _ := friend.do("PUT", "/api/nodes/"+node+"/private-location",
		map[string]any{"latitude": 1.0, "longitude": 2.0}, true); resp.StatusCode != 403 {
		t.Errorf("friend PUT should be 403, got %d", resp.StatusCode)
	}
	if resp, _ := friend.do("POST", "/api/nodes/"+node+"/location-shares",
		map[string]string{"email": "stranger@example.com"}, true); resp.StatusCode != 403 {
		t.Errorf("friend re-share should be 403, got %d", resp.StatusCode)
	}
	if resp, _ := stranger.do("GET", "/api/nodes/"+node+"/private-location", nil, false); resp.StatusCode != 403 {
		t.Errorf("stranger GET should be 403, got %d", resp.StatusCode)
	}

	// Owner can list (200) and revoke friend by id.
	if resp, _ := member.do("GET", "/api/nodes/"+node+"/location-shares", nil, false); resp.StatusCode != 200 {
		t.Errorf("owner shares list should be 200, got %d", resp.StatusCode)
	}
	if resp, _ := member.do("DELETE", "/api/nodes/"+node+"/location-shares/"+itoa(friendID), nil, true); resp.StatusCode != 200 {
		t.Errorf("revoke should be 200, got %d", resp.StatusCode)
	}
	if resp, _ := friend.do("GET", "/api/nodes/"+node+"/private-location", nil, false); resp.StatusCode != 403 {
		t.Errorf("revoked friend GET should be 403, got %d", resp.StatusCode)
	}

	// Re-share so the friend has a share for the discovery checks below.
	member.do("POST", "/api/nodes/"+node+"/location-shares", map[string]string{"email": "friend@example.com"}, true)

	// Discovery: the friend's /me now reports an unseen share (badge), and
	// /api/shares/mine returns 200. Marking seen clears the badge.
	_, fme := friend.do("GET", "/api/auth/me", nil, false)
	if int(fme["unseenShares"].(float64)) != 1 {
		t.Errorf("friend should have 1 unseen share, got %v", fme["unseenShares"])
	}
	if resp, _ := friend.do("GET", "/api/shares/mine", nil, false); resp.StatusCode != 200 {
		t.Errorf("shares/mine should be 200, got %d", resp.StatusCode)
	}
	if mine, _ := st.SharesForUser(memberID); len(mine) != 0 {
		t.Errorf("owner is not a grantee; expected 0, got %d", len(mine))
	}
	friend.do("POST", "/api/shares/mark-seen", nil, true)
	_, fme2 := friend.do("GET", "/api/auth/me", nil, false)
	if int(fme2["unseenShares"].(float64)) != 0 {
		t.Errorf("badge should clear after mark-seen, got %v", fme2["unseenShares"])
	}

	// Releasing ownership → shares (and location) are dropped.
	member.do("DELETE", "/api/claims/"+node, nil, true)
	if shares, _ := st.ListLocationShares(node); len(shares) != 0 {
		t.Errorf("shares should be dropped on ownership release, got %d", len(shares))
	}
}

func itoa(n int64) string {
	return strconv.FormatInt(n, 10)
}
