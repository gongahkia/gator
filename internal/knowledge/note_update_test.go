package knowledge

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNoteUpdateKeepsIdentityEvidenceAndRejectsStaleRetry(t *testing.T) {
	root := t.TempDir()
	s, _ := Open(root)
	note, _ := s.AddNoteWithRefs("Project", "Date: November 20", "", []string{"user:original"})
	mutation := "suggestion-0123456789abcdef01234567"
	apply := func() (Document, error) {
		return s.ApplyNoteUpdate(note.ID, "", note.Revision(), mutation, "Project", "Date: November 24", []string{"user:correction"})
	}
	changed, err := apply()
	if err != nil || changed.ID != note.ID || strings.Join(changed.SourceRefs, ",") != "user:original,user:correction" {
		t.Fatalf("update: %#v %v", changed, err)
	}
	s, _ = Open(root)
	again, err := apply()
	if err != nil || again.Revision() != changed.Revision() {
		t.Fatalf("retry changed note: %v", err)
	}
	notes, _ := s.ListNotes("")
	if len(notes) != 1 {
		t.Fatal("duplicate note created")
	}
	if _, err := s.UpdateNote(note.ID, "Project", "Date: November 25"); err != nil {
		t.Fatal(err)
	}
	if _, err := apply(); err == nil {
		t.Fatal("old acceptance overwrote newer owner edit")
	}
	current, _ := s.LoadNote(note.ID)
	if current.Body != "Date: November 25" {
		t.Fatal("owner edit lost")
	}
}

func TestNoteJournalRecoversEveryInterruptedWriteAndProtectsDirectEdits(t *testing.T) {
	for _, phase := range []string{"prepared", "body-written", "both-written", "external-edit"} {
		t.Run(phase, func(t *testing.T) {
			root := t.TempDir()
			s, _ := Open(root)
			before, _ := s.AddNote("Plan", "Old date", "")
			after := before
			after.Body = "Corrected date"
			after.UpdatedAt = s.nextUpdate(before.UpdatedAt)
			journal := noteUpdateJournal{Before: before, BeforeBody: before.Body, After: after, AfterBody: after.Body}
			data, _ := json.Marshal(journal)
			dir := s.dir("note", before.ID)
			if err := os.WriteFile(filepath.Join(dir, "update.json"), data, 0600); err != nil {
				t.Fatal(err)
			}
			switch phase {
			case "body-written":
				if err := os.WriteFile(filepath.Join(dir, "content.md"), []byte(after.Body), 0600); err != nil {
					t.Fatal(err)
				}
			case "both-written":
				if err := writeFiles(dir, after); err != nil {
					t.Fatal(err)
				}
			case "external-edit":
				if err := os.WriteFile(filepath.Join(dir, "content.md"), []byte("Owner's newer date"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			s, _ = Open(root)
			got, err := s.LoadNote(before.ID)
			if phase == "external-edit" {
				if err == nil || !strings.Contains(err.Error(), "outside an interrupted update") {
					t.Fatalf("lost conflict: %v", err)
				}
				body, _ := os.ReadFile(filepath.Join(dir, "content.md"))
				if string(body) != "Owner's newer date" {
					t.Fatal("overwrote external edit")
				}
				return
			}
			if err != nil || got.Revision() != after.Revision() {
				t.Fatalf("recovery: %#v %v", got, err)
			}
			if _, err := os.Stat(filepath.Join(dir, "update.json")); !os.IsNotExist(err) {
				t.Fatal("journal not cleared")
			}
		})
	}
}
