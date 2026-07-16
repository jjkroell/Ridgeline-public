package store

import (
	"testing"
	"time"
)

// CreateVerifiedClaim records ownership directly (the private-key proof path):
// no code, no advert. It must enforce single-owner, be idempotent, and promote a
// prior pending claim in place.
func TestCreateVerifiedClaim(t *testing.T) {
	st := testStore(t)
	u1, _ := st.CreateUser("owner@example.com", "h", "Owner")
	u2, _ := st.CreateUser("other@example.com", "h", "Other")

	// Direct verification with no prior claim.
	c, err := st.CreateVerifiedClaim(claimNode, u1.ID)
	if err != nil {
		t.Fatalf("create verified: %v", err)
	}
	if c.Status != "verified" || c.Code != "" {
		t.Fatalf("expected verified claim with empty code, got %+v", c)
	}
	if owner, ok, _ := st.NodeOwner(claimNode); !ok || owner.UserID != u1.ID {
		t.Fatalf("owner not set to u1: %+v ok=%v", owner, ok)
	}
	// A key-verified claim never needs a name reset (no code in the name).
	if needs, _ := st.NameHasVerificationCode(claimNode, u1.ID); needs {
		t.Error("key-verified claim should not report NameNeedsReset")
	}

	// Idempotent for the same owner.
	if c2, err := st.CreateVerifiedClaim(claimNode, u1.ID); err != nil || c2.Status != "verified" {
		t.Fatalf("re-verify by owner should be a no-op verified claim: %+v err=%v", c2, err)
	}

	// A different user is blocked.
	if _, err := st.CreateVerifiedClaim(claimNode, u2.ID); err != ErrNodeClaimed {
		t.Errorf("expected ErrNodeClaimed for second user, got %v", err)
	}
}

// A pending advert-code claim should be promotable to verified via the key path.
func TestCreateVerifiedClaimPromotesPending(t *testing.T) {
	st := testStore(t)
	u, _ := st.CreateUser("u@example.com", "h", "U")

	if _, err := st.CreateOrRefreshClaim(claimNode, u.ID, "K7X4QP", 30*time.Minute); err != nil {
		t.Fatalf("open pending: %v", err)
	}
	if !st.HasPendingClaim(claimNode) {
		t.Fatal("node should be pending before key verification")
	}
	if _, err := st.CreateVerifiedClaim(claimNode, u.ID); err != nil {
		t.Fatalf("promote to verified: %v", err)
	}
	if st.HasPendingClaim(claimNode) {
		t.Error("node should no longer be pending after key verification")
	}
	if owner, ok, _ := st.NodeOwner(claimNode); !ok || owner.UserID != u.ID {
		t.Errorf("owner not set after promotion: %+v ok=%v", owner, ok)
	}
}
