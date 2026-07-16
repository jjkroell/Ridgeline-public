package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/jjkroell/ridgeline/internal/store"
)

// maxNoteLen bounds a note body so a single note can't be abused for bulk storage.
const maxNoteLen = 4000

// noteView is a note plus whether the requester may edit/delete it.
type noteView struct {
	store.Note
	Mine bool `json:"mine"`
}

// notesResponse wraps the visible notes with the caller's posting rights so the
// UI can show/hide the "team" note option.
type notesResponse struct {
	Notes    []noteView `json:"notes"`
	CanTeam  bool       `json:"canTeam"`  // caller may post team notes (owner or shared-with)
	LoggedIn bool       `json:"loggedIn"` // caller may post at all
}

// nodeNotes returns the notes visible to the caller for a node: all public
// notes, the caller's own notes, and (for the node's owner or a shared-with
// user) the node's "team" notes. Public (optional auth).
func (s *Server) nodeNotes(w http.ResponseWriter, r *http.Request) {
	pubkey := strings.ToUpper(r.PathValue("pubkey"))
	var viewerID int64
	var isAdmin bool
	if user, _, ok := s.currentUser(r); ok {
		viewerID = user.ID
		isAdmin = user.IsAdmin
	}
	inCircle, err := s.inNodeCircle(pubkey, viewerID)
	if err != nil {
		s.fail(w, err)
		return
	}
	notes, err := s.store.NotesForNode(pubkey, viewerID, inCircle)
	if err != nil {
		s.fail(w, err)
		return
	}
	// A note is editable by its author; the node owner/admin can moderate (delete).
	canModerate := isAdmin
	if viewerID != 0 && !canModerate {
		if owner, ok, _ := s.store.NodeOwner(pubkey); ok && owner.UserID == viewerID {
			canModerate = true
		}
	}
	out := make([]noteView, 0, len(notes))
	for _, n := range notes {
		out = append(out, noteView{Note: n, Mine: n.UserID == viewerID || canModerate})
	}
	writeJSON(w, notesResponse{Notes: out, CanTeam: inCircle, LoggedIn: viewerID != 0})
}

// noteCreate adds a note to a node (any authenticated user).
func (s *Server) noteCreate(w http.ResponseWriter, r *http.Request, user store.User) {
	pubkey := strings.ToUpper(r.PathValue("pubkey"))
	if !validPubkey(pubkey) {
		writeErr(w, http.StatusBadRequest, "invalid node public key")
		return
	}
	var req struct {
		Body       string `json:"body"`
		Visibility string `json:"visibility"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request body")
		return
	}
	body, vis, ok := validateNote(w, req.Body, req.Visibility)
	if !ok {
		return
	}
	if exists, err := s.store.NodeExists(pubkey); err != nil {
		s.fail(w, err)
		return
	} else if !exists {
		writeErr(w, http.StatusNotFound, "unknown node")
		return
	}
	// A "team" note is visible to the node's whole trusted circle, so only the
	// owner or a shared-with user may post one.
	if vis == "team" {
		if ok, err := s.requireCircle(w, pubkey, user.ID); err != nil {
			s.fail(w, err)
			return
		} else if !ok {
			return
		}
	}
	note, err := s.store.CreateNote(pubkey, user.ID, vis, body)
	if err != nil {
		s.fail(w, err)
		return
	}
	// Let the node's owner know someone commented (public/team notes only; async,
	// best-effort — never blocks or fails the request).
	s.notifyOwnerOfNote(pubkey, user, note)
	writeJSON(w, noteView{Note: note, Mine: true})
}

// noteUpdate edits one of the caller's own notes.
func (s *Server) noteUpdate(w http.ResponseWriter, r *http.Request, user store.User) {
	id, ok := noteID(w, r)
	if !ok {
		return
	}
	var req struct {
		Body       string `json:"body"`
		Visibility string `json:"visibility"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request body")
		return
	}
	body, vis, valid := validateNote(w, req.Body, req.Visibility)
	if !valid {
		return
	}
	// Promoting a note to "team" requires circle membership on its node.
	if vis == "team" {
		if n, found, err := s.store.GetNote(id); err != nil {
			s.fail(w, err)
			return
		} else if found {
			if ok, err := s.requireCircle(w, n.NodePubkey, user.ID); err != nil {
				s.fail(w, err)
				return
			} else if !ok {
				return
			}
		}
	}
	note, found, err := s.store.UpdateNote(id, user.ID, vis, body)
	if err == store.ErrNotAuthor {
		writeErr(w, http.StatusForbidden, "you can only edit your own notes")
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	if !found {
		writeErr(w, http.StatusNotFound, "note not found")
		return
	}
	writeJSON(w, noteView{Note: note, Mine: true})
}

// noteDelete removes a note. Author can delete their own; node owner/admin can
// moderate any note on the node.
func (s *Server) noteDelete(w http.ResponseWriter, r *http.Request, user store.User) {
	id, ok := noteID(w, r)
	if !ok {
		return
	}
	// Determine moderation right from the note's node.
	canModerate := user.IsAdmin
	if !canModerate {
		if n, found, err := s.store.GetNote(id); err == nil && found {
			if owner, ok, _ := s.store.NodeOwner(n.NodePubkey); ok && owner.UserID == user.ID {
				canModerate = true
			}
		}
	}
	found, err := s.store.DeleteNote(id, user.ID, canModerate)
	if err == store.ErrNotAuthor {
		writeErr(w, http.StatusForbidden, "you can only delete your own notes")
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	if !found {
		writeErr(w, http.StatusNotFound, "note not found")
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func validateNote(w http.ResponseWriter, body, visibility string) (string, string, bool) {
	body = strings.TrimSpace(body)
	if body == "" {
		writeErr(w, http.StatusBadRequest, "note body is required")
		return "", "", false
	}
	if len(body) > maxNoteLen {
		writeErr(w, http.StatusBadRequest, "note is too long")
		return "", "", false
	}
	if visibility != "public" && visibility != "private" && visibility != "team" {
		writeErr(w, http.StatusBadRequest, "visibility must be public, private, or team")
		return "", "", false
	}
	return body, visibility, true
}

// requireCircle writes a 403 and returns ok=false unless the user is in the
// node's trusted circle (owner or shared-with).
func (s *Server) requireCircle(w http.ResponseWriter, pubkey string, userID int64) (bool, error) {
	in, err := s.inNodeCircle(pubkey, userID)
	if err != nil {
		return false, err
	}
	if !in {
		writeErr(w, http.StatusForbidden, "only the node's owner or shared-with users can post team notes")
		return false, nil
	}
	return true, nil
}

func noteID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid note id")
		return 0, false
	}
	return id, true
}
