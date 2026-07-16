package store

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Note is a user-authored annotation on a node. AuthorName is the author's
// display name (or email), resolved for display.
type Note struct {
	ID         int64  `json:"id"`
	NodePubkey string `json:"nodePubkey"`
	UserID     int64  `json:"userId"`
	AuthorName string `json:"authorName"`
	Visibility string `json:"visibility"` // public | private | team
	Body       string `json:"body"`
	CreatedAt  string `json:"createdAt"`
	UpdatedAt  string `json:"updatedAt"`
}

// CreateNote adds a note authored by userID on a node. visibility must be
// "public", "private", or "team" (the caller enforces that a team note's author
// is in the node's shared circle).
func (s *Store) CreateNote(nodePubkey string, userID int64, visibility, body string) (Note, error) {
	nodePubkey = strings.ToUpper(nodePubkey)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := s.db.Exec(`
		INSERT INTO node_notes (node_pubkey, user_id, visibility, body, created_at, updated_at)
		VALUES (?,?,?,?,?,?)`, nodePubkey, userID, visibility, body, now, now)
	if err != nil {
		return Note{}, err
	}
	id, _ := res.LastInsertId()
	n, _, err := s.GetNote(id)
	return n, err
}

// GetNote fetches a single note (with author name). ok=false if it doesn't exist.
func (s *Store) GetNote(id int64) (Note, bool, error) {
	var n Note
	err := s.db.QueryRow(`
		SELECT nt.id, nt.node_pubkey, nt.user_id,
		       COALESCE(NULLIF(u.display_name,''), u.email), nt.visibility, nt.body,
		       nt.created_at, nt.updated_at
		FROM node_notes nt JOIN users u ON u.id = nt.user_id
		WHERE nt.id = ?`, id).
		Scan(&n.ID, &n.NodePubkey, &n.UserID, &n.AuthorName, &n.Visibility, &n.Body,
			&n.CreatedAt, &n.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Note{}, false, nil
	}
	if err != nil {
		return Note{}, false, err
	}
	return n, true, nil
}

// NotesForNode returns the notes visible to viewerUserID on a node: every public
// note, the viewer's own notes, and — when inCircle is true (the viewer is the
// node's owner or a shared-with user) — the node's "team" notes. Pass
// viewerUserID=0 for an anonymous viewer (public only). Newest first.
func (s *Store) NotesForNode(nodePubkey string, viewerUserID int64, inCircle bool) ([]Note, error) {
	nodePubkey = strings.ToUpper(nodePubkey)
	rows, err := s.db.Query(`
		SELECT nt.id, nt.node_pubkey, nt.user_id,
		       COALESCE(NULLIF(u.display_name,''), u.email), nt.visibility, nt.body,
		       nt.created_at, nt.updated_at
		FROM node_notes nt JOIN users u ON u.id = nt.user_id
		WHERE nt.node_pubkey = ?
		  AND (nt.visibility = 'public'
		       OR nt.user_id = ?
		       OR (nt.visibility = 'team' AND ?))
		ORDER BY nt.created_at DESC`, nodePubkey, viewerUserID, inCircle)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Note
	for rows.Next() {
		var n Note
		if err := rows.Scan(&n.ID, &n.NodePubkey, &n.UserID, &n.AuthorName, &n.Visibility,
			&n.Body, &n.CreatedAt, &n.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// UpdateNote edits a note's body/visibility. Only the author may edit. Returns
// ok=false if the note doesn't exist; ErrNotAuthor if userID isn't the author.
func (s *Store) UpdateNote(id, userID int64, visibility, body string) (Note, bool, error) {
	existing, ok, err := s.GetNote(id)
	if err != nil || !ok {
		return Note{}, ok, err
	}
	if existing.UserID != userID {
		return Note{}, true, ErrNotAuthor
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := s.db.Exec(`UPDATE node_notes SET visibility = ?, body = ?, updated_at = ? WHERE id = ?`,
		visibility, body, now, id); err != nil {
		return Note{}, true, err
	}
	n, _, err := s.GetNote(id)
	return n, true, err
}

// DeleteNote removes a note. The author may always delete their own note; if
// canModerate is true (node owner or admin) any note may be deleted. Returns
// ok=false if the note doesn't exist; ErrNotAuthor if not permitted.
func (s *Store) DeleteNote(id, userID int64, canModerate bool) (bool, error) {
	existing, ok, err := s.GetNote(id)
	if err != nil || !ok {
		return ok, err
	}
	if existing.UserID != userID && !canModerate {
		return true, ErrNotAuthor
	}
	_, err = s.db.Exec(`DELETE FROM node_notes WHERE id = ?`, id)
	return true, err
}

// ErrNotAuthor is returned when a user tries to modify a note they don't own
// (and lack moderation rights).
var ErrNotAuthor = errors.New("store: not the note author")
