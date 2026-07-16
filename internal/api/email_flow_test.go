package api

import (
	"io"
	"log/slog"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jjkroell/ridgeline/internal/meshcore"
	"github.com/jjkroell/ridgeline/internal/store"
)

// fakeMailer captures outbound mail instead of sending it, so tests exercise the
// full verification / notification flow without touching a real SMTP relay.
type fakeMailer struct {
	mu   sync.Mutex
	base string
	sent []sentMail
}

type sentMail struct{ kind, to, subject, text, html string }

func (f *fakeMailer) Enabled() bool   { return true }
func (f *fakeMailer) BaseURL() string { return f.base }
func (f *fakeMailer) SendAsync(kind, to, subject, text, html string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, sentMail{kind, to, subject, text, html})
}
func (f *fakeMailer) ofKind(kind string) []sentMail {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []sentMail
	for _, m := range f.sent {
		if m.kind == kind {
			out = append(out, m)
		}
	}
	return out
}

// mailEnv is newAuthEnv plus an injected capturing mailer.
func mailEnv(t *testing.T) (*store.Store, string, *fakeMailer, func()) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "api.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	srv := New(st, slog.New(slog.NewTextHandler(io.Discard, nil)), "test", "")
	fm := &fakeMailer{base: "https://ridgeline.example.com"}
	srv.SetMailer(fm)
	ts := httptest.NewServer(srv.Handler())
	return st, ts.URL, fm, func() { ts.Close(); st.Close() }
}

func tokenFromLink(t *testing.T, body string) string {
	t.Helper()
	const marker = "verify-email?token="
	i := strings.Index(body, marker)
	if i < 0 {
		t.Fatalf("no verification link in email body:\n%s", body)
	}
	rest := body[i+len(marker):]
	// token runs until a quote, whitespace, or angle bracket.
	end := strings.IndexAny(rest, "\"'<> \r\n")
	if end >= 0 {
		rest = rest[:end]
	}
	tok, err := url.QueryUnescape(rest)
	if err != nil {
		t.Fatalf("unescape token: %v", err)
	}
	return tok
}

func TestEmailVerificationAndNoteNotifyFlow(t *testing.T) {
	st, base, fm, cleanup := mailEnv(t)
	defer cleanup()

	// 1) Owner (first account) is auto-verified and logged straight in — no email.
	owner := newClient(t, base)
	_, ob := owner.do("POST", "/api/auth/register",
		map[string]string{"email": "owner@example.com", "password": "hunter2hunter2", "displayName": "Owner"}, false)
	if owner.user(ob) == nil {
		t.Fatal("owner should be logged in on registration")
	}
	ownerID := int64(owner.user(ob)["id"].(float64))
	if len(fm.ofKind("verification")) != 0 {
		t.Errorf("owner should not receive a verification email, got %d", len(fm.ofKind("verification")))
	}

	// 2) Member (second account) gets a verification email, no session.
	member := newClient(t, base)
	resp, mb := member.do("POST", "/api/auth/register",
		map[string]string{"email": "member@example.com", "password": "hunter2hunter2", "displayName": "Member"}, false)
	if resp.StatusCode != 200 || mb["verificationSent"] != true {
		t.Fatalf("member register should report verificationSent, got %d %v", resp.StatusCode, mb)
	}
	if member.user(mb) != nil {
		t.Error("member should NOT be logged in before verifying")
	}
	vs := fm.ofKind("verification")
	if len(vs) != 1 || vs[0].to != "member@example.com" {
		t.Fatalf("expected 1 verification email to member, got %+v", vs)
	}

	// 3) Login before verifying is refused with an unverified marker.
	if r, lb := member.do("POST", "/api/auth/login",
		map[string]string{"email": "member@example.com", "password": "hunter2hunter2"}, false); r.StatusCode != 403 || lb["unverified"] != true {
		t.Fatalf("unverified login should be 403 unverified, got %d %v", r.StatusCode, lb)
	}

	// 4) Resend, then verify with the latest token → logs in.
	member.do("POST", "/api/auth/resend-verification", map[string]string{"email": "member@example.com"}, false)
	latest := fm.ofKind("verification")
	token := tokenFromLink(t, latest[len(latest)-1].text)
	if r, vb := member.do("POST", "/api/auth/verify", map[string]string{"token": token}, false); r.StatusCode != 200 || member.user(vb) == nil {
		t.Fatalf("verify should log the member in, got %d %v", r.StatusCode, vb)
	}

	// 5) Login now succeeds.
	if r, _ := member.do("POST", "/api/auth/login",
		map[string]string{"email": "member@example.com", "password": "hunter2hunter2"}, false); r.StatusCode != 200 {
		t.Fatalf("verified login should be 200, got %d", r.StatusCode)
	}

	// 6) Owner owns a node; a public note from the member emails the owner.
	pubkey := strings.ToUpper(strings.Repeat("ab", 32))
	if err := st.Record(store.Observation{
		Packet:     &meshcore.Packet{MessageHash: "note0001", Advert: &meshcore.Advert{PublicKey: pubkey, HasName: true, Name: "Owner's Repeater", SignatureValid: true}},
		RawHex:     "00",
		ReceivedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed node: %v", err)
	}
	if _, err := st.CreateVerifiedClaim(pubkey, ownerID); err != nil {
		t.Fatalf("owner claim: %v", err)
	}

	if r, _ := member.do("POST", "/api/nodes/"+pubkey+"/notes",
		map[string]string{"body": "Saw this repeater on a ridge!", "visibility": "public"}, true); r.StatusCode != 200 {
		t.Fatalf("post note should be 200, got %d", r.StatusCode)
	}
	notes := fm.ofKind("note")
	if len(notes) != 1 || notes[0].to != "owner@example.com" {
		t.Fatalf("owner should get exactly one note email, got %+v", notes)
	}
	if !strings.Contains(notes[0].text, "Saw this repeater") || !strings.Contains(notes[0].text, pubkey) {
		t.Errorf("note email missing body/link: %q", notes[0].text)
	}

	// 7) The owner commenting on their own node does NOT self-notify.
	before := len(fm.ofKind("note"))
	owner.do("POST", "/api/nodes/"+pubkey+"/notes",
		map[string]string{"body": "my own note", "visibility": "public"}, true)
	if after := len(fm.ofKind("note")); after != before {
		t.Errorf("owner self-note should not send email: before=%d after=%d", before, after)
	}
}
