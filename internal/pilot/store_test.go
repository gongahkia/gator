package pilot

import (
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/workhistory"
)

func TestPilotCountsOnlyExplicitReviewsAndDeduplicatesRunUsage(t *testing.T) {
	state := t.TempDir()
	history, _ := workhistory.Open(state)
	now := time.Now().UTC()
	for _, id := range []string{"work-reviewed", "work-unreviewed", "work-running"} {
		_, err := history.Start(workhistory.Start{ID: id, Objective: "Project reply", Mode: action.Draft, ExternalActions: action.Forbid, StartedAt: now.Add(-time.Second)})
		if err != nil {
			t.Fatal(err)
		}
		if id != "work-running" {
			if _, err := history.Finish(id, workhistory.Finish{Succeeded: true, FinishedAt: now, VerificationStatus: workhistory.VerificationUnavailable}); err != nil {
				t.Fatal(err)
			}
		}
	}
	store, _ := Open(state)
	if _, err := store.Save(Review{WorkID: "work-running", Reviewer: "one", Draft: "usable"}); err == nil {
		t.Fatal("running Work received a success judgment")
	}
	if _, err := store.Save(Review{WorkID: "work-reviewed", Reviewer: "one", Draft: "light-edits", Corrections: 2}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Save(Review{WorkID: "work-reviewed", Reviewer: "one", Draft: "usable", Corrections: 0}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Save(Review{WorkID: "work-reviewed", Reviewer: "two", Draft: "rewrite", Corrections: 4, Followup: "unhelpful"}); err != nil {
		t.Fatal(err)
	}
	report, err := store.Report("")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Reviews) != 2 || report.UniqueRuns != 1 || report.Usable != 1 || report.Rewrite != 1 || report.Corrections != 4 || report.UsefulFollowups != 0 || report.UnhelpfulFollowups != 1 {
		t.Fatalf("review denominators are wrong: %#v", report)
	}
	if report.KnownCostUSD != nil || !report.UsageIncomplete {
		t.Fatal("missing cost was treated as zero")
	}
}
