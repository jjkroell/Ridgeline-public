package store

import (
	"testing"
	"time"
)

func TestCreateUserBootstrapsFirstAdmin(t *testing.T) {
	st := testStore(t)

	u1, err := st.CreateUser("Owner@Example.com", "hash1", "Owner")
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	if u1.Email != "owner@example.com" {
		t.Errorf("email should be lowercased, got %q", u1.Email)
	}
	if !u1.IsAdmin || !u1.CanClaim || !u1.IsOwner {
		t.Error("first user should be bootstrapped as admin + can_claim + owner")
	}

	u2, err := st.CreateUser("second@example.com", "hash2", "")
	if err != nil {
		t.Fatalf("create second: %v", err)
	}
	if u2.IsAdmin || u2.IsOwner {
		t.Error("subsequent users must not be admin/owner by default")
	}
	// Claiming is universal — every account can claim from the start.
	if !u2.CanClaim {
		t.Error("every user should have can_claim")
	}
}

func TestBlockUserVoidsSessions(t *testing.T) {
	st := testStore(t)
	st.CreateUser("owner@example.com", "h", "") // first = owner
	u, _ := st.CreateUser("blocked@example.com", "h", "")
	st.CreateSession("hash-b", u.ID, "csrf", time.Hour)

	// Block + invalidate: the session must stop resolving.
	if err := st.SetUserBlocked(u.ID, true); err != nil {
		t.Fatalf("block: %v", err)
	}
	st.DeleteUserSessions(u.ID)
	if _, _, ok, _ := st.SessionUser("hash-b"); ok {
		t.Error("blocked user's session should not resolve")
	}
	// Even a fresh session must not resolve while blocked.
	st.CreateSession("hash-b2", u.ID, "csrf", time.Hour)
	if _, _, ok, _ := st.SessionUser("hash-b2"); ok {
		t.Error("a blocked account must not resolve any session")
	}
	// Unblock restores access.
	st.SetUserBlocked(u.ID, false)
	st.CreateSession("hash-b3", u.ID, "csrf", time.Hour)
	if _, _, ok, _ := st.SessionUser("hash-b3"); !ok {
		t.Error("unblocked account should resolve again")
	}
}

func TestDeleteUser(t *testing.T) {
	st := testStore(t)
	st.CreateUser("owner@example.com", "h", "")
	u, _ := st.CreateUser("gone@example.com", "h", "")
	st.CreateSession("hash-g", u.ID, "csrf", time.Hour)

	if err := st.DeleteUser(u.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, ok, _ := st.GetUserByID(u.ID); ok {
		t.Error("deleted user should not be found")
	}
	if _, _, ok, _ := st.SessionUser("hash-g"); ok {
		t.Error("deleted user's session should be gone")
	}
}

func TestCreateUserDuplicateEmail(t *testing.T) {
	st := testStore(t)
	if _, err := st.CreateUser("dup@example.com", "h", ""); err != nil {
		t.Fatalf("first: %v", err)
	}
	if _, err := st.CreateUser("DUP@example.com", "h2", ""); err != ErrEmailTaken {
		t.Errorf("expected ErrEmailTaken for case-insensitive dup, got %v", err)
	}
}

func TestGetUserByEmailAndID(t *testing.T) {
	st := testStore(t)
	created, _ := st.CreateUser("a@example.com", "h", "A")

	got, ok, err := st.GetUserByEmail("A@EXAMPLE.COM")
	if err != nil || !ok {
		t.Fatalf("by email: ok=%v err=%v", ok, err)
	}
	if got.ID != created.ID || got.PasswordHash != "h" {
		t.Error("by-email lookup returned wrong row")
	}
	if _, ok, _ := st.GetUserByEmail("nobody@example.com"); ok {
		t.Error("missing email should return ok=false")
	}
	if _, ok, _ := st.GetUserByID(created.ID); !ok {
		t.Error("by-id lookup failed")
	}
}

func TestSetUserAdmin(t *testing.T) {
	st := testStore(t)
	st.CreateUser("owner@example.com", "h", "") // first = admin
	u, _ := st.CreateUser("member@example.com", "h", "")
	if err := st.SetUserAdmin(u.ID, true); err != nil {
		t.Fatalf("grant admin: %v", err)
	}
	got, _, _ := st.GetUserByID(u.ID)
	if !got.IsAdmin || !got.CanClaim {
		t.Errorf("expected admin granted, claim retained: admin=%v canClaim=%v", got.IsAdmin, got.CanClaim)
	}
	if err := st.SetUserAdmin(u.ID, false); err != nil {
		t.Fatalf("revoke admin: %v", err)
	}
	if got, _, _ := st.GetUserByID(u.ID); got.IsAdmin {
		t.Error("admin should be revoked")
	}
}

func TestSessionLifecycle(t *testing.T) {
	st := testStore(t)
	u, _ := st.CreateUser("s@example.com", "h", "")

	if err := st.CreateSession("hash-abc", u.ID, "csrf-1", time.Hour); err != nil {
		t.Fatalf("create session: %v", err)
	}
	sess, gotUser, ok, err := st.SessionUser("hash-abc")
	if err != nil || !ok {
		t.Fatalf("resolve session: ok=%v err=%v", ok, err)
	}
	if gotUser.ID != u.ID || sess.CSRF != "csrf-1" {
		t.Error("session resolved to wrong user/csrf")
	}

	if err := st.DeleteSession("hash-abc"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, _, ok, _ := st.SessionUser("hash-abc"); ok {
		t.Error("deleted session should not resolve")
	}
}

func TestSessionExpiryAndPrune(t *testing.T) {
	st := testStore(t)
	u, _ := st.CreateUser("e@example.com", "h", "")

	// Already-expired session: resolves as not-ok and is dropped on use.
	if err := st.CreateSession("hash-exp", u.ID, "csrf", -time.Minute); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, _, ok, _ := st.SessionUser("hash-exp"); ok {
		t.Error("expired session must not resolve")
	}

	// Prune removes expired rows below the cutoff.
	st.CreateSession("hash-old", u.ID, "csrf", -time.Hour)
	st.CreateSession("hash-live", u.ID, "csrf", time.Hour)
	n, err := st.PruneSessions(time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if n < 1 {
		t.Errorf("expected to prune at least the expired row, pruned %d", n)
	}
	if _, _, ok, _ := st.SessionUser("hash-live"); !ok {
		t.Error("live session should survive prune")
	}
}

func TestDeleteUserAndReleaseNodes(t *testing.T) {
	st := testStore(t)
	st.CreateUser("owner@example.com", "h", "Owner") // first = protected owner
	u, _ := st.CreateUser("claimer@example.com", "h", "Claimer")
	now := time.Now().UTC().Format(time.RFC3339Nano)

	const nodeA = "AA11223344556677889900AABBCCDDEEFF00112233445566778899AABBCCDDEE"
	const nodeB = "BB11223344556677889900AABBCCDDEEFF00112233445566778899AABBCCDDEE"
	for _, pk := range []string{nodeA, nodeB} {
		st.db.Exec(`INSERT INTO nodes (pubkey,name,role,has_location,first_seen,last_seen,advert_count,advert_tx_count,hash_size)
			VALUES (?,?, 'Repeater',0,?,?,1,1,3)`, pk, "Node", now, now)
	}

	// The user owns nodeA (verified) and has a pending claim on nodeB.
	if _, err := st.CreateVerifiedClaim(nodeA, u.ID); err != nil {
		t.Fatalf("verify nodeA: %v", err)
	}
	if _, err := st.CreateOrRefreshClaim(nodeB, u.ID, "PEND01", 30*time.Minute); err != nil {
		t.Fatalf("pending nodeB: %v", err)
	}
	if !st.HasPendingClaim(nodeB) {
		t.Fatal("nodeB should have a pending claim before deletion")
	}

	if err := st.DeleteUserAndReleaseNodes(u.ID, "Claimer"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	// Account gone.
	if _, ok, _ := st.GetUserByID(u.ID); ok {
		t.Error("user should be deleted")
	}
	// nodeA released (no owner) but stamped with the previous owner.
	if _, ok, _ := st.NodeOwner(nodeA); ok {
		t.Error("nodeA should have no current owner")
	}
	if prev, _ := st.NodePrevOwner(nodeA); prev != "Claimer" {
		t.Errorf("nodeA prev owner = %q, want Claimer", prev)
	}
	// The pending claim on nodeB is gone (cascade) and not stamped (never owned).
	if st.HasPendingClaim(nodeB) {
		t.Error("nodeB pending claim should be cleared after deletion")
	}
	if prev, _ := st.NodePrevOwner(nodeB); prev != "" {
		t.Errorf("nodeB prev owner = %q, want empty (never owned)", prev)
	}

	// A new owner claiming nodeA clears the previous-owner marker.
	u2, _ := st.CreateUser("next@example.com", "h", "Next")
	if _, err := st.CreateVerifiedClaim(nodeA, u2.ID); err != nil {
		t.Fatalf("reclaim nodeA: %v", err)
	}
	if prev, _ := st.NodePrevOwner(nodeA); prev != "" {
		t.Errorf("nodeA prev owner = %q after reclaim, want empty", prev)
	}
}
