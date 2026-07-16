package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/jjkroell/ridgeline/internal/store"
)

// usersSearch powers the share autocomplete: up to 8 registered users whose
// display name matches ?q. Auth-gated (any signed-in user) and returns only
// id + display name — never emails or flags — so it can't be used to enumerate
// accounts or harvest addresses.
func (s *Server) usersSearch(w http.ResponseWriter, r *http.Request, user store.User) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) < 2 {
		writeJSON(w, []store.UserBrief{})
		return
	}
	if len(q) > 64 {
		q = q[:64]
	}
	res, err := s.store.SearchUsersByName(q, user.ID, 8)
	if err != nil {
		s.fail(w, err)
		return
	}
	if res == nil {
		res = []store.UserBrief{}
	}
	writeJSON(w, res)
}

// sharesMine returns the nodes whose private location has been shared WITH the
// caller (the grantee-facing "Shared with me" list).
func (s *Server) sharesMine(w http.ResponseWriter, _ *http.Request, user store.User) {
	shares, err := s.store.SharesForUser(user.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	if shares == nil {
		shares = []store.SharedWithMe{}
	}
	writeJSON(w, shares)
}

// sharesMarkSeen clears the caller's unseen-share flags (called when they view
// their Shared-with-me list), so the account badge resets.
func (s *Server) sharesMarkSeen(w http.ResponseWriter, _ *http.Request, user store.User) {
	if err := s.store.MarkSharesSeen(user.ID); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// locationSharesList returns the users a node's private location is shared with.
// Owner-only.
func (s *Server) locationSharesList(w http.ResponseWriter, r *http.Request, user store.User) {
	pubkey := strings.ToUpper(r.PathValue("pubkey"))
	if !validPubkey(pubkey) {
		writeErr(w, http.StatusBadRequest, "invalid node public key")
		return
	}
	owns, err := s.ownsNode(pubkey, user.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	if !owns {
		writeErr(w, http.StatusForbidden, "only the node's owner can manage location sharing")
		return
	}
	shares, err := s.store.ListLocationShares(pubkey)
	if err != nil {
		s.fail(w, err)
		return
	}
	if shares == nil {
		shares = []store.LocationShare{}
	}
	writeJSON(w, shares)
}

// locationShareCreate grants a registered user (by email) read access to a
// node's private location. Owner-only.
func (s *Server) locationShareCreate(w http.ResponseWriter, r *http.Request, user store.User) {
	pubkey := strings.ToUpper(r.PathValue("pubkey"))
	if !validPubkey(pubkey) {
		writeErr(w, http.StatusBadRequest, "invalid node public key")
		return
	}
	owns, err := s.ownsNode(pubkey, user.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	if !owns {
		writeErr(w, http.StatusForbidden, "only the node's owner can share this location")
		return
	}
	// Grant by user id (from the autocomplete picker) or, as a fallback, by email.
	var req struct {
		UserID int64  `json:"userId"`
		Email  string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request body")
		return
	}
	var grantee store.User
	var ok bool
	if req.UserID > 0 {
		grantee, ok, err = s.store.GetUserByID(req.UserID)
	} else {
		email := strings.ToLower(strings.TrimSpace(req.Email))
		if email == "" {
			writeErr(w, http.StatusBadRequest, "pick a user to share with")
			return
		}
		grantee, ok, err = s.store.GetUserByEmail(email)
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	if !ok {
		writeErr(w, http.StatusNotFound, "that user isn't registered")
		return
	}
	if grantee.ID == user.ID {
		writeErr(w, http.StatusBadRequest, "you already own this node")
		return
	}
	if grantee.Blocked {
		writeErr(w, http.StatusBadRequest, "that account is suspended")
		return
	}
	if err := s.store.ShareLocation(pubkey, user.ID, grantee.ID); err != nil {
		s.fail(w, err)
		return
	}
	s.log.Info("private location shared", "node", pubkey, "owner", user.ID, "grantee", grantee.ID)
	// Return the refreshed list so the UI can re-render in one round trip.
	shares, err := s.store.ListLocationShares(pubkey)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, shares)
}

// locationShareDelete revokes a grantee's access. Owner-only.
func (s *Server) locationShareDelete(w http.ResponseWriter, r *http.Request, user store.User) {
	pubkey := strings.ToUpper(r.PathValue("pubkey"))
	if !validPubkey(pubkey) {
		writeErr(w, http.StatusBadRequest, "invalid node public key")
		return
	}
	granteeID, err := strconv.ParseInt(r.PathValue("userId"), 10, 64)
	if err != nil || granteeID <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid user id")
		return
	}
	owns, err := s.ownsNode(pubkey, user.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	if !owns {
		writeErr(w, http.StatusForbidden, "only the node's owner can revoke sharing")
		return
	}
	if _, err := s.store.UnshareLocation(pubkey, granteeID); err != nil {
		s.fail(w, err)
		return
	}
	s.log.Info("private location share revoked", "node", pubkey, "owner", user.ID, "grantee", granteeID)
	writeJSON(w, map[string]bool{"ok": true})
}
