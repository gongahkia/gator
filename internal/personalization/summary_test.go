package personalization

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/learning"
	"github.com/gongahkia/gator/internal/memory"
)

func TestSummaryUsesOwningStoresAndSeparatesUnconfirmedAndProjectContext(t *testing.T) {
	dir := t.TempDir()
	memories, _ := memory.Open(dir)
	personal, err := memories.RememberChat(memory.ChatInput{Subject: "preference.response_length", Kind: "preference", Quote: "I prefer concise answers.", ConversationID: "work-personal", RunID: "run-personal"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := memories.Add("fact", "PRIVATE PROJECT FACT", "goal-"+strings.Repeat("a", 24), "user", time.Time{}); err != nil {
		t.Fatal(err)
	}
	if _, err := memories.Add("fact", "EXPIRED FACT", "", "user", time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "source.txt")
	if err := os.WriteFile(file, []byte("one"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := memories.AddFromFile("fact", "CHANGED SOURCE FACT", "", file, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("two"), 0600); err != nil {
		t.Fatal(err)
	}
	lessons, _ := learning.Open(dir)
	candidate, err := lessons.Create(learning.Create{Type: learning.Procedure, Key: "candidate", Content: "UNCONFIRMED LESSON", Origin: learning.Inferred, Confidence: 50, Scope: learning.Scope{Kind: learning.Global}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lessons.Create(learning.Create{Type: learning.Procedure, Key: "project", Content: "PRIVATE PROJECT GUIDANCE", Scope: learning.Scope{Kind: learning.Project, Value: t.TempDir()}}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(snapshot)
	for _, hidden := range []string{"PRIVATE", "EXPIRED FACT", "CHANGED SOURCE FACT", "UNCONFIRMED LESSON"} {
		if strings.Contains(string(data), hidden) {
			t.Fatal("leaked inactive/scoped content", hidden)
		}
	}
	if len(snapshot.Entries) != 1 || snapshot.PendingLessons != 1 || snapshot.NeedsReview != 2 || snapshot.Entries[0].ConversationID != "work-personal" || snapshot.Entries[0].ConfirmedAt.IsZero() {
		t.Fatalf("incomplete summary: %#v", snapshot)
	}
	if _, err := memories.Correct(personal.ID, "I prefer detailed reviews.", "user"); err != nil {
		t.Fatal(err)
	}
	if _, err := lessons.Enable(candidate.ID); err != nil {
		t.Fatal(err)
	}
	snapshot, err = Read(dir)
	if err != nil || len(snapshot.Entries) != 2 || snapshot.PendingLessons != 0 || strings.Contains(strings.Join(snapshot.Lines(), "\n"), "concise answers") {
		t.Fatal("summary retained stale correction", snapshot, err)
	}
	if err := memories.Forget(personal.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := lessons.Disable(candidate.ID); err != nil {
		t.Fatal(err)
	}
	snapshot, err = Read(dir)
	if err != nil || len(snapshot.Entries) != 0 {
		t.Fatal("forgotten or disabled content survived", snapshot, err)
	}
}

func TestSummaryPreservesHealthyFactsWithVisibleDiagnosticAndBounds(t *testing.T) {
	dir := t.TempDir()
	store, _ := memory.Open(dir)
	for i := 0; i < maxEntries+3; i++ {
		if _, err := store.Add("fact", "Healthy personal fact", "", "user", time.Time{}); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "gator", "memory", "broken.json"), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := Read(dir)
	if err != nil || len(snapshot.Entries) != maxEntries || snapshot.Omitted != 3 || len(snapshot.Warnings) != 1 {
		t.Fatalf("summary: %#v %v", snapshot, err)
	}
	if !strings.Contains(strings.Join(snapshot.Lines(), "\n"), "Warning:") {
		t.Fatal("hidden corruption")
	}
}
