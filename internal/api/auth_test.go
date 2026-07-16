package api

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/jjkroell/ridgeline/internal/store"
)

// authTestClient is one browser: an httptest server-backed client with its own
// cookie jar (so each user keeps separate session cookies).
type authTestClient struct {
	t    *testing.T
	base string
	http *http.Client
	csrf string // last CSRF token learned from an auth response
}

func newAuthEnv(t *testing.T) (*store.Store, string, func()) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "api.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	srv := New(st, slog.New(slog.NewTextHandler(io.Discard, nil)), "test", "")
	ts := httptest.NewServer(srv.Handler())
	return st, ts.URL, func() { ts.Close(); st.Close() }
}

func newClient(t *testing.T, base string) *authTestClient {
	jar, _ := cookiejar.New(nil)
	return &authTestClient{t: t, base: base, http: &http.Client{Jar: jar}}
}

func (c *authTestClient) do(method, path string, body any, csrf bool) (*http.Response, map[string]any) {
	c.t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.base+path, r)
	if err != nil {
		c.t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if csrf {
		req.Header.Set(csrfHeader, c.csrf)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatalf("do %s %s: %v", method, path, err)
	}
	var out map[string]any
	dec, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	json.Unmarshal(dec, &out)
	// Learn CSRF token from auth responses.
	if tok, ok := out["csrfToken"].(string); ok && tok != "" {
		c.csrf = tok
	}
	return resp, out
}

// user extracts the "user" object from an auth response body (nil if absent).
func (c *authTestClient) user(body map[string]any) map[string]any {
	u, _ := body["user"].(map[string]any)
	return u
}

func TestAuthFlow(t *testing.T) {
	_, base, cleanup := newAuthEnv(t)
	defer cleanup()

	owner := newClient(t, base)

	// Unauthenticated /me returns a null user.
	if _, body := owner.do("GET", "/api/auth/me", nil, false); body["user"] != nil {
		t.Fatalf("expected null user before login, got %v", body["user"])
	}

	// Register the first account — bootstrapped as admin.
	resp, body := owner.do("POST", "/api/auth/register",
		map[string]string{"email": "owner@example.com", "password": "hunter2hunter2", "displayName": "Owner"}, false)
	if resp.StatusCode != 200 {
		t.Fatalf("register owner: status %d body %v", resp.StatusCode, body)
	}
	u, _ := body["user"].(map[string]any)
	if u == nil || u["isAdmin"] != true || u["canClaim"] != true {
		t.Fatalf("first user should be admin+canClaim, got %v", u)
	}
	if owner.csrf == "" {
		t.Fatal("register did not return a CSRF token")
	}

	// The session cookie now authenticates /me.
	if _, body := owner.do("GET", "/api/auth/me", nil, false); body["user"] == nil {
		t.Fatal("expected authenticated user from /me after register")
	}

	// Weak password is rejected.
	if resp, _ := owner.do("POST", "/api/auth/register",
		map[string]string{"email": "x@example.com", "password": "short"}, false); resp.StatusCode != 400 {
		t.Errorf("weak password should be 400, got %d", resp.StatusCode)
	}

	// Duplicate email is a conflict.
	dup := newClient(t, base)
	if resp, _ := dup.do("POST", "/api/auth/register",
		map[string]string{"email": "Owner@example.com", "password": "hunter2hunter2"}, false); resp.StatusCode != 409 {
		t.Errorf("duplicate email should be 409, got %d", resp.StatusCode)
	}

	// A second account is a plain member (not admin) but can claim from the start.
	member := newClient(t, base)
	_, mbody := member.do("POST", "/api/auth/register",
		map[string]string{"email": "member@example.com", "password": "hunter2hunter2"}, false)
	mu, _ := mbody["user"].(map[string]any)
	if mu["isAdmin"] == true {
		t.Errorf("second user should not be admin, got %v", mu)
	}
	if mu["canClaim"] != true {
		t.Errorf("every user should be able to claim, got %v", mu)
	}
	memberID := int64(mu["id"].(float64))

	// Wrong-password login fails with a generic error.
	fresh := newClient(t, base)
	if resp, _ := fresh.do("POST", "/api/auth/login",
		map[string]string{"email": "owner@example.com", "password": "wrongwrongwrong"}, false); resp.StatusCode != 401 {
		t.Errorf("bad login should be 401, got %d", resp.StatusCode)
	}
	// Correct login succeeds and establishes a session.
	if resp, _ := fresh.do("POST", "/api/auth/login",
		map[string]string{"email": "owner@example.com", "password": "hunter2hunter2"}, false); resp.StatusCode != 200 {
		t.Errorf("good login should be 200, got %d", resp.StatusCode)
	}

	// --- Authorization + CSRF on a protected mutating endpoint ---

	// A member cannot reach the admin user list.
	if resp, _ := member.do("GET", "/api/admin/users", nil, false); resp.StatusCode != 403 {
		t.Errorf("member should get 403 on admin users, got %d", resp.StatusCode)
	}
	// The admin can.
	if resp, _ := owner.do("GET", "/api/admin/users", nil, false); resp.StatusCode != 200 {
		t.Errorf("admin should get 200 on admin users, got %d", resp.StatusCode)
	}

	// Mutating admin call WITHOUT the CSRF header is rejected.
	if resp, _ := owner.do("POST", "/api/admin/users/flags",
		map[string]any{"id": memberID, "isAdmin": true}, false); resp.StatusCode != 403 {
		t.Errorf("missing CSRF should be 403, got %d", resp.StatusCode)
	}
	// WITH the CSRF header it succeeds and grants admin.
	if resp, _ := owner.do("POST", "/api/admin/users/flags",
		map[string]any{"id": memberID, "isAdmin": true}, true); resp.StatusCode != 200 {
		t.Errorf("valid CSRF should be 200, got %d", resp.StatusCode)
	}
	if _, mb := member.do("GET", "/api/auth/me", nil, false); mb["user"].(map[string]any)["isAdmin"] != true {
		t.Error("member should be admin after the grant")
	}

	// Logout clears the session.
	owner.do("POST", "/api/auth/logout", nil, false)
	if _, body := owner.do("GET", "/api/auth/me", nil, false); body["user"] != nil {
		t.Error("expected null user after logout")
	}
	if resp, _ := owner.do("GET", "/api/admin/users", nil, false); resp.StatusCode != 401 {
		t.Errorf("after logout admin route should be 401, got %d", resp.StatusCode)
	}
}

func TestAdminModeration(t *testing.T) {
	_, base, cleanup := newAuthEnv(t)
	defer cleanup()

	// Owner (first account) + a member we'll promote to admin + a victim.
	owner := newClient(t, base)
	_, ob := owner.do("POST", "/api/auth/register",
		map[string]string{"email": "owner@example.com", "password": "hunter2hunter2"}, false)
	if owner.user(ob)["isOwner"] != true {
		t.Fatalf("first account should be the protected owner, got %v", owner.user(ob))
	}
	ownerID := int64(owner.user(ob)["id"].(float64))

	member := newClient(t, base)
	_, mb := member.do("POST", "/api/auth/register",
		map[string]string{"email": "member@example.com", "password": "hunter2hunter2"}, false)
	memberID := int64(member.user(mb)["id"].(float64))

	victim := newClient(t, base)
	_, vb := victim.do("POST", "/api/auth/register",
		map[string]string{"email": "victim@example.com", "password": "hunter2hunter2"}, false)
	victimID := int64(victim.user(vb)["id"].(float64))

	// Owner promotes member to admin.
	if resp, _ := owner.do("POST", "/api/admin/users/flags",
		map[string]any{"id": memberID, "isAdmin": true, "canClaim": false}, true); resp.StatusCode != 200 {
		t.Fatalf("promote member: %d", resp.StatusCode)
	}
	member.do("GET", "/api/auth/me", nil, false) // refresh member's admin state + csrf

	// Another admin (member) CANNOT touch the protected owner.
	for _, tc := range []struct {
		path string
		body map[string]any
	}{
		{"/api/admin/users/flags", map[string]any{"id": ownerID, "isAdmin": false, "canClaim": true}},
		{"/api/admin/users/block", map[string]any{"id": ownerID, "blocked": true}},
		{"/api/admin/users/delete", map[string]any{"id": ownerID}},
	} {
		if resp, _ := member.do("POST", tc.path, tc.body, true); resp.StatusCode != 403 {
			t.Errorf("admin acting on owner via %s should be 403, got %d", tc.path, resp.StatusCode)
		}
	}
	// Owner still admin + owner after the attempts.
	if _, b := owner.do("GET", "/api/auth/me", nil, false); owner.user(b)["isAdmin"] != true {
		t.Error("owner should still be admin")
	}

	// Block the victim: existing session dies, fresh login is refused.
	if resp, _ := owner.do("POST", "/api/admin/users/block",
		map[string]any{"id": victimID, "blocked": true}, true); resp.StatusCode != 200 {
		t.Fatalf("block victim: %d", resp.StatusCode)
	}
	if _, b := victim.do("GET", "/api/auth/me", nil, false); b["user"] != nil {
		t.Error("blocked user's existing session should be void")
	}
	relog := newClient(t, base)
	if resp, _ := relog.do("POST", "/api/auth/login",
		map[string]string{"email": "victim@example.com", "password": "hunter2hunter2"}, false); resp.StatusCode != 403 {
		t.Errorf("blocked user login should be 403, got %d", resp.StatusCode)
	}
	// Unblock → can log in again.
	owner.do("POST", "/api/admin/users/block", map[string]any{"id": victimID, "blocked": false}, true)
	if resp, _ := relog.do("POST", "/api/auth/login",
		map[string]string{"email": "victim@example.com", "password": "hunter2hunter2"}, false); resp.StatusCode != 200 {
		t.Errorf("unblocked user login should be 200, got %d", resp.StatusCode)
	}

	// Self-guards: owner cannot block or delete their own account.
	if resp, _ := owner.do("POST", "/api/admin/users/block", map[string]any{"id": ownerID, "blocked": true}, true); resp.StatusCode != 400 {
		t.Errorf("self-block should be 400, got %d", resp.StatusCode)
	}
	if resp, _ := owner.do("POST", "/api/admin/users/delete", map[string]any{"id": ownerID}, true); resp.StatusCode != 400 {
		t.Errorf("self-delete should be 400, got %d", resp.StatusCode)
	}

	// Delete the victim for good.
	if resp, _ := owner.do("POST", "/api/admin/users/delete", map[string]any{"id": victimID}, true); resp.StatusCode != 200 {
		t.Fatalf("delete victim: %d", resp.StatusCode)
	}
	if resp, _ := relog.do("POST", "/api/auth/login",
		map[string]string{"email": "victim@example.com", "password": "hunter2hunter2"}, false); resp.StatusCode != 401 {
		t.Errorf("deleted user login should be 401, got %d", resp.StatusCode)
	}
}
