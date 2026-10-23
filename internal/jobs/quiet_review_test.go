package jobs

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/artifact"
)

func TestQuietReviewGuardsConcurrentAssessmentsAndRetainsAttempt(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	definition, err := store.Save(Definition{Name: "Watch deadline", GoalID: "goal-0123456789abcdef01234567", GoalWatch: true, ChangeOnly: true, RefreshSnapshot: true, Missed: "run_once", Schedule: "* * * * *", SourcePath: t.TempDir(), Objective: "Watch the deadline", Mode: action.Inspect, Contract: artifact.InspectionContract()})
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := store.Begin(definition, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	attempt.Status, attempt.Summary, attempt.FinishedAt = "completed", "No deadline change", attempt.StartedAt.Add(time.Second)
	attempt.WatchDecision = &WatchDecision{Status: "quiet", Summary: attempt.Summary}
	if err := store.Record(attempt); err != nil {
		t.Fatal(err)
	}
	revision := (QuietReview{}).Revision()
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, verdict := range []string{"correct", "missed"} {
		wg.Add(1)
		go func(verdict string) {
			defer wg.Done()
			_, err := store.ReviewQuietWatchReviewed(definition.ID, attempt.ID, verdict, "The published deadline changed", revision, time.Now())
			results <- err
		}(verdict)
	}
	wg.Wait()
	close(results)
	succeeded := 0
	for err := range results {
		if err == nil {
			succeeded++
		}
	}
	if succeeded != 1 {
		t.Fatalf("%d conflicting reviews succeeded", succeeded)
	}
	audit, err := store.QuietWatchAudit(definition.ID, 50)
	if err != nil || audit.Total != 1 || audit.Unreviewed != 0 {
		t.Fatalf("audit: %+v, %v", audit, err)
	}
	if audit.Entries[0].Attempt.Summary != attempt.Summary || audit.Entries[0].Attempt.WatchAttention {
		t.Fatal("feedback changed the original check")
	}
	current := audit.Entries[0].Review.Revision()
	if _, err := store.ReviewQuietWatchReviewed(definition.ID, attempt.ID, "clear", "", revision, time.Now()); err == nil {
		t.Fatal("stale clear erased newer review")
	}
	now := time.Now()
	cleared, err := store.ReviewQuietWatchReviewed(definition.ID, attempt.ID, "clear", "", current, now)
	if err != nil {
		t.Fatal(err)
	}
	if cleared.Version != 2 || cleared.Sequence != audit.Entries[0].Review.Sequence+1 || cleared.Verdict != "" || cleared.Note != "" {
		t.Fatal("clear did not retain a newer, empty assessment", cleared)
	}
	audit, err = store.QuietWatchAudit(definition.ID, 50)
	if err != nil || audit.Total != 1 || audit.Unreviewed != 1 || audit.Correct != 0 || audit.Missed != 0 || audit.Entries[0].Review.Revision() != cleared.Revision() {
		t.Fatalf("cleared audit: %+v, %v", audit, err)
	}
	for _, stale := range []string{revision, current} {
		if _, err := store.ReviewQuietWatchReviewed(definition.ID, attempt.ID, "correct", "", stale, now); err == nil {
			t.Fatal("clear made an older review token valid again")
		}
	}
	// Even identical wall times and repeated clears must advance the revision.
	again, err := store.ReviewQuietWatchReviewed(definition.ID, attempt.ID, "clear", "", cleared.Revision(), now)
	if err != nil || again.Sequence != cleared.Sequence+1 || again.Revision() == cleared.Revision() {
		t.Fatal("repeated clear did not advance revision", again, err)
	}
	if _, err := store.ReviewQuietWatchReviewed(definition.ID, attempt.ID, "correct", "", cleared.Revision(), now); err == nil {
		t.Fatal("earlier cleared revision accepted")
	}
	if _, err := store.ReviewQuietWatchReviewed(definition.ID, attempt.ID, "missed", "", again.Revision(), time.Now()); err == nil {
		t.Fatal("missed assessment without evidence accepted")
	}
	if _, err := store.ReviewQuietWatchReviewed(definition.ID, attempt.ID, "correct", "", "", time.Now()); err == nil {
		t.Fatal("unreviewed mutation accepted")
	}
}

func TestQuietReviewLoadsLegacyAndValidatesTombstoneSchema(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := store.quietReviewPath("job-one", "attempt-one")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	legacy := QuietReview{Version: 1, JobID: "job-one", AttemptID: "attempt-one", Verdict: "correct", RatedAt: time.Now()}
	tombstone := legacy
	tombstone.Version, tombstone.Sequence, tombstone.Verdict = 2, 1, ""
	for _, review := range []QuietReview{legacy, tombstone} {
		if err := writeJSON(path, review); err != nil {
			t.Fatal(err)
		}
		loaded, err := store.loadQuietReview("job-one", "attempt-one")
		if err != nil || loaded.Revision() != review.Revision() {
			t.Fatal("valid retained review did not load", loaded, err)
		}
	}
	for name, mutate := range map[string]func(*QuietReview){
		"unsupported version":   func(r *QuietReview) { r.Version = 3 },
		"missing sequence":      func(r *QuietReview) { r.Sequence = 0 },
		"legacy sequence":       func(r *QuietReview) { r.Version, r.Verdict = 1, "correct" },
		"legacy tombstone":      func(r *QuietReview) { r.Version, r.Sequence = 1, 0 },
		"retained cleared note": func(r *QuietReview) { r.Note = "Should have been removed" },
		"missing time":          func(r *QuietReview) { r.RatedAt = time.Time{} },
		"wrong target":          func(r *QuietReview) { r.JobID = "job-two" },
	} {
		t.Run(name, func(t *testing.T) {
			review := tombstone
			mutate(&review)
			if err := writeJSON(path, review); err != nil {
				t.Fatal(err)
			}
			if _, err := store.loadQuietReview("job-one", "attempt-one"); err == nil {
				t.Fatal("invalid tombstone schema loaded")
			}
		})
	}
}

func TestQuietReviewRejectsOrdinaryScheduledJob(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	definition, err := store.Save(Definition{Name: "Daily brief", Enabled: true, Schedule: "0 9 * * *", Timezone: "UTC", SourcePath: t.TempDir(), Objective: "Write brief", Mode: action.Draft, Contract: artifact.DefaultContract("brief.md")})
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := store.Begin(definition, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	attempt.Status = "completed"
	attempt.FinishedAt = attempt.StartedAt.Add(time.Second)
	if err := store.Record(attempt); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReviewQuietWatch(definition.ID, attempt.ID, "correct", "", time.Now()); err == nil {
		t.Fatal("ordinary scheduled job was reviewed as a quiet watch")
	}
	if _, err := store.QuietWatchAudit(definition.ID, 50); err == nil {
		t.Fatal("ordinary scheduled job had a quiet watch audit")
	}
}
