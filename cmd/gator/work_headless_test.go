package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/eval"
	"github.com/gongahkia/gator/internal/jobs"
	"github.com/gongahkia/gator/internal/tools"
	"github.com/gongahkia/gator/internal/workrun"
)

func TestWorkHeadlessControlsRunningApprovalAndPreservesContract(t *testing.T) {
	request := workrun.Request{SourcePath: t.TempDir(), Objective: "Prepare reviewed output", Contract: artifact.DefaultContract("report.md")}
	request.Contract.MaxArtifactBytes = 4096
	state := t.TempDir()
	service := func() workrun.Service {
		return workrun.Service{Executor: workrun.Executor{StateDir: state, Model: &eval.ScriptedModel{Turns: []agent.Turn{depthCall("delegate_agents", map[string]any{"tasks": []any{map[string]string{"agent": "code", "task": "reviewed command"}}}), depthCall("write_artifact", map[string]string{"path": "report.md", "content": "Reviewed output"}), {Text: "Ready"}}}, Code: func(ctx context.Context, r workrun.CodeRequest) (workrun.CodeResult, error) {
			decision, err := r.Approve(ctx, []string{"reviewed-command"})
			if decision != tools.CommandAllowOnce {
				t.Error("exact approval was not delivered")
			}
			return workrun.CodeResult{Summary: "reviewed"}, err
		}}}
	}
	input, control := io.Pipe()
	output, writer := io.Pipe()
	defer input.Close()
	defer control.Close()
	defer output.Close()
	defer writer.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	finished := make(chan error, 1)
	go func() {
		finished <- serveWorkOperation(ctx, service(), request, bufio.NewScanner(input), writer)
		writer.Close()
	}()
	decoder := json.NewDecoder(output)
	approved, inspected := false, false
	var result struct {
		ManifestPath string `json:"manifest_path"`
		Conversation string `json:"conversation_id"`
		Revision     string `json:"revision_id"`
		Status       string `json:"status"`
	}
	for {
		var frame struct {
			Version int
			Type    string
			Data    json.RawMessage
		}
		if err := decoder.Decode(&frame); err != nil {
			t.Fatal(err)
		}
		if frame.Version != 1 {
			t.Fatal("wrong protocol version")
		}
		if frame.Type == "approval" {
			var interaction workrun.Interaction
			if err := json.Unmarshal(frame.Data, &interaction); err != nil {
				t.Fatal(err)
			}
			if interaction.Kind != "command" || !strings.Contains(string(frame.Data), "reviewed-command") {
				t.Fatal(string(frame.Data))
			}
			if err := json.NewEncoder(control).Encode(workFrame{Version: 1, Type: "inspect_task", TaskID: "subagent-001"}); err != nil {
				t.Fatal(err)
			}
			if err := json.NewEncoder(control).Encode(workFrame{Version: 1, Type: "approval", InteractionID: interaction.ID, Approved: true}); err != nil {
				t.Fatal(err)
			}
			approved = true
		}
		if frame.Type == "task" {
			inspected = true
		}
		if frame.Type == "result" {
			if err := json.Unmarshal(frame.Data, &result); err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	if !approved || !inspected || result.Status != "completed" || result.Conversation == "" || result.Revision == "" {
		t.Fatalf("approval=%t inspect=%t result=%+v", approved, inspected, result)
	}
	bundle, err := artifact.OpenBundle(result.ManifestPath)
	if err != nil {
		t.Fatal(err)
	}
	digest, _ := request.Contract.Digest()
	if bundle.Manifest.ContractSHA256 != digest {
		t.Fatal("full contract lost")
	}
	direct := service()
	request.ApproveCodeCommand = func(context.Context, []string) (tools.CommandDecision, error) { return tools.CommandAllowOnce, nil }
	outcome, err := direct.Execute(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Manifest.PolicySHA256 != bundle.Manifest.PolicySHA256 {
		t.Fatal("headless changed effective policy")
	}
	jobRequest := jobWorkRequest(jobs.Definition{SourcePath: request.SourcePath, Objective: request.Objective, Contract: request.Contract})
	jobRequest.ApproveCodeCommand = request.ApproveCodeCommand
	outcome, err = service().Execute(ctx, jobRequest)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Manifest.PolicySHA256 != bundle.Manifest.PolicySHA256 {
		t.Fatal("scheduled typed path changed effective policy")
	}
}

func TestWorkProtocolRejectsUnknownFieldsAndMultipleValues(t *testing.T) {
	for _, input := range []string{`{"version":1,"type":"cancel","grant":"all"}`, `{"version":1,"type":"cancel"} {}`} {
		var frame workFrame
		if decodeWorkFrame([]byte(input), &frame) == nil {
			t.Fatal("invalid frame accepted")
		}
	}
}
