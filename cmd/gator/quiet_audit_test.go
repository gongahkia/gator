package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/jobs"
)

func TestQuietWatchAuditLabelsMissedChecksWithoutCountingUnchangedOrAlerts(t *testing.T) {
	state, source := t.TempDir(), t.TempDir()
	t.Setenv("GATOR_STATE_DIR", state)
	path := filepath.Join(source, "venue.txt")
	if err := os.WriteFile(path, []byte("Venue A available\n"), 0600); err != nil {
		t.Fatal(err)
	}
	goals, err := goal.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	current, err := goals.Add("Plan offsite", "Confirm a venue", source)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := goalCommand([]string{"watch", current.ID, "--task", "Watch venue changes"}, &output); err != nil {
		t.Fatal(err)
	}
	store, err := jobs.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	definitions, err := store.List()
	if err != nil || len(definitions) != 1 {
		t.Fatalf("watch definitions = %+v, %v", definitions, err)
	}
	jobID := definitions[0].ID
	now := time.Now().UTC().Truncate(time.Minute).Add(time.Minute)
	run := func(at time.Time, attention bool, summary string) {
		t.Helper()
		runDueJobsContextWith(context.Background(), store, state, false, at, func(context.Context, jobs.Definition, string, ...string) (jobResult, error) {
			return jobResult{Status: "completed", Summary: summary, ResultSHA256: summary, WatchAttention: attention}, nil
		})
	}
	run(now, false, "No action needed")
	if err := os.WriteFile(path, []byte("Venue A cancelled\n"), 0600); err != nil {
		t.Fatal(err)
	}
	run(now.Add(time.Minute), true, "Venue A cancelled")
	if err := os.WriteFile(path, []byte("Venue B confirmed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	run(now.Add(2*time.Minute), false, "No action needed after confirmation")
	run(now.Add(3*time.Minute), false, "This callback should not run")
	history, err := store.History(jobID, 0)
	if err != nil || len(history) != 4 {
		t.Fatalf("watch history = %+v, %v", history, err)
	}
	quietIDs := make([]string, 0, 2)
	var alertedID, unchangedID string
	for _, attempt := range history {
		switch {
		case attempt.Status == "unchanged":
			unchangedID = attempt.ID
		case attempt.WatchAttention:
			alertedID = attempt.ID
		case attempt.Status == "completed":
			quietIDs = append(quietIDs, attempt.ID)
		}
	}
	if len(quietIDs) != 2 || alertedID == "" || unchangedID == "" {
		t.Fatalf("watch decision categories = %+v", history)
	}
	limited, err := store.QuietWatchAudit(jobID, 1)
	if err != nil || limited.Total != 2 || limited.Unreviewed != 2 || len(limited.Entries) != 1 {
		t.Fatalf("limited quiet audit lost total counts: %+v, %v", limited, err)
	}
	output.Reset()
	if err := quietAuditCommand([]string{jobID}, &output); err != nil || !strings.Contains(output.String(), "2 total, 0 missed, 0 correct, 2 unreviewed") {
		t.Fatalf("initial quiet audit = %q, %v", output.String(), err)
	}
	if err := quietAuditCommand([]string{jobID, quietIDs[0], "missed"}, &output); err == nil {
		t.Fatal("missed verdict without explanation was accepted")
	}
	if err := quietAuditCommand([]string{jobID, quietIDs[0], "missed", "--note", "The venue deadline mattered"}, &output); err != nil {
		t.Fatal(err)
	}
	if err := quietAuditCommand([]string{jobID, quietIDs[1], "correct"}, &output); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{alertedID, unchangedID, "../outside"} {
		if err := quietAuditCommand([]string{jobID, id, "correct"}, &output); err == nil {
			t.Fatalf("ineligible attempt %q was rated", id)
		}
	}
	output.Reset()
	if err := quietAuditCommand([]string{jobID}, &output); err != nil || !strings.Contains(output.String(), "2 total, 1 missed, 1 correct, 0 unreviewed") || !strings.Contains(output.String(), `note="The venue deadline mattered"`) {
		t.Fatalf("rated quiet audit = %q, %v", output.String(), err)
	}
	if err := quietAuditCommand([]string{jobID, quietIDs[0], "clear"}, &output); err != nil {
		t.Fatal(err)
	}
	if err := store.Remove(jobID); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := quietAuditCommand([]string{jobID}, &output); err != nil || !strings.Contains(output.String(), "2 total, 0 missed, 1 correct, 1 unreviewed") {
		t.Fatalf("audit after correction and job removal = %q, %v", output.String(), err)
	}
	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outside, []byte(`{"version":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	reviewPath := filepath.Join(store.Root(), "history", jobID, quietIDs[0], "quiet-review.json")
	if err := os.Remove(reviewPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, reviewPath); err != nil {
		t.Fatal(err)
	}
	if err := quietAuditCommand([]string{jobID}, &output); err == nil {
		t.Fatal("quiet audit followed a review symlink outside state")
	}
}
