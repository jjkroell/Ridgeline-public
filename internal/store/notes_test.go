package store

import "testing"

const noteNode = "AABBCCDDEEFF00112233445566778899AABBCCDDEEFF00112233445566778899"

func TestNotesVisibility(t *testing.T) {
	st := testStore(t)
	a, _ := st.CreateUser("a@example.com", "h", "Alice")
	b, _ := st.CreateUser("b@example.com", "h", "Bob")

	pub, _ := st.CreateNote(noteNode, a.ID, "public", "public note by alice")
	st.CreateNote(noteNode, a.ID, "private", "alice's private note")
	st.CreateNote(noteNode, b.ID, "private", "bob's private note")
	st.CreateNote(noteNode, a.ID, "team", "alice's team note")

	// Anonymous viewer sees only public notes (never team).
	anon, _ := st.NotesForNode(noteNode, 0, false)
	if len(anon) != 1 || anon[0].Body != "public note by alice" {
		t.Fatalf("anon should see only the public note, got %d", len(anon))
	}
	if anon[0].AuthorName != "Alice" {
		t.Errorf("author name should resolve, got %q", anon[0].AuthorName)
	}

	// Alice (author, in circle) sees public + her own private + her team note.
	av, _ := st.NotesForNode(noteNode, a.ID, true)
	if len(av) != 3 {
		t.Errorf("alice should see 3 notes (public + her private + team), got %d", len(av))
	}
	for _, n := range av {
		if n.Body == "bob's private note" {
			t.Error("alice must not see bob's private note")
		}
	}

	// Bob NOT in the circle sees public + his own private, but NOT alice's team note.
	bv, _ := st.NotesForNode(noteNode, b.ID, false)
	if len(bv) != 2 {
		t.Errorf("out-of-circle bob should see 2 notes, got %d", len(bv))
	}
	for _, n := range bv {
		if n.Visibility == "team" {
			t.Error("out-of-circle bob must not see the team note")
		}
	}

	// Bob once in the circle sees the team note too (public + his private + team).
	bvc, _ := st.NotesForNode(noteNode, b.ID, true)
	if len(bvc) != 3 {
		t.Errorf("in-circle bob should see 3 notes, got %d", len(bvc))
	}

	// Only the author can edit.
	if _, _, err := st.UpdateNote(pub.ID, b.ID, "public", "hijack"); err != ErrNotAuthor {
		t.Errorf("non-author edit should be ErrNotAuthor, got %v", err)
	}
	if _, _, err := st.UpdateNote(pub.ID, a.ID, "private", "now private"); err != nil {
		t.Errorf("author edit should succeed: %v", err)
	}

	// Non-author without moderation can't delete; a moderator can.
	if _, err := st.DeleteNote(pub.ID, b.ID, false); err != ErrNotAuthor {
		t.Errorf("non-author delete should be ErrNotAuthor, got %v", err)
	}
	if _, err := st.DeleteNote(pub.ID, b.ID, true); err != nil {
		t.Errorf("moderator delete should succeed: %v", err)
	}
	if _, ok, _ := st.GetNote(pub.ID); ok {
		t.Error("note should be gone after delete")
	}
}
