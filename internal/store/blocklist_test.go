package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/jjkroell/ridgeline/internal/meshcore"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func advertPkt(pubkey string, path ...string) *meshcore.Packet {
	return &meshcore.Packet{
		MessageHash: "deadbeef",
		Path:        path,
		Advert:      &meshcore.Advert{PublicKey: pubkey},
	}
}

func TestBlocklistShouldDrop(t *testing.T) {
	st := testStore(t)
	bridge := "AB11223344556677889900AABBCCDDEEFF00112233445566778899AABBCCDDEE"
	foreign := "CD99887766554433221100FFEEDDCCBBAA99887766554433221100FFEEDDCCBB"

	// Nothing blocked yet.
	if st.ShouldDrop(advertPkt(foreign, "AB", "16"), "obs-A") {
		t.Fatal("dropped with empty blocklist")
	}

	// Block the bridge -> any packet whose path transits its prefix is dropped.
	if err := st.AddBlock(BlockBridge, bridge, "Bridge Repeater", "rf bridge"); err != nil {
		t.Fatal(err)
	}
	if !st.ShouldDrop(advertPkt(foreign, "AB", "16"), "obs-A") {
		t.Error("foreign packet via bridge prefix AB not dropped")
	}
	if !st.ShouldDrop(advertPkt(bridge), "obs-A") { // bridge's own advert
		t.Error("bridge's own advert not dropped")
	}
	if st.ShouldDrop(advertPkt(foreign, "16", "1C"), "obs-A") {
		t.Error("local-only path wrongly dropped (no bridge hop)")
	}
	if !st.IsNodeBlocked(bridge) {
		t.Error("IsNodeBlocked false for blocked bridge")
	}

	// Block a rogue observer -> anything it publishes drops.
	if err := st.AddBlock(BlockObserver, "rogue-obs", "Rogue", "mqtt inject"); err != nil {
		t.Fatal(err)
	}
	if !st.ShouldDrop(advertPkt(foreign, "16"), "rogue-obs") {
		t.Error("rogue observer's packet not dropped")
	}

	// Unblock the bridge -> its traffic flows again (observer still blocked).
	if err := st.RemoveBlock(BlockBridge, bridge); err != nil {
		t.Fatal(err)
	}
	if st.ShouldDrop(advertPkt(foreign, "AB", "16"), "obs-A") {
		t.Error("still dropping after bridge unblocked")
	}
	if st.IsNodeBlocked(bridge) {
		t.Error("IsNodeBlocked true after unblock")
	}

	list, err := st.ListBlocks()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Kind != BlockObserver {
		t.Errorf("blocklist = %+v, want one observer entry", list)
	}
}

func TestAllowlistDoesNotBlock(t *testing.T) {
	st := testStore(t)
	pk := "A1B2C3D4E5F60718293A4B5C6D7E8F90A1B2C3D4E5F60718293A4B5C6D7E8F90"
	if err := st.AddBlock(BlockAllow, pk, "Legit Hub", "dismissed"); err != nil {
		t.Fatal(err)
	}
	if !st.IsAllowed(pk) {
		t.Error("IsAllowed false after allow")
	}
	if st.IsNodeBlocked(pk) {
		t.Error("allow must not block the node")
	}
	if st.ShouldDrop(advertPkt(pk), "o") {
		t.Error("allow must not drop the node's traffic")
	}
	if err := st.RemoveBlock(BlockAllow, "a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f90"); err != nil {
		t.Fatal(err)
	}
	if st.IsAllowed(pk) {
		t.Error("still allowed after release (case-insensitive remove)")
	}
}

func TestBlocklistCaseInsensitiveNodeKey(t *testing.T) {
	st := testStore(t)
	pk := "AbCdEf0123456789AbCdEf0123456789AbCdEf0123456789AbCdEf0123456789"
	if err := st.AddBlock(BlockNode, pk, "n", "r"); err != nil {
		t.Fatal(err)
	}
	if !st.IsNodeBlocked("abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789") {
		t.Error("node block not case-insensitive")
	}
	if !st.ShouldDrop(advertPkt("ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789"), "o") {
		t.Error("advert from blocked node (upper) not dropped")
	}
}

// TestScrubNodesCascadesNodeData covers the orphan bug: an admin scrub used to
// delete only observations and the nodes row, leaving the user-authored data
// keyed to it behind. A stale verified claim kept rendering in "Claimed Nodes"
// and on badges pointing at a node that no longer existed, and — since the
// verified-owner index is unique per node — blocked any future re-claim.
func TestScrubNodesCascadesNodeData(t *testing.T) {
	st := testStore(t)
	node := "AABBCCDDEEFF00112233445566778899AABBCCDDEEFF00112233445566778899"

	st.CreateUser("owner@example.com", "h", "Owner") // first = admin/owner
	u, _ := st.CreateUser("claimer@example.com", "h", "Claimer")
	g, _ := st.CreateUser("grantee@example.com", "h", "Grantee")

	if _, err := st.CreateVerifiedClaim(node, u.ID); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if _, err := st.CreateNote(node, u.ID, "public", "rooftop repeater"); err != nil {
		t.Fatalf("note: %v", err)
	}
	if _, err := st.SetPrivateLocation(node, u.ID, 49.25, -123.65, "rooftop"); err != nil {
		t.Fatalf("location: %v", err)
	}
	if err := st.ShareLocation(node, u.ID, g.ID); err != nil {
		t.Fatalf("share: %v", err)
	}

	res, err := st.ScrubNodes(nil, nil, []string{node})
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if res.Claims != 1 || res.Notes != 1 || res.Locations != 1 || res.Shares != 1 {
		t.Fatalf("cascade counts: %+v", res)
	}

	// Nothing keyed to the node survives.
	if _, ok, _ := st.NodeOwner(node); ok {
		t.Error("verified claim survived the purge")
	}
	if claimed, _ := st.ClaimedNodeKeys(); len(claimed) != 0 {
		t.Errorf("node still reports as claimed: %v", claimed)
	}
	if cs, _ := st.ListUserClaims(u.ID); len(cs) != 0 {
		t.Errorf("orphan claim still listed for user: %+v", cs)
	}
	if _, ok, _ := st.GetPrivateLocation(node); ok {
		t.Error("private location survived the purge")
	}
	if sh, _ := st.SharesForUser(g.ID); len(sh) != 0 {
		t.Error("location share survived the purge")
	}

	// The node is re-claimable, which the unique verified-owner index would
	// have prevented while the ghost claim existed.
	if _, err := st.CreateVerifiedClaim(node, g.ID); err != nil {
		t.Fatalf("re-claim after purge: %v", err)
	}
}

// TestPurgeTargetsPreservesUserData is the counterpart to the scrub test: the
// automatic retention sweep uses PurgeTargets, and a node pruned for going
// silent is expected to return on its next advert. Its owner must keep the
// claim, notes and private location across that gap — cascading here would
// silently destroy an operator's data every time their repeater went quiet for
// the retention window.
func TestPurgeTargetsPreservesUserData(t *testing.T) {
	st := testStore(t)
	node := "BBCCDDEEFF00112233445566778899AABBCCDDEEFF00112233445566778899AA"

	st.CreateUser("owner@example.com", "h", "Owner")
	u, _ := st.CreateUser("claimer@example.com", "h", "Claimer")
	g, _ := st.CreateUser("grantee@example.com", "h", "Grantee")

	if _, err := st.CreateVerifiedClaim(node, u.ID); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if _, err := st.CreateNote(node, u.ID, "public", "rooftop repeater"); err != nil {
		t.Fatalf("note: %v", err)
	}
	if _, err := st.SetPrivateLocation(node, u.ID, 49.25, -123.65, "rooftop"); err != nil {
		t.Fatalf("location: %v", err)
	}
	if err := st.ShareLocation(node, u.ID, g.ID); err != nil {
		t.Fatalf("share: %v", err)
	}

	res, err := st.PurgeTargets(nil, nil, []string{node})
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if res.Claims != 0 || res.Notes != 0 || res.Locations != 0 || res.Shares != 0 {
		t.Fatalf("retention purge must not cascade user data: %+v", res)
	}

	// Ownership and the owner's data survive the node row going away.
	if owner, ok, _ := st.NodeOwner(node); !ok || owner.UserID != u.ID {
		t.Error("verified claim must survive a retention purge")
	}
	if _, ok, _ := st.GetPrivateLocation(node); !ok {
		t.Error("private location must survive a retention purge")
	}
	if sh, _ := st.SharesForUser(g.ID); len(sh) != 1 {
		t.Error("location share must survive a retention purge")
	}
}

// TestScrubNodesRefreshesPendingClaimCache guards the cache half of the cascade:
// node_claims rows are deleted with raw SQL inside the purge transaction, so the
// in-memory pending set that gates the ingest advert verifier has to be reloaded
// afterwards. Without it HasPendingClaim keeps reporting a scrubbed node as
// pending until the daemon restarts.
func TestScrubNodesRefreshesPendingClaimCache(t *testing.T) {
	st := testStore(t)
	node := "CCDDEEFF00112233445566778899AABBCCDDEEFF00112233445566778899AABB"

	st.CreateUser("owner@example.com", "h", "Owner")
	u, _ := st.CreateUser("claimer@example.com", "h", "Claimer")
	if _, err := st.CreateOrRefreshClaim(node, u.ID, "K7X4QP", 30*time.Minute); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if !st.HasPendingClaim(node) {
		t.Fatal("precondition: node should start with a pending claim")
	}

	if _, err := st.ScrubNodes(nil, nil, []string{node}); err != nil {
		t.Fatalf("scrub: %v", err)
	}
	if st.HasPendingClaim(node) {
		t.Error("scrubbed node still reported as having a pending claim")
	}
}

// TestKnownBridgeIsNotBlocked covers the sanctioned-bridge registry. Marking a
// bridge known asserts that it IS a bridge and is wanted — the opposite of
// "allow", which asserts a candidate is not one. Neither may affect ingest: a
// known bridge's traffic is exactly the traffic the operator wants to keep.
func TestKnownBridgeIsNotBlocked(t *testing.T) {
	st := testStore(t)
	bridge := "0485454C383E00112233445566778899AABBCCDDEEFF00112233445566778899"

	if err := st.AddBlock(BlockKnown, bridge, "Bridge (far half)", "operator's own bridge"); err != nil {
		t.Fatalf("mark known: %v", err)
	}
	if st.ShouldDrop(advertPkt(bridge), "obs-a") {
		t.Error("a known bridge's own adverts must not be dropped at ingest")
	}
	if st.ShouldDrop(advertPkt("AABBCC00112233445566778899AABBCCDDEEFF00112233445566778899AABBCC", "0485"), "obs-a") {
		t.Error("traffic relayed by a known bridge must not be dropped at ingest")
	}
	if st.IsNodeBlocked(bridge) {
		t.Error("a known bridge must not be hidden from the API")
	}
	if k := st.KnownBridges(); !k[bridge] {
		t.Errorf("KnownBridges did not report the marked bridge: %v", k)
	}
	// Unmarking removes it from the registry without touching anything else.
	if err := st.RemoveBlock(BlockKnown, bridge); err != nil {
		t.Fatalf("unmark: %v", err)
	}
	if k := st.KnownBridges(); k[bridge] {
		t.Error("KnownBridges still reports the bridge after unmarking")
	}
}
