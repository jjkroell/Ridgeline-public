package api

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/jjkroell/ridgeline/internal/meshcore"
	"github.com/jjkroell/ridgeline/internal/store"
)

// End-to-end private-key ownership proof: a user requests a challenge, signs it
// with the node's Ed25519 private key, and the server verifies the signature and
// records ownership — no advert or name change involved.
func TestClaimByPrivateKey(t *testing.T) {
	st, base, cleanup := newAuthEnv(t)
	defer cleanup()

	// A node whose private key we control.
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	pubHex := strings.ToUpper(hex.EncodeToString(pub))
	pkt := &meshcore.Packet{
		MessageHash: "0a1b2c3d",
		Advert:      &meshcore.Advert{PublicKey: pubHex, HasName: true, Name: "Test Repeater", SignatureValid: true},
	}
	if err := st.Record(store.Observation{Packet: pkt, RawHex: "00", ReceivedAt: time.Now()}); err != nil {
		t.Fatalf("seed node: %v", err)
	}

	user := newClient(t, base) // first account = admin, may claim
	user.do("POST", "/api/auth/register",
		map[string]string{"email": "owner@example.com", "password": "hunter2hunter2"}, false)

	chalPath := "/api/nodes/" + pubHex + "/claim/key-challenge"
	verifyPath := "/api/nodes/" + pubHex + "/claim/key-verify"

	// Unknown node → 404.
	unknown := strings.Repeat("AB", 32)
	if resp, _ := user.do("POST", "/api/nodes/"+unknown+"/claim/key-challenge", nil, true); resp.StatusCode != 404 {
		t.Errorf("challenge for unknown node should be 404, got %d", resp.StatusCode)
	}

	// Verify with no active challenge → 400.
	if resp, _ := user.do("POST", verifyPath, map[string]string{"signature": strings.Repeat("00", 64)}, true); resp.StatusCode != 400 {
		t.Errorf("verify without challenge should be 400, got %d", resp.StatusCode)
	}

	// Wrong key → 401 (and consumes the challenge, proving single-use).
	_, cb := user.do("POST", chalPath, nil, true)
	challenge, _ := cb["challenge"].(string)
	if challenge == "" {
		t.Fatal("no challenge issued")
	}
	_, wrongPriv, _ := ed25519.GenerateKey(rand.Reader)
	badSig := hex.EncodeToString(ed25519.Sign(wrongPriv, []byte(challenge)))
	if resp, _ := user.do("POST", verifyPath, map[string]string{"signature": badSig}, true); resp.StatusCode != 401 {
		t.Errorf("wrong-key signature should be 401, got %d", resp.StatusCode)
	}
	// The consumed challenge can't be reused.
	goodSigStale := hex.EncodeToString(ed25519.Sign(priv, []byte(challenge)))
	if resp, _ := user.do("POST", verifyPath, map[string]string{"signature": goodSigStale}, true); resp.StatusCode != 400 {
		t.Errorf("reusing a consumed challenge should be 400, got %d", resp.StatusCode)
	}

	// Correct key over a fresh challenge → 200 + ownership.
	_, cb2 := user.do("POST", chalPath, nil, true)
	challenge2 := cb2["challenge"].(string)
	goodSig := hex.EncodeToString(ed25519.Sign(priv, []byte(challenge2)))
	if resp, body := user.do("POST", verifyPath, map[string]string{"signature": goodSig}, true); resp.StatusCode != 200 {
		t.Fatalf("valid signature should verify, got %d body=%v", resp.StatusCode, body)
	} else if body["status"] != "verified" {
		t.Fatalf("claim should be verified, got %v", body["status"])
	}

	// Status endpoint now reports ownership by this user.
	_, sb := user.do("GET", "/api/nodes/"+pubHex+"/claim", nil, false)
	if sb["ownedByMe"] != true {
		t.Errorf("node should be owned by the caller, got %v", sb["ownedByMe"])
	}
}
