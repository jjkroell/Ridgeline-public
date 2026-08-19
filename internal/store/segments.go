package store

// Network segments behind sanctioned bridges.
//
// A bridge joins two RF networks that this deployment cannot both listen to. If
// every observer sits on one side, the nodes on the other side are only ever
// heard after their traffic crosses the wire — so "reached only across bridge X"
// is the sole evidence that a node lives on the far segment, and the far side's
// radio settings cannot be observed at all.
//
// That last point is why BridgeLink carries an operator-declared Radio. A
// far-side node's nodes.radio is inherited from whichever observer heard it,
// which is a receiver on THIS side, so it describes the listener rather than the
// node. The API suppresses it for far-side nodes and reports the declared value
// instead.

import "strings"

// BridgeLink is a sanctioned bridge with both ends known: the near end (the
// relay detection named) and the peer the operator identified as the far side.
type BridgeLink struct {
	Near      string // uppercase pubkey, this side of the wire
	Far       string // uppercase pubkey, the far side
	NearName  string
	FarName   string
	PeerRadio string // operator-declared "freq,bw,sf,cr" of the far segment
}

// KnownBridgeLinks returns sanctioned bridges that have a peer recorded. A
// known bridge with no peer can't define a segment — a link needs two ends —
// so it is skipped rather than guessed at.
func (s *Store) KnownBridgeLinks() ([]BridgeLink, error) {
	rows, err := s.db.Query(`
		SELECT b.key, b.peer, COALESCE(b.name,''), COALESCE(nf.name,''), COALESCE(b.peer_radio,'')
		FROM blocklist b
		LEFT JOIN nodes nf ON UPPER(nf.pubkey) = b.peer
		WHERE b.kind = ? AND b.peer IS NOT NULL AND b.peer <> ''`, BlockKnown)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BridgeLink{}
	for rows.Next() {
		var l BridgeLink
		if err := rows.Scan(&l.Near, &l.Far, &l.NearName, &l.FarName, &l.PeerRadio); err != nil {
			return nil, err
		}
		l.Near, l.Far = strings.ToUpper(l.Near), strings.ToUpper(l.Far)
		out = append(out, l)
	}
	return out, rows.Err()
}

// SetBridgePeerRadio records the far segment's radio config on a sanctioned
// bridge. Empty clears it.
func (s *Store) SetBridgePeerRadio(key, radio string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`UPDATE blocklist SET peer_radio = ? WHERE kind = ? AND key = ?`,
		nullStr(strings.TrimSpace(radio)), BlockKnown, strings.ToUpper(key))
	return err
}

// SegmentMember is one node found to live beyond a bridge.
type SegmentMember struct {
	NodeKey    string // uppercase pubkey
	BridgeNear string // uppercase pubkey of the bridge it is reached through
	Confidence string // "confirmed" | "probable"
}

// ApplySegments replaces the whole far-side assignment in one transaction.
//
// Replace rather than merge: membership is a conclusion drawn from a rolling
// window, so a node that stops crossing must stop being labelled. Merging would
// make the label sticky and it would slowly drift out of agreement with the
// traffic — the same failure mode as a cache nobody invalidates.
func (s *Store) ApplySegments(members []SegmentMember) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`UPDATE nodes SET via_bridge = NULL, via_bridge_conf = NULL WHERE via_bridge IS NOT NULL`); err != nil {
		return 0, err
	}
	n := 0
	for _, m := range members {
		res, err := tx.Exec(`UPDATE nodes SET via_bridge = ?, via_bridge_conf = ? WHERE UPPER(pubkey) = ?`,
			strings.ToUpper(m.BridgeNear), m.Confidence, strings.ToUpper(m.NodeKey))
		if err != nil {
			return 0, err
		}
		if c, _ := res.RowsAffected(); c > 0 {
			n++
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return n, nil
}
