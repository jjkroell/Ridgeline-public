package store

import (
	"path/filepath"
	"testing"

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
	bridge := "3FDCCE5715F97C7701DF03C3D5DE0CC1EE08A637F24E944316791C628B728F67"
	foreign := "89AFE58A97AD541D915ED00890CAAAD09E09AD038F9B62BD3C651A0D754F0256"

	// Nothing blocked yet.
	if st.ShouldDrop(advertPkt(foreign, "3F", "16"), "obs-A") {
		t.Fatal("dropped with empty blocklist")
	}

	// Block the bridge -> any packet whose path transits its prefix is dropped.
	if err := st.AddBlock(BlockBridge, bridge, "Dager-Mesh-Repeater", "rf bridge"); err != nil {
		t.Fatal(err)
	}
	if !st.ShouldDrop(advertPkt(foreign, "3F", "16"), "obs-A") {
		t.Error("foreign packet via bridge prefix 3F not dropped")
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
	if st.ShouldDrop(advertPkt(foreign, "3F", "16"), "obs-A") {
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
