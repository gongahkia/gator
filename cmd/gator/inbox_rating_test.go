package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/inbox"
)

func TestJobInboxRatingAppearsInGoalActivity(t *testing.T) {
	state := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", state)
	goals, err := goal.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	current, err := goals.Add("Daily brief", "", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	mailbox, err := inbox.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	entry, err := mailbox.Add(inbox.Entry{Kind: "job", GoalID: current.ID, JobID: "job-brief", Title: "Changed deadline", Summary: "The submission deadline moved to Friday.", Status: "completed"})
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := inboxCommand([]string{"show", entry.ID}, &output); err != nil || !strings.Contains(output.String(), "submission deadline moved to Friday") || !strings.Contains(output.String(), "unrated") {
		t.Fatalf("inbox show = %q, %v", output.String(), err)
	}
	output.Reset()
	if err := inboxCommand([]string{"rate", entry.ID, "useful"}, &output); err != nil || !strings.Contains(output.String(), "rated useful") {
		t.Fatalf("inbox rating = %q, %v", output.String(), err)
	}
	output.Reset()
	if err := inboxCommand([]string{"report", current.ID}, &output); err != nil || !strings.Contains(output.String(), "1 total, 1 useful, 0 noise, 0 wrong, 0 unrated") || !strings.Contains(output.String(), "does not measure missed alerts") {
		t.Fatalf("goal alert report = %q, %v", output.String(), err)
	}
	output.Reset()
	if err := renderGoalActivity(state, current.ID, &output); err != nil || !strings.Contains(output.String(), "Shown job alerts: 1 (1 useful") {
		t.Fatalf("goal activity rating = %q, %v", output.String(), err)
	}
}
