package auth

import "testing"

func TestHashPasswordRoundTrip(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if !VerifyPassword("correct horse battery staple", hash) {
		t.Error("correct password did not verify")
	}
	if VerifyPassword("wrong password", hash) {
		t.Error("wrong password verified")
	}
}

func TestHashPasswordUniqueSalts(t *testing.T) {
	a, _ := HashPassword("same")
	b, _ := HashPassword("same")
	if a == b {
		t.Error("two hashes of the same password should differ (random salt)")
	}
}

func TestVerifyRejectsMalformed(t *testing.T) {
	for _, bad := range []string{"", "notahash", "$argon2id$v=19$bad", "$bcrypt$..."} {
		if VerifyPassword("x", bad) {
			t.Errorf("malformed hash %q should not verify", bad)
		}
	}
}

func TestSessionTokenHashing(t *testing.T) {
	token, hash, err := NewSessionToken()
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	if token == "" || hash == "" || token == hash {
		t.Fatalf("token/hash malformed: token=%q hash=%q", token, hash)
	}
	if HashToken(token) != hash {
		t.Error("HashToken must reproduce the session hash")
	}
	// Distinct tokens must hash distinctly.
	t2, h2, _ := NewSessionToken()
	if token == t2 || hash == h2 {
		t.Error("session tokens must be unique")
	}
}
