package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/eval"
	gatorrun "github.com/gongahkia/gator/internal/run"
	"github.com/gongahkia/gator/internal/workrun"
)

type depthModel func(context.Context, agent.TurnRequest) (agent.Turn, error)

func (m depthModel) Complete(ctx context.Context, r agent.TurnRequest) (agent.Turn, error) {
	return m(ctx, r)
}
func depthCall(name string, args any) agent.Turn {
	data, _ := json.Marshal(args)
	return agent.Turn{ToolCalls: []agent.ToolCall{{ID: name, Name: name, Arguments: data}}}
}
func TestWorkCodeFollowupUsesAcceptedBaselineAndFrozenProfile(t *testing.T) {
	source := t.TempDir()
	for name, text := range map[string]string{"a.txt": "base\n", "b.txt": "base\n", ".gator/agents.json": `{"version":1,"profiles":[{"name":"implementer","instructions":"Retain amber configuration."}]}`} {
		path := filepath.Join(source, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	state := t.TempDir()
	request := workrun.Request{RunID: "first", SourcePath: source, Objective: "Stage alpha", Contract: artifact.DefaultContract("report.md"), Code: workrun.CodePolicy{Profile: "implementer"}}
	manager := func(run, task, baseline string) agent.Model {
		return &eval.ScriptedModel{Turns: []agent.Turn{depthCall("delegate_agents", map[string]any{"tasks": []any{map[string]string{"agent": "code", "task": task, "baseline_patch": baseline}}}), depthCall("integrate_code", map[string]any{"patches": []string{"code/" + run + "-subagent-001.patch"}}), depthCall("write_artifact", map[string]string{"path": "report.md", "content": "Staged verified " + task}), {Text: "Prepared."}}}
	}
	service := workrun.Service{Executor: workrun.Executor{Model: manager("first", "alpha", ""), StateDir: state}}
	service.Executor.Code = func(ctx context.Context, r workrun.CodeRequest) (workrun.CodeResult, error) {
		name, value := "a.txt", "alpha"
		if r.Task == "beta" {
			name, value = "b.txt", "beta"
			if !strings.Contains(string(r.Baseline), "+alpha") {
				t.Error("followup did not receive accepted alpha baseline")
			}
		}
		script := &eval.ScriptedModel{Turns: []agent.Turn{depthCall("apply_patch", map[string]string{"patch": "diff --git a/" + name + " b/" + name + "\n--- a/" + name + "\n+++ b/" + name + "\n@@ -1 +1 @@\n-base\n+" + value + "\n"}), depthCall("git_status", map[string]any{}), depthCall("git_diff", map[string]any{}), depthCall("run_command", map[string]any{"argv": []string{"git", "diff", "--check"}}), {Text: "Verified."}}}
		backend := &nativeWorkBackend{provider: "scripted", model: "fixture", code: gatorrun.Executor{Model: depthModel(func(ctx context.Context, turn agent.TurnRequest) (agent.Turn, error) {
			if !strings.Contains(turn.System, "amber") || strings.Contains(turn.System, "violet") {
				t.Error("Code did not load frozen profile")
			}
			return script.Complete(ctx, turn)
		})}}
		return backend.codeDelegate(state)(ctx, r)
	}
	first, err := service.Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Manifest.Candidates) != 1 || first.Manifest.Candidates[0].Status != "verified" {
		t.Fatalf("first candidate: %+v; messages: %+v", first.Manifest.Candidates, first.Result.Messages)
	}
	if err := os.WriteFile(filepath.Join(source, ".gator/agents.json"), []byte(`{"version":1,"profiles":[{"name":"implementer","instructions":"violet mutable replacement"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	request.RunID = "second"
	request.ConversationID = first.ConversationID
	request.Objective = "Add beta on the accepted alpha changes"
	service.Executor.Model = manager("second", "beta", first.Manifest.Candidates[0].PatchPath)
	second, err := service.Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Manifest.Candidates) != 1 || len(second.Manifest.Candidates[0].ChangedPaths) != 2 {
		t.Fatalf("followup candidate: %+v", second.Manifest.Candidates)
	}
	patch, err := second.Work.Output.ReadRegularFile(second.Manifest.Candidates[0].PatchPath, 1024*1024)
	if err != nil || !strings.Contains(string(patch), "+alpha") || !strings.Contains(string(patch), "+beta") {
		t.Fatalf("cumulative patch: %s %v", patch, err)
	}
	original, err := os.ReadFile(filepath.Join(source, "a.txt"))
	if err != nil || string(original) != "base\n" {
		t.Fatal("live source changed")
	}
}
