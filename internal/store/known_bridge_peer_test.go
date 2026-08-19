package store

import "testing"

// A sanctioned bridge records the far side of the link it carries, so the
// console can show "this node -> that node" instead of naming only one end.
func TestKnownBridgeRecordsPeer(t *testing.T) {
	st := testStore(t)
	const near = "AA11BB22CC33DD44"
	const far = "EE55FF66AA77BB88"

	if err := st.AddBlockPeer(BlockKnown, near, "Near End", "operator's own bridge", far); err != nil {
		t.Fatal(err)
	}

	e := blockByKey(t, st, BlockKnown, near)
	if e.Peer != far {
		t.Errorf("peer = %q, want %q", e.Peer, far)
	}
	// Marking a bridge must not block anything.
	if st.IsNodeBlocked(near) || st.IsNodeBlocked(far) {
		t.Error("sanctioning a bridge blocked a node")
	}
}

// The peer's NAME is resolved at read time, so renaming the far-end node
// updates the link instead of leaving the label it had when it was recorded.
func TestKnownBridgePeerNameFollowsRename(t *testing.T) {
	st := testStore(t)
	const near = "AA11BB22CC33DD44"
	const far = "EE55FF66AA77BB88"

	if _, err := st.db.Exec(
		`INSERT INTO nodes (pubkey,name,role,has_location,first_seen,last_seen) VALUES (?,?,?,0,?,?)`,
		far, "Far End", "Repeater", "2026-08-01T00:00:00Z", "2026-08-01T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	if err := st.AddBlockPeer(BlockKnown, near, "Near End", "known bridge", far); err != nil {
		t.Fatal(err)
	}
	if got := blockByKey(t, st, BlockKnown, near).PeerName; got != "Far End" {
		t.Fatalf("peerName = %q, want %q", got, "Far End")
	}

	if _, err := st.db.Exec(`UPDATE nodes SET name = ? WHERE pubkey = ?`, "Far End (moved)", far); err != nil {
		t.Fatal(err)
	}
	if got := blockByKey(t, st, BlockKnown, near).PeerName; got != "Far End (moved)" {
		t.Errorf("peerName after rename = %q, want the new name", got)
	}
}

// Re-marking a bridge without naming a peer must not silently forget which link
// it is; forgetting is its own explicit call.
func TestKnownBridgePeerSurvivesReMarkAndClears(t *testing.T) {
	st := testStore(t)
	const near = "AA11BB22CC33DD44"
	const far = "EE55FF66AA77BB88"

	if err := st.AddBlockPeer(BlockKnown, near, "Near End", "known bridge", far); err != nil {
		t.Fatal(err)
	}
	// Re-mark with no peer (what the plain AddBlock path sends).
	if err := st.AddBlock(BlockKnown, near, "Near End", "known bridge"); err != nil {
		t.Fatal(err)
	}
	if got := blockByKey(t, st, BlockKnown, near).Peer; got != far {
		t.Errorf("peer lost on re-mark: %q", got)
	}

	if err := st.ClearBlockPeer(BlockKnown, near); err != nil {
		t.Fatal(err)
	}
	if got := blockByKey(t, st, BlockKnown, near).Peer; got != "" {
		t.Errorf("peer = %q after clear, want empty", got)
	}
}

// A bridge cannot be its own far side — that would render "X -> X".
func TestKnownBridgeRejectsSelfPeer(t *testing.T) {
	st := testStore(t)
	const near = "AA11BB22CC33DD44"

	if err := st.AddBlockPeer(BlockKnown, near, "Near End", "known bridge", near); err != nil {
		t.Fatal(err)
	}
	if got := blockByKey(t, st, BlockKnown, near).Peer; got != "" {
		t.Errorf("peer = %q, want empty (a bridge is not its own peer)", got)
	}
}

// Pubkeys are stored uppercase everywhere else; the peer must match so the
// nodes join resolves regardless of what case the caller sent.
func TestKnownBridgePeerIsUppercased(t *testing.T) {
	st := testStore(t)
	if err := st.AddBlockPeer(BlockKnown, "aa11bb22", "Near", "known bridge", "ee55ff66"); err != nil {
		t.Fatal(err)
	}
	e := blockByKey(t, st, BlockKnown, "AA11BB22")
	if e.Peer != "EE55FF66" {
		t.Errorf("peer = %q, want uppercase", e.Peer)
	}
}

func blockByKey(t *testing.T, st *Store, kind, key string) BlockEntry {
	t.Helper()
	blocks, err := st.ListBlocks()
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range blocks {
		if b.Kind == kind && b.Key == key {
			return b
		}
	}
	t.Fatalf("block %s/%s not found in %+v", kind, key, blocks)
	return BlockEntry{}
}
