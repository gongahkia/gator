package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/followup"
	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/inbox"
	"github.com/gongahkia/gator/internal/jobs"
	"github.com/gongahkia/gator/internal/workhistory"
	"github.com/gongahkia/gator/internal/workhome"
	"github.com/gongahkia/gator/internal/workrun"
	"github.com/gongahkia/gator/internal/worksession"
)

func TestReviewedFollowupPersistsScopeLimitsAndSurvivesSchedulerUpdates(t *testing.T) {
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	state, source := t.TempDir(), t.TempDir()
	sessions, _ := worksession.Open(state)
	chat, err := sessions.Create("Plan the offsite", source, "snap-origin", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	draft, err := followup.New(source, "")
	if err != nil {
		t.Fatal(err)
	}
	draft.OriginConversationID = chat.ID
	record, definition, err := saveFollowup(state, draft)
	if err != nil {
		t.Fatal(err)
	}
	if definition.OriginConversationID != chat.ID {
		t.Fatal("follow-up lost requesting conversation")
	}
	update, err := recordJobInbox(state, definition, jobs.Attempt{ID: "attempt-origin", Conversation: "chat-job", Status: "failed", Error: "No connection"})
	if err != nil || update.OriginConversationID != chat.ID || update.ConversationID != "chat-job" || update.AttemptID != "attempt-origin" {
		t.Fatalf("origin/execution mixup: %+v, %v", update, err)
	}
	mailbox, _ := inbox.Open(state)
	if err := mailbox.MarkRead(update.ID, time.Now()); err != nil {
		t.Fatal(err)
	}
	again, err := recordJobInbox(state, definition, jobs.Attempt{ID: "attempt-origin", Conversation: "chat-job", Status: "failed", Error: "No connection"})
	if err != nil || again.ID != update.ID || again.ReadAt.IsZero() {
		t.Fatalf("retry duplicated notification: %+v, %v", again, err)
	}
	if definition.Mode != action.Draft || definition.Contract.ExternalActions != action.Forbid || definition.PollMinutes != 15 || definition.MaxChecksPerDay != 24 || definition.AlertWhen != draft.AlertWhen {
		t.Fatalf("reviewed agreement changed: %#v", definition)
	}
	store, _ := jobs.Open(state)
	draft.GoalID, draft.JobID = record.ID, definition.ID
	draft.ExpectedDigest, _ = definition.PolicyDigest()
	if _, _, err := store.Claim(definition.ID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	draft.AlertWhen = "Only changed deadlines"
	_, edited, err := saveFollowup(state, draft)
	if err != nil {
		t.Fatal(err)
	}
	if edited.LastScheduledAt.IsZero() {
		t.Fatal("editing lost scheduler bookkeeping")
	}
	if _, _, err := saveFollowup(state, draft); err == nil {
		t.Fatal("stale review overwrote a newer agreement")
	}
	draft.ExpectedDigest, _ = edited.PolicyDigest()
	draft.OriginConversationID = ""
	if _, _, err := saveFollowup(state, draft); err == nil {
		t.Fatal("editing detached update routing")
	}
	checks, err := workhome.ReadChecks(state, false, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(checks) != 1 || checks[0].State != "offline" || !checks[0].LastSuccess.IsZero() {
		t.Fatalf("unrun check health: %#v", checks)
	}
}

func TestReviewedProjectFollowupProducesSourceSupportedDraftForReview(t *testing.T) {
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	state, source := t.TempDir(), t.TempDir()
	data, err := os.ReadFile("../../examples/project-followup/changes/deadline.md")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "project.md"), data, 0600); err != nil {
		t.Fatal(err)
	}
	draft, _ := followup.New(source, "")
	record, definition, err := saveFollowup(state, draft)
	if err != nil {
		t.Fatal(err)
	}
	model := &workScriptedModel{turns: []agent.Turn{
		{ToolCalls: []agent.ToolCall{{ID: "read", Name: "read_file", Arguments: json.RawMessage(`{"path":"source/project.md"}`)}}},
		{ToolCalls: []agent.ToolCall{{ID: "draft", Name: "write_artifact", Arguments: json.RawMessage(`{"path":"reply.md","content":"Subject: Arbor planning deadline\nMina, I acknowledge the earlier 3 November deadline. Please confirm accessibility and capacity before the reply.\n"}`)}}},
		{ToolCalls: []agent.ToolCall{{ID: "status", Name: "artifact_status", Arguments: json.RawMessage(`{}`)}}},
		{Text: `{"attention":true,"urgent":false,"summary":"The reply deadline moved earlier; review the reply draft.","next_step":"Confirm venue accessibility and capacity with Mina.","rationale":"The selected planning note changed the deadline.","evidence":["source/project.md:5"],"change_ids":[]}`},
	}}
	service := workrun.Service{Executor: workrun.Executor{Model: model, StateDir: state}}
	var observed workrun.Outcome
	result, err := runJobProcessWith(context.Background(), definition, state, func(ctx context.Context, _, _, _ string, request workrun.Request) (workrun.Outcome, error) {
		observed, err = service.Execute(ctx, request)
		return observed, err
	}, "work-project-followup")
	if err != nil {
		t.Fatal(err)
	}
	if result.WatchDecision == nil || result.WatchDecision.Status != "attention" || result.WatchDecision.NextStep == "" || len(result.WatchDecision.Evidence) == 0 {
		t.Fatalf("missing useful retained decision: %#v", result)
	}
	if len(observed.Manifest.Actions) != 0 {
		t.Fatal("unattended follow-up performed an external action")
	}
	if _, err := os.Stat(filepath.Join(observed.Work.Path, "output", "reply.md")); err != nil {
		t.Fatal("draft was not retained for review", err)
	}
	history, _ := workhistory.Open(state)
	retained, err := history.Load("work-project-followup")
	if err != nil {
		t.Fatal(err)
	}
	if retained.Decision.GoalID != record.ID || retained.Decision.Origin != "job" {
		t.Fatal("reviewed goal route not retained")
	}
	quietModel := &workScriptedModel{turns: []agent.Turn{{ToolCalls: []agent.ToolCall{{ID: "read-quiet", Name: "read_file", Arguments: json.RawMessage(`{"path":"source/project.md"}`)}}}, {Text: `{"attention":false,"urgent":false,"summary":"No further material change.","rationale":"The selected deadline and owner are unchanged."}`}}}
	quietService := workrun.Service{Executor: workrun.Executor{Model: quietModel, StateDir: state}}
	quiet, err := runJobProcessWith(context.Background(), definition, state, func(ctx context.Context, _, _, _ string, request workrun.Request) (workrun.Outcome, error) {
		return quietService.Execute(ctx, request)
	}, "work-project-quiet")
	if err != nil || quiet.WatchAttention || quiet.WatchDecision == nil || quiet.WatchDecision.Status != "quiet" {
		t.Fatalf("optional drafts forced a noisy check: %#v %v", quiet, err)
	}
	if _, err := quietService.Execute(context.Background(), workrun.Request{SourcePath: record.SourcePath, Objective: "Create a required draft", Mode: action.Draft, Contract: artifact.InspectionContract()}); err == nil {
		t.Fatal("ordinary draft silently lost its artifact requirement")
	}
}

func TestDailyFollowupCapSkipsModelWithoutCreatingAnAlert(t *testing.T) {
	state, source := t.TempDir(), t.TempDir()
	goals, _ := goal.Open(state)
	record, err := goals.Add("Project", "", source)
	if err != nil {
		t.Fatal(err)
	}
	draft, _ := followup.New(source, record.ID)
	draft.MaxChecksPerDay = 1
	definition, _ := draft.Definition(record.ID, nil)
	store, _ := jobs.Open(state)
	definition, err = store.Save(definition)
	if err != nil {
		t.Fatal(err)
	}
	prior, err := store.Begin(definition, time.Now().Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	prior.Status, prior.FinishedAt, prior.Runs = "completed", time.Now().UTC(), []jobs.RunReference{{Try: 1, RunID: "work-prior"}}
	if err := store.Record(prior); err != nil {
		t.Fatal(err)
	}
	attempt, err := store.Begin(definition, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	result := executeJobContextWith(context.Background(), definition, attempt, state, false, func(context.Context, jobs.Definition, string, ...string) (jobResult, error) {
		calls++
		return jobResult{Status: string(artifact.Completed)}, nil
	})
	if calls != 0 || result.Status != "limited" {
		t.Fatalf("daily cap called model: %d %s", calls, result.Status)
	}
	entry, err := recordJobInbox(state, definition, result)
	if err != nil {
		t.Fatal(err)
	}
	if entry.ID != "" {
		t.Fatal("budget limit became an urgent model alert")
	}
}
