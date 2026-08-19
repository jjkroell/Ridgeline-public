package api

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jjkroell/ridgeline/internal/store"
)

const testAudience = "mqtt.example.com"

// newMQTTAuthEnv brings up a server with observer auth configured, mirroring
// what the JWT broker talks to.
func newMQTTAuthEnv(t *testing.T) (string, func()) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "api.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	srv := New(st, slog.New(slog.NewTextHandler(io.Discard, nil)), "test", "")
	srv.SetMQTTAuth(MQTTAuthConfig{
		Audience:         testAudience,
		ConsumerUsername: "ridgelined",
		ConsumerPassword: "consumer-secret",
	})
	ts := httptest.NewServer(srv.Handler())
	return ts.URL, func() { ts.Close(); st.Close() }
}

// post speaks mosquitto-go-auth's json params mode and decodes its json
// response mode: a 2XX plus {Ok, Error}, where BOTH must be satisfied.
func post(t *testing.T, base, path string, body any) (int, mqttAuthReply) {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp, err := http.Post(base+path, "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("post %s: %v", path, err)
	}
	defer resp.Body.Close()
	var out mqttAuthReply
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func observerToken(t *testing.T, priv ed25519.PrivateKey, pubHex, aud string, exp time.Time) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"Ed25519","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(
		`{"publicKey":"` + pubHex + `","aud":"` + aud +
			`","iat":` + strconv.FormatInt(time.Now().Add(-time.Minute).Unix(), 10) +
			`,"exp":` + strconv.FormatInt(exp.Unix(), 10) + `}`))
	sig := ed25519.Sign(priv, []byte(header+"."+payload))
	return header + "." + payload + "." + strings.ToUpper(hex.EncodeToString(sig))
}

func newObserver(t *testing.T) (pubHex string, priv ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	return strings.ToUpper(hex.EncodeToString(pub)), priv
}

func TestMQTTAuthUser(t *testing.T) {
	base, done := newMQTTAuthEnv(t)
	defer done()

	pubHex, priv := newObserver(t)
	user := "v1_" + pubHex
	valid := observerToken(t, priv, pubHex, testAudience, time.Now().Add(24*time.Hour))

	t.Run("valid observer is allowed", func(t *testing.T) {
		code, reply := post(t, base, "/api/mqtt-auth/user", map[string]any{
			"username": user, "password": valid, "clientid": "obs-1",
		})
		if code != http.StatusOK || !reply.Ok {
			t.Fatalf("got status=%d ok=%v err=%q, want 200/true", code, reply.Ok, reply.Error)
		}
	})

	t.Run("denials are 200 with Ok=false so the reason reaches the broker log", func(t *testing.T) {
		code, reply := post(t, base, "/api/mqtt-auth/user", map[string]any{
			"username": user, "password": "garbage", "clientid": "obs-1",
		})
		if code != http.StatusOK {
			t.Fatalf("status = %d, want 200", code)
		}
		if reply.Ok {
			t.Fatal("Ok = true for a garbage token, want false")
		}
		if reply.Error == "" {
			t.Error("Error is empty; the denial reason should be reported")
		}
	})

	t.Run("token for another broker is refused", func(t *testing.T) {
		other := observerToken(t, priv, pubHex, "mqtt-us-v1.letsmesh.net", time.Now().Add(24*time.Hour))
		if _, reply := post(t, base, "/api/mqtt-auth/user", map[string]any{
			"username": user, "password": other, "clientid": "obs-1",
		}); reply.Ok {
			t.Fatal("Ok = true for a token minted for another audience")
		}
	})

	t.Run("cannot connect as another observer", func(t *testing.T) {
		otherHex, _ := newObserver(t)
		if _, reply := post(t, base, "/api/mqtt-auth/user", map[string]any{
			"username": "v1_" + otherHex, "password": valid, "clientid": "obs-1",
		}); reply.Ok {
			t.Fatal("Ok = true when the username named a different observer")
		}
	})

	t.Run("ingest consumer authenticates by password", func(t *testing.T) {
		if _, reply := post(t, base, "/api/mqtt-auth/user", map[string]any{
			"username": "ridgelined", "password": "consumer-secret", "clientid": "ridgelined",
		}); !reply.Ok {
			t.Fatalf("consumer rejected: %q", reply.Error)
		}
		if _, reply := post(t, base, "/api/mqtt-auth/user", map[string]any{
			"username": "ridgelined", "password": "wrong", "clientid": "ridgelined",
		}); reply.Ok {
			t.Fatal("Ok = true for the consumer with a wrong password")
		}
	})
}

func TestMQTTAuthACL(t *testing.T) {
	base, done := newMQTTAuthEnv(t)
	defer done()

	pubHex, _ := newObserver(t)
	otherHex, _ := newObserver(t)
	user := "v1_" + pubHex

	cases := []struct {
		name  string
		user  string
		topic string
		acc   int
		want  bool
	}{
		{"publish to own packets topic", user, "meshcore/AAA/" + pubHex + "/packets", mosqACLWrite, true},
		{"publish to own status topic", user, "meshcore/AAA/" + pubHex + "/status", mosqACLWrite, true},
		{"publish under another observer", user, "meshcore/AAA/" + otherHex + "/packets", mosqACLWrite, false},
		{"publish to a wildcard identity", user, "meshcore/AAA/+/packets", mosqACLWrite, false},
		{"subscribe to everything", user, "meshcore/#", mosqACLSubscribe, false},
		{"read another observer", user, "meshcore/AAA/" + otherHex + "/packets", mosqACLRead, false},
		{"consumer subscribes across observers", "ridgelined", "meshcore/+/+/packets", mosqACLSubscribe, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, reply := post(t, base, "/api/mqtt-auth/acl", map[string]any{
				"username": tc.user, "clientid": "c", "topic": tc.topic, "acc": tc.acc,
			})
			if reply.Ok != tc.want {
				t.Fatalf("Ok = %v, want %v (err=%q)", reply.Ok, tc.want, reply.Error)
			}
		})
	}
}

func TestMQTTAuthSuperuser(t *testing.T) {
	base, done := newMQTTAuthEnv(t)
	defer done()

	pubHex, _ := newObserver(t)
	if _, reply := post(t, base, "/api/mqtt-auth/superuser", map[string]any{
		"username": "ridgelined",
	}); !reply.Ok {
		t.Error("consumer should be superuser so it can subscribe across observers")
	}
	if _, reply := post(t, base, "/api/mqtt-auth/superuser", map[string]any{
		"username": "v1_" + pubHex,
	}); reply.Ok {
		t.Error("an observer must never be a superuser")
	}
}

// Deployments running only the anonymous broker must not expose these at all.
func TestMQTTAuthDisabledWithoutAudience(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "api.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	srv := New(st, slog.New(slog.NewTextHandler(io.Discard, nil)), "test", "")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	for _, path := range []string{"/api/mqtt-auth/user", "/api/mqtt-auth/superuser", "/api/mqtt-auth/acl"} {
		code, _ := post(t, ts.URL, path, map[string]any{"username": "x"})
		if code != http.StatusNotFound {
			t.Errorf("%s: status = %d, want 404 when no audience is configured", path, code)
		}
	}
}
