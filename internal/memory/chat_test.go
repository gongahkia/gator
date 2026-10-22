package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestChatMemoryConsolidatesWithProvenanceAndRespectsCorrectionForget(t *testing.T) {
	s, _ := Open(t.TempDir())
	input := ChatInput{Subject: "preference.response_length", Kind: "preference", Quote: "I prefer short replies.", ConversationID: "work-first", RunID: "run-first"}
	first, err := s.RememberChat(input)
	if err != nil {
		t.Fatal(err)
	}
	input.ExcludedIDs = []string{first.ID}
	if _, err := s.RememberChat(input); err == nil {
		t.Fatal("an excluded subject could be read or updated through remembering")
	}
	input.ExcludedIDs = nil
	again, err := s.RememberChat(input)
	if err != nil || again.ID != first.ID {
		t.Fatal("duplicate memory", err)
	}
	input.Quote = "I prefer detailed replies."
	if _, err = s.RememberChat(input); err == nil {
		t.Fatal("silent conflicting overwrite")
	}
	s.now = func() time.Time { return first.UpdatedAt }
	input.ReplaceID, input.ExpectedUpdated = first.ID, first.UpdatedAt.Format(time.RFC3339Nano)
	next, err := s.RememberChat(input)
	if err != nil || next.ID != first.ID || next.Origin.Quote != input.Quote {
		t.Fatal(next, err)
	}
	corrected, err := s.Correct(first.ID, "Use bullet points, with detail when needed.", "user")
	if err != nil {
		t.Fatal(err)
	}
	if corrected.Origin.Basis != "user-corrected" || corrected.Origin.Quote != "" {
		t.Fatal("correction retained a misleading quote")
	}
	if _, err = s.RememberChat(input); err == nil {
		t.Fatal("stale agent overwrote user correction")
	}
	profile, err := s.Profile(nil)
	if err != nil || len(profile) != 1 || profile[0].ID != first.ID {
		t.Fatal(profile, err)
	}
	profile, err = s.Profile([]string{first.ID})
	if err != nil || len(profile) != 0 {
		t.Fatal("excluded memory entered capsule")
	}
	if err = s.Forget(first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Load(first.ID); !os.IsNotExist(err) {
		t.Fatal("forgotten memory still exists")
	}
	input.ReplaceID, input.ExpectedUpdated = "", ""
	if _, err = s.RememberChat(input); err == nil {
		t.Fatal("forgotten subject silently relearned")
	}
	entries, _ := os.ReadDir(filepath.Join(s.root, ".forgotten"))
	for _, entry := range entries {
		b, _ := os.ReadFile(filepath.Join(s.root, ".forgotten", entry.Name()))
		if strings.Contains(string(b), "replies") || strings.Contains(entry.Name(), "preference") {
			t.Fatal("suppression retained forgotten text")
		}
	}
	input.Explicit = true
	if _, err = s.RememberChat(input); err != nil {
		t.Fatal("explicit re-remember failed", err)
	}
	scoped := input
	scoped.Subject = "preference.response_length"
	scoped.GoalID = "goal-0123456789abcdef01234567"
	scoped.Quote = "Use detailed research in this goal."
	scoped.Explicit = false
	if _, err = s.RememberChat(scoped); err != nil {
		t.Fatal(err)
	}
	profile, _ = s.Profile(nil)
	if len(profile) != 1 || profile[0].GoalID != "" {
		t.Fatal("goal memory leaked into profile")
	}
}
