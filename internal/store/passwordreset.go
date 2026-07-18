package store

import (
	"database/sql"
	"errors"
	"time"

	"github.com/jjkroell/ridgeline/internal/auth"
)

// passwordResetTTL is how long an emailed password-reset link stays valid. Kept
// short (1h) — a reset link is a live credential to take over the account, so it
// should not linger the way a verification link can.
const passwordResetTTL = time.Hour

// CreatePasswordReset issues a fresh reset token for a user, replacing any
// outstanding tokens for that user (so a new request invalidates older links).
// Returns the plaintext token to embed in the emailed link; only its hash is
// stored.
func (s *Store) CreatePasswordReset(userID int64) (string, error) {
	token, err := auth.NewRandomToken(32)
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	if _, err := s.db.Exec(`DELETE FROM password_resets WHERE user_id = ?`, userID); err != nil {
		return "", err
	}
	_, err = s.db.Exec(`
		INSERT INTO password_resets (token_hash, user_id, created_at, expires_at)
		VALUES (?,?,?,?)`,
		auth.HashToken(token), userID,
		now.Format(time.RFC3339Nano), now.Add(passwordResetTTL).Format(time.RFC3339Nano))
	if err != nil {
		return "", err
	}
	return token, nil
}

// ConsumePasswordReset validates and consumes a reset token. If it is valid and
// unexpired, ALL of the user's reset tokens are removed (single use) and the
// associated user is returned. ok is false for an unknown or expired token. The
// caller is responsible for setting the new password and revoking sessions.
func (s *Store) ConsumePasswordReset(token string) (User, bool, error) {
	if token == "" {
		return User{}, false, nil
	}
	hash := auth.HashToken(token)
	var userID int64
	var expiresAt string
	err := s.db.QueryRow(`SELECT user_id, expires_at FROM password_resets WHERE token_hash = ?`, hash).
		Scan(&userID, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, false, nil
	}
	if err != nil {
		return User{}, false, err
	}
	if exp, perr := time.Parse(time.RFC3339Nano, expiresAt); perr == nil && time.Now().After(exp) {
		s.db.Exec(`DELETE FROM password_resets WHERE token_hash = ?`, hash)
		return User{}, false, nil
	}
	// Consume every reset token for this user before returning, so the token can't
	// be replayed even if the subsequent password update fails.
	if _, err := s.db.Exec(`DELETE FROM password_resets WHERE user_id = ?`, userID); err != nil {
		return User{}, false, err
	}
	return s.GetUserByID(userID)
}

// PruneExpiredPasswordResets deletes reset tokens past their expiry.
func (s *Store) PruneExpiredPasswordResets() (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := s.db.Exec(`DELETE FROM password_resets WHERE expires_at < ?`, now)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}
