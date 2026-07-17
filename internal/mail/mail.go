// Package mail sends Ridgeline's outbound transactional email (account
// verification and node-note notifications) over an authenticated SMTP relay.
// It is intentionally small: one relay, plain auth, TLS. When the relay is not
// configured the Mailer is disabled and Send is a no-op, so the rest of the app
// runs unchanged in development.
package mail

import (
	"crypto/rand"
	"crypto/tls"
	"fmt"
	"log/slog"
	"mime"
	"net/smtp"
	"strings"
	"time"

	"github.com/jjkroell/ridgeline/internal/config"
)

// Mailer sends mail through a configured SMTP relay.
type Mailer struct {
	cfg config.Email
	log *slog.Logger
}

// New returns a Mailer for the given email config. It is always non-nil; when
// cfg is not Enabled(), Send/SendAsync are no-ops (a warning is logged once).
func New(cfg config.Email, log *slog.Logger) *Mailer {
	if !cfg.Enabled() {
		log.Warn("email disabled: no SMTP relay configured (verification + note emails will be skipped)")
	}
	return &Mailer{cfg: cfg, log: log}
}

// Enabled reports whether the relay is configured.
func (m *Mailer) Enabled() bool { return m.cfg.Enabled() }

// BaseURL is the public site origin used to build links in emails.
func (m *Mailer) BaseURL() string { return strings.TrimRight(m.cfg.BaseURL, "/") }

// SendAsync sends in a goroutine and logs the outcome, so request handlers never
// block on SMTP. what is a short label for logs (e.g. "verification").
func (m *Mailer) SendAsync(what, to, subject, text, html string) {
	if !m.Enabled() {
		return
	}
	go func() {
		if err := m.Send(to, subject, text, html); err != nil {
			m.log.Error("email send failed", "kind", what, "to", to, "err", err)
			return
		}
		m.log.Info("email sent", "kind", what, "to", to)
	}()
}

// Send delivers one multipart (text + HTML) message synchronously. Returns nil
// immediately if the mailer is disabled.
func (m *Mailer) Send(to, subject, text, html string) error {
	if !m.Enabled() {
		return nil
	}
	msg := m.build(to, subject, text, html)
	addr := fmt.Sprintf("%s:%d", m.cfg.Host, m.cfg.Port)
	auth := smtp.PlainAuth("", m.cfg.Username, m.cfg.Password, m.cfg.Host)

	c, err := m.dial(addr)
	if err != nil {
		return err
	}
	defer c.Close()
	if err := c.Auth(auth); err != nil {
		return fmt.Errorf("smtp auth: %w", err)
	}
	if err := c.Mail(m.cfg.From); err != nil {
		return fmt.Errorf("smtp from: %w", err)
	}
	if err := c.Rcpt(to); err != nil {
		return fmt.Errorf("smtp rcpt: %w", err)
	}
	wc, err := c.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := wc.Write(msg); err != nil {
		wc.Close()
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := wc.Close(); err != nil {
		return fmt.Errorf("smtp close: %w", err)
	}
	return c.Quit()
}

// dial connects and secures the SMTP session: implicit TLS on 465, otherwise
// STARTTLS after the greeting.
func (m *Mailer) dial(addr string) (*smtp.Client, error) {
	tlsCfg := &tls.Config{ServerName: m.cfg.Host}
	if m.cfg.Port == 465 {
		conn, err := tls.Dial("tcp", addr, tlsCfg)
		if err != nil {
			return nil, fmt.Errorf("smtp tls dial: %w", err)
		}
		c, err := smtp.NewClient(conn, m.cfg.Host)
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("smtp client: %w", err)
		}
		return c, nil
	}
	c, err := smtp.Dial(addr)
	if err != nil {
		return nil, fmt.Errorf("smtp dial: %w", err)
	}
	if err := c.StartTLS(tlsCfg); err != nil {
		c.Close()
		return nil, fmt.Errorf("smtp starttls: %w", err)
	}
	return c, nil
}

// sanitizeHeader strips CR/LF from a header value so a stray newline (from a
// node name, display name, or a future unvalidated address) can never inject
// additional SMTP headers (Bcc, etc.). Defense-in-depth: header values are also
// validated/encoded upstream, but we never emit a raw newline at the boundary.
func sanitizeHeader(v string) string {
	return strings.NewReplacer("\r", "", "\n", "").Replace(v)
}

// build assembles an RFC 5322 multipart/alternative message.
func (m *Mailer) build(to, subject, text, html string) []byte {
	to = sanitizeHeader(to)
	from := sanitizeHeader(m.cfg.From)
	if m.cfg.FromName != "" {
		from = fmt.Sprintf("%s <%s>", mime.QEncoding.Encode("utf-8", m.cfg.FromName), sanitizeHeader(m.cfg.From))
	}
	boundary := "rl_" + randToken()
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", to)
	// Q-encoding already escapes control chars; sanitize first as belt-and-suspenders.
	fmt.Fprintf(&b, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", sanitizeHeader(subject)))
	fmt.Fprintf(&b, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	fmt.Fprintf(&b, "Message-ID: <%s@%s>\r\n", randToken(), hostOf(m.cfg.From))
	b.WriteString("MIME-Version: 1.0\r\n")
	fmt.Fprintf(&b, "Content-Type: multipart/alternative; boundary=%q\r\n\r\n", boundary)

	fmt.Fprintf(&b, "--%s\r\n", boundary)
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
	b.WriteString(text)
	b.WriteString("\r\n\r\n")

	fmt.Fprintf(&b, "--%s\r\n", boundary)
	b.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
	b.WriteString(html)
	b.WriteString("\r\n\r\n")

	fmt.Fprintf(&b, "--%s--\r\n", boundary)
	return []byte(b.String())
}

func hostOf(addr string) string {
	if i := strings.LastIndex(addr, "@"); i >= 0 {
		return addr[i+1:]
	}
	return "ridgeline"
}

func randToken() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("%x", b)
}
