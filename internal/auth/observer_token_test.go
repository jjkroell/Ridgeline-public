package auth

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"strconv"
	"strings"
	"testing"
	"time"
)

// mintToken reproduces what MeshCore's JWTHelper::createAuthToken emits:
// base64url(header) "." base64url(payload) "." UPPERCASE-HEX(signature), signed
// over the ASCII "header.payload". Keeping a local minter means these tests
// pin the wire format rather than just round-tripping our own verifier.
func mintToken(t *testing.T, priv ed25519.PrivateKey, payloadJSON string) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"Ed25519","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(payloadJSON))
	sig := ed25519.Sign(priv, []byte(header+"."+payload))
	return header + "." + payload + "." + strings.ToUpper(hex.EncodeToString(sig))
}

func testIdentity(t *testing.T) (pubHex string, priv ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	return strings.ToUpper(hex.EncodeToString(pub)), priv
}

func payloadFor(pubHex, aud string, exp int64) string {
	return `{"publicKey":"` + pubHex + `","aud":"` + aud +
		`","iat":1755000000,"exp":` + strconv.FormatInt(exp, 10) +
		`,"client":"meshcore/1.15-observer"}`
}

func TestVerifyObserverToken(t *testing.T) {
	pubHex, priv := testIdentity(t)
	now := time.Unix(1755000100, 0)
	aud := "mqtt.example.com"
	user := ObserverUsernamePrefix + pubHex

	t.Run("accepts a firmware-shaped token", func(t *testing.T) {
		tok := mintToken(t, priv, payloadFor(pubHex, aud, 1755086400))
		claims, err := VerifyObserverToken(user, tok, aud, now)
		if err != nil {
			t.Fatalf("want valid, got error: %v", err)
		}
		if claims.PublicKey != pubHex {
			t.Errorf("PublicKey = %q, want %q", claims.PublicKey, pubHex)
		}
	})

	t.Run("accepts a lowercase username", func(t *testing.T) {
		tok := mintToken(t, priv, payloadFor(pubHex, aud, 1755086400))
		if _, err := VerifyObserverToken(strings.ToLower(user), tok, aud, now); err != nil {
			t.Fatalf("want valid, got error: %v", err)
		}
	})

	t.Run("rejects a token signed by a different key", func(t *testing.T) {
		_, otherPriv := testIdentity(t)
		tok := mintToken(t, otherPriv, payloadFor(pubHex, aud, 1755086400))
		if _, err := VerifyObserverToken(user, tok, aud, now); err == nil {
			t.Fatal("want error for a signature from the wrong key, got nil")
		}
	})

	t.Run("rejects a username naming another observer", func(t *testing.T) {
		// The attack this exists to stop: a real node with a real key of its own
		// trying to connect as somebody else.
		otherHex, _ := testIdentity(t)
		tok := mintToken(t, priv, payloadFor(pubHex, aud, 1755086400))
		if _, err := VerifyObserverToken(ObserverUsernamePrefix+otherHex, tok, aud, now); err == nil {
			t.Fatal("want error for username/publicKey mismatch, got nil")
		}
	})

	t.Run("rejects a tampered payload", func(t *testing.T) {
		tok := mintToken(t, priv, payloadFor(pubHex, aud, 1755086400))
		parts := strings.Split(tok, ".")
		forged := base64.RawURLEncoding.EncodeToString(
			[]byte(payloadFor(pubHex, aud, 4000000000))) // push expiry out
		if _, err := VerifyObserverToken(user, parts[0]+"."+forged+"."+parts[2], aud, now); err == nil {
			t.Fatal("want error for a re-dated payload, got nil")
		}
	})

	t.Run("rejects the wrong audience", func(t *testing.T) {
		tok := mintToken(t, priv, payloadFor(pubHex, "mqtt-us-v1.letsmesh.net", 1755086400))
		if _, err := VerifyObserverToken(user, tok, aud, now); err == nil {
			t.Fatal("want error for a token minted for another broker, got nil")
		}
	})

	t.Run("rejects an expired token but allows clock leeway", func(t *testing.T) {
		expired := mintToken(t, priv, payloadFor(pubHex, aud, now.Add(-time.Hour).Unix()))
		if _, err := VerifyObserverToken(user, expired, aud, now); err == nil {
			t.Fatal("want error for an expired token, got nil")
		}
		recent := mintToken(t, priv, payloadFor(pubHex, aud, now.Add(-time.Minute).Unix()))
		if _, err := VerifyObserverToken(user, recent, aud, now); err != nil {
			t.Fatalf("want token just inside the leeway to pass, got: %v", err)
		}
	})

	t.Run("rejects a base64url signature", func(t *testing.T) {
		// i.e. a standards-compliant JWT. The firmware emits hex; accepting both
		// would mean accepting signatures we never verified the shape of.
		header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"Ed25519","typ":"JWT"}`))
		payload := base64.RawURLEncoding.EncodeToString([]byte(payloadFor(pubHex, aud, 1755086400)))
		sig := ed25519.Sign(priv, []byte(header+"."+payload))
		std := header + "." + payload + "." + base64.RawURLEncoding.EncodeToString(sig)
		if _, err := VerifyObserverToken(user, std, aud, now); err == nil {
			t.Fatal("want error for a base64url signature, got nil")
		}
	})

	t.Run("rejects alg none", func(t *testing.T) {
		header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
		payload := base64.RawURLEncoding.EncodeToString([]byte(payloadFor(pubHex, aud, 1755086400)))
		sig := ed25519.Sign(priv, []byte(header+"."+payload))
		tok := header + "." + payload + "." + strings.ToUpper(hex.EncodeToString(sig))
		if _, err := VerifyObserverToken(user, tok, aud, now); err == nil {
			t.Fatal(`want error for alg "none", got nil`)
		}
	})

	t.Run("rejects malformed input", func(t *testing.T) {
		for name, tok := range map[string]string{
			"empty":       "",
			"two parts":   "a.b",
			"four parts":  "a.b.c.d",
			"bad base64":  "!!!.!!!.00",
			"short sig":   mintToken(t, priv, payloadFor(pubHex, aud, 1755086400))[:40],
			"not a token": "hunter2",
		} {
			if _, err := VerifyObserverToken(user, tok, aud, now); err == nil {
				t.Errorf("%s: want error, got nil", name)
			}
		}
	})
}

func TestAuthorizeObserverTopic(t *testing.T) {
	const pk = "AABBCCDDEEFF00112233445566778899AABBCCDDEEFF00112233445566778899"
	other := strings.Repeat("11", 32)

	allowed := []string{
		"meshcore/AAA/" + pk + "/packets",
		"meshcore/AAA/" + pk + "/status",
		"meshcore/BBB/" + strings.ToLower(pk) + "/raw",
		"meshcore/AAA/" + pk + "/neighbors",
	}
	for _, topic := range allowed {
		if !AuthorizeObserverTopic(pk, topic, true) {
			t.Errorf("publish to own topic %q: want allowed", topic)
		}
	}

	denied := []string{
		"meshcore/AAA/" + other + "/packets", // another observer's identity
		"meshcore/AAA/+/packets",             // wildcard in the identity segment
		"meshcore/AAA/#",
		"#",
		"meshcore/AAA/" + pk, // too short to carry a type
		"other/AAA/" + pk + "/packets",
		"",
	}
	for _, topic := range denied {
		if AuthorizeObserverTopic(pk, topic, true) {
			t.Errorf("publish to %q: want denied", topic)
		}
	}

	if AuthorizeObserverTopic(pk, "meshcore/AAA/"+pk+"/packets", false) {
		t.Error("read access: want denied (observers only publish)")
	}
	if AuthorizeObserverTopic("", "meshcore/AAA/"+pk+"/packets", true) {
		t.Error("empty pubkey: want denied")
	}
}
