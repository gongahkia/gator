package inbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAddListAndMarkRead(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	entry, err := store.Add(Entry{Kind: "job", Title: "Weekly brief", Summary: "Completed", Status: "completed"})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := store.List(true, 10)
	if err != nil || len(entries) != 1 || entries[0].ID != entry.ID {
		t.Fatalf("List = %#v, %v", entries, err)
	}
	if err := store.MarkRead(entry.ID, time.Now()); err != nil {
		t.Fatal(err)
	}
	entries, err = store.List(true, 10)
	if err != nil || len(entries) != 0 {
		t.Fatalf("unread List = %#v, %v", entries, err)
	}
}

func TestMarkReadRejectsTraversal(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.MarkRead("../outside", time.Now()); err == nil {
		t.Fatal("inbox traversal ID was accepted")
	}
}

func TestGoalAlertRatingsPreserveReadStateAndCanBeCorrected(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	goalA := "goal-" + strings.Repeat("a", 24)
	goalB := "goal-" + strings.Repeat("b", 24)
	first, err := store.Add(Entry{Kind: "job", GoalID: goalA, Title: "Venue changed", Status: "completed"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Add(Entry{Kind: "job", GoalID: goalA, Title: "Another update", Status: "completed"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Add(Entry{Kind: "job", GoalID: goalB, Title: "Other goal", Status: "completed"}); err != nil {
		t.Fatal(err)
	}
	reminder, err := store.Add(Entry{Kind: "goal_review", GoalID: goalA, Title: "Review due", Status: "review_due"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Rate(reminder.ID, "noise", time.Now()); err == nil {
		t.Fatal("goal reminder was treated as a job alert")
	}
	if _, err := store.Rate(first.ID, "ignored", time.Now()); err == nil {
		t.Fatal("unknown rating was accepted")
	}
	when := time.Date(2026, 10, 17, 8, 0, 0, 0, time.UTC)
	if _, err := store.Rate(first.ID, "useful", when); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Rate(second.ID, "noise", when); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkRead(first.ID, when.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	retained, err := store.Load(first.ID)
	if err != nil || retained.Rating != "useful" || !retained.RatedAt.Equal(when) || retained.ReadAt.IsZero() {
		t.Fatalf("read changed rating = %+v, %v", retained, err)
	}
	summary, err := store.RatingsForGoal(goalA)
	if err != nil || summary.Total != 2 || summary.Useful != 1 || summary.Noise != 1 || summary.Wrong != 0 || summary.Unrated != 0 {
		t.Fatalf("goal rating summary = %+v, %v", summary, err)
	}
	if _, err := store.Rate(first.ID, "wrong", when.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Rate(second.ID, "clear", when.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	summary, err = store.RatingsForGoal(goalA)
	if err != nil || summary.Total != 2 || summary.Useful != 0 || summary.Noise != 0 || summary.Wrong != 1 || summary.Unrated != 1 {
		t.Fatalf("corrected goal ratings = %+v, %v", summary, err)
	}
	other, err := store.RatingsForGoal(goalB)
	if err != nil || other.Total != 1 || other.Unrated != 1 {
		t.Fatalf("other goal ratings = %+v, %v", other, err)
	}
}

func TestInboxLoadRejectsSymlink(t *testing.T) {
	state := t.TempDir()
	store, err := Open(state)
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outside, []byte(`{"version":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(state, "gator", "inbox", "inbox-link.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load("inbox-link"); err == nil {
		t.Fatal("inbox symlink was read")
	}
}
