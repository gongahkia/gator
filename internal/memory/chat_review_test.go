package memory

import (
	"testing"
	"time"
)

func TestChatReceiptCorrectionRejectsStaleUndoAndOtherChats(t *testing.T) {
	s, _ := Open(t.TempDir())
	input := ChatInput{Subject: "preference.response_length", Kind: "preference", Quote: "I like short replies.", ConversationID: "work-first", RunID: "run-first"}
	r, err := s.RememberChat(input)
	if err != nil {
		t.Fatal(err)
	}
	version := r.UpdatedAt.Format(time.RFC3339Nano)
	if err := s.ReviewChat(r.ID, "work-other", "", version, "forget", ""); err == nil {
		t.Fatal("cross-chat forget accepted")
	}
	if err := s.ReviewChat(r.ID, "work-first", "", version, "correct", "I now want detailed replies."); err != nil {
		t.Fatal(err)
	}
	if err := s.ReviewChat(r.ID, "work-first", "", version, "undo", ""); err == nil {
		t.Fatal("stale undo deleted a correction")
	}
	current, _ := s.Load(r.ID)
	if current.Text != "I now want detailed replies." || current.Origin.Basis != "user-corrected" || current.Origin.Quote != "" {
		t.Fatal("correction retained misleading user quote", current)
	}
	version = current.UpdatedAt.Format(time.RFC3339Nano)
	if err := s.ReviewChat(r.ID, "work-first", "", version, "undo", ""); err == nil {
		t.Fatal("undo of changed save accepted")
	}
	if err := s.ReviewChat(r.ID, "work-first", "", version, "forget", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load(r.ID); err == nil {
		t.Fatal("forgotten receipt still saved")
	}
	if _, err := s.RememberChat(input); err == nil {
		t.Fatal("forgotten memory silently relearned")
	}
}
func TestUndoNewMemorySurvivesRestartAndPreservesGoalBoundary(t *testing.T) {
	directory := t.TempDir()
	s, _ := Open(directory)
	goalID := "goal-0123456789abcdef01234567"
	r, err := s.RememberChat(ChatInput{Subject: "decision.venue", Kind: "decision", Quote: "Use Birch Hall.", ConversationID: "work-first", RunID: "run-first", GoalID: goalID})
	if err != nil {
		t.Fatal(err)
	}
	s, _ = Open(directory)
	version := r.UpdatedAt.Format(time.RFC3339Nano)
	if err := s.ReviewChat(r.ID, "work-first", "", version, "undo", ""); err == nil {
		t.Fatal("other goal changed")
	}
	if err := s.ReviewChat(r.ID, "work-first", goalID, version, "undo", ""); err != nil {
		t.Fatal(err)
	}
	if records, _ := s.List(goalID, false); len(records) != 0 {
		t.Fatal("undo retained memory", records)
	}
}
