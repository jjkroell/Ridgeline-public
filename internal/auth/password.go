// Package auth provides password hashing and session-token helpers for
// Ridgeline's user accounts. Passwords are hashed with Argon2id and stored as
// self-describing PHC strings; sessions are opaque random tokens whose SHA-256
// is what the store persists (so a database leak never yields a usable token).
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters. These target ~50-60ms per hash on a modern CPU, which is
// a reasonable balance for an interactive login on a low-volume community site.
const (
	argonTime    = 3         // iterations
	argonMemory  = 64 * 1024 // 64 MiB
	argonThreads = 2
	argonKeyLen  = 32
	argonSaltLen = 16
)

// HashPassword returns a PHC-format Argon2id hash of the password:
//
//	$argon2id$v=19$m=65536,t=3,p=2$<b64 salt>$<b64 hash>
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	b64 := base64.RawStdEncoding.EncodeToString
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads, b64(salt), b64(key)), nil
}

// VerifyPassword reports whether password matches the stored PHC hash. It
// recomputes with the hash's own parameters and compares in constant time.
func VerifyPassword(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	// ["", "argon2id", "v=19", "m=..,t=..,p=..", salt, hash]
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false
	}
	var mem, time uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &mem, &time, &threads); err != nil {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, time, mem, threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

// NewSessionToken returns a fresh opaque session token (URL-safe base64 of 32
// random bytes) alongside its SHA-256 hex digest. The plaintext token goes in
// the user's cookie; only the digest is stored, so the DB never holds anything
// that could be replayed as a session.
func NewSessionToken() (token, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, HashToken(token), nil
}

// HashToken returns the SHA-256 hex digest of a session (or CSRF) token.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// NewRandomToken returns a URL-safe random token of n bytes of entropy — used
// for the double-submit CSRF token (which is not hashed, just compared).
func NewRandomToken(n int) (string, error) {
	if n <= 0 {
		return "", errors.New("auth: token length must be positive")
	}
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// claimCodeAlphabet excludes visually ambiguous characters (0/O, 1/I/L) so a
// node-ownership code is easy to read and type into a node's name field.
const claimCodeAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

// NewClaimCode returns a short, unambiguous uppercase code (6 chars) for the
// node-ownership challenge. ~31^6 ≈ 887M combinations — collision with real
// name text is negligible, and it's short enough to fit a capped advert name.
func NewClaimCode() (string, error) {
	const n = 6
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	out := make([]byte, n)
	for i := range b {
		out[i] = claimCodeAlphabet[int(b[i])%len(claimCodeAlphabet)]
	}
	return string(out), nil
}
