package api

import (
	"net/http"
	"testing"
)

// TestPasswordResetFlow drives a reset end to end: forgot is non-enumerating,
// a valid token sets a new password + logs the user in, the old password stops
// working, and the account's prior sessions are revoked.
func TestPasswordResetFlow(t *testing.T) {
	st, base, cleanup := newAuthEnv(t)
	defer cleanup()

	// Owner (bootstrap) then a member. Email is disabled in tests, so both are
	// auto-verified and the member client ends up logged in.
	newClient(t, base).do("POST", "/api/auth/register",
		map[string]string{"email": "owner@example.com", "password": "ownerpass1", "displayName": "Owner"}, false)
	member := newClient(t, base)
	member.do("POST", "/api/auth/register",
		map[string]string{"email": "member@example.com", "password": "oldpassword1", "displayName": "Member"}, false)

	// forgot is non-enumerating: 200 for both a missing and a real address.
	if resp, body := newClient(t, base).do("POST", "/api/auth/forgot",
		map[string]string{"email": "nobody@example.com"}, false); resp.StatusCode != http.StatusOK || body["ok"] != true {
		t.Fatalf("forgot(missing) = %d %v; want 200 ok", resp.StatusCode, body)
	}
	if resp, _ := newClient(t, base).do("POST", "/api/auth/forgot",
		map[string]string{"email": "member@example.com"}, false); resp.StatusCode != http.StatusOK {
		t.Fatalf("forgot(real) = %d; want 200", resp.StatusCode)
	}

	mem, _, _ := st.GetUserByEmail("member@example.com")

	// A garbage/short token is refused.
	if resp, _ := newClient(t, base).do("POST", "/api/auth/reset",
		map[string]string{"token": "not-real", "password": "brandnew12"}, false); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("reset(bad token) = %d; want 400", resp.StatusCode)
	}
	tok, err := st.CreatePasswordReset(mem.ID)
	if err != nil {
		t.Fatalf("create reset token: %v", err)
	}
	if resp, _ := newClient(t, base).do("POST", "/api/auth/reset",
		map[string]string{"token": tok, "password": "short"}, false); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("reset(short password) = %d; want 400", resp.StatusCode)
	}

	// A valid token: new password is set and the caller is logged in.
	resetter := newClient(t, base)
	resp, body := resetter.do("POST", "/api/auth/reset",
		map[string]string{"token": tok, "password": "brandnewpass1"}, false)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reset(valid) = %d %v; want 200", resp.StatusCode, body)
	}
	if u := resetter.user(body); u == nil || u["email"] != "member@example.com" {
		t.Fatalf("reset should log the user in; got user=%v", resetter.user(body))
	}

	// The member's earlier session was revoked by the reset.
	if _, body := member.do("GET", "/api/auth/me", nil, false); body["user"] != nil {
		t.Error("prior session should be revoked after a password reset")
	}

	// The token is single-use.
	if resp, _ := newClient(t, base).do("POST", "/api/auth/reset",
		map[string]string{"token": tok, "password": "anotherone1"}, false); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("reset(reused token) = %d; want 400", resp.StatusCode)
	}

	// Old password no longer works; new one does.
	if resp, _ := newClient(t, base).do("POST", "/api/auth/login",
		map[string]string{"email": "member@example.com", "password": "oldpassword1"}, false); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("login(old password) = %d; want 401", resp.StatusCode)
	}
	if resp, _ := newClient(t, base).do("POST", "/api/auth/login",
		map[string]string{"email": "member@example.com", "password": "brandnewpass1"}, false); resp.StatusCode != http.StatusOK {
		t.Fatalf("login(new password) = %d; want 200", resp.StatusCode)
	}
}

// TestLoginRateLimit confirms repeated failed logins for one account are
// throttled with 429 (brute-force protection) rather than allowed indefinitely.
func TestLoginRateLimit(t *testing.T) {
	_, base, cleanup := newAuthEnv(t)
	defer cleanup()
	newClient(t, base).do("POST", "/api/auth/register",
		map[string]string{"email": "target@example.com", "password": "correcthorse1", "displayName": "T"}, false)

	got429 := false
	for i := 0; i < 8; i++ {
		resp, _ := newClient(t, base).do("POST", "/api/auth/login",
			map[string]string{"email": "target@example.com", "password": "wrong-guess"}, false)
		if resp.StatusCode == http.StatusTooManyRequests {
			got429 = true
			break
		}
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status %d; want 401 (or 429)", i, resp.StatusCode)
		}
	}
	if !got429 {
		t.Error("expected a 429 after several failed logins; brute-force limiter not engaging")
	}
}
