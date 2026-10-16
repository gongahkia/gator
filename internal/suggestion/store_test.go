package suggestion

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/gongahkia/gator/internal/goal"
)

func TestSuggestionRemainsPendingUntilOneReviewDecision(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Create(Record{Kind: "note", Title: "Offsite budget", Body: "Budget is $2k", WorkID: "work-1"})
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != "pending" {
		t.Fatalf("created status = %q", record.Status)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := store.Resolve(record.ID, "accept", func(Record) (string, error) { return "note-1", nil })
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("successful decisions = %d", successes)
	}
	loaded, err := store.Load(record.ID)
	if err != nil || loaded.Status != "accepted" || loaded.ResultID != "note-1" {
		t.Fatalf("resolved = %#v, %v", loaded, err)
	}
	info, err := os.Stat(store.path(record.ID))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("private record = %v, %v", info, err)
	}
}

func TestSuggestionRejectsSymlinkAndInvalidPlan(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(Record{Kind: "plan", GoalID: "goal-000000000000000000000000", WorkID: "work-1"}); err == nil {
		t.Fatal("invalid plan accepted")
	}
	if _, err := store.Create(Record{Kind: "schedule", GoalID: "goal-000000000000000000000000", WorkID: "work-1", Cron: "every minute", Timezone: "UTC", Task: "Check"}); err == nil {
		t.Fatal("invalid schedule accepted")
	}
	if _, err := store.Create(Record{Kind: "plan", GoalID: "goal-000000000000000000000000", WorkID: "work-1", PlanChange: goal.PlanChange{Kind: "set_outcome", Outcome: "valid", ExpectedRevision: 0}}); err != nil {
		t.Fatal(err)
	}
	record, err := store.Create(Record{Kind: "note", Title: "Title", Body: "Body", WorkID: "work-1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(store.path(record.ID)); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(t.TempDir(), "outside.json"), store.path(record.ID)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(record.ID); err == nil {
		t.Fatal("symlinked suggestion accepted")
	}
}
