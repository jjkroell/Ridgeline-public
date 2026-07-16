package store

import (
	"database/sql"
	"errors"
	"time"

	"github.com/jjkroell/ridgeline/internal/auth"
)

// emailVerifyTTL is how long an emailed verification link stays valid.
const emailVerifyTTL = 24 * time.Hour

// CreateEmailVerification issues a fresh verification token for a user, replacing
// any outstanding tokens for that user (so a resend invalidates older links).
// Returns the plaintext token to embed in the emailed link; only its hash is
// stored.
func (s *Store) CreateEmailVerification(userID int64) (string, error) {
	token, err := auth.NewRandomToken(32)
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	_, err = s.db.Exec(`DELETE FROM email_verifications WHERE user_id = ?`, userID)
	if err != nil {
		return "", err
	}
	_, err = s.db.Exec(`
		INSERT INTO email_verifications (token_hash, user_id, created_at, expires_at)
		VALUES (?,?,?,?)`,
		auth.HashToken(token), userID,
		now.Format(time.RFC3339Nano), now.Add(emailVerifyTTL).Format(time.RFC3339Nano))
	if err != nil {
		return "", err
	}
	return token, nil
}

// VerifyEmailToken consumes a verification token: if it is valid and unexpired,
// the associated account is marked verified and all its tokens are removed.
// Returns the verified user. ok is false for an unknown or expired token.
func (s *Store) VerifyEmailToken(token string) (User, bool, error) {
	if token == "" {
		return User{}, false, nil
	}
	hash := auth.HashToken(token)
	var userID int64
	var expiresAt string
	err := s.db.QueryRow(`SELECT user_id, expires_at FROM email_verifications WHERE token_hash = ?`, hash).
		Scan(&userID, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, false, nil
	}
	if err != nil {
		return User{}, false, err
	}
	if exp, perr := time.Parse(time.RFC3339Nano, expiresAt); perr == nil && time.Now().After(exp) {
		s.db.Exec(`DELETE FROM email_verifications WHERE token_hash = ?`, hash)
		return User{}, false, nil
	}
	if _, err := s.db.Exec(`UPDATE users SET email_verified = 1 WHERE id = ?`, userID); err != nil {
		return User{}, false, err
	}
	s.db.Exec(`DELETE FROM email_verifications WHERE user_id = ?`, userID)
	return s.GetUserByID(userID)
}

// MarkEmailVerified flags an account verified directly (used as a fallback when
// email delivery is not configured, so accounts aren't stranded).
func (s *Store) MarkEmailVerified(userID int64) error {
	_, err := s.db.Exec(`UPDATE users SET email_verified = 1 WHERE id = ?`, userID)
	return err
}

// PruneExpiredEmailVerifications deletes verification tokens past their expiry.
func (s *Store) PruneExpiredEmailVerifications() (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := s.db.Exec(`DELETE FROM email_verifications WHERE expires_at < ?`, now)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}
