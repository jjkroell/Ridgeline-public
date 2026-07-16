package api

import (
	"testing"
)

// Self-service account editing: display name, password (re-auth), and email
// (re-auth + re-verification).
func TestAccountSelfEdit(t *testing.T) {
	_, base, fm, cleanup := mailEnv(t)
	defer cleanup()

	// Owner is auto-verified; use a second (verified) member for realistic gating.
	owner := newClient(t, base)
	owner.do("POST", "/api/auth/register",
		map[string]string{"email": "owner@example.com", "password": "hunter2hunter2"}, false)

	member := newClient(t, base)
	member.do("POST", "/api/auth/register",
		map[string]string{"email": "member@example.com", "password": "origpassword", "displayName": "Old"}, false)
	// Verify the member so they have a session (register doesn't log in unverified).
	vs := fm.ofKind("verification")
	member.do("POST", "/api/auth/verify", map[string]string{"token": tokenFromLink(t, vs[len(vs)-1].text)}, false)

	// Display name.
	if r, body := member.do("PUT", "/api/account/profile",
		map[string]string{"displayName": "New Callsign"}, true); r.StatusCode != 200 || body["displayName"] != "New Callsign" {
		t.Fatalf("profile update failed: %d %v", r.StatusCode, body)
	}

	// Password: wrong current password is rejected.
	if r, _ := member.do("POST", "/api/account/password",
		map[string]string{"currentPassword": "nope", "newPassword": "brandnewpass"}, true); r.StatusCode != 403 {
		t.Errorf("wrong current password should be 403, got %d", r.StatusCode)
	}
	// Too-short new password.
	if r, _ := member.do("POST", "/api/account/password",
		map[string]string{"currentPassword": "origpassword", "newPassword": "short"}, true); r.StatusCode != 400 {
		t.Errorf("short new password should be 400, got %d", r.StatusCode)
	}
	// Correct change, then the new password logs in and the old one doesn't.
	if r, _ := member.do("POST", "/api/account/password",
		map[string]string{"currentPassword": "origpassword", "newPassword": "brandnewpass"}, true); r.StatusCode != 200 {
		t.Fatalf("password change should be 200, got %d", r.StatusCode)
	}
	fresh := newClient(t, base)
	if r, _ := fresh.do("POST", "/api/auth/login",
		map[string]string{"email": "member@example.com", "password": "brandnewpass"}, false); r.StatusCode != 200 {
		t.Errorf("login with new password should be 200, got %d", r.StatusCode)
	}
	if r, _ := fresh.do("POST", "/api/auth/login",
		map[string]string{"email": "member@example.com", "password": "origpassword"}, false); r.StatusCode != 401 {
		t.Errorf("login with old password should be 401, got %d", r.StatusCode)
	}

	// Email: taking another account's address is rejected.
	if r, _ := member.do("POST", "/api/account/email",
		map[string]string{"currentPassword": "brandnewpass", "newEmail": "owner@example.com"}, true); r.StatusCode != 409 {
		t.Errorf("email collision should be 409, got %d", r.StatusCode)
	}
	// Changing to a fresh address succeeds, flips emailVerified false, and sends a link.
	before := len(fm.ofKind("verification"))
	if r, body := member.do("POST", "/api/account/email",
		map[string]string{"currentPassword": "brandnewpass", "newEmail": "member2@example.com"}, true); r.StatusCode != 200 || body["email"] != "member2@example.com" || body["emailVerified"] != false {
		t.Fatalf("email change failed: %d %v", r.StatusCode, body)
	}
	if len(fm.ofKind("verification")) != before+1 {
		t.Error("email change should send a verification email to the new address")
	}
	// Now login is blocked until the new address is confirmed.
	blocked := newClient(t, base)
	if r, b := blocked.do("POST", "/api/auth/login",
		map[string]string{"email": "member2@example.com", "password": "brandnewpass"}, false); r.StatusCode != 403 || b["unverified"] != true {
		t.Errorf("login after email change should be 403 unverified, got %d %v", r.StatusCode, b)
	}
}
