package api

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/jjkroell/ridgeline/internal/auth"
	"github.com/jjkroell/ridgeline/internal/store"
)

// claimTTL is how long a pending ownership code stays valid. Long enough to log
// into a repeater over BLE/serial, change its name, and let an advert flood out.
const claimTTL = 30 * time.Minute

// claimStatus is the shape returned by GET /api/nodes/{pubkey}/claim. Owner is
// public (a "claimed by" badge); Mine + CanClaim reflect the requesting user.
type claimStatus struct {
	Owner *store.OwnerInfo `json:"owner,omitempty"`
	// PreviousOwner is the display name of the node's last owner, retained after
	// that owner deleted their account. Only set when the node currently has no
	// owner; lets the page show "previously owned by …".
	PreviousOwner string       `json:"previousOwner,omitempty"`
	OwnedByMe     bool         `json:"ownedByMe"`
	Mine          *store.Claim `json:"mine,omitempty"`
	LoggedIn      bool         `json:"loggedIn"`
	CanClaim      bool         `json:"canClaim"` // requester is allowed to start a claim
	// NameNeedsReset is true when the caller owns the node and its current
	// advertised name still contains the code used to verify — i.e. the owner
	// hasn't yet restored the real name and re-advertised.
	NameNeedsReset bool `json:"nameNeedsReset"`
}

// nodeClaimStatus reports a node's ownership + the caller's own claim. Public
// (no auth required); owner display name is shown to everyone, the caller's
// pending code only to the caller.
func (s *Server) nodeClaimStatus(w http.ResponseWriter, r *http.Request) {
	pubkey := strings.ToUpper(r.PathValue("pubkey"))
	var out claimStatus

	if owner, ok, err := s.store.NodeOwner(pubkey); err != nil {
		s.fail(w, err)
		return
	} else if ok {
		out.Owner = &owner
	} else {
		// No current owner — surface a "previously owned by" marker if one was
		// left behind by an owner who deleted their account.
		if prev, err := s.store.NodePrevOwner(pubkey); err == nil {
			out.PreviousOwner = prev
		}
	}

	if user, _, ok := s.currentUser(r); ok {
		out.LoggedIn = true
		if mine, has, err := s.store.UserClaim(pubkey, user.ID); err != nil {
			s.fail(w, err)
			return
		} else if has {
			out.OwnedByMe = mine.Status == "verified"
			if out.OwnedByMe {
				// If the node's current name still carries the verification code, the
				// owner hasn't restored it yet — prompt them to. Once they re-advert
				// with a clean name this flips false and the UI nudge disappears.
				if needs, err := s.store.NameHasVerificationCode(pubkey, user.ID); err == nil {
					out.NameNeedsReset = needs
				}
				mine.Code = "" // never expose the spent code
			}
			out.Mine = &mine
		}
		// Any signed-in user may start a claim on a node no one else owns.
		out.CanClaim = out.Owner == nil || out.Owner.UserID == user.ID
	}
	writeJSON(w, out)
}

// claimCreate opens or refreshes the caller's pending claim on a node, returning
// a fresh verification code to embed in the node's advertised name.
func (s *Server) claimCreate(w http.ResponseWriter, r *http.Request, user store.User) {
	var req struct {
		Pubkey string `json:"pubkey"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request body")
		return
	}
	pubkey := strings.ToUpper(strings.TrimSpace(req.Pubkey))
	if !validPubkey(pubkey) {
		writeErr(w, http.StatusBadRequest, "a full 64-character node public key is required")
		return
	}
	if ok, err := s.store.NodeExists(pubkey); err != nil {
		s.fail(w, err)
		return
	} else if !ok {
		writeErr(w, http.StatusNotFound, "unknown node — Ridgeline hasn't heard this node yet")
		return
	}

	// Generate a code and open the claim. Every live code is unique (DB-enforced);
	// on the astronomically rare collision with another open claim, regenerate.
	var claim store.Claim
	for attempt := 0; ; attempt++ {
		code, err := auth.NewClaimCode()
		if err != nil {
			s.fail(w, err)
			return
		}
		claim, err = s.store.CreateOrRefreshClaim(pubkey, user.ID, code, claimTTL)
		if err == store.ErrCodeCollision && attempt < 5 {
			continue
		}
		if err == store.ErrNodeClaimed {
			writeErr(w, http.StatusConflict, "this node is already claimed by another user")
			return
		}
		if err != nil {
			s.fail(w, err)
			return
		}
		break
	}
	s.log.Info("node claim opened", "node", pubkey, "user", user.ID, "status", claim.Status)
	writeJSON(w, claim)
}

// claimDelete cancels the caller's pending claim or releases their ownership.
func (s *Server) claimDelete(w http.ResponseWriter, r *http.Request, user store.User) {
	pubkey := strings.ToUpper(r.PathValue("pubkey"))
	if !validPubkey(pubkey) {
		writeErr(w, http.StatusBadRequest, "invalid node public key")
		return
	}
	// If the caller is releasing verified ownership, drop the node's private
	// exact location too so a future owner can't inherit their coordinates.
	if owns, err := s.ownsNode(pubkey, user.ID); err != nil {
		s.fail(w, err)
		return
	} else if owns {
		if _, err := s.store.DeletePrivateLocation(pubkey); err != nil {
			s.fail(w, err)
			return
		}
		if err := s.store.DeleteLocationShares(pubkey); err != nil {
			s.fail(w, err)
			return
		}
	}
	removed, err := s.store.DeleteClaim(pubkey, user.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	if !removed {
		writeErr(w, http.StatusNotFound, "you have no claim on this node")
		return
	}
	s.log.Info("node claim released", "node", pubkey, "user", user.ID)
	writeJSON(w, map[string]bool{"ok": true})
}

// claimsMine lists the caller's claims (pending + owned) with node display info.
func (s *Server) claimsMine(w http.ResponseWriter, _ *http.Request, user store.User) {
	claims, err := s.store.ListUserClaims(user.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	if claims == nil {
		claims = []store.ClaimWithNode{}
	}
	// Only a pending claim's code is meaningful to show; hide spent codes.
	for i := range claims {
		if claims[i].Status != "pending" {
			claims[i].Code = ""
		}
	}
	writeJSON(w, claims)
}

// validPubkey reports whether s is a full 32-byte (64 hex char) public key.
func validPubkey(s string) bool {
	if len(s) != 64 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}
