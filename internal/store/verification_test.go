package store

import "testing"

func TestBootstrapOwnerAutoVerified(t *testing.T) {
	st := testStore(t)
	owner, err := st.CreateUser("owner@example.com", "h", "Owner")
	if err != nil {
		t.Fatalf("create owner: %v", err)
	}
	if !owner.EmailVerified {
		t.Error("bootstrap owner should be auto-verified")
	}
	member, err := st.CreateUser("member@example.com", "h", "Member")
	if err != nil {
		t.Fatalf("create member: %v", err)
	}
	if member.EmailVerified {
		t.Error("a non-bootstrap account should start unverified")
	}
	// Confirm it persists through a reload.
	if got, ok, _ := st.GetUserByID(member.ID); !ok || got.EmailVerified {
		t.Error("member should read back unverified")
	}
}

func TestEmailVerificationTokenLifecycle(t *testing.T) {
	st := testStore(t)
	st.CreateUser("owner@example.com", "h", "Owner") // bootstrap
	u, _ := st.CreateUser("member@example.com", "h", "Member")

	token, err := st.CreateEmailVerification(u.ID)
	if err != nil || token == "" {
		t.Fatalf("create verification: token=%q err=%v", token, err)
	}

	// A bad token verifies nobody.
	if _, ok, _ := st.VerifyEmailToken("not-a-real-token"); ok {
		t.Error("garbage token must not verify")
	}

	// The real token verifies and marks the account.
	got, ok, err := st.VerifyEmailToken(token)
	if err != nil || !ok {
		t.Fatalf("verify: ok=%v err=%v", ok, err)
	}
	if got.ID != u.ID || !got.EmailVerified {
		t.Fatalf("wrong/unverified user returned: %+v", got)
	}
	if cur, _, _ := st.GetUserByID(u.ID); !cur.EmailVerified {
		t.Error("account should be verified in the DB")
	}

	// Token is single-use: it's consumed on success.
	if _, ok, _ := st.VerifyEmailToken(token); ok {
		t.Error("a consumed token must not verify again")
	}
}

func TestResendReplacesPriorToken(t *testing.T) {
	st := testStore(t)
	st.CreateUser("owner@example.com", "h", "Owner")
	u, _ := st.CreateUser("member@example.com", "h", "Member")

	first, _ := st.CreateEmailVerification(u.ID)
	second, _ := st.CreateEmailVerification(u.ID) // resend

	if _, ok, _ := st.VerifyEmailToken(first); ok {
		t.Error("the superseded token should no longer be valid")
	}
	if _, ok, _ := st.VerifyEmailToken(second); !ok {
		t.Error("the latest token should verify")
	}
}
