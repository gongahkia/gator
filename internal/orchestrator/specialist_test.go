package orchestrator

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/tools"
	"github.com/gongahkia/gator/internal/workspace"
)

func TestHostedSpecialistPreservesResultEvidence(t *testing.T) {
	specialist := HostedSpecialist("code", "return a patch", func(context.Context, Invocation) (Result, error) {
		return Result{
			Summary: "changed main.go", Steps: 4, BaselineSHA256: "base",
			ArtifactPath: "code/run-subagent-001.patch", ArtifactSHA256: "abcd",
			Usage: agent.Usage{InputTokens: 10},
		}, nil
	})
	if specialist.Backend != BackendHosted {
		t.Fatalf("backend=%q", specialist.Backend)
	}
	result, err := specialist.Run(context.Background(), Invocation{ID: "subagent-001", Task: "implement"})
	if err != nil || result.Summary != "changed main.go" || result.ArtifactPath == "" || result.BaselineSHA256 != "base" || result.Usage.InputTokens != 10 {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	tools, err := Tools([]Specialist{specialist}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := tools[0].Execute(context.Background(), json.RawMessage(`{"tasks":[{"agent":"code","task":"implement"}]}`))
	if err != nil || !strings.Contains(payload.Content, `"artifact_path":"code/run-subagent-001.patch"`) || !strings.Contains(payload.Content, `"baseline_sha256":"base"`) {
		t.Fatalf("payload=%s err=%v", payload.Content, err)
	}
}

func TestToolsRejectsMissingBackend(t *testing.T) {
	_, err := Tools([]Specialist{{Name: "research", Description: "read", Run: func(context.Context, Invocation) (Result, error) {
		return Result{}, nil
	}}}, Options{})
	if err == nil || !strings.Contains(err.Error(), "backend") {
		t.Fatalf("backend error = %v", err)
	}
}

type reviewReadModel struct{ step int }

func (m *reviewReadModel) Complete(_ context.Context, _ agent.TurnRequest) (agent.Turn, error) {
	m.step++
	if m.step == 1 {
		calls := []agent.ToolCall{}
		for i, path := range []string{"candidate/changed.txt", "candidate/deleted.txt", "source/deleted.txt"} {
			data, _ := json.Marshal(map[string]string{"path": path})
			calls = append(calls, agent.ToolCall{ID: string(rune('a' + i)), Name: "read_file", Arguments: data})
		}
		return agent.Turn{ToolCalls: calls}, nil
	}
	return agent.Turn{Text: "Reviewed."}, nil
}

func TestLLMReviewRetainsActualReadsAndMissingCandidateEvidence(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"candidate", "source"} {
		if err := os.Mkdir(filepath.Join(root, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "candidate", "changed.txt"), []byte("changed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "source", "deleted.txt"), []byte("original\n"), 0600); err != nil {
		t.Fatal(err)
	}
	candidate, err := workspace.Open(filepath.Join(root, "candidate"))
	if err != nil {
		t.Fatal(err)
	}
	source, err := workspace.Open(filepath.Join(root, "source"))
	if err != nil {
		t.Fatal(err)
	}
	reader := tools.ReadFile{Root: candidate, NamedRoots: []workspace.NamedRoot{{Name: "candidate", Root: candidate}, {Name: "source", Root: source}}}
	role := LLMSpecialist("artifact_reviewer", "Read actual candidate files", &reviewReadModel{}, []agent.Tool{reader}, "Review files.", 3, nil)
	result, err := role.Run(context.Background(), Invocation{Task: "Inspect changed and deleted files."})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.InspectedPaths) != 2 || result.InspectedPaths[0] != "candidate/changed.txt" || result.InspectedPaths[1] != "source/deleted.txt" || len(result.MissingPaths) != 1 || result.MissingPaths[0] != "candidate/deleted.txt" {
		t.Fatalf("incorrect host read evidence: %+v", result)
	}
}

func TestLLMSpecialistRejectsHostPolicyTools(t *testing.T) {
	model := &oneTurnModel{}
	specialist := LLMSpecialist("research", "read", model, []agent.Tool{namedTool{name: "delegate_agents"}}, "system", 1, nil)
	if _, err := specialist.Run(context.Background(), Invocation{Task: "look"}); err == nil || !strings.Contains(err.Error(), "delegate_agents") {
		t.Fatalf("delegation error = %v", err)
	}
	if model.request.System != "" {
		t.Fatalf("model ran despite host policy: %#v", model.request)
	}
	publisher := LLMSpecialist("connected_researcher", "read", model, []agent.Tool{namedTool{name: "connector_release_publish"}}, "system", 1, nil)
	if _, err := publisher.Run(context.Background(), Invocation{Task: "send"}); err == nil || !strings.Contains(err.Error(), "publish") {
		t.Fatalf("publish error = %v", err)
	}
}

type namedTool struct{ name string }

func (t namedTool) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{Name: t.name, Parameters: json.RawMessage(`{"type":"object"}`)}
}

func (namedTool) Execute(context.Context, json.RawMessage) (agent.ToolResult, error) {
	return agent.ToolResult{}, nil
}
