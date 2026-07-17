package mail

import (
	"log/slog"
	"strings"
	"testing"

	"github.com/jjkroell/ridgeline/internal/config"
)

func TestBuildStripsHeaderInjection(t *testing.T) {
	m := New(config.Email{
		Host: "smtp.example.com", Port: 587, Username: "u", Password: "p",
		From: "noreply@example.com", FromName: "Ridgeline", BaseURL: "https://x/",
	}, slog.Default())

	// A recipient and a subject both carrying a CRLF + injected Bcc header.
	msg := string(m.build(
		"victim@example.com\r\nBcc: attacker@evil.com",
		"New note on Node\r\nBcc: attacker@evil.com",
		"plain body", "<p>html body</p>",
	))

	// No header line may begin with the injected Bcc — the CRLF must be stripped
	// (To) or escaped (Subject Q-encoding), never emitted raw.
	if strings.Contains(msg, "\r\nBcc:") {
		t.Errorf("header injection succeeded — a raw Bcc line appeared:\n%s", msg)
	}
	// The To header stays a single line with the newline removed.
	if !strings.Contains(msg, "To: victim@example.comBcc: attacker@evil.com\r\n") {
		t.Errorf("To header not sanitized as expected:\n%s", msg)
	}
}

func TestDisabledMailerIsNoOp(t *testing.T) {
	m := New(config.Email{}, slog.Default())
	if m.Enabled() {
		t.Error("mailer with no host should be disabled")
	}
	if err := m.Send("x@y.z", "s", "t", "h"); err != nil {
		t.Errorf("disabled Send should be a silent no-op, got %v", err)
	}
}

func TestBuildMessage(t *testing.T) {
	m := New(config.Email{
		Host: "smtp.example.com", Port: 587, Username: "apikey", Password: "SG.test",
		From: "noreply@example.com", FromName: "Ridgeline",
		BaseURL: "https://ridgeline.example.com/",
	}, slog.Default())

	if !m.Enabled() {
		t.Fatal("mailer should be enabled")
	}
	if m.BaseURL() != "https://ridgeline.example.com" {
		t.Errorf("BaseURL should be trimmed of trailing slash, got %q", m.BaseURL())
	}

	msg := string(m.build("you@example.com", "Hi & welcome", "plain body", "<p>html body</p>"))
	for _, want := range []string{
		"From: ", "noreply@example.com",
		"To: you@example.com",
		"Subject: ",
		"MIME-Version: 1.0",
		"multipart/alternative",
		"text/plain; charset=UTF-8",
		"text/html; charset=UTF-8",
		"plain body",
		"<p>html body</p>",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("built message missing %q\n---\n%s", want, msg)
		}
	}
}
