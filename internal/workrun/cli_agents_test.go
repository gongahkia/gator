package workrun

import (
	"context"
	"encoding/json"
	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/cliagent"
	"github.com/gongahkia/gator/internal/orchestrator"
	"github.com/gongahkia/gator/internal/sandbox"
	"github.com/gongahkia/gator/internal/workspace"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type agentDiscoveryModel struct {
	result string
	step   int
}

type nativeContinuationModel struct {
	step     int
	previous string
}

func (m *nativeContinuationModel) Complete(_ context.Context, _ agent.TurnRequest) (agent.Turn, error) {
	m.step++
	if m.step == 1 {
		args, _ := json.Marshal(map[string]any{"agent": "cli_codex", "task": "read-only fixture", "continue": m.previous})
		return agent.Turn{ToolCalls: []agent.ToolCall{{ID: "start", Name: "start_agent", Arguments: args}}}, nil
	}
	if m.step == 2 {
		return agent.Turn{ToolCalls: []agent.ToolCall{{ID: "await", Name: "await_agent", Arguments: []byte(`{"id":"subagent-001"}`)}}}, nil
	}
	return agent.Turn{Text: "Fixture complete."}, nil
}

func TestExternalNativeSessionContinuesAcrossWorkConversationTurns(t *testing.T) {
	if _, err := sandbox.StrictAvailability(); err != nil {
		t.Skip(err)
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "source")
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "note.txt"), []byte("unchanged\n"), 0600); err != nil {
		t.Fatal(err)
	}
	script, err := os.ReadFile("../cliagent/testdata/codex_rpc.sh")
	if err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(root, "cli")
	if err := os.WriteFile(exe, script, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FIXTURE_API_KEY", "synthetic-test-key")
	cfg := cliagent.Config{Kind: "codex", Executable: exe, APIKeyEnv: "FIXTURE_API_KEY", TimeoutSeconds: 10}
	state := filepath.Join(root, "state")
	request := Request{RunID: "first", SourcePath: source, Objective: "read-only fixture", Mode: action.Inspect, Contract: artifact.InspectionContract(), ExternalAgents: []cliagent.Config{cfg}}
	service := Service{Executor: Executor{Model: &nativeContinuationModel{}, StateDir: state}}
	out, err := service.Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	first, err := orchestrator.ReadTasks(filepath.Join(state, "gator", "tasks", "first"))
	if err != nil || len(first) != 1 || first[0].Status != "completed" {
		t.Fatalf("first task: %+v %v", first, err)
	}
	// Create a sibling branch with a different native worker. It must not be
	// imported when the user returns to the first revision's main branch.
	service.Executor.Model = &nativeContinuationModel{}
	request.RunID, request.ConversationID = "sibling", out.ConversationID
	if _, err := service.Execute(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	sibling, err := orchestrator.ReadTasks(filepath.Join(state, "gator", "tasks", "sibling"))
	if err != nil || len(sibling) != 1 || sibling[0].Status != "completed" {
		t.Fatalf("sibling task: %+v %v", sibling, err)
	}
	// A turn with no delegated work must not hide earlier native threads.
	service.Executor.Model = &agentDiscoveryModel{step: 1}
	request.RunID, request.ConversationID, request.Objective = "unrelated", out.ConversationID, "Hello, how are you?"
	request.ParentRevisionID = "first"
	if _, err := service.Execute(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	service.Executor.Model = &nativeContinuationModel{previous: first[0].GlobalID}
	request.RunID, request.ConversationID = "second", out.ConversationID
	request.ParentRevisionID = "unrelated"
	if _, err := service.Execute(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	second, err := orchestrator.ReadTasks(filepath.Join(state, "gator", "tasks", "second"))
	if err != nil || len(second) != 1 || second[0].Status != "completed" {
		t.Fatalf("continued task: %+v %v", second, err)
	}
	if second[0].Result.SessionID != first[0].Result.SessionID || !strings.Contains(second[0].Result.Summary, "turn 2") {
		t.Fatalf("lost native conversation: %+v", second[0])
	}
	service.Executor.Model = &nativeContinuationModel{previous: sibling[0].GlobalID}
	request.RunID, request.ParentRevisionID = "wrong-branch", "second"
	if _, err := service.Execute(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	rejected, err := orchestrator.ReadTasks(filepath.Join(state, "gator", "tasks", "wrong-branch"))
	if err != nil || len(rejected) != 0 {
		t.Fatalf("resumed sibling branch task: %+v %v", rejected, err)
	}
}

func (m *agentDiscoveryModel) Complete(_ context.Context, r agent.TurnRequest) (agent.Turn, error) {
	m.step++
	if m.step == 1 {
		return agent.Turn{ToolCalls: []agent.ToolCall{{ID: "discover", Name: "list_agents", Arguments: []byte(`{}`)}}}, nil
	}
	for _, message := range r.Messages {
		if message.Role == "tool" {
			m.result += message.Content
		}
	}
	return agent.Turn{Text: "Agent discovery complete."}, nil
}
func TestExternalAgentsReachManagerAndPolicyIdentity(t *testing.T) {
	cfg := cliagent.Config{Kind: "codex", Executable: filepath.Join(t.TempDir(), "codex"), APIKeyEnv: "OPENAI_API_KEY", TimeoutSeconds: 60}
	for _, disabled := range []bool{false, true} {
		m := &agentDiscoveryModel{}
		r := Request{SourcePath: t.TempDir(), Objective: "List available specialist agents.", Mode: action.Inspect, Contract: artifact.InspectionContract(), ExternalAgents: []cliagent.Config{cfg}}
		if disabled {
			r.DisabledRoles = []string{"cli_codex"}
		}
		s := Service{Executor: Executor{Model: m, StateDir: t.TempDir()}, Defaults: &r}
		supplied := r
		supplied.ExternalAgents = nil
		if _, err := s.Execute(context.Background(), supplied); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(m.result, `"name":"cli_codex"`) == disabled {
			t.Fatalf("manager registry: %s", m.result)
		}
	}
	r := Request{ExternalAgents: []cliagent.Config{cfg}}
	before, err := PolicyDigest(r)
	if err != nil {
		t.Fatal(err)
	}
	r.ExternalAgents[0].AllowEdits = true
	after, err := PolicyDigest(r)
	if err != nil || before == after {
		t.Fatal("CLI authority not bound to policy", err)
	}
}
func TestExternalAgentEditAuthorityRequiresExplicitConfiguration(t *testing.T) {
	for _, test := range []struct {
		mode                   action.Mode
		enabled, allowed, want bool
	}{{action.Inspect, true, true, false}, {action.Draft, false, true, true}, {action.Draft, true, false, false}, {action.Draft, true, true, true}} {
		s := (Executor{}).externalCLISpecialist(Request{Mode: test.mode, EnableCode: test.enabled}, workspace.Work{}, cliagent.Config{Kind: "pi", AllowEdits: test.allowed})
		if strings.Contains(s.Description, "reviewable patch") != test.want {
			t.Fatal(s.Description)
		}
	}
}

func TestExternalPatchEntersVerifiedIntegration(t *testing.T) {
	if _, err := sandbox.StrictAvailability(); err != nil {
		t.Skip(err)
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "source")
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "note.txt"), []byte("before\n"), 0600); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(root, "fake-cli")
	script, err := os.ReadFile("../cliagent/testdata/codex_rpc.sh")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(executable, script, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FIXTURE_API_KEY", "test-only-credential")
	work, err := workspace.CreateWork(source, filepath.Join(root, "state"), "cli-integration", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	request := Request{StateDir: filepath.Join(root, "state"), RunID: "cli-integration", Mode: action.Draft, Contract: artifact.DefaultContract("report.md")}
	request.integration = &codeIntegration{request: request, work: work, patches: map[string]string{}, accepted: map[string]string{}}
	cfg := cliagent.Config{Kind: "codex", Executable: executable, APIKeyEnv: "FIXTURE_API_KEY", TimeoutSeconds: 10, AllowEdits: true}
	specialist := (Executor{}).externalCLISpecialist(request, work, cfg)
	result, err := specialist.Run(context.Background(), orchestrator.Invocation{ID: "subagent-1", Task: "Update note"})
	if err != nil {
		t.Fatal(err)
	}
	if result.ArtifactPath == "" || request.integration.patches[result.ArtifactPath] != result.ArtifactSHA256 {
		t.Fatal("patch not registered")
	}
	if result.SessionID == "" {
		t.Fatal("native session identity missing")
	}
	continued, err := specialist.Run(context.Background(), orchestrator.Invocation{ID: "subagent-2", SessionID: result.SessionID, Task: "Revise the same note"})
	if err != nil || continued.SessionID != result.SessionID || !strings.Contains(continued.Summary, "turn 2") {
		t.Fatalf("native continuation failed: %+v %v", continued, err)
	}
	result = continued
	input, _ := json.Marshal(map[string]any{"patches": []string{result.ArtifactPath}})
	integrated, err := request.integration.Execute(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(integrated.Content, `"status":"verified"`) {
		t.Fatal(integrated.Content)
	}
	original, err := os.ReadFile(filepath.Join(source, "note.txt"))
	if err != nil || string(original) != "before\n" {
		t.Fatal("source changed", err)
	}
}
