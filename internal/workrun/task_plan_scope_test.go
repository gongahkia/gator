package workrun

import (
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
	"github.com/gongahkia/gator/internal/workhistory"
	"github.com/gongahkia/gator/internal/worksession"
)

func claimedPlanFixture(t *testing.T, effects []string) (string, worksession.Store, Request) {
	t.Helper()
	state, source := t.TempDir(), t.TempDir()
	store, err := worksession.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	c, err := store.CreateChat(source, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	raw := json.RawMessage(`{"PlanFirst":true,"MaxSteps":24,"Code":{"MaxSteps":32},"ConnectorIDs":[],"WebOrigins":[]}`)
	j, r, err := store.AdmitPlanRequest(c.ID, "Create brief.md", raw, "plan-scope-client", nil, worksession.PlanBudget{MaxSteps: 24, CodeMaxSteps: 32}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	content := worksession.PlanContent{Outcome: "A short brief", Steps: []worksession.PlanStep{{ID: "step-1", Text: "Inspect and summarize"}}, Sources: []string{}, Effects: effects}
	j, err = store.ReviseTaskPlan(c.ID, r.ID, 0, "", content)
	if err != nil {
		t.Fatal(err)
	}
	r = j.Requests[0]
	j, _, err = store.ApproveTaskPlan(c.ID, r.ID, r.Plan.Revision, r.Plan.Digest)
	if err != nil {
		t.Fatal(err)
	}
	j.Requests[0].State = "running"
	j.Requests[0].Plan.Phase = "dispatching"
	if _, err = store.SaveRequests(c.ID, j); err != nil {
		t.Fatal(err)
	}
	return state, store, Request{StateDir: state, SourcePath: source, ConversationID: c.ID, Objective: r.Text, Mode: action.Draft, Contract: artifact.DefaultContract("brief.md"), MaxSteps: 24, TaskPlan: &TaskPlanReference{RequestID: r.ID, Revision: r.Plan.Revision, Digest: r.Plan.Digest}}
}
func TestTaskPlanScopeStopsBeforeOutOfScopeWriteAndLaterCalls(t *testing.T) {
	state, _, request := claimedPlanFixture(t, []string{"read"})
	if err := os.WriteFile(filepath.Join(request.SourcePath, "note.md"), []byte("Private note"), 0600); err != nil {
		t.Fatal(err)
	}
	model := &scriptedModel{turns: []agent.Turn{{ToolCalls: []agent.ToolCall{{ID: "outside", Name: "write_artifact", Arguments: json.RawMessage(`{"path":"brief.md","content":"Must never be written"}`)}, {ID: "later", Name: "read_file", Arguments: json.RawMessage(`{"path":"source/note.md"}`)}}}, {Text: "This turn must never run"}}}
	outcome, err := (Service{Executor: Executor{Model: model, StateDir: state}}).Execute(context.Background(), request)
	if err == nil || !strings.Contains(err.Error(), "reviewed plan does not include draft") {
		t.Fatal("scope change was not explicit", err)
	}
	if len(model.requests) != 1 {
		t.Fatal("scope denial continued model execution")
	}
	for _, tool := range model.requests[0].Tools {
		if tool.Name == "write_artifact" {
			t.Fatal("undeclared write advertised to model")
		}
	}
	if outcome.Work.Path == "" {
		t.Fatal("partial outcome not retained")
	}
	if _, err := os.Stat(filepath.Join(outcome.Work.Path, "output", "brief.md")); !os.IsNotExist(err) {
		t.Fatal("out-of-scope write executed", err)
	}
	if len(outcome.Manifest.Evidence) != 0 {
		t.Fatal("later allowed call executed after scope denial")
	}
}
func TestTaskPlanScopeAllowsReviewedDraftWithoutBypassingArtifactVerification(t *testing.T) {
	state, _, request := claimedPlanFixture(t, []string{"read", "draft"})
	model := &scriptedModel{turns: []agent.Turn{{ToolCalls: []agent.ToolCall{{ID: "write", Name: "write_artifact", Arguments: json.RawMessage(`{"path":"brief.md","content":"A bounded prepared brief.\n"}`)}}}, {Text: "Prepared brief.md for review."}}}
	outcome, err := (Service{Executor: Executor{Model: model, StateDir: state}}).Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Manifest.Status != artifact.Completed {
		t.Fatal("reviewed draft not checked")
	}
	if !strings.Contains(model.requests[0].System, "Human-reviewed task intent") || strings.Contains(model.requests[0].System, "steps are verified") {
		t.Fatal("reviewed intent missing")
	}
}
func TestTaskPlanScopeRejectsWrongTaskGenerationAndDispatchBinding(t *testing.T) {
	state, _, request := claimedPlanFixture(t, []string{"read", "draft"})
	for _, change := range []func(*Request){func(r *Request) { r.TaskPlan.Digest = strings.Repeat("0", 64) }, func(r *Request) { r.TaskPlan.Revision++ }, func(r *Request) { r.Objective = "Different task" }, func(r *Request) { r.ConnectorIDs = []string{"other"} }, func(r *Request) { r.MaxSteps = 25 }, func(r *Request) { r.SourcePath = t.TempDir() }} {
		copy := request
		ref := *request.TaskPlan
		copy.TaskPlan = &ref
		change(&copy)
		model := &scriptedModel{turns: []agent.Turn{{Text: "Must not run"}}}
		if _, err := (Service{Executor: Executor{Model: model, StateDir: state}}).Execute(context.Background(), copy); err == nil || len(model.requests) != 0 {
			t.Fatal("mismatched plan reached model")
		}
	}
}

func TestTaskPlanControlRejectsSteeringButPreservesStop(t *testing.T) {
	state := t.TempDir()
	history, err := workhistory.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := history.AcquireRun("plan-worker")
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	if _, err := history.Start(workhistory.Start{ID: "plan-worker", Objective: "Planned fixture", Mode: action.Inspect, ExternalActions: artifact.InspectionContract().ExternalActions, StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	inspection := newLiveInspection("plan-worker", "work-chat", time.Now(), nil)
	inspection.planBound = true
	control, steering, err := startWorkControl(ctx, state, "plan-worker", cancel, nil, nil, nil, nil, nil, nil, inspection)
	if err != nil {
		t.Fatal(err)
	}
	defer control.Close()
	if err := SendWorkControl(ctx, state, "plan-worker", "steer", "Change the task"); err == nil {
		t.Fatal("legacy steering changed approved scope")
	}
	select {
	case <-steering:
		t.Fatal("denied guidance reached runner")
	default:
	}
	operation := &Operation{planBound: true}
	if operation.Steer("New task") == nil || operation.SteerTask("subagent-001", "New task") == nil {
		t.Fatal("local operation bypassed plan scope")
	}
	if err := SendWorkControl(context.Background(), state, "plan-worker", "stop", ""); err != nil {
		t.Fatal("planned work could not stop", err)
	}
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("stop was only cosmetic")
	}
}
