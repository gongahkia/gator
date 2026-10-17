package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/inbox"
	"github.com/gongahkia/gator/internal/jobs"
	"github.com/gongahkia/gator/internal/suggestion"
	"github.com/gongahkia/gator/internal/workrun"
)

func TestGoalWatchChangedSourceIsQuietAcrossRestart(t *testing.T) {
	state, source := t.TempDir(), t.TempDir()
	t.Setenv("GATOR_STATE_DIR", state)
	if err := os.WriteFile(filepath.Join(source, "venue.txt"), []byte("Venue A available\n"), 0600); err != nil {
		t.Fatal(err)
	}
	goals, err := goal.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	record, err := goals.Add("Plan offsite", "Confirm a venue", source)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := goalCommand([]string{"watch", record.ID, "--task", "Watch venue changes"}, &output); err != nil {
		t.Fatal(err)
	}
	store, err := jobs.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	definitions, err := store.List()
	if err != nil || len(definitions) != 1 || !definitions[0].GoalWatch || !definitions[0].ChangeOnly || definitions[0].Mode != action.Inspect {
		t.Fatalf("watch definition = %#v, %v", definitions, err)
	}
	now := time.Now().UTC().Truncate(time.Minute).Add(time.Minute)
	calls := 0
	execute := func(_ context.Context, _ jobs.Definition, _ string, _ ...string) (jobResult, error) {
		calls++
		return jobResult{Status: "completed", Summary: "Venue A is available", ResultSHA256: "initial", WatchAttention: false}, nil
	}
	runDueJobsContextWith(context.Background(), store, state, false, now, execute)
	if calls != 1 {
		t.Fatalf("first watch called model %d times", calls)
	}
	mailbox, err := inbox.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := mailbox.List(false, 0)
	if err != nil || len(entries) != 0 {
		t.Fatalf("quiet watch inbox = %#v, %v", entries, err)
	}
	// A new store models a supervisor restart. No input change means no call.
	store, err = jobs.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	runDueJobsContextWith(context.Background(), store, state, false, now.Add(time.Minute), execute)
	if calls != 1 {
		t.Fatalf("unchanged restart called model %d times", calls)
	}
	if err := os.WriteFile(filepath.Join(source, "venue.txt"), []byte("Venue A cancelled; Venue B available\n"), 0600); err != nil {
		t.Fatal(err)
	}
	execute = func(_ context.Context, _ jobs.Definition, _ string, _ ...string) (jobResult, error) {
		calls++
		return jobResult{Status: "completed", Summary: "Venue A cancelled; review Venue B and a reply draft", ResultSHA256: "changed", WatchAttention: true}, nil
	}
	runDueJobsContextWith(context.Background(), store, state, false, now.Add(2*time.Minute), execute)
	entries, err = mailbox.List(false, 0)
	if err != nil || len(entries) != 1 || entries[0].GoalID != record.ID || calls != 2 {
		t.Fatalf("changed watch inbox = %#v; calls=%d; err=%v", entries, calls, err)
	}
	runDueJobsContextWith(context.Background(), store, state, false, now.Add(3*time.Minute), execute)
	entries, err = mailbox.List(false, 0)
	if err != nil || len(entries) != 1 || calls != 2 {
		t.Fatalf("duplicate after unchanged minute: entries=%#v calls=%d err=%v", entries, calls, err)
	}
	history, err := store.History(definitions[0].ID, 0)
	if err != nil || len(history) != 4 || history[0].Status != "unchanged" {
		t.Fatalf("watch attempts = %#v, %v", history, err)
	}
}

func TestGoalWatchHashCrossesDueTimeOnce(t *testing.T) {
	state, source := t.TempDir(), t.TempDir()
	goals, err := goal.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	record, err := goals.Add("Plan offsite", "", source)
	if err != nil {
		t.Fatal(err)
	}
	due := time.Now().UTC().Add(5 * time.Minute)
	plan, err := goals.ApplyPlanChange(record.ID, goal.PlanChange{Kind: "add_step", ExpectedRevision: 0, Title: "Confirm venue", DueAt: due})
	if err != nil {
		t.Fatal(err)
	}
	_, err = goals.ApplyPlanChange(record.ID, goal.PlanChange{Kind: "set_next_review", ExpectedRevision: plan.Revision, NextReviewAt: due})
	if err != nil {
		t.Fatal(err)
	}
	d := jobs.Definition{GoalID: record.ID, GoalWatch: true, ChangeOnly: true, RefreshSnapshot: true, SourcePath: source, Objective: "Watch"}
	before, err := localJobInputHashAt(d, state, due.Add(-time.Second))
	if err != nil {
		t.Fatal(err)
	}
	after, err := localJobInputHashAt(d, state, due.Add(time.Second))
	if err != nil || before == after {
		t.Fatalf("due crossing unchanged: %s %s %v", before, after, err)
	}
	later, err := localJobInputHashAt(d, state, due.Add(time.Hour))
	if err != nil || later != after {
		t.Fatalf("due state was not stable: %s %s %v", after, later, err)
	}
}

func TestGoalWatchAttentionCannotHideArtifactsOrSuggestions(t *testing.T) {
	quiet := `{"attention":false,"summary":"No new decision is needed.","evidence":[]}`
	if attention, _ := classifyGoalWatchResult(quiet, nil, nil); attention {
		t.Fatal("explicit quiet response was surfaced")
	}
	if attention, _ := classifyGoalWatchResult(quiet, nil, []artifact.File{{Path: "reply.md"}}); !attention {
		t.Fatal("draft artifact was hidden by quiet response")
	}
	if attention, _ := classifyGoalWatchResult(quiet, []agent.Message{{Role: agent.RoleTool, ToolName: "suggest_change", Content: `{"ok":true}`}}, nil); !attention {
		t.Fatal("reviewable suggestion was hidden by quiet response")
	}
	if attention, summary := classifyGoalWatchResult(quiet, []agent.Message{{Role: agent.RoleTool, ToolName: "suggest_change", Content: `{"ok":false,"error":"invalid plan"}`}}, nil); !attention || !strings.Contains(summary, "could not queue") {
		t.Fatalf("failed suggestion was hidden: attention=%t summary=%q", attention, summary)
	}
	if attention, _ := classifyGoalWatchResult("I found a changed date.", nil, nil); !attention {
		t.Fatal("unstructured response was hidden")
	}
}

func TestGoalWatchAlertShowsBoundedEvidenceOrReviewWarning(t *testing.T) {
	withEvidence := `{"attention":true,"summary":"Venue changed.","evidence":["source/venue.md:3","source/venue.md:3","goal plan step 2"]}`
	attention, summary := classifyGoalWatchResult(withEvidence, nil, nil)
	if !attention || !strings.Contains(summary, "[evidence: source/venue.md:3; goal plan step 2]") {
		t.Fatalf("evidence was not shown: %t %q", attention, summary)
	}
	withoutEvidence := `{"attention":true,"summary":"Venue changed.","evidence":[]}`
	attention, summary = classifyGoalWatchResult(withoutEvidence, nil, nil)
	if !attention || !strings.Contains(summary, "evidence missing; review Work run") {
		t.Fatalf("unsupported alert was not flagged: %t %q", attention, summary)
	}
	if value := boundedWatchEvidence([]string{"source/a.md:1\nspoofed", strings.Repeat("x", 120)}); strings.Contains(value, "\n") || len(value) > 210 {
		t.Fatalf("unsafe evidence: %q", value)
	}
}

func TestGoalWatchRetainsGateAndConversationAfterManyQuietMinutes(t *testing.T) {
	state, source := t.TempDir(), t.TempDir()
	goals, err := goal.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	record, err := goals.Add("Plan offsite", "", source)
	if err != nil {
		t.Fatal(err)
	}
	store, err := jobs.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	d, err := store.Save(jobs.Definition{Name: "Offsite", GoalID: record.ID, GoalWatch: true, Enabled: true, Schedule: "* * * * *", Missed: "run_once", SourcePath: source, RefreshSnapshot: true, ChangeOnly: true, Objective: "Watch goal", Mode: action.Inspect, Contract: artifact.InspectionContract()})
	if err != nil {
		t.Fatal(err)
	}
	hash, err := localJobInputHash(d, state)
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.Begin(d, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	first.Status, first.InputSHA256, first.NotificationPolicySHA256, first.Conversation, first.FinishedAt = "completed", hash, jobNotificationPolicyHash(d), "work-original", time.Now().UTC()
	if err := store.Record(first); err != nil {
		t.Fatal(err)
	}
	for range 51 {
		attempt, err := store.Begin(d, time.Now().UTC())
		if err != nil {
			t.Fatal(err)
		}
		attempt.Status, attempt.FinishedAt = "unchanged", time.Now().UTC()
		if err := store.Record(attempt); err != nil {
			t.Fatal(err)
		}
	}
	request, err := preparedJobWorkRequest(d, state)
	if err != nil || request.ConversationID != "work-original" {
		t.Fatalf("watch conversation after quiet minutes = %q, %v", request.ConversationID, err)
	}
	current, err := store.Begin(d, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	current = executeJobContextWith(context.Background(), d, current, state, false, func(_ context.Context, _ jobs.Definition, _ string, _ ...string) (jobResult, error) {
		calls++
		return jobResult{Status: "completed"}, nil
	})
	if current.Status != "unchanged" || calls != 0 {
		t.Fatalf("watch gate after quiet minutes = %s; calls=%d", current.Status, calls)
	}
}

func TestGoalWatchOffsiteDraftAndPlanSuggestionStayForReview(t *testing.T) {
	state, source := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "venue.txt"), []byte("Venue A cancelled. Venue B has availability.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	goals, err := goal.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	record, err := goals.Add("Plan offsite", "Decide venue", source)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := goals.ApplyPlanChange(record.ID, goal.PlanChange{Kind: "add_step", ExpectedRevision: 0, Title: "Confirm Venue A"})
	if err != nil {
		t.Fatal(err)
	}
	args, _ := json.Marshal(map[string]any{"kind": "plan", "title": "Recheck venue", "plan_change": goal.PlanChange{Kind: "update_step", ExpectedRevision: plan.Revision, StepID: plan.Steps[0].ID, Status: goal.StepBlocked, Evidence: []string{"source/venue.txt:1"}}})
	model := &workScriptedModel{turns: []agent.Turn{
		{ToolCalls: []agent.ToolCall{{ID: "read", Name: "read_file", Arguments: json.RawMessage(`{"path":"source/venue.txt"}`)}}},
		{ToolCalls: []agent.ToolCall{{ID: "suggest", Name: "suggest_change", Arguments: args}}},
		{ToolCalls: []agent.ToolCall{{ID: "draft", Name: "write_artifact", Arguments: json.RawMessage(`{"path":"venue-reply.md","content":"Subject: Offsite venue update\n\nCould you confirm Venue B capacity and hold it for us?\n"}`)}}},
		{ToolCalls: []agent.ToolCall{{ID: "status", Name: "artifact_status", Arguments: json.RawMessage(`{}`)}}},
		{Text: `{"attention":true,"summary":"Venue A cancelled; review the blocked plan step and the Venue B reply draft.","evidence":["source/venue.txt:1"]}`},
	}}
	service := workrun.Service{Executor: workrun.Executor{Model: model, StateDir: state}}
	var observed workrun.Outcome
	store, err := jobs.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	definition, err := store.Save(jobs.Definition{Name: "Offsite", GoalID: record.ID, GoalWatch: true, Enabled: true, Schedule: "* * * * *", Timezone: "UTC", Missed: "run_once", SourcePath: source, Objective: "Review venue change and draft a reply", Mode: action.Draft, Contract: artifact.DefaultContract("venue-reply.md"), MaxSteps: 8, RefreshSnapshot: true, ChangeOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	result, err := runJobProcessWith(context.Background(), definition, state, func(ctx context.Context, _, _, _ string, request workrun.Request) (workrun.Outcome, error) {
		observed, err = service.Execute(ctx, request)
		return observed, err
	}, "offsite-watch")
	if err != nil || result.Status != "completed" || !result.WatchAttention || !strings.Contains(result.Summary, "Venue A cancelled") {
		t.Fatalf("watch result = %#v, %v", result, err)
	}
	if _, err := os.Stat(filepath.Join(result.ManifestPath, "..", "output", "venue-reply.md")); err != nil {
		t.Fatalf("draft artifact absent: %v", err)
	}
	suggestions, err := suggestion.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	pending, err := suggestions.List("pending")
	if err != nil || len(pending) != 1 || pending[0].GoalID != record.ID {
		t.Fatalf("plan proposal = %#v, %v; tool results=%v", pending, err, observed.Result.Messages)
	}
	unchangedPlan, err := goals.LoadPlan(record.ID)
	if err != nil || unchangedPlan.Steps[0].Status != goal.StepPending {
		t.Fatalf("unreviewed plan was changed: %#v, %v", unchangedPlan, err)
	}
}
