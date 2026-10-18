package jobs

import (
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/artifact"
)

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
