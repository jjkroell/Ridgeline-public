package store

import (
	"strings"
	"time"

	"github.com/jjkroell/ridgeline/internal/meshcore"
)

// Block kinds.
const (
	BlockObserver = "observer" // rogue MQTT publisher (observer id)
	BlockBridge   = "bridge"   // RF bridge node (pubkey) — drops anything it relays
	BlockNode     = "node"     // a single injected node (pubkey) — drops its own adverts
	BlockAllow    = "allow"    // dismissed candidate (pubkey) — excluded from detection, NOT blocked
	// BlockKnown marks a SANCTIONED bridge (pubkey): a real bridge the operator
	// runs on purpose. Nothing is blocked and nothing is hidden — unlike "allow",
	// which asserts a candidate is not a bridge, this asserts it is one and that
	// it is wanted. Detection still reports it, labelled, so it stops reading as
	// a finding that needs acting on every single scan.
	BlockKnown = "known"
)

// pubkeyKind reports whether a block kind's key is a node pubkey (so it should be
// stored/compared upper-cased) rather than a free-form observer id.
func pubkeyKind(kind string) bool {
	return kind == BlockNode || kind == BlockBridge || kind == BlockAllow || kind == BlockKnown
}

// BlockEntry is one blocklist row.
type BlockEntry struct {
	Kind      string `json:"kind"`
	Key       string `json:"key"`
	Name      string `json:"name,omitempty"`
	Reason    string `json:"reason,omitempty"`
	CreatedAt string `json:"createdAt"`
}

// loadBlocklist refreshes the in-memory blocklist cache from the table.
func (s *Store) loadBlocklist() error {
	rows, err := s.db.Query(`SELECT kind, key FROM blocklist`)
	if err != nil {
		return err
	}
	defer rows.Close()
	obs := map[string]bool{}
	nodes := map[string]bool{}
	allow := map[string]bool{}
	known := map[string]bool{}
	var bridges []string
	for rows.Next() {
		var kind, key string
		if err := rows.Scan(&kind, &key); err != nil {
			return err
		}
		switch kind {
		case BlockObserver:
			obs[key] = true
		case BlockBridge:
			k := strings.ToUpper(key)
			nodes[k] = true
			bridges = append(bridges, k)
		case BlockNode:
			nodes[strings.ToUpper(key)] = true
		case BlockAllow:
			allow[strings.ToUpper(key)] = true
		case BlockKnown:
			known[strings.ToUpper(key)] = true
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	s.blockMu.Lock()
	s.blockedObservers, s.blockedNodes, s.blockedBridges, s.allowedNodes = obs, nodes, bridges, allow
	s.knownBridges = known
	s.blockMu.Unlock()
	return nil
}

// ShouldDrop reports whether an incoming observation is blocklisted and must not
// be ingested. Called on the hot path, so it only reads the cache. It drops:
//   - anything published by a blocked observer,
//   - an advert originated by a blocked node or bridge,
//   - any packet whose flood path transits a blocked bridge (a hop prefix of the
//     bridge's pubkey) — this is what kills the foreign traffic an RF bridge injects.
func (s *Store) ShouldDrop(p *meshcore.Packet, observerID string) bool {
	if p == nil {
		return false
	}
	s.blockMu.RLock()
	defer s.blockMu.RUnlock()
	if len(s.blockedObservers) == 0 && len(s.blockedNodes) == 0 && len(s.blockedBridges) == 0 {
		return false
	}
	if observerID != "" && s.blockedObservers[observerID] {
		return true
	}
	if p.Advert != nil && p.Advert.PublicKey != "" && s.blockedNodes[strings.ToUpper(p.Advert.PublicKey)] {
		return true
	}
	for _, hop := range p.Path {
		if hop == "" {
			continue
		}
		h := strings.ToUpper(hop)
		for _, b := range s.blockedBridges {
			if strings.HasPrefix(b, h) {
				return true
			}
		}
	}
	return false
}

// IsNodeBlocked reports whether a node/bridge pubkey is blocklisted (for hiding
// it from public API responses).
func (s *Store) IsNodeBlocked(pubkey string) bool {
	s.blockMu.RLock()
	defer s.blockMu.RUnlock()
	return s.blockedNodes[strings.ToUpper(pubkey)]
}

// NodeBlock returns the blocklist entry that quarantines a node/bridge pubkey,
// or nil if it isn't blocked. Prefers a "bridge" entry over a plain "node" one.
func (s *Store) NodeBlock(pubkey string) *BlockEntry {
	var e BlockEntry
	err := s.db.QueryRow(`
		SELECT kind, key, COALESCE(name,''), COALESCE(reason,''), created_at
		FROM blocklist
		WHERE key = ? AND kind IN ('node','bridge')
		ORDER BY CASE kind WHEN 'bridge' THEN 0 ELSE 1 END
		LIMIT 1`, strings.ToUpper(pubkey)).Scan(&e.Kind, &e.Key, &e.Name, &e.Reason, &e.CreatedAt)
	if err != nil {
		return nil
	}
	return &e
}

// IsAllowed reports whether a node pubkey has been dismissed as a detection
// candidate (allowlisted). Such nodes are excluded from injection detection but
// are NOT blocked.
func (s *Store) IsAllowed(pubkey string) bool {
	s.blockMu.RLock()
	defer s.blockMu.RUnlock()
	return s.allowedNodes[strings.ToUpper(pubkey)]
}

// AddBlock inserts (or updates) a blocklist entry and refreshes the cache.
func (s *Store) AddBlock(kind, key, name, reason string) error {
	if pubkeyKind(kind) {
		key = strings.ToUpper(key)
	}
	s.mu.Lock()
	_, err := s.db.Exec(`
		INSERT INTO blocklist (kind, key, name, reason, created_at)
		VALUES (?,?,?,?,?)
		ON CONFLICT(kind, key) DO UPDATE SET
			name   = COALESCE(NULLIF(excluded.name,''), blocklist.name),
			reason = COALESCE(NULLIF(excluded.reason,''), blocklist.reason)`,
		kind, key, nullStr(name), nullStr(reason), time.Now().UTC().Format(time.RFC3339))
	s.mu.Unlock()
	if err != nil {
		return err
	}
	return s.loadBlocklist()
}

// RemoveBlock deletes a blocklist entry (un-quarantine) and refreshes the cache.
func (s *Store) RemoveBlock(kind, key string) error {
	if pubkeyKind(kind) {
		key = strings.ToUpper(key)
	}
	s.mu.Lock()
	_, err := s.db.Exec(`DELETE FROM blocklist WHERE kind = ? AND key = ?`, kind, key)
	s.mu.Unlock()
	if err != nil {
		return err
	}
	return s.loadBlocklist()
}

// ListBlocks returns all blocklist entries, newest first.
func (s *Store) ListBlocks() ([]BlockEntry, error) {
	rows, err := s.db.Query(`SELECT kind, key, COALESCE(name,''), COALESCE(reason,''), created_at FROM blocklist ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BlockEntry{}
	for rows.Next() {
		var e BlockEntry
		if err := rows.Scan(&e.Kind, &e.Key, &e.Name, &e.Reason, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// PurgeResult reports what a purge removed.
type PurgeResult struct {
	Observations int64 `json:"observations"`
	// Telemetry counts observer device-telemetry rows removed alongside a
	// deleted observer.
	Telemetry int64 `json:"telemetry"`
	Nodes     int64 `json:"nodes"`
	Claims       int64 `json:"claims"`
	Notes        int64 `json:"notes"`
	Locations    int64 `json:"locations"`
	Shares       int64 `json:"shares"`
	// SkippedClaimed lists keys the caller held back from the delete because a
	// user has claimed them (set by the purge handler, not by the store).
	SkippedClaimed []string `json:"skippedClaimed,omitempty"`
}

// PurgeTargets hard-deletes stored data for the given targets in a single
// observation scan: observations published by any observer in observers,
// observations whose flood path transits any bridge pubkey in bridges, and the
// own adverts + node rows of any pubkey in nodes (and bridges). Pubkeys are
// matched case-insensitively; bridge path matching is by hash-prefix.
//
// User-authored data keyed to the node (claims, notes, private location,
// location shares) is PRESERVED. This is the path the automatic retention sweep
// uses, where a node pruned for going silent is expected to come back on its
// next advert — an operator who takes a repeater down for a week must not lose
// their ownership claim or private location. Use ScrubNodes for deliberate
// admin removal, which does cascade.
func (s *Store) PurgeTargets(observers, bridges, nodes []string) (PurgeResult, error) {
	return s.purgeTargets(observers, bridges, nodes, false)
}

// ScrubNodes hard-deletes nodes AND the user-authored data keyed to them
// (claims, notes, private location, location shares). This is the admin scrub /
// purge path, where removal is deliberate and permanent — the node is bogus,
// foreign, or injected, so its claim should not survive. Leaving the claim
// behind orphaned it: it still rendered in "Claimed Nodes" and on badges
// pointing at a node that no longer exists, and — because idx_claims_one_owner
// is unique per node — it would block the node from ever being re-claimed if it
// advertised again.
func (s *Store) ScrubNodes(observers, bridges, nodes []string) (PurgeResult, error) {
	return s.purgeTargets(observers, bridges, nodes, true)
}

func (s *Store) purgeTargets(observers, bridges, nodes []string, cascadeUserData bool) (PurgeResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var res PurgeResult
	obsSet := map[string]bool{}
	for _, o := range observers {
		obsSet[o] = true
	}
	bridgeSet := make([]string, 0, len(bridges))
	for _, b := range bridges {
		bridgeSet = append(bridgeSet, strings.ToUpper(b))
	}
	nodeSet := map[string]bool{}
	for _, n := range nodes {
		nodeSet[strings.ToUpper(n)] = true
	}
	for _, b := range bridgeSet {
		nodeSet[b] = true // a purged bridge's own node row goes too
	}

	tx, err := s.db.Begin()
	if err != nil {
		return res, err
	}
	defer tx.Rollback()

	// Scan observations once; collect ids to delete by re-decoding raw_hex.
	rows, err := tx.Query(`SELECT id, raw_hex, COALESCE(observer_id,'') FROM observations`)
	if err != nil {
		return res, err
	}
	var delIDs []int64
	for rows.Next() {
		var id int64
		var raw, obsID string
		if err := rows.Scan(&id, &raw, &obsID); err != nil {
			rows.Close()
			return res, err
		}
		if obsID != "" && obsSet[obsID] {
			delIDs = append(delIDs, id)
			continue
		}
		pkt, err := meshcore.DecodeHex(raw)
		if err != nil || pkt == nil {
			continue
		}
		if pkt.Advert != nil && nodeSet[strings.ToUpper(pkt.Advert.PublicKey)] {
			delIDs = append(delIDs, id)
			continue
		}
		if len(bridgeSet) > 0 {
			if hit := pathHitsBridge(pkt.Path, bridgeSet); hit {
				delIDs = append(delIDs, id)
			}
		}
	}
	rows.Close()

	for _, id := range delIDs {
		r, err := tx.Exec(`DELETE FROM observations WHERE id = ?`, id)
		if err != nil {
			return res, err
		}
		n, _ := r.RowsAffected()
		res.Observations += n
	}

	// Delete node rows for explicitly targeted nodes/bridges, along with the
	// user-authored data keyed to them (see the doc comment).
	for k := range nodeSet {
		r, err := tx.Exec(`DELETE FROM nodes WHERE UPPER(pubkey) = ?`, k)
		if err != nil {
			return res, err
		}
		n, _ := r.RowsAffected()
		res.Nodes += n

		if !cascadeUserData {
			continue
		}
		for _, c := range []struct {
			query string
			count *int64
		}{
			{`DELETE FROM node_claims WHERE UPPER(node_pubkey) = ?`, &res.Claims},
			{`DELETE FROM node_notes WHERE UPPER(node_pubkey) = ?`, &res.Notes},
			{`DELETE FROM node_private_locations WHERE UPPER(node_pubkey) = ?`, &res.Locations},
			{`DELETE FROM location_shares WHERE UPPER(node_pubkey) = ?`, &res.Shares},
		} {
			r, err := tx.Exec(c.query, k)
			if err != nil {
				return res, err
			}
			n, _ := r.RowsAffected()
			*c.count += n
		}
	}
	// Delete observer rows for purged observers, along with their device
	// telemetry. The telemetry series is keyed by observer id and nothing else
	// references it, so leaving it behind orphans rows that no page can reach and
	// no sweep collects — invisible growth for every observer ever deleted.
	for o := range obsSet {
		r, err := tx.Exec(`DELETE FROM observer_telemetry WHERE observer_id = ?`, o)
		if err != nil {
			return res, err
		}
		n, _ := r.RowsAffected()
		res.Telemetry += n
		if _, err := tx.Exec(`DELETE FROM observers WHERE id = ?`, o); err != nil {
			return res, err
		}
	}

	if err := tx.Commit(); err != nil {
		return res, err
	}

	// Deleting node_claims rows invalidates the pending-claim cache that gates
	// the ingest advert verifier, so refresh it like every other writer of that
	// table does. This MUST run after Commit: loadPendingClaims issues its own
	// query, and with SetMaxOpenConns(1) it would block forever waiting on the
	// single connection an open transaction still holds.
	if res.Claims > 0 {
		if err := s.loadPendingClaims(); err != nil {
			return res, err
		}
	}
	return res, nil
}

func pathHitsBridge(path []string, bridges []string) bool {
	for _, hop := range path {
		if hop == "" {
			continue
		}
		h := strings.ToUpper(hop)
		for _, b := range bridges {
			if strings.HasPrefix(b, h) {
				return true
			}
		}
	}
	return false
}

// KnownBridges returns the set of pubkeys (uppercase) an operator has marked as
// sanctioned bridges. Detection labels these rather than hiding them: the bridge
// is real and should still be visible, it just isn't news.
func (s *Store) KnownBridges() map[string]bool {
	s.blockMu.RLock()
	defer s.blockMu.RUnlock()
	out := make(map[string]bool, len(s.knownBridges))
	for k := range s.knownBridges {
		out[k] = true
	}
	return out
}
