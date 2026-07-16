package api

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jjkroell/ridgeline/internal/store"
)

// keyChallengeTTL bounds how long a private-key challenge stays signable. Short:
// the user pastes a key and signs immediately, all client-side.
const keyChallengeTTL = 5 * time.Minute

// keyChallengeStore holds pending private-key ownership challenges in memory,
// keyed by (user, node). Challenges are single-use and short-lived, so losing
// them on restart is harmless — the user just requests a new one.
type keyChallengeStore struct {
	mu sync.Mutex
	m  map[string]keyChallenge
}

type keyChallenge struct {
	challenge string
	expires   time.Time
}

func newKeyChallengeStore() *keyChallengeStore {
	return &keyChallengeStore{m: make(map[string]keyChallenge)}
}

func chalKey(userID int64, pubkey string) string {
	return fmt.Sprintf("%d|%s", userID, strings.ToUpper(pubkey))
}

// issue creates and stores a fresh challenge for (userID, pubkey), replacing any
// previous one. It opportunistically drops expired entries.
func (k *keyChallengeStore) issue(userID int64, pubkey, challenge string) {
	k.mu.Lock()
	defer k.mu.Unlock()
	now := time.Now()
	for key, c := range k.m {
		if now.After(c.expires) {
			delete(k.m, key)
		}
	}
	k.m[chalKey(userID, pubkey)] = keyChallenge{challenge: challenge, expires: now.Add(keyChallengeTTL)}
}

// take returns and removes the live challenge for (userID, pubkey). ok is false
// when there is none or it has expired.
func (k *keyChallengeStore) take(userID int64, pubkey string) (string, bool) {
	k.mu.Lock()
	defer k.mu.Unlock()
	key := chalKey(userID, pubkey)
	c, ok := k.m[key]
	if !ok {
		return "", false
	}
	delete(k.m, key)
	if time.Now().After(c.expires) {
		return "", false
	}
	return c.challenge, true
}

// claimKeyChallenge issues a random challenge for the caller to sign with the
// node's private key. Requires the same can-claim gate as the advert method.
func (s *Server) claimKeyChallenge(w http.ResponseWriter, r *http.Request, user store.User) {
	pubkey := strings.ToUpper(r.PathValue("pubkey"))
	if !validPubkey(pubkey) {
		writeErr(w, http.StatusBadRequest, "invalid node public key")
		return
	}
	if ok, err := s.store.NodeExists(pubkey); err != nil {
		s.fail(w, err)
		return
	} else if !ok {
		writeErr(w, http.StatusNotFound, "unknown node — Ridgeline hasn't heard this node yet")
		return
	}
	// Refuse if another user already owns it (mirrors CreateOrRefreshClaim).
	if owner, ok, err := s.store.NodeOwner(pubkey); err != nil {
		s.fail(w, err)
		return
	} else if ok && owner.UserID != user.ID {
		writeErr(w, http.StatusConflict, "this node is already claimed by another user")
		return
	}

	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		s.fail(w, err)
		return
	}
	// The exact bytes the client signs. Binding the pubkey + nonce stops a
	// signature being replayed onto a different node or reused.
	challenge := fmt.Sprintf("ridgeline-claim:%s:%s", pubkey, hex.EncodeToString(nonce))
	s.keyChal.issue(user.ID, pubkey, challenge)
	writeJSON(w, map[string]string{"challenge": challenge})
}

// claimKeyVerify checks a signature over the issued challenge against the node's
// public key. A valid signature proves the caller holds the node's private key,
// so ownership is recorded immediately — no advert or name change required.
func (s *Server) claimKeyVerify(w http.ResponseWriter, r *http.Request, user store.User) {
	pubkey := strings.ToUpper(r.PathValue("pubkey"))
	if !validPubkey(pubkey) {
		writeErr(w, http.StatusBadRequest, "invalid node public key")
		return
	}
	var req struct {
		Signature string `json:"signature"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request body")
		return
	}

	challenge, ok := s.keyChal.take(user.ID, pubkey)
	if !ok {
		writeErr(w, http.StatusBadRequest, "no active challenge — request one and sign it promptly")
		return
	}
	pubBytes, err := hex.DecodeString(pubkey)
	if err != nil || len(pubBytes) != ed25519.PublicKeySize {
		writeErr(w, http.StatusBadRequest, "invalid node public key")
		return
	}
	sig, err := hex.DecodeString(strings.TrimSpace(req.Signature))
	if err != nil || len(sig) != ed25519.SignatureSize {
		writeErr(w, http.StatusBadRequest, "signature must be 128 hex characters")
		return
	}
	if !ed25519.Verify(ed25519.PublicKey(pubBytes), []byte(challenge), sig) {
		writeErr(w, http.StatusUnauthorized, "signature did not match this node's key — check you pasted the right private key")
		return
	}

	claim, err := s.store.CreateVerifiedClaim(pubkey, user.ID)
	if err == store.ErrNodeClaimed {
		writeErr(w, http.StatusConflict, "this node is already claimed by another user")
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	s.log.Info("node claim verified by key", "node", pubkey, "user", user.ID)
	writeJSON(w, claim)
}
