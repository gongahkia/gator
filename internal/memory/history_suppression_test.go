package memory

import (
	"fmt"
	"testing"
)

func TestForgetSuppressesEveryTrackedOriginAfterEvidenceRotation(t *testing.T) {
	dir := t.TempDir()
	store, _ := Open(dir)
	input := ChatInput{Subject: "profile.city", Kind: "fact", Quote: "I live in Harbor City."}
	var record Record
	for i := 0; i < 14; i++ {
		input.ConversationID = fmt.Sprintf("work-source-%d", i)
		input.RunID = fmt.Sprintf("run-%d", i)
		var err error
		record, err = store.RememberChat(input)
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(record.Evidence) != maxMemoryEvidence {
		t.Fatal("fixture did not rotate evidence")
	}
	if err := store.Forget(record.ID); err != nil {
		t.Fatal(err)
	}
	store, _ = Open(dir)
	for i := 0; i < 14; i++ {
		blocked, err := store.HistorySuppressed(fmt.Sprintf("work-source-%d", i), "")
		if err != nil || !blocked {
			t.Fatal("forgotten source resurfaced", i, err)
		}
	}
	if blocked, err := store.HistorySuppressed("work-unrelated", ""); err != nil || blocked {
		t.Fatal("unrelated chat suppressed", err)
	}
	input.Explicit = true
	input.ConversationID = "work-restored"
	input.RunID = "run-restored"
	if _, err := store.RememberChat(input); err != nil {
		t.Fatal(err)
	}
	if blocked, err := store.HistorySuppressed("work-source-0", ""); err != nil || !blocked {
		t.Fatal("restoring fact re-enabled old history", err)
	}
	if blocked, err := store.HistorySuppressed("work-restored", ""); err != nil || blocked {
		t.Fatal("new evidence suppressed", err)
	}
}

func TestHistorySuppressionRetainsGoalBoundary(t *testing.T) {
	store, _ := Open(t.TempDir())
	goal := "goal-0123456789abcdef01234567"
	record, err := store.RememberChat(ChatInput{Subject: "decision.venue", Kind: "decision", GoalID: goal, Quote: "Use Harbor Hall.", ConversationID: "work-source", RunID: "run-one"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Forget(record.ID); err != nil {
		t.Fatal(err)
	}
	if blocked, err := store.HistorySuppressed("work-source", goal); err != nil || !blocked {
		t.Fatal("goal suppression lost", err)
	}
	for _, scope := range []string{"", "goal-1123456789abcdef01234567"} {
		if blocked, err := store.HistorySuppressed("work-source", scope); err != nil || blocked {
			t.Fatal("scope crossed", scope, err)
		}
	}
}
