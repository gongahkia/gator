package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/jobs"
	"github.com/gongahkia/gator/internal/workrun"
)

func TestGoalCommandsCreateAndManageOngoingGoal(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", stateDir)
	var output bytes.Buffer
	if err := run([]string{"goal", "add", "--context", "Decide by Friday", "Plan the offsite"}, &output); err != nil {
		t.Fatal(err)
	}
	id := strings.Fields(output.String())[0]
	store, err := goal.Open(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load(id)
	if err != nil || record.SourcePath == "" || record.Context != "Decide by Friday" {
		t.Fatalf("created goal = %#v, %v", record, err)
	}
	output.Reset()
	if err := run([]string{"goal", "pause", id}, &output); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := run([]string{"goal", "list"}, &output); err != nil || !strings.Contains(output.String(), "paused") {
		t.Fatalf("goal list = %q, %v", output.String(), err)
	}
}

func TestScheduledGoalUsesFreshContextAndStopsWhenPaused(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", stateDir)
	var output bytes.Buffer
	if err := run([]string{"goal", "add", "--context", "Decision due Friday", "Prepare launch"}, &output); err != nil {
		t.Fatal(err)
	}
	goalID := strings.Fields(output.String())[0]
	output.Reset()
	if err := run([]string{"goal", "schedule", goalID, "--cron", "0 9 * * *", "--timezone", "Asia/Singapore", "--task", "Check new source facts and summarize changes"}, &output); err != nil {
		t.Fatal(err)
	}
	jobStore, err := jobs.Open(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	definitions, err := jobStore.List()
	if err != nil || len(definitions) != 1 {
		t.Fatalf("jobs = %#v, %v", definitions, err)
	}
	if definitions[0].GoalID != goalID || !definitions[0].RefreshSnapshot {
		t.Fatalf("linked job = %#v", definitions[0])
	}
	if err := goalCommand([]string{"context", goalID, "Decision moved to Monday"}, &output); err != nil {
		t.Fatal(err)
	}
	request, err := preparedJobWorkRequest(definitions[0], stateDir)
	if err != nil || request.GoalID != goalID || !strings.Contains(request.GoalContext, "Monday") || !request.RefreshSource {
		t.Fatalf("prepared request = %#v, %v", request, err)
	}
	attempt, err := jobStore.Begin(definitions[0], time.Now())
	if err != nil {
		t.Fatal(err)
	}
	attempt.Status = "completed"
	attempt.Conversation = "work-prior-scheduled-run"
	attempt.FinishedAt = time.Now().UTC()
	if err := jobStore.Record(attempt); err != nil {
		t.Fatal(err)
	}
	continued, err := preparedJobWorkRequest(definitions[0], stateDir)
	if err != nil || continued.ConversationID != attempt.Conversation {
		t.Fatalf("recurring conversation = %q, %v", continued.ConversationID, err)
	}
	if err := goalCommand([]string{"pause", goalID}, &output); err != nil {
		t.Fatal(err)
	}
	if _, err := preparedJobWorkRequest(definitions[0], stateDir); err == nil {
		t.Fatal("paused goal was allowed to run")
	}
}

func TestGoalScheduleCanSelectConfiguredReadConnector(t *testing.T) {
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	stateDir := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", stateDir)
	if err := connectorCommandWithIO([]string{"add", "planning", "--url", "https://example.com/planning.json"}, strings.NewReader(""), &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	goals, err := goal.Open(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	record, err := goals.Add("Review planning updates", "", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	arguments := []string{"schedule", record.ID, "--cron", "0 9 * * *", "--connector", "planning", "--task", "Check new planning facts"}
	if err := goalCommand(arguments, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	store, err := jobs.Open(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	definitions, err := store.List()
	if err != nil || len(definitions) != 1 || len(definitions[0].ConnectorIDs) != 1 || definitions[0].ConnectorIDs[0] != "planning" || definitions[0].ChangeOnly {
		t.Fatalf("connected goal job = %#v, %v", definitions, err)
	}
	request, err := preparedJobWorkRequest(definitions[0], stateDir)
	if err != nil || len(request.ConnectorIDs) != 1 || request.ConnectorIDs[0] != "planning" {
		t.Fatalf("connected goal request = %#v, %v", request, err)
	}
	if err := goalCommand([]string{"schedule", record.ID, "--cron", "0 9 * * *", "--connector", "missing", "--task", "Check"}, &bytes.Buffer{}); err == nil {
		t.Fatal("unknown connector was scheduled")
	}
	if err := goalCommand([]string{"schedule", record.ID, "--cron", "0 9 * * *", "--connector", "planning", "--change-only", "--task", "Check"}, &bytes.Buffer{}); err == nil {
		t.Fatal("local-only change gate was used with a connector")
	}
}

func TestGoalPlanCommandsAppearInWorkContext(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", stateDir)
	var output bytes.Buffer
	if err := run([]string{"goal", "add", "Plan offsite"}, &output); err != nil {
		t.Fatal(err)
	}
	id := strings.Fields(output.String())[0]
	output.Reset()
	if err := run([]string{"goal", "plan", id, "outcome", "Book a venue by Friday"}, &output); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := run([]string{"goal", "plan", id, "add", "--title", "Confirm headcount", "--due", "2026-10-20T09:00:00+08:00", "--evidence", "user: offsite request"}, &output); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := run([]string{"goal", "plan", id}, &output); err != nil || !strings.Contains(output.String(), "Confirm headcount") || !strings.Contains(output.String(), "Book a venue") {
		t.Fatalf("plan output = %q, %v", output.String(), err)
	}
	request := &workrun.Request{GoalID: id}
	if err := applyGoalPlanContext(stateDir, request); err != nil || !strings.Contains(request.GoalPlanContext, "Confirm headcount") || !strings.Contains(request.GoalPlanContext, "user: offsite request") {
		t.Fatalf("plan Work context = %q, %v", request.GoalPlanContext, err)
	}
}
