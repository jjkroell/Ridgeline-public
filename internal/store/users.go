package store

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

// ErrEmailTaken is returned by CreateUser when the email is already registered.
var ErrEmailTaken = errors.New("store: email already registered")

// User is a registered account. PasswordHash is never serialised to JSON.
type User struct {
	ID           int64  `json:"id"`
	Email        string `json:"email"`
	PasswordHash string `json:"-"`
	DisplayName  string `json:"displayName"`
	IsAdmin      bool   `json:"isAdmin"`
	CanClaim     bool   `json:"canClaim"`
	// Blocked accounts cannot log in and their sessions are invalidated.
	Blocked bool `json:"blocked"`
	// EmailVerified is set once the account confirms its address. New accounts
	// (except the bootstrap owner) start unverified and cannot log in until they do.
	EmailVerified bool `json:"emailVerified"`
	// IsOwner marks the protected initial admin: it cannot be demoted, blocked,
	// or removed by anyone (guarantees the deployment always keeps its owner).
	IsOwner   bool   `json:"isOwner"`
	CreatedAt string `json:"createdAt"`
	LastLogin string `json:"lastLogin,omitempty"`
}

// Session is a server-side login session keyed by the SHA-256 of the cookie
// token. The plaintext token lives only in the user's cookie.
type Session struct {
	TokenHash string
	UserID    int64
	CSRF      string
	CreatedAt string
	ExpiresAt string
	LastSeen  string
}

// UserCount returns the number of registered accounts (used to bootstrap the
// first account as admin).
func (s *Store) UserCount() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

// CreateUser inserts a new account. The email is lowercased and must be unique.
// The first account ever created is bootstrapped as admin with can_claim set, so
// the deployment has an owner who can grant claim rights to others. Returns
// ErrEmailTaken when the email is already registered.
func (s *Store) CreateUser(email, passwordHash, displayName string) (User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	now := time.Now().UTC().Format(time.RFC3339Nano)

	// Bootstrap: the first user is the owner/admin.
	n, err := s.UserCount()
	if err != nil {
		return User{}, err
	}
	admin := 0
	if n == 0 {
		admin = 1
	}

	// Every registered user may claim nodes (no admin approval needed). The
	// bootstrap account is additionally admin + the protected owner.
	// The bootstrap owner is auto-verified (there's no one to email a link to yet,
	// and the deployment must always have a usable owner account).
	res, err := s.db.Exec(`
		INSERT INTO users (email, password_hash, display_name, is_admin, can_claim, protected, email_verified, created_at)
		VALUES (?,?,?,?,1,?,?,?)`,
		email, passwordHash, nullStr(displayName), admin, admin, admin, now)
	if err != nil {
		if isUniqueViolation(err) {
			return User{}, ErrEmailTaken
		}
		return User{}, err
	}
	id, _ := res.LastInsertId()
	return User{
		ID: id, Email: email, PasswordHash: passwordHash, DisplayName: displayName,
		IsAdmin: admin == 1, CanClaim: true, IsOwner: admin == 1, EmailVerified: admin == 1, CreatedAt: now,
	}, nil
}

// GetUserByEmail looks up an account by (lowercased) email. ok is false when no
// such user exists.
func (s *Store) GetUserByEmail(email string) (User, bool, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	return s.scanUser(s.db.QueryRow(`
		SELECT id, email, password_hash, COALESCE(display_name,''), is_admin, can_claim,
		       blocked, protected, created_at, COALESCE(last_login,''), email_verified
		FROM users WHERE email = ?`, email))
}

// GetUserByID looks up an account by id. ok is false when no such user exists.
func (s *Store) GetUserByID(id int64) (User, bool, error) {
	return s.scanUser(s.db.QueryRow(`
		SELECT id, email, password_hash, COALESCE(display_name,''), is_admin, can_claim,
		       blocked, protected, created_at, COALESCE(last_login,''), email_verified
		FROM users WHERE id = ?`, id))
}

func (s *Store) scanUser(row *sql.Row) (User, bool, error) {
	var u User
	err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.DisplayName, &u.IsAdmin,
		&u.CanClaim, &u.Blocked, &u.IsOwner, &u.CreatedAt, &u.LastLogin, &u.EmailVerified)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, false, nil
	}
	if err != nil {
		return User{}, false, err
	}
	return u, true, nil
}

// SetUserLastLogin stamps a successful login time.
func (s *Store) SetUserLastLogin(id int64) error {
	_, err := s.db.Exec(`UPDATE users SET last_login = ? WHERE id = ?`,
		time.Now().UTC().Format(time.RFC3339Nano), id)
	return err
}

// ListUsers returns all accounts (admin view), newest first.
func (s *Store) ListUsers() ([]User, error) {
	rows, err := s.db.Query(`
		SELECT id, email, password_hash, COALESCE(display_name,''), is_admin, can_claim,
		       blocked, protected, created_at, COALESCE(last_login,''), email_verified
		FROM users ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.DisplayName, &u.IsAdmin,
			&u.CanClaim, &u.Blocked, &u.IsOwner, &u.CreatedAt, &u.LastLogin, &u.EmailVerified); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// UserBrief is a minimal public identity for pickers/autocomplete: just enough
// to show and select a registered user, never their email or flags.
type UserBrief struct {
	ID          int64  `json:"id"`
	DisplayName string `json:"displayName"`
}

// SearchUsersByName finds up to limit non-blocked users whose display name
// contains q (case-insensitive), for the share autocomplete. Users without a
// display name are excluded (they have no public handle to match). excludeID
// drops one user (typically the requester) from the results. Prefix matches rank
// ahead of mid-string matches.
func (s *Store) SearchUsersByName(q string, excludeID int64, limit int) ([]UserBrief, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil, nil
	}
	like := "%" + strings.ToLower(q) + "%"
	prefix := strings.ToLower(q) + "%"
	rows, err := s.db.Query(`
		SELECT id, display_name FROM users
		WHERE blocked = 0
		  AND display_name IS NOT NULL AND display_name != ''
		  AND id != ?
		  AND LOWER(display_name) LIKE ?
		ORDER BY (LOWER(display_name) LIKE ?) DESC, display_name COLLATE NOCASE
		LIMIT ?`, excludeID, like, prefix, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UserBrief
	for rows.Next() {
		var u UserBrief
		if err := rows.Scan(&u.ID, &u.DisplayName); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// SetDisplayName updates an account's public display name / callsign.
func (s *Store) SetDisplayName(id int64, name string) error {
	_, err := s.db.Exec(`UPDATE users SET display_name = ? WHERE id = ?`, nullStr(name), id)
	return err
}

// UpdatePassword replaces an account's password hash. Existing sessions stay
// valid (they aren't derived from the password); the caller decides whether to
// revoke them.
func (s *Store) UpdatePassword(id int64, passwordHash string) error {
	_, err := s.db.Exec(`UPDATE users SET password_hash = ? WHERE id = ?`, passwordHash, id)
	return err
}

// UpdateEmail changes an account's email and resets its verified flag, so the new
// address must be confirmed. The email is lowercased and must be unique; returns
// ErrEmailTaken on a collision.
func (s *Store) UpdateEmail(id int64, newEmail string) error {
	newEmail = strings.ToLower(strings.TrimSpace(newEmail))
	_, err := s.db.Exec(`UPDATE users SET email = ?, email_verified = 0 WHERE id = ?`, newEmail, id)
	if err != nil && isUniqueViolation(err) {
		return ErrEmailTaken
	}
	return err
}

// SetUserAdmin grants or revokes an account's admin rights (admin action).
// Claiming is universal, so can_claim is no longer an admin-managed flag.
func (s *Store) SetUserAdmin(id int64, isAdmin bool) error {
	_, err := s.db.Exec(`UPDATE users SET is_admin = ? WHERE id = ?`, boolInt(isAdmin), id)
	return err
}

// SetUserBlocked suspends (blocked=true) or restores (blocked=false) an account.
// Blocking also invalidates the user's active sessions so access is cut
// immediately; call DeleteUserSessions from the caller for that.
func (s *Store) SetUserBlocked(id int64, blocked bool) error {
	_, err := s.db.Exec(`UPDATE users SET blocked = ? WHERE id = ?`, boolInt(blocked), id)
	return err
}

// DeleteUser permanently removes an account and its sessions (the sessions FK is
// ON DELETE CASCADE, but this deletes them explicitly so it works regardless of
// pragma state).
func (s *Store) DeleteUser(id int64) error {
	s.db.Exec(`DELETE FROM sessions WHERE user_id = ?`, id)
	_, err := s.db.Exec(`DELETE FROM users WHERE id = ?`, id)
	return err
}

// DeleteUserAndReleaseNodes permanently removes an account and, for every node it
// verifiably owned, records ownerLabel as the node's previous owner (so the public
// page can show "previously owned by …") before the ownership claim is cascaded
// away. Deleting the user row cascades their claims, notes, private locations,
// location shares, and sessions (all FK ON DELETE CASCADE). Runs in one
// transaction so a node is never left both un-owned and un-stamped.
func (s *Store) DeleteUserAndReleaseNodes(id int64, ownerLabel string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() // no-op after a successful Commit

	// Stamp every node this user verifiably owned, before the cascade drops the
	// claim rows. Pending claims are not ownership and are intentionally skipped.
	rows, err := tx.Query(`SELECT node_pubkey FROM node_claims WHERE user_id = ? AND status = 'verified'`, id)
	if err != nil {
		return err
	}
	var owned []string
	for rows.Next() {
		var pk string
		if err := rows.Scan(&pk); err != nil {
			rows.Close()
			return err
		}
		owned = append(owned, pk)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, pk := range owned {
		if _, err := tx.Exec(`UPDATE nodes SET prev_owner_name = ? WHERE pubkey = ?`, ownerLabel, pk); err != nil {
			return err
		}
	}

	// Explicit session delete (as DeleteUser does) plus the user row; the user's
	// claims/notes/locations/shares cascade off the users FK.
	if _, err := tx.Exec(`DELETE FROM sessions WHERE user_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM users WHERE id = ?`, id); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	// The removed account may have held pending claims; refresh the ingest cache.
	s.loadPendingClaims()
	return nil
}

// DeleteUserSessions removes all of a user's login sessions (used when blocking
// so existing logins stop working immediately).
func (s *Store) DeleteUserSessions(userID int64) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE user_id = ?`, userID)
	return err
}

// --- Sessions ---

// CreateSession stores a session row. tokenHash is the SHA-256 of the cookie
// token; csrf is the double-submit token. The session expires at ttl from now.
func (s *Store) CreateSession(tokenHash string, userID int64, csrf string, ttl time.Duration) error {
	now := time.Now().UTC()
	_, err := s.db.Exec(`
		INSERT INTO sessions (token_hash, user_id, csrf, created_at, expires_at, last_seen)
		VALUES (?,?,?,?,?,?)`,
		tokenHash, userID, csrf,
		now.Format(time.RFC3339Nano),
		now.Add(ttl).Format(time.RFC3339Nano),
		now.Format(time.RFC3339Nano))
	return err
}

// SessionUser resolves a session token hash to its (unexpired) session and user.
// ok is false when the token is unknown or expired; an expired session is
// deleted opportunistically. It also refreshes last_seen.
func (s *Store) SessionUser(tokenHash string) (Session, User, bool, error) {
	var sess Session
	err := s.db.QueryRow(`
		SELECT token_hash, user_id, csrf, created_at, expires_at, last_seen
		FROM sessions WHERE token_hash = ?`, tokenHash).
		Scan(&sess.TokenHash, &sess.UserID, &sess.CSRF, &sess.CreatedAt, &sess.ExpiresAt, &sess.LastSeen)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, User{}, false, nil
	}
	if err != nil {
		return Session{}, User{}, false, err
	}
	if exp, perr := time.Parse(time.RFC3339Nano, sess.ExpiresAt); perr == nil && time.Now().After(exp) {
		s.db.Exec(`DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
		return Session{}, User{}, false, nil
	}
	user, ok, err := s.GetUserByID(sess.UserID)
	if err != nil || !ok {
		return Session{}, User{}, false, err
	}
	// A blocked account's sessions are void — drop this one and deny.
	if user.Blocked {
		s.db.Exec(`DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
		return Session{}, User{}, false, nil
	}
	// Best-effort activity stamp; ignore errors (a read path shouldn't fail here).
	s.db.Exec(`UPDATE sessions SET last_seen = ? WHERE token_hash = ?`,
		time.Now().UTC().Format(time.RFC3339Nano), tokenHash)
	return sess, user, true, nil
}

// DeleteSession removes a single session (logout).
func (s *Store) DeleteSession(tokenHash string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
	return err
}

// PruneSessions deletes sessions whose expiry is before the given RFC3339 time.
func (s *Store) PruneSessions(before string) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM sessions WHERE expires_at < ?`, before)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// isUniqueViolation reports whether err is a SQLite UNIQUE constraint failure
// (used to map a duplicate email to ErrEmailTaken).
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}
