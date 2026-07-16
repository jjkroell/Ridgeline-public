package api

import (
	"fmt"
	"html"
	"net/url"
	"strings"

	"github.com/jjkroell/ridgeline/internal/store"
)

// sendVerificationEmail emails a user a link to confirm their address. No-op when
// the mailer is disabled. Best-effort: send failures are logged, not surfaced to
// the registrant (they can request a resend).
func (s *Server) sendVerificationEmail(user store.User) {
	if !s.mailEnabled() {
		return
	}
	token, err := s.store.CreateEmailVerification(user.ID)
	if err != nil {
		s.log.Error("create verification token", "user", user.ID, "err", err)
		return
	}
	link := fmt.Sprintf("%s/verify-email?token=%s", s.mail.BaseURL(), url.QueryEscape(token))
	name := user.DisplayName
	if name == "" {
		name = "there"
	}

	subject := "Confirm your Ridgeline account"
	text := fmt.Sprintf(`Hi %s,

Welcome to Ridgeline — a MeshCore mesh observatory.

Confirm your email to activate your account:
%s

This link expires in 24 hours. If you didn't create a Ridgeline account, you can ignore this message.

— Ridgeline
`, name, link)

	body := fmt.Sprintf(`<p>Hi %s,</p>
<p>Welcome to <strong>Ridgeline</strong> — a MeshCore mesh observatory.</p>
<p>Confirm your email to activate your account:</p>
<p><a href="%s" style="display:inline-block;padding:10px 18px;background:#1f9d55;color:#fff;border-radius:6px;text-decoration:none;font-weight:600">Confirm my email</a></p>
<p style="color:#666;font-size:13px">Or paste this link into your browser:<br><a href="%s">%s</a></p>
<p style="color:#666;font-size:13px">This link expires in 24 hours. If you didn't create a Ridgeline account, you can ignore this message.</p>
<p>— Ridgeline</p>`, html.EscapeString(name), htmlAttr(link), htmlAttr(link), html.EscapeString(link))

	s.mail.SendAsync("verification", user.Email, subject, text, body)
}

// notifyOwnerOfNote emails a node's verified owner when someone else leaves a
// public or team note on their node. No-op for private notes, self-notes, unowned
// nodes, or when email is disabled.
func (s *Server) notifyOwnerOfNote(pubkey string, author store.User, note store.Note) {
	if !s.mailEnabled() || note.Visibility == "private" {
		return
	}
	owner, ok, err := s.store.NodeOwner(pubkey)
	if err != nil || !ok || owner.UserID == author.ID {
		return
	}
	ownerUser, ok, err := s.store.GetUserByID(owner.UserID)
	if err != nil || !ok || !ownerUser.EmailVerified {
		return
	}

	nodeName := s.nodeDisplayName(pubkey)
	authorName := author.DisplayName
	if authorName == "" {
		authorName = "Someone"
	}
	link := fmt.Sprintf("%s/nodes/%s", s.mail.BaseURL(), url.PathEscape(pubkey))
	kind := "note"
	if note.Visibility == "team" {
		kind = "team note"
	}

	subject := fmt.Sprintf("New %s on %s", kind, nodeName)
	text := fmt.Sprintf(`%s left a %s on your node "%s":

%s

View it: %s

You're receiving this because you own this node on Ridgeline.
`, authorName, kind, nodeName, note.Body, link)

	body := fmt.Sprintf(`<p><strong>%s</strong> left a %s on your node <strong>%s</strong>:</p>
<blockquote style="margin:0;padding:8px 14px;border-left:3px solid #1f9d55;color:#333">%s</blockquote>
<p><a href="%s">View it on Ridgeline</a></p>
<p style="color:#666;font-size:13px">You're receiving this because you own this node on Ridgeline.</p>`,
		html.EscapeString(authorName), html.EscapeString(kind), html.EscapeString(nodeName),
		html.EscapeString(note.Body), htmlAttr(link))

	s.mail.SendAsync("note", ownerUser.Email, subject, text, body)
}

// nodeDisplayName returns a node's advertised name, falling back to a short key.
func (s *Server) nodeDisplayName(pubkey string) string {
	if name := s.store.NodeName(pubkey); name != "" {
		return name
	}
	if len(pubkey) >= 12 {
		return pubkey[:12]
	}
	return pubkey
}

// htmlAttr escapes a string for use inside an HTML attribute value (e.g. href).
func htmlAttr(s string) string {
	return strings.NewReplacer(`"`, "%22", `'`, "%27", "<", "%3C", ">", "%3E").Replace(s)
}
