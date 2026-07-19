package store

import (
	"strings"
	"testing"
	"time"
)

const claimNode = "AABBCCDDEEFF00112233445566778899AABBCCDDEEFF00112233445566778899"

func TestClaimLifecycle(t *testing.T) {
	st := testStore(t)
	st.CreateUser("owner@example.com", "h", "Owner") // first = admin/owner
	u, _ := st.CreateUser("claimer@example.com", "h", "Claimer")

	c, err := st.CreateOrRefreshClaim(claimNode, u.ID, "K7X4QP", 30*time.Minute)
	if err != nil {
		t.Fatalf("create claim: %v", err)
	}
	if c.Status != "pending" || c.Code != "K7X4QP" {
		t.Fatalf("unexpected claim: %+v", c)
	}
	if !st.HasPendingClaim(claimNode) {
		t.Error("pending-claim cache should include the node")
	}

	// A name without the code does not verify.
	if v, _ := st.VerifyPendingClaims(claimNode, "Just A Repeater"); len(v) != 0 {
		t.Error("code-less name must not verify a claim")
	}
	if _, ok, _ := st.NodeOwner(claimNode); ok {
		t.Error("node should still be unowned")
	}

	// The code embedded in the name (case-insensitive) verifies the claim.
	v, err := st.VerifyPendingClaims(claimNode, "MyRepeater k7x4qp")
	if err != nil || len(v) != 1 {
		t.Fatalf("verify: n=%d err=%v", len(v), err)
	}
	owner, ok, _ := st.NodeOwner(claimNode)
	if !ok || owner.UserID != u.ID || owner.DisplayName != "Claimer" {
		t.Fatalf("owner not set correctly: %+v ok=%v", owner, ok)
	}
	if st.HasPendingClaim(claimNode) {
		t.Error("verified node should no longer be a pending claim")
	}

	// A second user cannot claim an owned node.
	u2, _ := st.CreateUser("other@example.com", "h", "")
	if _, err := st.CreateOrRefreshClaim(claimNode, u2.ID, "ZZZ999", 30*time.Minute); err != ErrNodeClaimed {
		t.Errorf("expected ErrNodeClaimed, got %v", err)
	}

	// Release ownership.
	if removed, _ := st.DeleteClaim(claimNode, u.ID); !removed {
		t.Error("release should remove the claim")
	}
	if _, ok, _ := st.NodeOwner(claimNode); ok {
		t.Error("node should be unowned after release")
	}
}

func TestClaimedNodeKeys(t *testing.T) {
	st := testStore(t)
	st.CreateUser("owner@example.com", "h", "Owner") // first = admin/owner
	u, _ := st.CreateUser("claimer@example.com", "h", "Claimer")

	// A pending (unverified) claim must NOT count as claimed.
	if _, err := st.CreateOrRefreshClaim(claimNode, u.ID, "K7X4QP", 30*time.Minute); err != nil {
		t.Fatalf("create claim: %v", err)
	}
	if keys, _ := st.ClaimedNodeKeys(); keys[claimNode] {
		t.Error("pending claim must not appear in ClaimedNodeKeys")
	}

	// Verifying it flips the node to claimed.
	if v, err := st.VerifyPendingClaims(claimNode, "MyRepeater k7x4qp"); err != nil || len(v) != 1 {
		t.Fatalf("verify: n=%d err=%v", len(v), err)
	}
	keys, err := st.ClaimedNodeKeys()
	if err != nil {
		t.Fatalf("ClaimedNodeKeys: %v", err)
	}
	if !keys[claimNode] {
		t.Errorf("verified node %s missing from ClaimedNodeKeys (keys=%v)", claimNode, keys)
	}
	if len(keys) != 1 {
		t.Errorf("expected 1 claimed node, got %d", len(keys))
	}
}

func TestNameHasVerificationCode(t *testing.T) {
	st := testStore(t)
	u, _ := st.CreateUser("u@example.com", "h", "U")
	code := "K7X4QP"
	now := time.Now().UTC().Format(time.RFC3339Nano)
	// Seed the node with a name that still carries the code (as it would after the
	// verifying advert was recorded).
	st.db.Exec(`INSERT INTO nodes (pubkey,name,role,has_location,first_seen,last_seen,advert_count,advert_tx_count,hash_size)
		VALUES (?,?, 'Repeater',0,?,?,1,1,3)`, claimNode, "Cranberry "+code, now, now)

	st.CreateOrRefreshClaim(claimNode, u.ID, code, 30*time.Minute)
	if _, err := st.VerifyPendingClaims(claimNode, "Cranberry "+code); err != nil {
		t.Fatalf("verify: %v", err)
	}

	// Name still contains the code → needs reset.
	if b, _ := st.NameHasVerificationCode(claimNode, u.ID); !b {
		t.Error("expected true while the advertised name still carries the code")
	}
	// Owner restores the real name (re-advert) → no longer needs reset.
	st.db.Exec(`UPDATE nodes SET name = ? WHERE pubkey = ?`, "Cranberry", claimNode)
	if b, _ := st.NameHasVerificationCode(claimNode, u.ID); b {
		t.Error("expected false once the name no longer contains the code")
	}
	// No verified claim → false.
	if b, _ := st.NameHasVerificationCode(claimNode, 9999); b {
		t.Error("expected false for a user with no claim")
	}
}

func TestClaimCodeUniqueAcrossOpenClaims(t *testing.T) {
	st := testStore(t)
	a, _ := st.CreateUser("a@example.com", "h", "A")
	b, _ := st.CreateUser("b@example.com", "h", "B")
	nodeB := "1111111111111111111111111111111111111111111111111111111111111111"

	if _, err := st.CreateOrRefreshClaim(claimNode, a.ID, "DUP123", 30*time.Minute); err != nil {
		t.Fatalf("first claim: %v", err)
	}
	// A different user claiming a different node cannot reuse the live code.
	if _, err := st.CreateOrRefreshClaim(nodeB, b.ID, "DUP123", 30*time.Minute); err != ErrCodeCollision {
		t.Fatalf("expected ErrCodeCollision for a reused live code, got %v", err)
	}
	// A distinct code works.
	if _, err := st.CreateOrRefreshClaim(nodeB, b.ID, "OTHER9", 30*time.Minute); err != nil {
		t.Fatalf("distinct code should succeed: %v", err)
	}
	// Once the first claim verifies, its (now spent) code no longer blocks a new
	// pending claim reusing that string (the unique index is pending-only).
	st.db.Exec(`INSERT OR IGNORE INTO nodes (pubkey,name,role,has_location,first_seen,last_seen,advert_count,advert_tx_count,hash_size)
		VALUES (?,?, 'Repeater',0,?,?,1,1,3)`, claimNode, "R DUP123",
		time.Now().UTC().Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano))
	st.VerifyPendingClaims(claimNode, "R DUP123")
	c, _ := st.CreateUser("c@example.com", "h", "C")
	nodeC := "2222222222222222222222222222222222222222222222222222222222222222"
	if _, err := st.CreateOrRefreshClaim(nodeC, c.ID, "DUP123", 30*time.Minute); err != nil {
		t.Errorf("a spent (verified) code should not block a new pending code: %v", err)
	}
}

func TestExpiredClaimDoesNotVerify(t *testing.T) {
	st := testStore(t)
	u, _ := st.CreateUser("u@example.com", "h", "")
	// Already-expired pending claim.
	if _, err := st.CreateOrRefreshClaim(claimNode, u.ID, "CODE22", -time.Minute); err != nil {
		t.Fatalf("create: %v", err)
	}
	if v, _ := st.VerifyPendingClaims(claimNode, "node CODE22 here"); len(v) != 0 {
		t.Error("an expired claim must not verify")
	}
	// Prune clears it.
	if n, _ := st.PruneExpiredClaims(); n != 1 {
		t.Errorf("expected to prune 1 expired claim, got %d", n)
	}
}

func TestClaimVerifyIgnoredWhenAlreadyOwned(t *testing.T) {
	st := testStore(t)
	a, _ := st.CreateUser("a@example.com", "h", "A")
	b, _ := st.CreateUser("b@example.com", "h", "B")

	st.CreateOrRefreshClaim(claimNode, a.ID, "AAAAAA", 30*time.Minute)
	st.VerifyPendingClaims(claimNode, "x AAAAAA") // a now owns it

	// b somehow has a stale pending row (created before a won) — force it in.
	st.db.Exec(`INSERT INTO node_claims (node_pubkey, user_id, code, status, created_at, expires_at)
		VALUES (?,?,?, 'pending', ?, ?)`, claimNode, b.ID, "BBBBBB",
		time.Now().UTC().Format(time.RFC3339Nano),
		time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano))
	if v, _ := st.VerifyPendingClaims(claimNode, "x BBBBBB"); len(v) != 0 {
		t.Error("no second owner should be verified once a node is owned")
	}
	owner, _, _ := st.NodeOwner(claimNode)
	if owner.UserID != a.ID {
		t.Errorf("owner should still be A, got user %d", owner.UserID)
	}
}

// TestListUserClaimsNodePresent covers the dormant-claim signal: a claim outlives
// its node row when the retention sweep prunes a node that went silent, and the
// UI needs to tell that apart from a node that is present but unnamed.
func TestListUserClaimsNodePresent(t *testing.T) {
	st := testStore(t)
	st.CreateUser("owner@example.com", "h", "Owner")
	u, _ := st.CreateUser("claimer@example.com", "h", "Claimer")

	present := "1111111111111111111111111111111111111111111111111111111111111111"
	absent := "2222222222222222222222222222222222222222222222222222222222222222"
	now := time.Now().UTC().Format(time.RFC3339Nano)
	// A present but UNNAMED node — must still report as present.
	st.db.Exec(`INSERT INTO nodes(pubkey,name,role,first_seen,last_seen,advert_count,advert_tx_count,hash_size)
		VALUES(?,'','',?,?,0,0,0)`, present, now, now)
	for _, k := range []string{present, absent} {
		if _, err := st.CreateVerifiedClaim(k, u.ID); err != nil {
			t.Fatalf("claim %s: %v", k[:4], err)
		}
	}

	got := map[string]bool{}
	cs, err := st.ListUserClaims(u.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, c := range cs {
		got[c.NodePubkey] = c.NodePresent
	}
	if !got[present] {
		t.Error("unnamed but present node must report nodePresent=true")
	}
	if got[absent] {
		t.Error("pruned node must report nodePresent=false")
	}
}

// TestPartitionClaimed guards the rule that keeps heuristic sweeps (artifact
// scrub, bridge purge) from deleting a node someone has proved they own.
func TestPartitionClaimed(t *testing.T) {
	st := testStore(t)
	st.CreateUser("owner@example.com", "h", "Owner")
	u, _ := st.CreateUser("claimer@example.com", "h", "Claimer")

	verified := "7777777777777777777777777777777777777777777777777777777777777777"
	pending := "8888888888888888888888888888888888888888888888888888888888888888"
	free := "9999999999999999999999999999999999999999999999999999999999999999"
	if _, err := st.CreateVerifiedClaim(verified, u.ID); err != nil {
		t.Fatalf("verified claim: %v", err)
	}
	if _, err := st.CreateOrRefreshClaim(pending, u.ID, "K7X4QP", 30*time.Minute); err != nil {
		t.Fatalf("pending claim: %v", err)
	}

	// Lowercase input must still match — callers pass keys straight from detectors.
	unclaimed, claimed, err := st.PartitionClaimed([]string{
		strings.ToLower(verified), pending, free,
	})
	if err != nil {
		t.Fatalf("partition: %v", err)
	}
	if len(unclaimed) != 1 || unclaimed[0] != free {
		t.Errorf("unclaimed = %v, want just the free key", unclaimed)
	}
	if len(claimed) != 2 {
		t.Errorf("claimed = %v, want both the verified and pending keys", claimed)
	}
}
