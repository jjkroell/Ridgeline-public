package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/jjkroell/ridgeline/internal/store"
)

// Owner-initiated node lifecycle: retire (reversible, keeps history) and scrub
// (permanent, deletes history and the claim).
//
// Both require the caller to be the node's VERIFIED owner. A claim is proved
// cryptographically — an ed25519 advert or a challenge signature checked against
// the node's own public key — so "must have claimed it first" is a signature
// check, not a soft flag. Admins already have the equivalent powers via
// /api/admin/delete; nothing here widens what an admin can do.
//
// Users deliberately get NO access to the blocklist. Blocking prevents re-ingest,
// and scrubbing cascades the claim away — so block+scrub would leave a node that
// can never reappear AND can never be re-claimed, since there would be no node
// left to claim. That combination stays admin-only.

// retireRequest is shared by retire/unretire; scrub takes an explicit confirm.
type nodeScrubRequest struct {
	// DeleteHistory must be true to hard-delete. Requiring it means a
	// mis-wired client cannot destroy history by omitting a field.
	DeleteHistory bool `json:"deleteHistory"`
}

// ownedNodeAction resolves {pubkey}, checks verified ownership, and rate-limits.
// Returns the normalised pubkey and false when it has already written a response.
func (s *Server) ownedNodeAction(w http.ResponseWriter, r *http.Request, u store.User, action string) (string, bool) {
	pubkey := strings.ToUpper(strings.TrimSpace(r.PathValue("pubkey")))
	if pubkey == "" {
		writeErr(w, http.StatusBadRequest, "missing node")
		return "", false
	}
	// Destructive and irreversible; bound it per user rather than per IP so a
	// shared NAT can't be used to exhaust someone else's budget.
	if !s.nodeLifecycleLimiter.Allow(u.Email) {
		writeErr(w, http.StatusTooManyRequests, "too many requests, try again shortly")
		return "", false
	}
	owns, err := s.ownsNode(pubkey, u.ID)
	if err != nil {
		s.fail(w, err)
		return "", false
	}
	if !owns {
		// Same response whether the node is unclaimed, claimed by someone else, or
		// absent: a non-owner learns nothing about it from this endpoint.
		writeErr(w, http.StatusForbidden, "you must be the verified owner of this node")
		return "", false
	}
	_ = action
	return pubkey, true
}

// audit records the action best-effort. A failure here must not fail the user's
// request, but it is logged loudly — this is the only record that survives a
// scrub, which deletes the claim that would otherwise show who owned the node.
func (s *Server) audit(u store.User, action, target, detail string) {
	if err := s.store.WriteAudit(time.Now().UTC().Format(time.RFC3339Nano), u.ID, u.Email, action, target, detail); err != nil {
		s.log.Error("audit write failed", "action", action, "target", target, "err", err)
	}
}

// nodeRetire withdraws the caller's node from the map and node lists, keeping
// every packet it sent. Reversible. This is the right action for a
// decommissioned node: the row is kept, so a node still briefly on air does not
// pop back on its next advert.
func (s *Server) nodeRetire(w http.ResponseWriter, r *http.Request, u store.User) {
	pubkey, ok := s.ownedNodeAction(w, r, u, "retire")
	if !ok {
		return
	}
	if err := s.store.RetireNode(pubkey, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		s.fail(w, err)
		return
	}
	s.audit(u, "node_retire", pubkey, "")
	s.log.Info("node retired by owner", "node", pubkey, "user", u.Email)
	writeJSON(w, map[string]any{"retired": true})
}

// nodeUnretire returns the caller's node to the map and node lists.
func (s *Server) nodeUnretire(w http.ResponseWriter, r *http.Request, u store.User) {
	pubkey, ok := s.ownedNodeAction(w, r, u, "unretire")
	if !ok {
		return
	}
	if err := s.store.UnretireNode(pubkey); err != nil {
		s.fail(w, err)
		return
	}
	s.audit(u, "node_unretire", pubkey, "")
	s.log.Info("node unretired by owner", "node", pubkey, "user", u.Email)
	writeJSON(w, map[string]any{"retired": false})
}

// nodeScrub permanently deletes the caller's node and everything keyed to it.
//
// Two things the UI must say out loud before calling this:
//
//  1. It releases the claim. ScrubNodes cascades claims deliberately — an
//     orphaned claim keeps rendering on badges and, because idx_claims_one_owner
//     is unique per node, would block the node from ever being re-claimed. So
//     the caller gives up ownership; if the node advertises again it returns
//     unclaimed and anyone may claim it.
//  2. It deletes observations of this node reported by OTHER operators'
//     receivers. Retire keeps those.
func (s *Server) nodeScrub(w http.ResponseWriter, r *http.Request, u store.User) {
	pubkey, ok := s.ownedNodeAction(w, r, u, "scrub")
	if !ok {
		return
	}
	var req nodeScrubRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request body")
		return
	}
	if !req.DeleteHistory {
		writeErr(w, http.StatusBadRequest, "scrub requires deleteHistory:true — use retire to withdraw the node without deleting its history")
		return
	}
	// Written BEFORE the cascade: ScrubNodes deletes the claim, so afterwards
	// there is nothing left linking this node to the user who removed it.
	s.audit(u, "node_scrub", pubkey, "owner-initiated hard delete")

	res, err := s.store.ScrubNodes(nil, nil, []string{pubkey})
	if err != nil {
		s.fail(w, err)
		return
	}
	s.log.Info("node scrubbed by owner", "node", pubkey, "user", u.Email,
		"observationsDeleted", res.Observations, "nodesDeleted", res.Nodes,
		"claimsDeleted", res.Claims, "notesDeleted", res.Notes,
		"locationsDeleted", res.Locations, "sharesDeleted", res.Shares)
	writeJSON(w, res)
}
