package store

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

// ErrNodeClaimed is returned when a node already has a different verified owner.
var ErrNodeClaimed = errors.New("store: node already claimed by another user")

// ErrCodeCollision is returned when the supplied verification code is already in
// use by another open (pending) claim. The caller should generate a new code and
// retry — with an ~887M code space and few concurrent claims this is essentially
// never hit, but it guarantees every live code is unique.
var ErrCodeCollision = errors.New("store: verification code already in use")

// Claim is a user's claim on a node. Code is only exposed to the claim's own
// owner (never serialised for other users) — the API decides.
type Claim struct {
	ID         int64  `json:"id"`
	NodePubkey string `json:"nodePubkey"`
	UserID     int64  `json:"userId"`
	Code       string `json:"code,omitempty"`
	Status     string `json:"status"` // pending | verified
	CreatedAt  string `json:"createdAt"`
	ExpiresAt  string `json:"expiresAt,omitempty"`
	VerifiedAt string `json:"verifiedAt,omitempty"`
}

// ClaimWithNode augments a claim with its node's display fields (for "my nodes").
type ClaimWithNode struct {
	Claim
	NodeName string `json:"nodeName"`
	NodeRole string `json:"nodeRole"`
	// NodePresent reports whether the claimed node is currently in the mesh. A
	// claim outlives its node row: the retention sweep prunes nodes that go
	// silent past the threshold, and the owner keeps the claim so ownership
	// survives a repeater being down. Callers render these as dormant rather
	// than linking to a node page that would 404. Distinguishing this from an
	// unnamed node is why it isn't inferred from an empty NodeName.
	NodePresent bool `json:"nodePresent"`
}

// OwnerInfo identifies the verified owner of a node (public-facing).
type OwnerInfo struct {
	UserID      int64  `json:"userId"`
	DisplayName string `json:"displayName"`
}

func (s *Store) loadPendingClaims() error {
	rows, err := s.db.Query(`SELECT node_pubkey FROM node_claims WHERE status = 'pending'`)
	if err != nil {
		return err
	}
	defer rows.Close()
	set := make(map[string]bool)
	for rows.Next() {
		var pk string
		if err := rows.Scan(&pk); err != nil {
			return err
		}
		set[strings.ToUpper(pk)] = true
	}
	s.claimMu.Lock()
	s.pendingClaimNodes = set
	s.claimMu.Unlock()
	return rows.Err()
}

// HasPendingClaim reports whether a node has an open pending claim (hot-path
// gate for the ingest advert verifier).
func (s *Store) HasPendingClaim(pubkey string) bool {
	s.claimMu.RLock()
	defer s.claimMu.RUnlock()
	return s.pendingClaimNodes[strings.ToUpper(pubkey)]
}

// NodeOwner returns the verified owner of a node, if any.
func (s *Store) NodeOwner(nodePubkey string) (OwnerInfo, bool, error) {
	nodePubkey = strings.ToUpper(nodePubkey)
	var o OwnerInfo
	err := s.db.QueryRow(`
		SELECT u.id, COALESCE(NULLIF(u.display_name,''), u.email)
		FROM node_claims c JOIN users u ON u.id = c.user_id
		WHERE c.node_pubkey = ? AND c.status = 'verified'`, nodePubkey).
		Scan(&o.UserID, &o.DisplayName)
	if errors.Is(err, sql.ErrNoRows) {
		return OwnerInfo{}, false, nil
	}
	if err != nil {
		return OwnerInfo{}, false, err
	}
	return o, true, nil
}

// ClaimedNodeKeys returns the set of node pubkeys (uppercase) that have a
// verified owner. It's a single small scan of node_claims (verified claims are
// few) used to flag "claimed" nodes in the public nodes list without an N+1
// per-node owner lookup. Ownership is already public (the node-detail claim
// endpoint shows the owner to everyone), so this exposes nothing new — and it
// deliberately returns only the boolean set, never owner identities, so no
// display-name/email fallback leaks into the bulk list.
func (s *Store) ClaimedNodeKeys() (map[string]bool, error) {
	rows, err := s.db.Query(`SELECT DISTINCT node_pubkey FROM node_claims WHERE status = 'verified'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	set := make(map[string]bool)
	for rows.Next() {
		var pk string
		if err := rows.Scan(&pk); err != nil {
			return nil, err
		}
		set[strings.ToUpper(pk)] = true
	}
	return set, rows.Err()
}

// NodePrevOwner returns the display name recorded when the node's last verified
// owner deleted their account (empty when there is none). Meaningful only for a
// node that currently has no owner — it is cleared the moment a node is re-claimed.
func (s *Store) NodePrevOwner(nodePubkey string) (string, error) {
	var name sql.NullString
	err := s.db.QueryRow(`SELECT prev_owner_name FROM nodes WHERE pubkey = ?`, strings.ToUpper(nodePubkey)).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return name.String, nil
}

// UserClaim returns a user's claim on a node, if any.
func (s *Store) UserClaim(nodePubkey string, userID int64) (Claim, bool, error) {
	nodePubkey = strings.ToUpper(nodePubkey)
	var c Claim
	var verifiedAt sql.NullString
	err := s.db.QueryRow(`
		SELECT id, node_pubkey, user_id, code, status, created_at, expires_at, verified_at
		FROM node_claims WHERE node_pubkey = ? AND user_id = ?`, nodePubkey, userID).
		Scan(&c.ID, &c.NodePubkey, &c.UserID, &c.Code, &c.Status, &c.CreatedAt, &c.ExpiresAt, &verifiedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Claim{}, false, nil
	}
	if err != nil {
		return Claim{}, false, err
	}
	c.VerifiedAt = verifiedAt.String
	return c, true, nil
}

// CreateOrRefreshClaim opens (or re-issues the code for) a pending claim by
// userID on nodePubkey. It fails with ErrNodeClaimed when another user already
// owns the node. If the user already owns it, their verified claim is returned
// unchanged. ttl bounds how long the pending code stays valid.
func (s *Store) CreateOrRefreshClaim(nodePubkey string, userID int64, code string, ttl time.Duration) (Claim, error) {
	nodePubkey = strings.ToUpper(nodePubkey)

	if owner, ok, err := s.NodeOwner(nodePubkey); err != nil {
		return Claim{}, err
	} else if ok && owner.UserID != userID {
		return Claim{}, ErrNodeClaimed
	}
	// Already the verified owner → return the existing claim, no code churn.
	if existing, ok, err := s.UserClaim(nodePubkey, userID); err != nil {
		return Claim{}, err
	} else if ok && existing.Status == "verified" {
		return existing, nil
	}

	now := time.Now().UTC()
	nowS := now.Format(time.RFC3339Nano)
	expS := now.Add(ttl).Format(time.RFC3339Nano)
	// Upsert the pending claim (re-request just refreshes code + expiry).
	_, err := s.db.Exec(`
		INSERT INTO node_claims (node_pubkey, user_id, code, status, created_at, expires_at)
		VALUES (?,?,?,'pending',?,?)
		ON CONFLICT(node_pubkey, user_id) DO UPDATE SET
			code = excluded.code, status = 'pending',
			created_at = excluded.created_at, expires_at = excluded.expires_at,
			verified_at = NULL`,
		nodePubkey, userID, code, nowS, expS)
	if err != nil {
		// A collision on the pending-code unique index means the code is taken by
		// another open claim; surface it so the caller retries with a fresh code.
		if strings.Contains(err.Error(), "node_claims.code") {
			return Claim{}, ErrCodeCollision
		}
		return Claim{}, err
	}
	s.loadPendingClaims()
	c, _, err := s.UserClaim(nodePubkey, userID)
	return c, err
}

// CreateVerifiedClaim records verified ownership of a node by userID directly,
// without the advert-name code dance — used by the private-key ownership proof
// once the caller has produced a valid signature over a server challenge. It
// fails with ErrNodeClaimed when another user already owns the node. If the user
// already owns it, the existing verified claim is returned unchanged. Any prior
// pending claim by the same user is promoted in place. The code is stored empty
// (no name reset is ever needed for a key-verified claim).
func (s *Store) CreateVerifiedClaim(nodePubkey string, userID int64) (Claim, error) {
	nodePubkey = strings.ToUpper(nodePubkey)

	if owner, ok, err := s.NodeOwner(nodePubkey); err != nil {
		return Claim{}, err
	} else if ok && owner.UserID != userID {
		return Claim{}, ErrNodeClaimed
	}
	if existing, ok, err := s.UserClaim(nodePubkey, userID); err != nil {
		return Claim{}, err
	} else if ok && existing.Status == "verified" {
		return existing, nil
	}

	now := time.Now().UTC()
	nowS := now.Format(time.RFC3339Nano)
	// expires_at is NOT NULL but meaningless for a verified claim; set it to now.
	_, err := s.db.Exec(`
		INSERT INTO node_claims (node_pubkey, user_id, code, status, created_at, expires_at, verified_at)
		VALUES (?,?,'','verified',?,?,?)
		ON CONFLICT(node_pubkey, user_id) DO UPDATE SET
			code = '', status = 'verified', verified_at = excluded.verified_at`,
		nodePubkey, userID, nowS, nowS, nowS)
	if err != nil {
		return Claim{}, err
	}
	// A new verified owner supersedes any "previously owned by" marker.
	s.db.Exec(`UPDATE nodes SET prev_owner_name = NULL WHERE pubkey = ?`, nodePubkey)
	s.loadPendingClaims() // a promoted pending claim leaves the pending set
	c, _, err := s.UserClaim(nodePubkey, userID)
	return c, err
}

// DeleteClaim removes a user's claim on a node (cancels a pending code or
// releases ownership). Returns whether a row was removed.
func (s *Store) DeleteClaim(nodePubkey string, userID int64) (bool, error) {
	nodePubkey = strings.ToUpper(nodePubkey)
	res, err := s.db.Exec(`DELETE FROM node_claims WHERE node_pubkey = ? AND user_id = ?`, nodePubkey, userID)
	if err != nil {
		return false, err
	}
	s.loadPendingClaims()
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// VerifyPendingClaims promotes any of a node's pending, unexpired claims to
// verified when advertName contains the claim's code (case-insensitive). Called
// from the ingest path ONLY for signature-valid adverts, so a forged advert
// cannot complete a claim. If the node already has a different verified owner,
// nothing is promoted. Returns the claims that were verified.
func (s *Store) VerifyPendingClaims(nodePubkey, advertName string) ([]Claim, error) {
	nodePubkey = strings.ToUpper(nodePubkey)
	s.mu.Lock()
	defer s.mu.Unlock()

	// If someone already owns it, pending claims can't win the node.
	var ownerCount int
	s.db.QueryRow(`SELECT COUNT(*) FROM node_claims WHERE node_pubkey = ? AND status = 'verified'`, nodePubkey).Scan(&ownerCount)
	if ownerCount > 0 {
		return nil, nil
	}

	nowT := time.Now().UTC()
	now := nowT.Format(time.RFC3339Nano)
	rows, err := s.db.Query(`
		SELECT id, user_id, code FROM node_claims
		WHERE node_pubkey = ? AND status = 'pending' AND expires_at > ?`, nodePubkey, now)
	if err != nil {
		return nil, err
	}
	type cand struct {
		id     int64
		userID int64
		code   string
	}
	var cands []cand
	for rows.Next() {
		var c cand
		if err := rows.Scan(&c.id, &c.userID, &c.code); err != nil {
			rows.Close()
			return nil, err
		}
		cands = append(cands, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	upperName := strings.ToUpper(advertName)
	var verified []Claim
	for _, c := range cands {
		if !strings.Contains(upperName, strings.ToUpper(c.code)) {
			continue
		}
		// Only one verified owner per node (partial unique index enforces it too);
		// stop after the first match wins. The code is kept (not cleared) so the
		// claim-status endpoint can tell whether the node's current advertised name
		// still contains it — i.e. whether the owner has yet to restore the name.
		if _, err := s.db.Exec(`
			UPDATE node_claims SET status = 'verified', verified_at = ?
			WHERE id = ? AND status = 'pending'`, now, c.id); err != nil {
			return verified, err
		}
		// A new verified owner supersedes any "previously owned by" marker.
		s.db.Exec(`UPDATE nodes SET prev_owner_name = NULL WHERE pubkey = ?`, nodePubkey)
		verified = append(verified, Claim{ID: c.id, NodePubkey: nodePubkey, UserID: c.userID, Status: "verified", VerifiedAt: now})
		break
	}
	if len(verified) > 0 {
		s.loadPendingClaims()
	}
	return verified, nil
}

// ListUserClaims returns a user's claims with node display fields, newest first.
func (s *Store) ListUserClaims(userID int64) ([]ClaimWithNode, error) {
	rows, err := s.db.Query(`
		SELECT c.id, c.node_pubkey, c.user_id, c.code, c.status, c.created_at, c.expires_at,
		       COALESCE(c.verified_at,''), COALESCE(n.name,''), COALESCE(n.role,''),
		       n.pubkey IS NOT NULL
		FROM node_claims c LEFT JOIN nodes n ON n.pubkey = c.node_pubkey
		WHERE c.user_id = ?
		ORDER BY c.created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ClaimWithNode
	for rows.Next() {
		var c ClaimWithNode
		if err := rows.Scan(&c.ID, &c.NodePubkey, &c.UserID, &c.Code, &c.Status, &c.CreatedAt,
			&c.ExpiresAt, &c.VerifiedAt, &c.NodeName, &c.NodeRole, &c.NodePresent); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// NodeExists reports whether a node with the given pubkey is known.
func (s *Store) NodeExists(pubkey string) (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM nodes WHERE pubkey = ?`, strings.ToUpper(pubkey)).Scan(&n)
	return n > 0, err
}

// NodeName returns a node's advertised name (empty string if the node is unknown
// or unnamed).
func (s *Store) NodeName(pubkey string) string {
	var name string
	s.db.QueryRow(`SELECT COALESCE(name,'') FROM nodes WHERE pubkey = ?`, strings.ToUpper(pubkey)).Scan(&name)
	return name
}

// NameHasVerificationCode reports whether the node's current advertised name
// still contains the code from the user's verified claim — i.e. the owner
// renamed the node to verify it and hasn't restored the real name yet. Returns
// false when there's no verified claim, no retained code, or the name is clean.
func (s *Store) NameHasVerificationCode(nodePubkey string, userID int64) (bool, error) {
	nodePubkey = strings.ToUpper(nodePubkey)
	var code, name string
	err := s.db.QueryRow(`
		SELECT c.code, COALESCE(n.name,'')
		FROM node_claims c JOIN nodes n ON n.pubkey = c.node_pubkey
		WHERE c.node_pubkey = ? AND c.user_id = ? AND c.status = 'verified'`,
		nodePubkey, userID).Scan(&code, &name)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if code == "" {
		return false, nil
	}
	return strings.Contains(strings.ToUpper(name), strings.ToUpper(code)), nil
}

// PruneExpiredClaims deletes pending claims whose code has expired.
func (s *Store) PruneExpiredClaims() (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := s.db.Exec(`DELETE FROM node_claims WHERE status = 'pending' AND expires_at < ?`, now)
	if err != nil {
		return 0, err
	}
	s.loadPendingClaims()
	n, _ := res.RowsAffected()
	return n, nil
}

// PartitionClaimed splits keys into those with no claim and those a user has
// claimed (verified or pending), matching case-insensitively.
//
// Callers that delete node data use it to leave claimed nodes alone. A claim is
// proof a human verified ownership over RF, so a claimed node is not a phantom
// corruption artifact and not injected foreign traffic, however a heuristic
// scores it — it's evidence the heuristic misfired. Deleting anyway would either
// strand the claim on a node that can never return or destroy the owner's notes
// and private location, neither of which an unblock can undo.
func (s *Store) PartitionClaimed(keys []string) (unclaimed, claimed []string, err error) {
	if len(keys) == 0 {
		return nil, nil, nil
	}
	verified, err := s.ClaimedNodeKeys()
	if err != nil {
		return nil, nil, err
	}
	for _, k := range keys {
		if verified[strings.ToUpper(k)] || s.HasPendingClaim(k) {
			claimed = append(claimed, k)
			continue
		}
		unclaimed = append(unclaimed, k)
	}
	return unclaimed, claimed, nil
}
