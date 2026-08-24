package journal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReviewFeedbackIsPrivateValidatedAndProducesBoundedFollowUp(t *testing.T) {
	record := openJournalForReview(t)
	saved, err := SaveReviewFeedback(record.StatePath, ReviewFeedback{
		File:        "internal/tui/view.go",
		HunkID:      "0123456789abcdef01234567",
		Side:        "new",
		StartLine:   42,
		EndLine:     43,
		Before:      []string{"- old state"},
		After:       []string{"+ new state"},
		Instruction: "Preserve the empty state and add a focused test.",
	})
	if err != nil {
		t.Fatalf("save feedback: %v", err)
	}
	if saved.ID == "" || saved.Version != reviewFeedbackVersion {
		t.Fatalf("saved feedback = %#v", saved)
	}
	path := filepath.Join(record.StatePath, "review-feedback.json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("feedback mode = %o, want 600", info.Mode().Perm())
	}
	values, err := ListReviewFeedback(record.StatePath)
	if err != nil || len(values) != 1 || values[0].ID != saved.ID {
		t.Fatalf("loaded feedback = %#v, %v", values, err)
	}
	prompt := ReviewFollowUp(saved)
	for _, expected := range []string{"Address this developer review request", "internal/tui/view.go", "Selected new lines 42-43", "untrusted code context", "Preserve the empty state"} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("follow-up missing %q:\n%s", expected, prompt)
		}
	}
}

func TestReviewFeedbackRejectsUnsafeOrOversizedInput(t *testing.T) {
	record := openJournalForReview(t)
	_, err := SaveReviewFeedback(record.StatePath, ReviewFeedback{File: "../secret", Side: "new", StartLine: 1, EndLine: 1, Instruction: "bad"})
	if err == nil || !strings.Contains(err.Error(), "outside") {
		t.Fatalf("unsafe path error = %v", err)
	}
	_, err = SaveReviewFeedback(record.StatePath, ReviewFeedback{File: "safe.go", Side: "new", StartLine: 1, EndLine: 1, Instruction: strings.Repeat("x", maximumReviewInstruction+1)})
	if err == nil || !strings.Contains(err.Error(), "instruction") {
		t.Fatalf("oversized instruction error = %v", err)
	}
}

func openJournalForReview(t *testing.T) Record {
	t.Helper()
	repository := t.TempDir()
	worktree := filepath.Join(repository, "worktree")
	if err := os.MkdirAll(worktree, 0o700); err != nil {
		t.Fatal(err)
	}
	journal, record, err := Open(repository, "run-review-001", worktree, t.TempDir(), time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	defer journal.Close()
	if err := journal.SaveSession(Session{Version: 2, Repository: repository, WorktreePath: worktree, Provider: "openai", Model: "test", Task: "Review changes"}); err != nil {
		t.Fatal(err)
	}
	return record
}
