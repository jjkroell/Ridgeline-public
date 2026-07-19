package store

import (
	"strings"
	"time"
)

// LocationShare is one grant of read access to a node's private location. The
// display fields identify the grantee for the owner's management UI.
type LocationShare struct {
	NodePubkey    string `json:"nodePubkey"`
	GranteeUserID int64  `json:"granteeUserId"`
	DisplayName   string `json:"displayName"`
	Email         string `json:"email"`
	CreatedAt     string `json:"createdAt"`
}

// ShareLocation grants granteeUserID read access to nodePubkey's private
// location. Idempotent (re-granting just refreshes the timestamp). Ownership is
// verified by the caller (API layer) before this is invoked.
func (s *Store) ShareLocation(nodePubkey string, ownerUserID, granteeUserID int64) error {
	nodePubkey = strings.ToUpper(nodePubkey)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.Exec(`
		INSERT INTO location_shares (node_pubkey, owner_user_id, grantee_user_id, created_at, seen)
		VALUES (?,?,?,?,0)
		ON CONFLICT(node_pubkey, grantee_user_id) DO UPDATE SET
			owner_user_id = excluded.owner_user_id,
			created_at    = excluded.created_at,
			seen          = 0`,
		nodePubkey, ownerUserID, granteeUserID, now)
	return err
}

// SharedWithMe is a node whose private location has been shared with a user,
// with the node's display fields and who shared it — for the grantee's
// "Shared with me" list.
type SharedWithMe struct {
	NodePubkey   string `json:"nodePubkey"`
	NodeName     string `json:"nodeName"`
	NodeRole     string `json:"nodeRole"`
	SharedByID   int64  `json:"sharedById"`
	SharedByName string `json:"sharedByName"`
	CreatedAt    string `json:"createdAt"`
	Seen         bool   `json:"seen"`
	// NodePresent reports whether the shared node is currently in the mesh. Like
	// a claim, a share outlives its node row when the retention sweep prunes a
	// node that went silent, so callers render those as dormant rather than
	// linking to a node page that would 404. See ClaimWithNode.NodePresent.
	NodePresent bool `json:"nodePresent"`
}

// SharesForUser returns the nodes shared with granteeUserID (newest first),
// each annotated with the node's name/role and who shared it.
func (s *Store) SharesForUser(granteeUserID int64) ([]SharedWithMe, error) {
	rows, err := s.db.Query(`
		SELECT ls.node_pubkey, COALESCE(n.name,''), COALESCE(n.role,''),
		       ls.owner_user_id, COALESCE(NULLIF(o.display_name,''), o.email),
		       ls.created_at, ls.seen, n.pubkey IS NOT NULL
		FROM location_shares ls
		JOIN users o ON o.id = ls.owner_user_id
		LEFT JOIN nodes n ON n.pubkey = ls.node_pubkey
		WHERE ls.grantee_user_id = ?
		ORDER BY ls.created_at DESC`, granteeUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SharedWithMe
	for rows.Next() {
		var sh SharedWithMe
		var seen int
		if err := rows.Scan(&sh.NodePubkey, &sh.NodeName, &sh.NodeRole, &sh.SharedByID,
			&sh.SharedByName, &sh.CreatedAt, &seen, &sh.NodePresent); err != nil {
			return nil, err
		}
		sh.Seen = seen != 0
		out = append(out, sh)
	}
	return out, rows.Err()
}

// UnseenShareCount returns how many nodes have been shared with the user that
// they haven't seen yet (drives the account badge).
func (s *Store) UnseenShareCount(granteeUserID int64) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM location_shares WHERE grantee_user_id = ? AND seen = 0`,
		granteeUserID).Scan(&n)
	return n, err
}

// MarkSharesSeen clears the unseen flag on all of a user's shares (called when
// they view their Shared-with-me list).
func (s *Store) MarkSharesSeen(granteeUserID int64) error {
	_, err := s.db.Exec(`UPDATE location_shares SET seen = 1 WHERE grantee_user_id = ? AND seen = 0`,
		granteeUserID)
	return err
}

// UnshareLocation revokes a grantee's access. Returns whether a row was removed.
func (s *Store) UnshareLocation(nodePubkey string, granteeUserID int64) (bool, error) {
	nodePubkey = strings.ToUpper(nodePubkey)
	res, err := s.db.Exec(`DELETE FROM location_shares WHERE node_pubkey = ? AND grantee_user_id = ?`,
		nodePubkey, granteeUserID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// HasLocationShare reports whether userID has been granted read access to a
// node's private location.
func (s *Store) HasLocationShare(nodePubkey string, userID int64) (bool, error) {
	nodePubkey = strings.ToUpper(nodePubkey)
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM location_shares WHERE node_pubkey = ? AND grantee_user_id = ?`,
		nodePubkey, userID).Scan(&n)
	return n > 0, err
}

// ListLocationShares returns a node's grantees with their display info, newest
// grant first.
func (s *Store) ListLocationShares(nodePubkey string) ([]LocationShare, error) {
	nodePubkey = strings.ToUpper(nodePubkey)
	rows, err := s.db.Query(`
		SELECT ls.node_pubkey, ls.grantee_user_id,
		       COALESCE(NULLIF(u.display_name,''), u.email), u.email, ls.created_at
		FROM location_shares ls JOIN users u ON u.id = ls.grantee_user_id
		WHERE ls.node_pubkey = ?
		ORDER BY ls.created_at DESC`, nodePubkey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LocationShare
	for rows.Next() {
		var sh LocationShare
		if err := rows.Scan(&sh.NodePubkey, &sh.GranteeUserID, &sh.DisplayName, &sh.Email, &sh.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, sh)
	}
	return out, rows.Err()
}

// DeleteLocationShares drops all of a node's shares (used when the owner
// releases the node).
func (s *Store) DeleteLocationShares(nodePubkey string) error {
	nodePubkey = strings.ToUpper(nodePubkey)
	_, err := s.db.Exec(`DELETE FROM location_shares WHERE node_pubkey = ?`, nodePubkey)
	return err
}
