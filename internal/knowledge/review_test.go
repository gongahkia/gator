package knowledge

import (
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestReviewedNoteRejectsStaleCorrectionAndDeletion(t *testing.T) {
	store, _ := Open(t.TempDir())
	note, err := store.AddNoteWithRefs("Plan", "Original plan", "", []string{"source/plan.md"})
	if err != nil {
		t.Fatal(err)
	}
	newer, err := store.UpdateNoteWithRefs(note.ID, "Revised plan", "New plan", []string{"source/new-plan.md"})
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"correct", "forget"} {
		if _, err := store.ReviewNote(note.ID, "", note.Revision(), action, "Stale plan"); err == nil || !strings.Contains(err.Error(), "changed") {
			t.Fatalf("%s accepted stale note: %v", action, err)
		}
	}
	current, _ := store.LoadNote(note.ID)
	if current.Revision() != newer.Revision() {
		t.Fatal("stale review changed current note")
	}
	corrected, err := store.ReviewNote(note.ID, "", newer.Revision(), "correct", " Reviewed correction ")
	if err != nil || corrected.Body != "Reviewed correction" || corrected.Title != newer.Title || strings.Join(corrected.SourceRefs, ",") != strings.Join(newer.SourceRefs, ",") {
		t.Fatalf("correction lost metadata: %#v, %v", corrected, err)
	}
	if _, err := store.ReviewNote(note.ID, "", corrected.Revision(), "forget", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadNote(note.ID); !os.IsNotExist(err) {
		t.Fatalf("forgotten note remains: %v", err)
	}
}

func TestReviewedNoteDetectsDirectFileEditAndGoalMismatch(t *testing.T) {
	store, _ := Open(t.TempDir())
	goal := "goal-0123456789abcdef01234567"
	note, _ := store.AddNote("Plan", "Original", goal)
	if _, err := store.ReviewNote(note.ID, "", note.Revision(), "forget", ""); err == nil {
		t.Fatal("accepted note from another goal")
	}
	path, _ := store.NotePath(note.ID)
	if err := os.WriteFile(path, []byte("Edited in another app"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReviewNote(note.ID, goal, note.Revision(), "correct", "Stale edit"); err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("missed external content change: %v", err)
	}
}

func TestReviewedNoteSerializesConcurrentEditsAndAdvancesRevision(t *testing.T) {
	store, _ := Open(t.TempDir())
	fixed := time.Now().UTC()
	store.now = func() time.Time { return fixed }
	note, _ := store.AddNote("Plan", "Original", "")
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, body := range []string{"First", "Second"} {
		wg.Add(1)
		go func(body string) {
			defer wg.Done()
			_, err := store.ReviewNote(note.ID, "", note.Revision(), "correct", body)
			results <- err
		}(body)
	}
	wg.Wait()
	close(results)
	succeeded := 0
	for err := range results {
		if err == nil {
			succeeded++
		}
	}
	if succeeded != 1 {
		t.Fatalf("same displayed version accepted %d edits", succeeded)
	}
	current, _ := store.LoadNote(note.ID)
	if !current.UpdatedAt.After(note.UpdatedAt) {
		t.Fatal("revision did not advance with a fixed clock")
	}
}
