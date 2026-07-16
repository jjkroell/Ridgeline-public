package api

import (
	"encoding/json"
	"net/http"
	"net/mail"
	"strings"

	"github.com/jjkroell/ridgeline/internal/auth"
	"github.com/jjkroell/ridgeline/internal/store"
)

// accountUpdateProfile changes the caller's display name / callsign. Returns the
// updated account (the client refreshes its cached user; the CSRF token is
// unchanged, so it is not re-issued here).
func (s *Server) accountUpdateProfile(w http.ResponseWriter, r *http.Request, user store.User) {
	var req struct {
		DisplayName string `json:"displayName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request body")
		return
	}
	name := strings.TrimSpace(req.DisplayName)
	if len(name) > 64 {
		writeErr(w, http.StatusBadRequest, "display name must be 64 characters or fewer")
		return
	}
	if err := s.store.SetDisplayName(user.ID, name); err != nil {
		s.fail(w, err)
		return
	}
	s.returnUpdatedUser(w, user.ID)
}

// accountChangePassword sets a new password after re-authenticating with the
// current one. The session stays valid (sessions aren't derived from the
// password), so the caller isn't logged out.
func (s *Server) accountChangePassword(w http.ResponseWriter, r *http.Request, user store.User) {
	var req struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request body")
		return
	}
	if !auth.VerifyPassword(req.CurrentPassword, user.PasswordHash) {
		writeErr(w, http.StatusForbidden, "current password is incorrect")
		return
	}
	if len(req.NewPassword) < 8 {
		writeErr(w, http.StatusBadRequest, "new password must be at least 8 characters")
		return
	}
	if len(req.NewPassword) > 200 {
		writeErr(w, http.StatusBadRequest, "new password is too long")
		return
	}
	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		s.fail(w, err)
		return
	}
	if err := s.store.UpdatePassword(user.ID, hash); err != nil {
		s.fail(w, err)
		return
	}
	s.log.Info("account password changed", "user", user.ID)
	writeJSON(w, map[string]bool{"ok": true})
}

// accountChangeEmail changes the caller's email after re-authenticating. The new
// address is marked unverified and a confirmation link is sent to it; the caller
// keeps their current session but must verify before their next sign-in (the
// owner is exempt from that gate). When email is disabled the new address is
// treated as verified so no one is stranded.
func (s *Server) accountChangeEmail(w http.ResponseWriter, r *http.Request, user store.User) {
	var req struct {
		CurrentPassword string `json:"currentPassword"`
		NewEmail        string `json:"newEmail"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request body")
		return
	}
	if !auth.VerifyPassword(req.CurrentPassword, user.PasswordHash) {
		writeErr(w, http.StatusForbidden, "current password is incorrect")
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.NewEmail))
	if addr, err := mail.ParseAddress(email); err != nil || addr.Address != email {
		writeErr(w, http.StatusBadRequest, "a valid email address is required")
		return
	}
	if email == user.Email {
		writeErr(w, http.StatusBadRequest, "that's already your email address")
		return
	}
	if err := s.store.UpdateEmail(user.ID, email); err == store.ErrEmailTaken {
		writeErr(w, http.StatusConflict, "that email is already registered to another account")
		return
	} else if err != nil {
		s.fail(w, err)
		return
	}
	s.log.Info("account email changed", "user", user.ID)

	updated, _, err := s.store.GetUserByID(user.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	if s.mailEnabled() {
		s.sendVerificationEmail(updated)
	} else {
		// No relay: don't strand the account behind an unconfirmable address.
		s.store.MarkEmailVerified(updated.ID)
		updated, _, _ = s.store.GetUserByID(user.ID)
	}
	writeJSON(w, updated)
}

// accountDelete permanently removes the caller's own account after
// re-authenticating with their password. Every node they verifiably owned is
// released and stamped "previously owned by <their name>". The protected owner
// cannot delete their account (the deployment must always keep an owner). On
// success the session cookies are cleared so the browser is signed out.
func (s *Server) accountDelete(w http.ResponseWriter, r *http.Request, user store.User) {
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request body")
		return
	}
	if !auth.VerifyPassword(req.Password, user.PasswordHash) {
		writeErr(w, http.StatusForbidden, "password is incorrect")
		return
	}
	if user.IsOwner {
		writeErr(w, http.StatusForbidden, "the owner account cannot be deleted")
		return
	}
	// The public label kept on released nodes. Fall back to a generic phrase
	// rather than the email so a deleted account's address isn't left on public
	// pages. (A live owner with no display name already shows their email; a
	// deleted one should not.)
	label := strings.TrimSpace(user.DisplayName)
	if label == "" {
		label = "a former member"
	}
	if err := s.store.DeleteUserAndReleaseNodes(user.ID, label); err != nil {
		s.fail(w, err)
		return
	}
	s.clearSessionCookies(w, r)
	s.log.Info("account self-deleted", "user", user.ID)
	writeJSON(w, map[string]bool{"ok": true})
}

// returnUpdatedUser re-reads and writes the account so the client can refresh its
// cached user after a profile change.
func (s *Server) returnUpdatedUser(w http.ResponseWriter, id int64) {
	u, ok, err := s.store.GetUserByID(id)
	if err != nil {
		s.fail(w, err)
		return
	}
	if !ok {
		writeErr(w, http.StatusNotFound, "account not found")
		return
	}
	writeJSON(w, u)
}
