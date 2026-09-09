package workrun

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/connector"
	"github.com/gongahkia/gator/internal/sandbox"
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

func TestExecutorDelegatesCodeAgainstFrozenSourceAndSealsPatchEvidence(t *testing.T) {
	source := t.TempDir()
	if err := os.Mkdir(filepath.Join(source, ".gator"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, ".gator", "agents.json"), []byte(`{"version":1,"profiles":[{"name":"implementer","instructions":"Implement bounded tasks"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "main.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	model := &scriptedModel{turns: []agent.Turn{
		{ToolCalls: []agent.ToolCall{{ID: "delegate-1", Name: "delegate_agents", Arguments: json.RawMessage(`{"tasks":[{"agent":"code","task":"Add a greeting"}]}`)}}},
		{ToolCalls: []agent.ToolCall{{ID: "write-1", Name: "write_artifact", Arguments: json.RawMessage(`{"path":"report.md","content":"Code patch prepared for review."}`)}}},
		{Text: "Prepared the report and a reviewable code patch."},
	}}
	var delegated CodeRequest
	patchContents := []byte("diff --git a/main.go b/main.go\n")
	outcome, err := (Executor{
		Model: model, StateDir: t.TempDir(),
		Code: func(_ context.Context, request CodeRequest) (CodeResult, error) {
			delegated = request
			contents, readErr := os.ReadFile(filepath.Join(request.SourcePath, "main.go"))
			if readErr != nil || string(contents) != "package main\n" {
				return CodeResult{}, errors.New("delegated source did not match frozen input")
			}
			return CodeResult{Summary: "Added greeting", Patch: patchContents, ChangedPaths: []string{"main.go"}, Steps: 4}, nil
		},
	}).Execute(context.Background(), Request{
		SourcePath: source, Objective: "Prepare a report and implement a greeting", RunID: "work-code-specialist",
		Contract: artifact.DefaultContract("report.md"), MaxSteps: 5, RequireCode: true,
		Code: CodePolicy{MaxSteps: 9, Verification: [][]string{{"go", "test", "./..."}}, Scopes: []string{"internal/greeting"}, Profile: "implementer", Sandbox: sandbox.DefaultPolicy(), Capabilities: []string{CodeCapabilityLSP}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if delegated.ID != "subagent-001" || delegated.SourcePath == source || delegated.ParentRunID != "work-code-specialist" {
		t.Fatalf("delegated request = %#v", delegated)
	}
	if delegated.Policy.MaxSteps != 9 || delegated.Policy.Profile != "implementer" || len(delegated.Policy.Verification) != 1 || !delegated.Policy.HasCapability(CodeCapabilityLSP) {
		t.Fatalf("delegated Code policy = %#v", delegated.Policy)
	}
	if len(outcome.Manifest.Subagents) != 1 {
		t.Fatalf("subagent evidence = %#v", outcome.Manifest.Subagents)
	}
	evidence := outcome.Manifest.Subagents[0]
	if evidence.Agent != "code" || evidence.Status != "completed" || evidence.ArtifactPath != "code/work-code-specialist-subagent-001.patch" || evidence.ArtifactSHA256 == "" {
		t.Fatalf("subagent evidence = %#v", evidence)
	}
	retained, err := os.ReadFile(filepath.Join(outcome.Work.Output.Path(), filepath.FromSlash(evidence.ArtifactPath)))
	if err != nil || string(retained) != string(patchContents) {
		t.Fatalf("retained patch = %q, %v", retained, err)
	}
	original, err := os.ReadFile(filepath.Join(source, "main.go"))
	if err != nil || string(original) != "package main\n" {
		t.Fatalf("live source changed: %q, %v", original, err)
	}
}

func TestExecutorPassesPromptAttachmentsOnlyToTheUserFacingManager(t *testing.T) {
	model := &scriptedModel{turns: []agent.Turn{{Text: "Inspected the supplied image."}}}
	_, err := (Executor{Model: model, StateDir: t.TempDir()}).Execute(context.Background(), Request{
		SourcePath: t.TempDir(), Objective: "Inspect the screenshot", RunID: "work-attachment", Mode: action.Inspect,
		Contract: artifact.InspectionContract(), MaxSteps: 1,
		Images: []agent.Image{{Name: "screen.png", MediaType: "image/png", Data: []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(model.requests) != 1 || len(model.requests[0].Messages) != 1 || len(model.requests[0].Messages[0].Images) != 1 {
		t.Fatalf("manager request = %#v", model.requests)
	}
}

func TestExecutorContinuationSeedsArtifactsAndRetainsParent(t *testing.T) {
	source, state := t.TempDir(), t.TempDir()
	firstModel := &scriptedModel{turns: []agent.Turn{{ToolCalls: []agent.ToolCall{{ID: "write", Name: "write_artifact", Arguments: json.RawMessage(`{"path":"report.md","content":"one"}`)}}}, {Text: "first"}}}
	first, err := (Executor{Model: firstModel, StateDir: state}).Execute(context.Background(), Request{SourcePath: source, Objective: "First report", RunID: "revision-one", Contract: artifact.DefaultContract("report.md"), MaxSteps: 3})
	if err != nil {
		t.Fatal(err)
	}
	secondModel := &scriptedModel{turns: []agent.Turn{{ToolCalls: []agent.ToolCall{{ID: "read", Name: "read_file", Arguments: json.RawMessage(`{"path":"previous/report.md"}`)}}}, {ToolCalls: []agent.ToolCall{{ID: "write", Name: "write_artifact", Arguments: json.RawMessage(`{"path":"report.md","content":"two"}`)}}}, {Text: "second"}}}
	second, err := (Executor{Model: secondModel, StateDir: state}).Execute(context.Background(), Request{SourcePath: source, Objective: "Revise report", RunID: "revision-two", ConversationID: first.ConversationID, Contract: artifact.DefaultContract("report.md"), MaxSteps: 4})
	if err != nil {
		t.Fatal(err)
	}
	parent, _ := os.ReadFile(filepath.Join(first.Work.Output.Path(), "report.md"))
	child, _ := os.ReadFile(filepath.Join(second.Work.Output.Path(), "report.md"))
	if string(parent) != "one" || string(child) != "two" || first.SnapshotID != second.SnapshotID {
		t.Fatalf("parent=%q child=%q snapshots=%q/%q", parent, child, first.SnapshotID, second.SnapshotID)
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

func TestExecutorRejectsUnownedCodeCapabilityEscalation(t *testing.T) {
	base := Request{SourcePath: t.TempDir(), Objective: "Prepare work", Contract: artifact.DefaultContract("report.md"), MaxSteps: 1}
	for name, policy := range map[string]CodePolicy{
		"unknown capability":    {Capabilities: []string{"host-admin"}},
		"browser without grant": {BrowserSession: "browser-one"},
	} {
		t.Run(name, func(t *testing.T) {
			request := base
			request.Code = policy
			_, err := (Executor{Model: &scriptedModel{}, StateDir: t.TempDir()}).Execute(context.Background(), request)
			if err == nil {
				t.Fatalf("policy was accepted: %#v", policy)
			}
		})
	}
}

func TestExecutorSealsExplicitConnectorProvenance(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"revenue_growth":12}`))
	}))
	defer server.Close()
	descriptor := connector.Descriptor{
		Version: connector.DescriptorVersion, ID: "metrics", Name: "Metrics",
		Kind: connector.KindHTTPJSON, Resource: server.URL, Authentication: connector.AuthNone,
	}
	registry, err := connector.NewRegistry([]connector.Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	model := &scriptedModel{turns: []agent.Turn{
		{ToolCalls: []agent.ToolCall{{ID: "source-1", Name: "connector_metrics_fetch", Arguments: json.RawMessage(`{}`)}}},
		{ToolCalls: []agent.ToolCall{{ID: "write-1", Name: "write_artifact", Arguments: json.RawMessage(`{"path":"report.md","content":"Revenue grew 12%."}`)}}},
		{Text: "Created report.md from the connected source."},
	}}
	outcome, err := (Executor{
		Model: model, StateDir: t.TempDir(), Connectors: connector.Runtime{Registry: registry, HTTPClient: server.Client()},
	}).Execute(context.Background(), Request{
		SourcePath: t.TempDir(), Objective: "Prepare a metrics report", RunID: "work-connector",
		Contract: artifact.DefaultContract("report.md"), ConnectorIDs: []string{"metrics"}, MaxSteps: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(outcome.Manifest.ConnectedSources) != 1 || outcome.Manifest.ConnectedSources[0].ConnectorID != "metrics" {
		t.Fatalf("connected provenance = %#v", outcome.Manifest.ConnectedSources)
	}
	if outcome.Manifest.ConnectedSources[0].SnapshotPath == "" {
		t.Fatal("connected source bytes were not retained in the Work bundle")
	}
	if _, err := os.Stat(filepath.Join(outcome.Work.Path, filepath.FromSlash(outcome.Manifest.ConnectedSources[0].SnapshotPath))); err != nil {
		t.Fatalf("connected snapshot missing: %v", err)
	}
	if !strings.Contains(model.requests[0].System, "Explicitly selected connected sources: metrics") {
		t.Fatalf("system prompt = %q", model.requests[0].System)
	}
}

func TestExecutorSealsPendingExternalActionWithoutExecuting(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) }))
	defer server.Close()
	descriptor := connector.Descriptor{
		Version: connector.DescriptorVersion, ID: "release", Name: "Release",
		Kind: connector.KindHTTPWebhook, Resource: server.URL, Authentication: connector.AuthNone,
	}
	registry, err := connector.NewRegistry([]connector.Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	model := &scriptedModel{turns: []agent.Turn{
		{ToolCalls: []agent.ToolCall{{ID: "action-1", Name: "connector_release_publish", Arguments: json.RawMessage(`{"payload":{"version":"v1"}}`)}}},
		{ToolCalls: []agent.ToolCall{{ID: "write-1", Name: "write_artifact", Arguments: json.RawMessage(`{"path":"report.md","content":"Release proposal ready."}`)}}},
		{Text: "Created report.md and one pending release proposal."},
	}}
	contract := artifact.DefaultContract("report.md")
	contract.ExternalActions = action.Propose
	outcome, err := (Executor{
		Model: model, StateDir: t.TempDir(), Connectors: connector.Runtime{Registry: registry, HTTPClient: server.Client()},
	}).Execute(context.Background(), Request{
		SourcePath: t.TempDir(), Objective: "Draft a release publication", RunID: "work-action-draft",
		Mode: action.Draft, Contract: contract, ConnectorIDs: []string{descriptor.ID}, MaxSteps: 4,
		ApproveAction: func(context.Context, action.Proposal) (action.Decision, error) {
			t.Fatal("draft work requested action approval")
			return action.Allow, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 0 || len(outcome.Manifest.Actions) != 1 || outcome.Manifest.Actions[0].Status != action.Pending {
		t.Fatalf("requests=%d actions=%#v", requests.Load(), outcome.Manifest.Actions)
	}
}

func TestExecutorExecutesExternalActionAfterApproval(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	descriptor := connector.Descriptor{
		Version: connector.DescriptorVersion, ID: "release", Name: "Release",
		Kind: connector.KindHTTPWebhook, Resource: server.URL, Authentication: connector.AuthNone,
	}
	registry, err := connector.NewRegistry([]connector.Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	model := &scriptedModel{turns: []agent.Turn{
		{ToolCalls: []agent.ToolCall{{ID: "action-1", Name: "connector_release_publish", Arguments: json.RawMessage(`{"payload":{"version":"v1"}}`)}}},
		{ToolCalls: []agent.ToolCall{{ID: "write-1", Name: "write_artifact", Arguments: json.RawMessage(`{"path":"receipt.md","content":"Release action completed."}`)}}},
		{Text: "Created receipt.md after the approved action executed."},
	}}
	contract := artifact.DefaultContract("receipt.md")
	contract.ExternalActions = action.Approve
	approvals := 0
	outcome, err := (Executor{
		Model: model, StateDir: t.TempDir(), Connectors: connector.Runtime{Registry: registry, HTTPClient: server.Client()},
	}).Execute(context.Background(), Request{
		SourcePath: t.TempDir(), Objective: "Publish a release", RunID: "work-action-act",
		Mode: action.Act, Contract: contract, ConnectorIDs: []string{descriptor.ID}, MaxSteps: 4,
		ApproveAction: func(_ context.Context, proposal action.Proposal) (action.Decision, error) {
			approvals++
			if proposal.Target != server.URL {
				t.Fatalf("approval target = %q", proposal.Target)
			}
			return action.Allow, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if approvals != 1 || requests.Load() != 1 || len(outcome.Manifest.Actions) != 1 || outcome.Manifest.Actions[0].Status != action.Executed {
		t.Fatalf("approvals=%d requests=%d actions=%#v", approvals, requests.Load(), outcome.Manifest.Actions)
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
