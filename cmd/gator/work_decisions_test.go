package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/tools"
	"github.com/gongahkia/gator/internal/workrun"
)

func TestWorkAnswerRequiresExactInteractiveResponse(t *testing.T) {
	state := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", state)
	model := &workScriptedModel{turns: []agent.Turn{
		{ToolCalls: []agent.ToolCall{{ID: "child", Name: "delegate_agents", Arguments: json.RawMessage(`{"tasks":[{"agent":"code","task":"approval"}]}`)}}},
		{ToolCalls: []agent.ToolCall{{ID: "write", Name: "write_artifact", Arguments: json.RawMessage(`{"path":"report.md","content":"approved"}`)}}},
		{Text: "done"},
	}}
	approved := false
	service := workrun.Service{Executor: workrun.Executor{Model: model, StateDir: state, Code: func(ctx context.Context, request workrun.CodeRequest) (workrun.CodeResult, error) {
		decision, err := request.Approve(ctx, []string{"reviewed-command"})
		approved = decision == tools.CommandAllowOnce
		return workrun.CodeResult{Summary: "Decision recorded"}, err
	}}}
	operation := service.Start(context.Background(), workrun.Request{RunID: "cli-answer", SourcePath: t.TempDir(), Objective: "Prepare", Contract: artifact.DefaultContract("report.md"), Mode: action.Draft, EnableCode: true})
	defer operation.Cancel()
	interaction := <-operation.Interactions
	if interaction.ID != 1 || !isWorkSessionCommand("answer") {
		t.Fatalf("pending answer command = %#v", interaction)
	}
	var output bytes.Buffer
	if err := workSessionCommand([]string{"answer", operation.RunID, "1"}, strings.NewReader(""), &output, nil); err == nil || !strings.Contains(err.Error(), "not answered") {
		t.Fatalf("empty answer changed pending decision: %v", err)
	}
	output.Reset()
	if err := workSessionCommand([]string{"answer", operation.RunID, "1"}, strings.NewReader("yes\n"), &output, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "reviewed-command") || !strings.Contains(output.String(), "exact preview (escaped)") || !strings.Contains(output.String(), "submitted") {
		t.Fatalf("answer prompt omitted the exact request: %q", output.String())
	}
	for range operation.Events {
	}
	if result := <-operation.Done; result.Err != nil || !approved {
		t.Fatalf("approved response did not reach live run: %v, approved=%t", result.Err, approved)
	}
}

func TestWorkDecisionsShowsPendingAcrossRunsAndHistoryForOneRun(t *testing.T) {
	state := t.TempDir()
	for _, item := range []struct {
		run    string
		status string
	}{{"run-one", "pending"}, {"run-two", "cancelled"}} {
		dir := filepath.Join(state, "gator", "interactions", item.run)
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		payload, err := json.Marshal(workrun.InteractionRecord{Version: 1, Status: item.status, Interaction: workrun.Interaction{ID: 1, Kind: "action", Preview: "private preview"}})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "1.json"), payload, 0600); err != nil {
			t.Fatal(err)
		}
	}
	var output bytes.Buffer
	if err := workDecisionsCommand(state, nil, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "run-one #1 pending (run unconfirmed) action") || strings.Contains(output.String(), "run-two") || strings.Contains(output.String(), "private preview") {
		t.Fatalf("pending view = %q", output.String())
	}
	if err := os.WriteFile(filepath.Join(state, "gator", "interactions", "run-one", "active.lock"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := workDecisionsCommand(state, nil, &output); err != nil || !strings.Contains(output.String(), "run-one #1 pending (stale) action") {
		t.Fatalf("stopped pending view = %q, %v", output.String(), err)
	}
	output.Reset()
	if err := workDecisionsCommand(state, []string{"run-two", "--json"}, &output); err != nil {
		t.Fatal(err)
	}
	var result struct {
		LiveStatus string                      `json:"live_status"`
		Decisions  []workrun.InteractionRecord `json:"decisions"`
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil || result.LiveStatus != "per_run_lock" || len(result.Decisions) != 1 || result.Decisions[0].Status != "cancelled" {
		t.Fatalf("retained decision JSON = %#v, %v", result, err)
	}
	denied := false
	for id := 2; id <= 261; id++ {
		payload, err := json.Marshal(workrun.InteractionRecord{Version: 1, Status: "resolved", Interaction: workrun.Interaction{ID: id, Kind: "action"}, Approved: &denied})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(state, "gator", "interactions", "run-two", fmt.Sprintf("%d.json", id)), payload, 0600); err != nil {
			t.Fatal(err)
		}
	}
	output.Reset()
	if err := workDecisionsCommand(state, nil, &output); err != nil || !strings.Contains(output.String(), "run-one #1 pending (stale) action") || strings.Contains(output.String(), "run-two") {
		t.Fatalf("pending view after resolved history = %q, %v", output.String(), err)
	}
}
