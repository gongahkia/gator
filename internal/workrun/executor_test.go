package workrun

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
)

func TestExecutorProducesSealedArtifactsFromNonGitSource(t *testing.T) {
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "notes.txt"), []byte("Revenue grew by 12 percent.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	model := &scriptedModel{turns: []agent.Turn{
		{ToolCalls: []agent.ToolCall{{ID: "read-1", Name: "read_file", Arguments: json.RawMessage(`{"path":"source/notes.txt"}`)}}},
		{ToolCalls: []agent.ToolCall{{ID: "write-1", Name: "write_artifact", Arguments: json.RawMessage(`{"path":"report.md","content":"# Brief\\n\\nRevenue grew by 12 percent.\\n"}`)}}},
		{ToolCalls: []agent.ToolCall{{ID: "status-1", Name: "artifact_status", Arguments: json.RawMessage(`{}`)}}},
		{Text: "Created report.md; all validations passed."},
	}}
	now := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	executor := Executor{Model: model, Now: func() time.Time { return now }, StateDir: t.TempDir()}
	outcome, err := executor.Execute(context.Background(), Request{
		SourcePath: source, Objective: "Prepare a concise revenue brief", RunID: "work-test",
		Contract: artifact.DefaultContract("report.md"), MaxSteps: 6,
	})
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Manifest.Status != artifact.Completed || len(outcome.Manifest.Artifacts) != 1 {
		t.Fatalf("manifest = %#v", outcome.Manifest)
	}
	if _, err := os.Stat(outcome.Work.ManifestPath); err != nil {
		t.Fatalf("manifest not written: %v", err)
	}
	sourceContents, err := os.ReadFile(filepath.Join(source, "notes.txt"))
	if err != nil || string(sourceContents) != "Revenue grew by 12 percent.\n" {
		t.Fatalf("source changed: %q, %v", sourceContents, err)
	}
	if len(model.requests) == 0 || !strings.Contains(model.requests[0].System, "source/... is") || !strings.Contains(model.requests[0].System, `"report.md"`) {
		t.Fatalf("system prompt = %q", model.requests[0].System)
	}
}

func TestExecutorBlocksPrematureCompletionUntilContractPasses(t *testing.T) {
	model := &scriptedModel{turns: []agent.Turn{
		{Text: "Done."},
		{ToolCalls: []agent.ToolCall{{ID: "write-1", Name: "write_artifact", Arguments: json.RawMessage(`{"path":"report.md","content":"ready"}`)}}},
		{Text: "Created and validated report.md."},
	}}
	var events []agent.Event
	outcome, err := (Executor{Model: model, StateDir: t.TempDir()}).Execute(context.Background(), Request{
		SourcePath: t.TempDir(), Objective: "Write a report", RunID: "work-evidence",
		Contract: artifact.DefaultContract("report.md"), MaxSteps: 4,
		OnEvent: func(event agent.Event) { events = append(events, event) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Result.Steps != 3 || !containsEvent(events, agent.EventCompletionBlocked) {
		t.Fatalf("steps=%d events=%#v", outcome.Result.Steps, events)
	}
}

func TestExecutorRetainsFailedManifestAtStepLimit(t *testing.T) {
	model := &scriptedModel{turns: []agent.Turn{{Text: "Done without an artifact."}}}
	outcome, err := (Executor{Model: model, StateDir: t.TempDir()}).Execute(context.Background(), Request{
		SourcePath: t.TempDir(), Objective: "Write a report", RunID: "work-incomplete",
		Contract: artifact.DefaultContract("report.md"), MaxSteps: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "step limit") {
		t.Fatalf("execution error = %v", err)
	}
	if outcome.Manifest.Status != artifact.Failed || outcome.Manifest.Failure == "" {
		t.Fatalf("failed manifest = %#v", outcome.Manifest)
	}
	if _, statErr := os.Stat(outcome.Work.ManifestPath); statErr != nil {
		t.Fatalf("failed manifest was not retained: %v", statErr)
	}
}

func TestExecutorInspectModeExposesNoWriteTools(t *testing.T) {
	model := &scriptedModel{turns: []agent.Turn{{Text: "The folder is empty."}}}
	outcome, err := (Executor{Model: model, StateDir: t.TempDir()}).Execute(context.Background(), Request{
		SourcePath: t.TempDir(), Objective: "Inspect this folder", RunID: "work-inspect",
		Mode: action.Inspect, Contract: artifact.InspectionContract(), MaxSteps: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Manifest.Status != artifact.Completed || len(model.requests) != 1 {
		t.Fatalf("outcome=%#v requests=%d", outcome, len(model.requests))
	}
	for _, definition := range model.requests[0].Tools {
		if definition.Name == "write_artifact" || definition.Name == "artifact_status" {
			t.Fatalf("inspect mode exposed %q", definition.Name)
		}
	}
}

func TestExecutorRejectsModeContractEscalationBeforeCreatingWorkspace(t *testing.T) {
	contract := artifact.DefaultContract("report.md")
	contract.ExternalActions = action.Approve
	_, err := (Executor{Model: &scriptedModel{}, StateDir: t.TempDir()}).Execute(context.Background(), Request{
		SourcePath: t.TempDir(), Objective: "Prepare and publish a report", Mode: action.Draft, Contract: contract,
	})
	if err == nil || !strings.Contains(err.Error(), "draft mode") {
		t.Fatalf("escalation error = %v", err)
	}
}

type scriptedModel struct {
	turns    []agent.Turn
	requests []agent.TurnRequest
}

func (m *scriptedModel) Complete(_ context.Context, request agent.TurnRequest) (agent.Turn, error) {
	m.requests = append(m.requests, request)
	if len(m.turns) == 0 {
		return agent.Turn{}, errors.New("unexpected model call")
	}
	turn := m.turns[0]
	m.turns = m.turns[1:]
	return turn, nil
}

func containsEvent(events []agent.Event, kind agent.EventKind) bool {
	for _, event := range events {
		if event.Kind == kind {
			return true
		}
	}
	return false
}
