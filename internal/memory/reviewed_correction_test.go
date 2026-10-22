package memory

import (
	"strings"
	"testing"
	"time"
)

func TestCorrectReviewedRejectsStaleAndMissingRevisions(t *testing.T) {
	store, _ := Open(t.TempDir())
	record, _ := store.Add("preference", "Original", "", "user", time.Time{})
	newer, err := store.Correct(record.ID, "Newer", "user")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CorrectReviewed(record.ID, "Stale edit", "user", record.UpdatedAt.Format(time.RFC3339Nano)); err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("accepted stale correction: %v", err)
	}
	if _, err := store.CorrectReviewed(record.ID, "Missing revision", "user", ""); err == nil {
		t.Fatal("accepted missing revision")
	}
	current, _ := store.Load(record.ID)
	if current.Text != newer.Text {
		t.Fatal("stale review overwrote newer fact")
	}
	if _, err := store.CorrectReviewed(record.ID, "Reviewed edit", "user", newer.UpdatedAt.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
}
