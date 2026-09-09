package main

import (
	"bytes"
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

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
)

func TestWorkCommandCreatesDefaultValidatedReport(t *testing.T) {
	t.Setenv("GATOR_STATE_DIR", t.TempDir())
	t.Setenv("GATOR_PROVIDER", "openai")
	t.Setenv("GATOR_MODEL", "test-model")
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "notes.txt"), []byte("reference\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	model := &workScriptedModel{turns: []agent.Turn{
		{ToolCalls: []agent.ToolCall{{ID: "write-1", Name: "write_artifact", Arguments: json.RawMessage(`{"path":"report.md","content":"# Report\\n"}`)}}},
		{Text: "The report is ready."},
	}}
	var output bytes.Buffer
	err := runWorkTask([]string{"--source", source, "--max-steps", "3", "prepare", "a", "report"}, strings.NewReader(""), &output, func(_, _, _ string) (agent.Model, error) {
		return model, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Status: completed") || !strings.Contains(output.String(), "report.md (text/markdown") {
		t.Fatalf("work output = %q", output.String())
	}
}

func TestWorkCommandReadsObjectiveFromStdinAndEmitsJSON(t *testing.T) {
	t.Setenv("GATOR_STATE_DIR", t.TempDir())
	t.Setenv("GATOR_PROVIDER", "openai")
	t.Setenv("GATOR_MODEL", "test-model")
	model := &workScriptedModel{turns: []agent.Turn{{Text: "Inspected."}}}
	var output bytes.Buffer
	err := runWorkTask([]string{"--source", t.TempDir(), "--mode", "inspect", "--json"}, strings.NewReader("inspect these notes\n"), &output, func(_, _, _ string) (agent.Model, error) {
		return model, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("JSON output = %q: %v", output.String(), err)
	}
	if result["status"] != "completed" || result["final_text"] != "Inspected." {
		t.Fatalf("JSON result = %#v", result)
	}
}

func TestInspectCommandForcesInspectionMode(t *testing.T) {
	t.Setenv("GATOR_STATE_DIR", t.TempDir())
	t.Setenv("GATOR_PROVIDER", "openai")
	t.Setenv("GATOR_MODEL", "test-model")
	model := &workScriptedModel{turns: []agent.Turn{{Text: "Inspected."}}}
	var output bytes.Buffer
	if err := runInspectTask([]string{"--source", t.TempDir(), "--json", "inspect", "these", "notes"}, strings.NewReader(""), &output, func(_, _, _ string) (agent.Model, error) { return model, nil }); err != nil {
		t.Fatal(err)
	}
	if len(model.requests) != 1 || strings.Contains(output.String(), "report.md") {
		t.Fatalf("inspect request=%#v output=%q", model.requests, output.String())
	}
	for _, definition := range model.requests[0].Tools {
		if strings.HasPrefix(definition.Name, "write_") || definition.Name == "artifact_status" {
			t.Fatalf("inspect exposed %q", definition.Name)
		}
	}
}

func TestWorkContractInfersStructuredValidators(t *testing.T) {
	contract, err := workContract(action.Draft, action.Forbid, artifactFlags{"data/results.csv", "summary.json"}, containsFlags{"summary.json=answer"})
	if err != nil {
		t.Fatal(err)
	}
	if contract.Artifacts[0].MediaTypes[0] != "text/csv" || contract.Artifacts[1].MediaTypes[0] != "application/json" {
		t.Fatalf("contract = %#v", contract)
	}
	if got := contract.Artifacts[1].Validations[len(contract.Artifacts[1].Validations)-1]; got.Kind != artifact.Contains || got.Value != "answer" {
		t.Fatalf("contains validation = %#v", got)
	}
}

func TestWorkContractRejectsInspectArtifactsAndUnknownContainsTarget(t *testing.T) {
	if _, err := workContract(action.Inspect, action.Forbid, artifactFlags{"report.md"}, nil); err == nil {
		t.Fatal("inspect artifact was accepted")
	}
	if _, err := workContract(action.Draft, action.Forbid, artifactFlags{"report.md"}, containsFlags{"other.md=marker"}); err == nil {
		t.Fatal("unknown contains target was accepted")
	}
	if _, err := workContract(action.Draft, action.Approve, nil, nil); err == nil {
		t.Fatal("draft mode approved external actions")
	}
}

func TestWorkCommandPublishesOnlyAfterExactInteractiveApproval(t *testing.T) {
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	t.Setenv("GATOR_STATE_DIR", t.TempDir())
	t.Setenv("GATOR_PROVIDER", "openai")
	t.Setenv("GATOR_MODEL", "test-model")
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	if err := connectorCommandWithIO([]string{"add", "release", "--kind", "webhook", "--url", server.URL}, strings.NewReader(""), &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	model := &workScriptedModel{turns: []agent.Turn{
		{ToolCalls: []agent.ToolCall{{ID: "publish-1", Name: "connector_release_publish", Arguments: json.RawMessage(`{"payload":{"version":"v1"}}`)}}},
		{ToolCalls: []agent.ToolCall{{ID: "write-1", Name: "write_artifact", Arguments: json.RawMessage(`{"path":"receipt.md","content":"Published v1.\n"}`)}}},
		{Text: "Published v1 and created receipt.md."},
	}}
	var output bytes.Buffer
	err := runWorkTask([]string{
		"--source", t.TempDir(), "--mode", "act", "--actions", "approve", "--connector", "release",
		"--artifact", "receipt.md", "--max-steps", "4", "publish", "release", "v1",
	}, strings.NewReader("y\n"), &output, func(_, _, _ string) (agent.Model, error) { return model, nil })
	if err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 1 || !strings.Contains(output.String(), "External action approval") || !strings.Contains(output.String(), "exact JSON (escaped)") || !strings.Contains(output.String(), "release/publish: executed") {
		t.Fatalf("requests=%d output=%q", requests.Load(), output.String())
	}
}

func TestWorkCommandKeepsApprovalOutOfJSONMode(t *testing.T) {
	t.Setenv("GATOR_STATE_DIR", t.TempDir())
	t.Setenv("GATOR_PROVIDER", "openai")
	t.Setenv("GATOR_MODEL", "test-model")
	err := runWorkTask([]string{"--source", t.TempDir(), "--mode", "act", "--actions", "approve", "--json", "publish"}, strings.NewReader("y\n"), &bytes.Buffer{}, func(_, _, _ string) (agent.Model, error) {
		t.Fatal("model factory called for invalid interactive JSON run")
		return nil, nil
	})
	if err == nil || !strings.Contains(err.Error(), "--json") {
		t.Fatalf("JSON approval error = %v", err)
	}
}

func TestWorkActionApproverTreatsClosedInputAsDenial(t *testing.T) {
	var output bytes.Buffer
	proposal, err := action.NewProposal("action-1", action.Publish, "release", "publish", "https://example.com/hook", `{"version":"v1"}`, []byte(`{"version":"v1"}`))
	if err != nil {
		t.Fatal(err)
	}
	decision, err := workActionApprover(strings.NewReader(""), &output)(context.Background(), proposal)
	if err != nil || decision != action.Deny || !strings.Contains(output.String(), "Approve this one action?") {
		t.Fatalf("decision=%q err=%v output=%q", decision, err, output.String())
	}
}

func TestWorkCommandUsesOnlyExplicitlySelectedConnector(t *testing.T) {
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	t.Setenv("GATOR_STATE_DIR", t.TempDir())
	t.Setenv("GATOR_PROVIDER", "openai")
	t.Setenv("GATOR_MODEL", "test-model")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"metric":9}`))
	}))
	defer server.Close()
	if err := connectorCommandWithIO([]string{"add", "metrics", "--url", server.URL}, strings.NewReader(""), &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	model := &workScriptedModel{turns: []agent.Turn{
		{ToolCalls: []agent.ToolCall{{ID: "fetch-1", Name: "connector_metrics_fetch", Arguments: json.RawMessage(`{}`)}}},
		{ToolCalls: []agent.ToolCall{{ID: "write-1", Name: "write_artifact", Arguments: json.RawMessage(`{"path":"report.md","content":"Metric: 9\\n"}`)}}},
		{Text: "Created report.md."},
	}}
	var output bytes.Buffer
	err := runWorkTask([]string{"--source", t.TempDir(), "--connector", "metrics", "--max-steps", "4", "prepare", "metrics"}, strings.NewReader(""), &output, func(_, _, _ string) (agent.Model, error) {
		return model, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(model.requests) == 0 || !hasDefinition(model.requests[0].Tools, "connector_metrics_fetch") || !strings.Contains(output.String(), "connectors: metrics") {
		t.Fatalf("request=%#v output=%q", model.requests, output.String())
	}
}

type workScriptedModel struct {
	turns    []agent.Turn
	requests []agent.TurnRequest
}

func (m *workScriptedModel) Complete(_ context.Context, request agent.TurnRequest) (agent.Turn, error) {
	m.requests = append(m.requests, request)
	if len(m.turns) == 0 {
		return agent.Turn{}, errors.New("unexpected model call")
	}
	turn := m.turns[0]
	m.turns = m.turns[1:]
	return turn, nil
}

func hasDefinition(definitions []agent.ToolDefinition, name string) bool {
	for _, definition := range definitions {
		if definition.Name == name {
			return true
		}
	}
	return false
}
