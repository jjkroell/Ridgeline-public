package store

import "testing"

func TestPasswordResetTokenLifecycle(t *testing.T) {
	st := testStore(t)
	st.CreateUser("owner@example.com", "h", "Owner") // bootstrap
	u, _ := st.CreateUser("member@example.com", "h", "Member")

	token, err := st.CreatePasswordReset(u.ID)
	if err != nil || token == "" {
		t.Fatalf("create reset: token=%q err=%v", token, err)
	}

	// A bad token consumes nobody.
	if _, ok, _ := st.ConsumePasswordReset("not-a-real-token"); ok {
		t.Error("garbage token must not resolve")
	}

	// The real token resolves to the right user.
	got, ok, err := st.ConsumePasswordReset(token)
	if err != nil || !ok {
		t.Fatalf("consume: ok=%v err=%v", ok, err)
	}
	if got.ID != u.ID {
		t.Fatalf("wrong user returned: %+v", got)
	}

	// Single-use: consuming again fails.
	if _, ok, _ := st.ConsumePasswordReset(token); ok {
		t.Error("a consumed reset token must not resolve again")
	}
}

func TestPasswordResetReplacesPriorToken(t *testing.T) {
	st := testStore(t)
	st.CreateUser("owner@example.com", "h", "Owner")
	u, _ := st.CreateUser("member@example.com", "h", "Member")

	first, _ := st.CreatePasswordReset(u.ID)
	second, _ := st.CreatePasswordReset(u.ID) // a fresh request supersedes the first

	if _, ok, _ := st.ConsumePasswordReset(first); ok {
		t.Error("the superseded reset token should no longer be valid")
	}
	if _, ok, _ := st.ConsumePasswordReset(second); !ok {
		t.Error("the latest reset token should resolve")
	}
}
