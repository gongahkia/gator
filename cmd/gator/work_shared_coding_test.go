package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/codeexec"
	"github.com/gongahkia/gator/internal/sandbox"
	"github.com/gongahkia/gator/internal/workrun"
)

func sharedCodeTurns(prefix string) []agent.Turn {
	return []agent.Turn{
		depthCall(prefix+"read_file", map[string]any{"path": "greeting.txt"}),
		depthCall(prefix+"apply_patch", map[string]any{"patch": "diff --git a/greeting.txt b/greeting.txt\n--- a/greeting.txt\n+++ b/greeting.txt\n@@ -1 +1 @@\n-base\n+changed\n"}),
		depthCall(prefix+"run_command", map[string]any{"argv": []string{"git", "diff", "--check"}}),
		depthCall(prefix+"git_status", map[string]any{}),
		depthCall(prefix+"git_diff", map[string]any{}),
	}
}

func TestConversationDirectCodingUsesHarnessWithoutAnotherModel(t *testing.T) {
	source, state := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "greeting.txt"), []byte("base\n"), 0600); err != nil {
		t.Fatal(err)
	}
	childCalls := 0
	backend := &nativeWorkBackend{code: codeexec.Executor{Model: depthModel(func(context.Context, agent.TurnRequest) (agent.Turn, error) {
		childCalls++
		return agent.Turn{}, errors.New("direct coding must not launch a specialist")
	})}}
	turns := append(sharedCodeTurns("code_"), depthCall("code_stage", map[string]any{}), agent.Turn{Text: "Staged a verified patch; source is unchanged."})
	step := 0
	model := depthModel(func(_ context.Context, r agent.TurnRequest) (agent.Turn, error) {
		if step == 0 {
			for _, name := range []string{"code_apply_patch", "code_run_command", "code_terminal_start", "code_stage", "delegate_agents"} {
				if !hasDefinition(r.Tools, name) {
					t.Errorf("main agent missing %s", name)
				}
			}
		}
		for _, message := range r.Messages {
			if message.Role == agent.RoleTool && strings.Contains(message.Content, `"error"`) {
				t.Errorf("tool failed: %s", message.Content)
			}
		}
		if step >= len(turns) {
			return agent.Turn{}, errors.New("unexpected main model call")
		}
		turn := turns[step]
		step++
		return turn, nil
	})
	contract := artifact.InspectionContract()
	contract.AllowOptionalArtifacts = true
	request := workrun.Request{RunID: "shared-direct", SourcePath: source, Objective: "Fix the greeting", Mode: action.Draft, EnableCode: true, Contract: contract, MaxSteps: 12, Code: workrun.CodePolicy{Sandbox: sandbox.Policy{Mode: sandbox.Off}, Verification: [][]string{{"git", "diff", "--check"}}, Capabilities: []string{workrun.CodeCapabilityTerminal}}}
	outcome, err := (workrun.Executor{Model: model, StateDir: state, Code: backend.codeDelegate(state), CodeSession: backend.codeSession(state)}).Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if childCalls != 0 {
		t.Fatalf("direct coding made %d child model calls", childCalls)
	}
	if len(outcome.Manifest.Candidates) != 1 || outcome.Manifest.Candidates[0].Status != "verified" {
		t.Fatalf("candidate evidence: %#v", outcome.Manifest.Candidates)
	}
	if len(outcome.Manifest.Subagents) != 0 {
		t.Fatalf("unexpected delegation: %#v", outcome.Manifest.Subagents)
	}
	original, _ := os.ReadFile(filepath.Join(source, "greeting.txt"))
	if string(original) != "base\n" {
		t.Fatal("live source changed")
	}
}

func TestCodingSessionFactorySharesCoreToolsAndFailsClosedOnCommandApproval(t *testing.T) {
	source, scratch := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "greeting.txt"), []byte("base\n"), 0600); err != nil {
		t.Fatal(err)
	}
	backend := &nativeWorkBackend{code: codeexec.Executor{}}
	session, err := backend.codeSession(t.TempDir())(context.Background(), workrun.CodeRequest{ID: "shared-tools", SourcePath: source, ScratchPath: scratch, Task: "Inspect", ParentRunID: "parent", Policy: workrun.CodePolicy{Sandbox: sandbox.Policy{Mode: sandbox.Off}, Capabilities: []string{workrun.CodeCapabilityTerminal}}})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Harness.Close()
	names := map[string]bool{}
	for _, tool := range session.Harness.Tools {
		name := tool.Definition().Name
		names[name] = true
		if name == "run_command" {
			payload, _ := json.Marshal(map[string]any{"argv": []string{"sh", "-c", "printf unsafe > should-not-exist"}})
			if _, err := tool.Execute(context.Background(), payload); err == nil {
				t.Fatal("unapproved command executed")
			}
		}
	}
	for _, name := range []string{"apply_patch", "run_command", "read_file", "git_diff", "git_status", "terminal_start", "terminal_read", "terminal_write", "terminal_stop"} {
		if !names[name] {
			t.Errorf("shared harness missing %s", name)
		}
	}
	if _, err := os.Stat(filepath.Join(session.Harness.Worktree.Path, "should-not-exist")); !os.IsNotExist(err) {
		t.Fatal("unapproved command had effects")
	}
}

func TestInternalCodingCompletionUsesVerificationAndIndependentReadOnlyReview(t *testing.T) {
	source, state := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "greeting.txt"), []byte("base\n"), 0600); err != nil {
		t.Fatal(err)
	}
	worker := &workScriptedModel{turns: append(sharedCodeTurns(""), agent.Turn{Text: "Changed the greeting."})}
	backend := &nativeWorkBackend{code: codeexec.Executor{Model: worker}}
	reviewed := false
	reviewer := depthModel(func(_ context.Context, r agent.TurnRequest) (agent.Turn, error) {
		for _, definition := range r.Tools {
			if definition.Name == "apply_patch" || definition.Name == "run_command" || definition.Name == "delegate_agents" {
				t.Error("read-only reviewer received mutation/delegation authority")
			}
		}
		if !reviewed {
			reviewed = true
			return depthCall("read_file", map[string]any{"path": "candidate/greeting.txt"}), nil
		}
		// The exact candidate digest comes from the host assignment, not a worker claim.
		prompt := ""
		for _, message := range r.Messages {
			if message.Role == agent.RoleUser {
				prompt = message.Content
			}
		}
		marker := `"candidate_sha256":"`
		i := strings.Index(prompt, marker)
		if i < 0 {
			return agent.Turn{}, errors.New("review assignment has no exact digest")
		}
		digest := strings.SplitN(prompt[i+len(marker):], `"`, 2)[0]
		payload, _ := json.Marshal(map[string]any{"passed": true, "candidate_sha256": digest, "findings": []string{}})
		return agent.Turn{Text: string(payload)}, nil
	})
	main := &workScriptedModel{turns: []agent.Turn{depthCall("complete_code", map[string]any{"agent": "code", "objective": "Fix greeting.txt", "acceptance": []string{"greeting is changed"}}), {Text: "Reviewed and staged."}}}
	contract := artifact.InspectionContract()
	contract.AllowOptionalArtifacts = true
	request := workrun.Request{RunID: "shared-completion", SourcePath: source, Objective: "Fix greeting", Mode: action.Draft, EnableCode: true, Contract: contract, MaxSteps: 10, Code: workrun.CodePolicy{Sandbox: sandbox.Policy{Mode: sandbox.Off}, Verification: [][]string{{"git", "diff", "--check"}}}}
	outcome, err := (workrun.Executor{Model: main, StateDir: state, Code: backend.codeDelegate(state), CodeSession: backend.codeSession(state), RoleModels: map[string]agent.Model{"artifact_reviewer": reviewer}}).Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	records, err := workrun.ReadCompletions(state, "shared-completion")
	if err != nil || len(records) != 1 || records[0].Status != "completed" {
		t.Fatalf("completion=%#v error=%v result=%#v", records, err, outcome.Result)
	}
	if !reviewed || len(outcome.Manifest.Candidates) != 1 {
		t.Fatal("verification/review path was skipped")
	}
	original, _ := os.ReadFile(filepath.Join(source, "greeting.txt"))
	if string(original) != "base\n" {
		t.Fatal("source changed")
	}
}

func TestInteractiveCodingContractAllowsStagedPatchWithoutExternalAuthority(t *testing.T) {
	contract, err := interactiveWorkContract(action.Draft, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !contract.AllowOptionalArtifacts || contract.ExternalActions != action.Forbid || len(contract.Artifacts) != 0 {
		t.Fatalf("unsafe coding contract: %#v", contract)
	}
	inspect, err := interactiveWorkContract(action.Inspect, nil)
	if err != nil || inspect.AllowOptionalArtifacts || inspect.ExternalActions != action.Forbid {
		t.Fatalf("inspection contract=%#v %v", inspect, err)
	}
}
