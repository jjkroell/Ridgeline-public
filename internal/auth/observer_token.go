package auth

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ObserverUsernamePrefix is the MQTT username prefix MeshCore observer firmware
// connects with: "v1_" followed by the node's uppercase hex public key.
const ObserverUsernamePrefix = "v1_"

// ClockLeeway absorbs drift between a node's NTP-synced clock and ours when
// checking token expiry.
const ClockLeeway = 5 * time.Minute

// ObserverClaims are the fields MeshCore's firmware puts in an observer token.
// Note that PublicKey is a non-standard claim: the format predates any intent
// to be RFC-compliant and there is no "sub".
type ObserverClaims struct {
	PublicKey string `json:"publicKey"` // 64 uppercase hex chars
	Audience  string `json:"aud"`
	IssuedAt  int64  `json:"iat"`
	Expires   int64  `json:"exp"`   // absent (0) when the firmware was built without a lifetime
	Owner     string `json:"owner"` // optional: operator's companion-node pubkey
	Client    string `json:"client"`
	Email     string `json:"email"`
}

// VerifyObserverToken checks a MeshCore observer's MQTT password and returns the
// verified node public key (uppercase hex).
//
// The token LOOKS like a JWT but is not one, and no off-the-shelf library will
// parse it. Three deviations, all from src/helpers/JWTHelper.cpp in the
// firmware:
//
//   - the header's "alg" is "Ed25519", not RFC 8037's "EdDSA";
//   - the signature segment is 128 UPPERCASE HEX chars, not base64url;
//   - identity lives in a "publicKey" claim rather than "sub".
//
// It is signed over the ASCII "header.payload" with the node's own Ed25519 key,
// and the verifying key is carried INSIDE the token. So a valid signature does
// not prove the connection belongs to any particular observer — only that
// whoever opened it holds the private key for the public key they named. Two
// checks turn that into an identity, and BOTH are required:
//
//  1. here, that the MQTT username is "v1_" + that same public key;
//  2. at ACL time, that the topic the client publishes to carries that public
//     key (see AuthorizeObserverTopic).
//
// Drop either one and a node can publish under another observer's identity,
// which is exactly what this is meant to prevent.
func VerifyObserverToken(username, token, wantAudience string, now time.Time) (ObserverClaims, error) {
	var c ObserverClaims

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return c, errors.New("malformed token: want 3 dot-separated segments")
	}

	// The firmware strips base64 padding, so this must be the Raw encoding.
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return c, fmt.Errorf("decoding payload: %w", err)
	}
	if err := json.Unmarshal(payload, &c); err != nil {
		return c, fmt.Errorf("parsing claims: %w", err)
	}

	header, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return c, fmt.Errorf("decoding header: %w", err)
	}
	var h struct {
		Alg string `json:"alg"`
		Typ string `json:"typ"`
	}
	if err := json.Unmarshal(header, &h); err != nil {
		return c, fmt.Errorf("parsing header: %w", err)
	}
	// Not load-bearing (the signature is always checked as Ed25519 below), but
	// refusing anything else keeps an "alg":"none" token from ever being read.
	if h.Alg != "Ed25519" {
		return c, fmt.Errorf("unsupported alg %q", h.Alg)
	}

	pub, err := hex.DecodeString(c.PublicKey)
	if err != nil {
		return c, fmt.Errorf("decoding publicKey: %w", err)
	}
	if len(pub) != ed25519.PublicKeySize {
		return c, fmt.Errorf("publicKey is %d bytes, want %d", len(pub), ed25519.PublicKeySize)
	}

	sig, err := hex.DecodeString(parts[2]) // uppercase hex, NOT base64url
	if err != nil {
		return c, fmt.Errorf("decoding signature: %w", err)
	}
	if len(sig) != ed25519.SignatureSize {
		return c, fmt.Errorf("signature is %d bytes, want %d", len(sig), ed25519.SignatureSize)
	}

	signingInput := []byte(parts[0] + "." + parts[1])
	if !ed25519.Verify(ed25519.PublicKey(pub), signingInput, sig) {
		return c, errors.New("signature does not verify")
	}

	// Binding check 1: the username must name the key that just signed.
	if !strings.EqualFold(username, ObserverUsernamePrefix+c.PublicKey) {
		return c, errors.New("username does not match the token's publicKey")
	}

	// A token minted for another broker must not be replayable here.
	if wantAudience != "" && !strings.EqualFold(c.Audience, wantAudience) {
		return c, fmt.Errorf("wrong audience %q", c.Audience)
	}

	// exp is absent when the firmware was built without a token lifetime; the
	// shipping builds always set one (24h default) and renew ahead of expiry.
	// A node whose NTP sync failed issues a 1970-epoch token, which lands here
	// as long-expired — deliberately, since we cannot date an unsynced claim.
	if c.Expires != 0 && now.After(time.Unix(c.Expires, 0).Add(ClockLeeway)) {
		return c, fmt.Errorf("token expired at %s", time.Unix(c.Expires, 0).UTC().Format(time.RFC3339))
	}

	c.PublicKey = strings.ToUpper(c.PublicKey)
	return c, nil
}

// AuthorizeObserverTopic reports whether a client authenticated as pubkey may
// use topic for the given access.
//
// MeshCore observers publish to meshcore/{IATA}/{PUBKEY}/{status,packets,raw,
// neighbors}, so the third segment is the identity claim being made. This is
// binding check 2 from VerifyObserverToken: without it a node with a valid
// token of its own could publish under any other observer's public key.
func AuthorizeObserverTopic(pubkey, topic string, write bool) bool {
	if pubkey == "" {
		return false
	}
	segs := strings.Split(topic, "/")
	if len(segs) < 4 || segs[0] != "meshcore" {
		return false
	}
	// Observers only ever publish; they have no reason to read the mesh feed,
	// and wildcards must never resolve to someone else's subtree.
	if !write {
		return false
	}
	if strings.ContainsAny(segs[2], "+#") {
		return false
	}
	return strings.EqualFold(segs[2], pubkey)
}
