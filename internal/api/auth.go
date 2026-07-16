package api

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/jjkroell/ridgeline/internal/auth"
	"github.com/jjkroell/ridgeline/internal/store"
)

const (
	sessionCookie = "rl_session"
	csrfCookie    = "rl_csrf"
	csrfHeader    = "X-CSRF-Token"
	// sessionTTL is how long a login lasts. 30 days balances convenience against
	// exposure for a low-stakes community site; sessions are server-side and can
	// be revoked at logout.
	sessionTTL = 30 * 24 * time.Hour
)

// userHandler is an authenticated handler that receives the resolved account.
type userHandler func(http.ResponseWriter, *http.Request, store.User)

// currentUser resolves the request's session cookie to its account and session.
// ok is false when there is no valid, unexpired session.
func (s *Server) currentUser(r *http.Request) (store.User, store.Session, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return store.User{}, store.Session{}, false
	}
	sess, user, ok, err := s.store.SessionUser(auth.HashToken(c.Value))
	if err != nil || !ok {
		return store.User{}, store.Session{}, false
	}
	return user, sess, true
}

// requireUser wraps a handler so it only runs for authenticated requests. For
// mutating methods it also enforces the double-submit CSRF token (the client
// must echo the session's CSRF token in the X-CSRF-Token header).
func (s *Server) requireUser(h userHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, sess, ok := s.currentUser(r)
		if !ok {
			writeErr(w, http.StatusUnauthorized, "authentication required")
			return
		}
		if isMutating(r.Method) && !csrfOK(r, sess) {
			writeErr(w, http.StatusForbidden, "invalid or missing CSRF token")
			return
		}
		h(w, r, user)
	}
}

// requireAdminUser is like requireUser but additionally requires an admin
// account (the is_admin flag, distinct from the static injection-console token).
func (s *Server) requireAdminUser(h userHandler) http.HandlerFunc {
	return s.requireUser(func(w http.ResponseWriter, r *http.Request, u store.User) {
		if !u.IsAdmin {
			writeErr(w, http.StatusForbidden, "admin only")
			return
		}
		h(w, r, u)
	})
}

func isMutating(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}

// csrfOK reports whether the request carries the session's CSRF token in the
// X-CSRF-Token header (synchronizer-token pattern; the token is delivered to the
// SPA via a readable cookie and /api/auth/me).
func csrfOK(r *http.Request, sess store.Session) bool {
	tok := r.Header.Get(csrfHeader)
	return tok != "" && subtle.ConstantTimeCompare([]byte(tok), []byte(sess.CSRF)) == 1
}

// authResp is the shape returned by register/login/me. User is null when not
// authenticated; csrfToken is included so the SPA can send it on mutations;
// unseenShares drives the account badge for newly shared-with nodes.
type authResp struct {
	User         *store.User `json:"user"`
	CSRFToken    string      `json:"csrfToken,omitempty"`
	UnseenShares int         `json:"unseenShares"`
}

// authRespFor builds the authenticated response, filling in the unseen-share
// count (best-effort; a lookup error just leaves it at zero).
func (s *Server) authRespFor(u store.User, csrf string) authResp {
	n, _ := s.store.UnseenShareCount(u.ID)
	return authResp{User: &u, CSRFToken: csrf, UnseenShares: n}
}

func (s *Server) authRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email       string `json:"email"`
		Password    string `json:"password"`
		DisplayName string `json:"displayName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request body")
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if addr, err := mail.ParseAddress(email); err != nil || addr.Address != email {
		writeErr(w, http.StatusBadRequest, "a valid email address is required")
		return
	}
	if len(req.Password) < 8 {
		writeErr(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	if len(req.Password) > 200 {
		writeErr(w, http.StatusBadRequest, "password is too long")
		return
	}
	name := strings.TrimSpace(req.DisplayName)
	if len(name) > 64 {
		writeErr(w, http.StatusBadRequest, "display name must be 64 characters or fewer")
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		s.fail(w, err)
		return
	}
	user, err := s.store.CreateUser(email, hash, name)
	if err == store.ErrEmailTaken {
		writeErr(w, http.StatusConflict, "that email is already registered")
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	s.log.Info("user registered", "id", user.ID, "email", user.Email, "admin", user.IsAdmin)

	// The bootstrap owner is auto-verified and logged straight in. Everyone else
	// must confirm their email before they can sign in.
	if user.EmailVerified {
		s.startSession(w, r, user)
		return
	}
	s.sendVerificationEmail(user)
	// If email isn't configured there's no way to verify, so don't strand the
	// account — log them in and note it. (Dev / misconfiguration safety valve.)
	if !s.mailEnabled() {
		s.log.Warn("email disabled: logging new user in without verification", "id", user.ID)
		s.store.MarkEmailVerified(user.ID)
		user.EmailVerified = true
		s.startSession(w, r, user)
		return
	}
	writeJSON(w, registerResp{VerificationSent: true, Email: user.Email})
}

// registerResp tells the client a verification email is on its way (no session
// is started until the address is confirmed).
type registerResp struct {
	VerificationSent bool   `json:"verificationSent"`
	Email            string `json:"email"`
}

func (s *Server) authLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request body")
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	user, ok, err := s.store.GetUserByEmail(email)
	if err != nil {
		s.fail(w, err)
		return
	}
	if !ok {
		// Run a verify against a dummy hash so a missing account and a wrong
		// password take about the same time (avoids account enumeration).
		auth.VerifyPassword(req.Password, dummyHash)
		writeErr(w, http.StatusUnauthorized, "incorrect email or password")
		return
	}
	if !auth.VerifyPassword(req.Password, user.PasswordHash) {
		writeErr(w, http.StatusUnauthorized, "incorrect email or password")
		return
	}
	// Checked only after the password verifies, so a blocked account isn't
	// distinguishable from a wrong password to an unauthenticated caller.
	if user.Blocked {
		writeErr(w, http.StatusForbidden, "this account has been blocked")
		return
	}
	// Unverified accounts cannot sign in. Signal it distinctly so the UI can offer
	// to resend the confirmation email. The protected owner is exempt so a botched
	// email change can never lock the deployment's owner out.
	if !user.EmailVerified && !user.IsOwner {
		writeJSONStatus(w, http.StatusForbidden, map[string]any{
			"error":      "please confirm your email address before signing in — check your inbox for the verification link",
			"unverified": true,
		})
		return
	}
	s.store.SetUserLastLogin(user.ID)
	s.startSession(w, r, user)
	s.log.Info("user login", "id", user.ID, "email", user.Email)
}

// authVerifyEmail consumes a verification token (from the emailed link) and, on
// success, marks the account verified and logs it in.
func (s *Server) authVerifyEmail(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request body")
		return
	}
	user, ok, err := s.store.VerifyEmailToken(strings.TrimSpace(req.Token))
	if err != nil {
		s.fail(w, err)
		return
	}
	if !ok {
		writeErr(w, http.StatusBadRequest, "this verification link is invalid or has expired — request a new one")
		return
	}
	s.store.SetUserLastLogin(user.ID)
	s.startSession(w, r, user)
	s.log.Info("email verified", "id", user.ID, "email", user.Email)
}

// authResendVerification re-sends a confirmation email. It always responds 200
// (never revealing whether the address exists or its state) to avoid account
// enumeration; the email only goes out for a real, still-unverified account.
func (s *Server) authResendVerification(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request body")
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if user, ok, err := s.store.GetUserByEmail(email); err == nil && ok && !user.EmailVerified {
		s.sendVerificationEmail(user)
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) authLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		s.store.DeleteSession(auth.HashToken(c.Value))
	}
	s.clearSessionCookies(w, r)
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) authMe(w http.ResponseWriter, r *http.Request) {
	user, sess, ok := s.currentUser(r)
	if !ok {
		writeJSON(w, authResp{User: nil})
		return
	}
	writeJSON(w, s.authRespFor(user, sess.CSRF))
}

// adminListUsers returns all accounts for the admin console (session-admin gated).
func (s *Server) adminListUsers(w http.ResponseWriter, _ *http.Request, _ store.User) {
	users, err := s.store.ListUsers()
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, users)
}

// adminSetUserFlags grants/revokes a user's admin rights. Claiming is universal
// (every account can claim), so it is no longer an admin-managed flag.
func (s *Server) adminSetUserFlags(w http.ResponseWriter, r *http.Request, actor store.User) {
	var req struct {
		ID      int64 `json:"id"`
		IsAdmin bool  `json:"isAdmin"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request body")
		return
	}
	if req.ID == 0 {
		writeErr(w, http.StatusBadRequest, "user id required")
		return
	}
	// Guard against an admin removing their own admin rights and locking the
	// deployment out of user administration.
	if req.ID == actor.ID && !req.IsAdmin {
		writeErr(w, http.StatusBadRequest, "you cannot remove your own admin rights")
		return
	}
	target, ok, err := s.store.GetUserByID(req.ID)
	if err != nil {
		s.fail(w, err)
		return
	} else if !ok {
		writeErr(w, http.StatusNotFound, "user not found")
		return
	}
	// The protected owner's admin rights can never be removed by anyone.
	if target.IsOwner && !req.IsAdmin {
		writeErr(w, http.StatusForbidden, "the owner's admin rights cannot be removed")
		return
	}
	if err := s.store.SetUserAdmin(req.ID, req.IsAdmin); err != nil {
		s.fail(w, err)
		return
	}
	s.log.Info("admin updated user admin flag", "actor", actor.ID, "target", req.ID, "isAdmin", req.IsAdmin)
	writeJSON(w, map[string]bool{"ok": true})
}

// adminModeratedUser loads the target of a block/delete action and applies the
// shared guards: the target must exist, must not be the acting admin, and must
// not be the protected owner. Returns the target and false (after writing the
// error) when the action must not proceed.
func (s *Server) adminModeratedUser(w http.ResponseWriter, actor store.User, id int64, verb, verbPast string) (store.User, bool) {
	if id == 0 {
		writeErr(w, http.StatusBadRequest, "user id required")
		return store.User{}, false
	}
	if id == actor.ID {
		writeErr(w, http.StatusBadRequest, "you cannot "+verb+" your own account")
		return store.User{}, false
	}
	target, ok, err := s.store.GetUserByID(id)
	if err != nil {
		s.fail(w, err)
		return store.User{}, false
	}
	if !ok {
		writeErr(w, http.StatusNotFound, "user not found")
		return store.User{}, false
	}
	if target.IsOwner {
		writeErr(w, http.StatusForbidden, "the owner account cannot be "+verbPast)
		return store.User{}, false
	}
	return target, true
}

// adminBlockUser suspends or restores an account. A blocked account cannot log
// in and its active sessions are invalidated immediately.
func (s *Server) adminBlockUser(w http.ResponseWriter, r *http.Request, actor store.User) {
	var req struct {
		ID      int64 `json:"id"`
		Blocked bool  `json:"blocked"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request body")
		return
	}
	if _, ok := s.adminModeratedUser(w, actor, req.ID, "block", "blocked"); !ok {
		return
	}
	if err := s.store.SetUserBlocked(req.ID, req.Blocked); err != nil {
		s.fail(w, err)
		return
	}
	if req.Blocked {
		s.store.DeleteUserSessions(req.ID) // cut existing logins immediately
	}
	s.log.Info("admin set user blocked", "actor", actor.ID, "target", req.ID, "blocked", req.Blocked)
	writeJSON(w, map[string]bool{"ok": true})
}

// adminDeleteUser permanently removes an account and its sessions.
func (s *Server) adminDeleteUser(w http.ResponseWriter, r *http.Request, actor store.User) {
	var req struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request body")
		return
	}
	if _, ok := s.adminModeratedUser(w, actor, req.ID, "remove", "removed"); !ok {
		return
	}
	if err := s.store.DeleteUser(req.ID); err != nil {
		s.fail(w, err)
		return
	}
	s.log.Info("admin removed user", "actor", actor.ID, "target", req.ID)
	writeJSON(w, map[string]bool{"ok": true})
}

// startSession mints a session + CSRF token, sets the cookies, and returns the
// account. On error it responds 500.
func (s *Server) startSession(w http.ResponseWriter, r *http.Request, user store.User) {
	token, hash, err := auth.NewSessionToken()
	if err != nil {
		s.fail(w, err)
		return
	}
	csrf, err := auth.NewRandomToken(32)
	if err != nil {
		s.fail(w, err)
		return
	}
	if err := s.store.CreateSession(hash, user.ID, csrf, sessionTTL); err != nil {
		s.fail(w, err)
		return
	}
	secure := secureReq(r)
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: token, Path: "/",
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
		MaxAge: int(sessionTTL.Seconds()),
	})
	http.SetCookie(w, &http.Cookie{
		Name: csrfCookie, Value: csrf, Path: "/",
		HttpOnly: false, Secure: secure, SameSite: http.SameSiteLaxMode,
		MaxAge: int(sessionTTL.Seconds()),
	})
	writeJSON(w, s.authRespFor(user, csrf))
}

func (s *Server) clearSessionCookies(w http.ResponseWriter, r *http.Request) {
	secure := secureReq(r)
	for _, name := range []string{sessionCookie, csrfCookie} {
		http.SetCookie(w, &http.Cookie{
			Name: name, Value: "", Path: "/",
			HttpOnly: name == sessionCookie, Secure: secure, SameSite: http.SameSiteLaxMode,
			MaxAge: -1,
		})
	}
}

// secureReq reports whether the browser's connection is HTTPS. Behind the
// Cloudflare tunnel the origin sees plain HTTP but Cloudflare sets
// X-Forwarded-Proto; direct localhost (curl in tests) is treated as insecure so
// cookies are still stored without TLS.
func secureReq(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// dummyHash is a valid Argon2id PHC string used to equalise login timing when an
// email doesn't exist (so a full verify runs either way, preventing account
// enumeration by response time). Generated once at startup from a random
// password so it always parses and costs the same as a real verify.
var dummyHash = mustDummyHash()

func mustDummyHash() string {
	pw, err := auth.NewRandomToken(24)
	if err != nil {
		pw = "ridgeline-timing-equaliser-fallback"
	}
	h, err := auth.HashPassword(pw)
	if err != nil {
		return ""
	}
	return h
}
