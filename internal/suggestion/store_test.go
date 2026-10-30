package suggestion

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/privatefs"
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
	for _, path := range []string{store.root, store.path(record.ID)} {
		if err := privatefs.Check(path); err != nil {
			t.Fatalf("private suggestion path %s: %v", path, err)
		}
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

func TestPendingPlanProposalReusesIdentityAcrossConcurrentRuns(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	base := Record{Kind: "plan", GoalID: "goal-aaaaaaaaaaaaaaaaaaaaaaaa", WorkID: "work-first", PlanChange: goal.PlanChange{Kind: "add_step", Title: "Ask for updated dates"}}
	ids := make(chan string, 4)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			record, err := store.Create(base)
			if err != nil {
				t.Error(err)
				return
			}
			ids <- record.ID
		}()
	}
	wg.Wait()
	close(ids)
	var id string
	for got := range ids {
		if id != "" && got != id {
			t.Fatal("duplicate pending identities")
		}
		id = got
	}
	base.WorkID = "work-later"
	again, err := store.Create(base)
	if err != nil || again.ID != id || again.WorkID != "work-first" {
		t.Fatalf("retry lost identity/provenance: %#v %v", again, err)
	}
	base.PlanChange.ExpectedRevision = 1
	changed, err := store.Create(base)
	if err != nil || changed.ID == id {
		t.Fatalf("different revision collapsed: %#v %v", changed, err)
	}
}
