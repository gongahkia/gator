package workrun

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/tools"
	"github.com/gongahkia/gator/internal/workspace"
)

func TestWorkDecisionAcrossProcesses(t *testing.T) {
	for _, approved := range []bool{true, false} {
		name := "deny"
		if approved {
			name = "approve"
		}
		t.Run(name, func(t *testing.T) {
			state, source := t.TempDir(), t.TempDir()
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestWorkDecisionHelperProcess$")
			child.Env = append(os.Environ(), "GATOR_DECISION_HELPER=1", "GATOR_CONTROL_STATE="+state, "GATOR_CONTROL_SOURCE="+source)
			stdout, err := child.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			var stderr bytes.Buffer
			child.Stderr = &stderr
			if err := child.Start(); err != nil {
				t.Fatal(err)
			}
			scanner := bufio.NewScanner(stdout)
			if !scanner.Scan() || scanner.Text() != "READY" {
				t.Fatalf("child did not ask for approval: %q, %v, %s", scanner.Text(), scanner.Err(), stderr.String())
			}
			records, err := ReadInteractionRecords(state, "cross-process-decision")
			if err != nil || len(records) != 1 || records[0].Status != "pending" || records[0].LiveStatus != "active" {
				t.Fatalf("pending decision = %#v, %v", records, err)
			}
			id := records[0].Interaction.ID
			digest, err := InteractionDigest(records[0].Interaction)
			if err != nil {
				t.Fatal(err)
			}
			if approved {
				if err := SendWorkDecision(ctx, state, "cross-process-decision", id, strings.Repeat("0", 64), true); err == nil || !strings.Contains(err.Error(), "HTTP 409") {
					t.Fatalf("wrong preview digest was accepted: %v", err)
				}
				root, err := workspace.Open(filepath.Join(state, "gator", "interactions", "cross-process-decision"))
				if err != nil {
					t.Fatal(err)
				}
				original, err := root.ReadRegularFile("1.json", 1024*1024)
				if err != nil {
					t.Fatal(err)
				}
				var tampered InteractionRecord
				if err := json.Unmarshal(original, &tampered); err != nil {
					t.Fatal(err)
				}
				tampered.Interaction.Preview = []string{"send all files"}
				changed, _ := json.Marshal(tampered)
				if err := root.WriteRegularFileAtomic("1.json", changed, 1024*1024); err != nil {
					t.Fatal(err)
				}
				forgedDigest, _ := InteractionDigest(tampered.Interaction)
				if err := SendWorkDecision(ctx, state, "cross-process-decision", id, forgedDigest, true); err == nil || !strings.Contains(err.Error(), "HTTP 409") {
					t.Fatalf("edited preview was accepted: %v", err)
				}
				if err := root.WriteRegularFileAtomic("1.json", original, 1024*1024); err != nil {
					t.Fatal(err)
				}
			}
			if err := SendWorkDecision(ctx, state, "cross-process-decision", id, digest, approved); err != nil {
				t.Fatal(err)
			}
			if err := SendWorkDecision(ctx, state, "cross-process-decision", id, digest, approved); err == nil || !strings.Contains(err.Error(), "HTTP 409") {
				t.Fatalf("decision replay was accepted: %v", err)
			}
			if !scanner.Scan() {
				t.Fatalf("child produced no result: %v, %s", scanner.Err(), stderr.String())
			}
			expected := "DENIED"
			if approved {
				expected = "APPROVED"
			}
			if scanner.Text() != expected {
				t.Fatalf("child result = %q, wanted %q", scanner.Text(), expected)
			}
			if err := child.Wait(); err != nil {
				t.Fatalf("child failed: %v, %s", err, stderr.String())
			}
			records, err = ReadInteractionRecords(state, "cross-process-decision")
			if err != nil || len(records) != 1 || records[0].Status != "resolved" || records[0].Approved == nil || *records[0].Approved != approved {
				t.Fatalf("resolved decision = %#v, %v", records, err)
			}
		})
	}
}

func TestWorkDecisionHelperProcess(t *testing.T) {
	if os.Getenv("GATOR_DECISION_HELPER") != "1" {
		return
	}
	state, source := os.Getenv("GATOR_CONTROL_STATE"), os.Getenv("GATOR_CONTROL_SOURCE")
	model := &scriptedModel{turns: []agent.Turn{
		{ToolCalls: []agent.ToolCall{{ID: "child", Name: "delegate_agents", Arguments: json.RawMessage(`{"tasks":[{"agent":"code","task":"approval"}]}`)}}},
		{ToolCalls: []agent.ToolCall{{ID: "write", Name: "write_artifact", Arguments: json.RawMessage(`{"path":"report.md","content":"decision recorded"}`)}}},
		{Text: "done"},
	}}
	wasApproved := false
	service := Service{Executor: Executor{Model: model, StateDir: state, Code: func(ctx context.Context, request CodeRequest) (CodeResult, error) {
		decision, err := request.Approve(ctx, []string{"reviewed-command"})
		if err != nil {
			return CodeResult{}, err
		}
		wasApproved = decision == tools.CommandAllowOnce
		return CodeResult{Summary: "Decision recorded"}, nil
	}}}
	operation := service.Start(context.Background(), Request{RunID: "cross-process-decision", SourcePath: source, Objective: "Prepare", Contract: artifact.DefaultContract("report.md"), EnableCode: true})
	<-operation.Interactions
	fmt.Println("READY")
	for range operation.Events {
	}
	result := <-operation.Done
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	if wasApproved {
		fmt.Println("APPROVED")
	} else {
		fmt.Println("DENIED")
	}
}

func TestWorkControlRejectsUnauthorizedAndRedirectedCommands(t *testing.T) {
	state := t.TempDir()
	model := &controlledModel{entered: make(chan struct{}), release: make(chan struct{})}
	operation := (Service{Executor: Executor{Model: model, StateDir: state}}).Start(context.Background(), Request{
		RunID: "protected-control", SourcePath: t.TempDir(), Objective: "Inspect", Mode: action.Inspect, Contract: artifact.InspectionContract(),
	})
	defer operation.Cancel()
	<-model.entered
	root, err := workspace.Open(filepath.Join(state, "gator", "work-control"))
	if err != nil {
		t.Fatal(err)
	}
	payload, err := root.ReadRegularFile("protected-control.json", 4096)
	if err != nil {
		t.Fatal(err)
	}
	var control workControlState
	if err := json.Unmarshal(payload, &control); err != nil {
		t.Fatal(err)
	}
	response, err := (&http.Client{Timeout: time.Second}).Post(control.URL+"/stop", "text/plain", nil)
	if err != nil || response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthorized stop response = %v, %v", response, err)
	}
	_ = response.Body.Close()
	control.URL = "http://example.com"
	changed, _ := json.Marshal(control)
	if err := root.WriteRegularFileAtomic("protected-control.json", changed, 4096); err != nil {
		t.Fatal(err)
	}
	if err := SendWorkControl(context.Background(), state, "protected-control", "stop", ""); err == nil || !strings.Contains(err.Error(), "literal loopback") {
		t.Fatalf("redirected control endpoint was accepted: %v", err)
	}
	if err := root.WriteRegularFileAtomic("protected-control.json", payload, 4096); err != nil {
		t.Fatal(err)
	}
	if err := SendWorkControl(context.Background(), state, "protected-control", "stop", ""); err != nil {
		t.Fatal(err)
	}
	if result := <-operation.Done; result.Err == nil {
		t.Fatal("stopped run completed normally")
	}
}

func TestWorkControlAcrossProcesses(t *testing.T) {
	for _, command := range []string{"steer", "stop"} {
		t.Run(command, func(t *testing.T) {
			state, source := t.TempDir(), t.TempDir()
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestWorkControlHelperProcess$")
			child.Env = append(os.Environ(), "GATOR_CONTROL_HELPER=1", "GATOR_CONTROL_STATE="+state, "GATOR_CONTROL_SOURCE="+source)
			stdout, err := child.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			stdin, err := child.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			var stderr bytes.Buffer
			child.Stderr = &stderr
			if err := child.Start(); err != nil {
				t.Fatal(err)
			}
			scanner := bufio.NewScanner(stdout)
			if !scanner.Scan() || scanner.Text() != "READY" {
				t.Fatalf("child did not start: %q, %v, %s", scanner.Text(), scanner.Err(), stderr.String())
			}
			root, err := workspace.Open(filepath.Join(state, "gator", "work-control"))
			if err != nil {
				t.Fatal(err)
			}
			controlFile := "cross-process-control.json"
			info, err := os.Stat(filepath.Join(root.Path(), controlFile))
			if err != nil || info.Mode().Perm() != 0600 {
				t.Fatalf("control file mode = %v, %v", info, err)
			}
			instruction := ""
			if command == "steer" {
				instruction = "Remember amber"
			}
			if err := SendWorkControl(ctx, state, "cross-process-control", command, instruction); err != nil {
				t.Fatal(err)
			}
			if command == "steer" {
				_, _ = io.WriteString(stdin, "release\n")
			}
			_ = stdin.Close()
			if !scanner.Scan() {
				t.Fatalf("child produced no result: %v, %s", scanner.Err(), stderr.String())
			}
			result := scanner.Text()
			if command == "steer" && !strings.Contains(result, "amber") {
				t.Fatalf("cross-process steering was not applied: %q", result)
			}
			if command == "stop" && result != "STOPPED" {
				t.Fatalf("cross-process stop failed: %q", result)
			}
			if err := child.Wait(); err != nil {
				t.Fatalf("child failed: %v, %s", err, stderr.String())
			}
			if _, err := root.ReadRegularFile(controlFile, 4096); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("completed run retained control state: %v", err)
			}
			if err := SendWorkControl(context.Background(), state, "cross-process-control", command, instruction); err == nil {
				t.Fatal("completed run accepted a control command")
			}
		})
	}
}

func TestWorkControlHelperProcess(t *testing.T) {
	if os.Getenv("GATOR_CONTROL_HELPER") != "1" {
		return
	}
	state, source := os.Getenv("GATOR_CONTROL_STATE"), os.Getenv("GATOR_CONTROL_SOURCE")
	model := &controlledModel{entered: make(chan struct{}), release: make(chan struct{})}
	operation := (Service{Executor: Executor{Model: model, StateDir: state}}).Start(context.Background(), Request{
		RunID: "cross-process-control", SourcePath: source, Objective: "Inspect", Mode: action.Inspect, Contract: artifact.InspectionContract(), MaxSteps: 2,
	})
	<-model.entered
	fmt.Println("READY")
	go func() {
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		if line == "release\n" {
			close(model.release)
		}
	}()
	result := <-operation.Done
	if result.Err != nil {
		fmt.Println("STOPPED")
		return
	}
	fmt.Println("FINAL:" + strings.ReplaceAll(result.Outcome.Result.FinalText, "\n", " "))
}

type specialistControlModel struct {
	calls   int
	release chan struct{}
}

func (m *specialistControlModel) Complete(ctx context.Context, _ agent.TurnRequest) (agent.Turn, error) {
	m.calls++
	switch m.calls {
	case 1:
		return agent.Turn{ToolCalls: []agent.ToolCall{{ID: "specialist", Name: "start_agent", Arguments: json.RawMessage(`{"agent":"code","task":"wait for cancellation"}`)}}}, nil
	case 2:
		select {
		case <-m.release:
			return agent.Turn{ToolCalls: []agent.ToolCall{{ID: "write", Name: "write_artifact", Arguments: json.RawMessage(`{"path":"report.md","content":"parent continued"}`)}}}, nil
		case <-ctx.Done():
			return agent.Turn{}, ctx.Err()
		}
	default:
		return agent.Turn{Text: "parent completed"}, nil
	}
}

func TestWorkSpecialistCancelAcrossProcesses(t *testing.T) {
	state, source := t.TempDir(), t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestWorkSpecialistCancelHelperProcess$")
	child.Env = append(os.Environ(), "GATOR_TASK_CANCEL_HELPER=1", "GATOR_CONTROL_STATE="+state, "GATOR_CONTROL_SOURCE="+source)
	stdout, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdin, err := child.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	child.Stderr = &stderr
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	scanner := bufio.NewScanner(stdout)
	if !scanner.Scan() || scanner.Text() != "READY" {
		t.Fatalf("specialist did not start: %q, %v, %s", scanner.Text(), scanner.Err(), stderr.String())
	}
	if err := SendWorkTaskCancel(ctx, state, "cross-process-specialist", "../subagent-001"); err == nil {
		t.Fatal("malformed specialist ID was accepted")
	}
	if err := SendWorkTaskCancel(ctx, state, "cross-process-specialist", "subagent-002"); err == nil || !strings.Contains(err.Error(), "HTTP 409") {
		t.Fatalf("unknown specialist was accepted: %v", err)
	}
	if err := SendWorkTaskCancel(ctx, state, "cross-process-specialist", "subagent-001"); err != nil {
		t.Fatal(err)
	}
	if !scanner.Scan() || scanner.Text() != "CANCELLED" {
		t.Fatalf("specialist did not cancel: %q, %v, %s", scanner.Text(), scanner.Err(), stderr.String())
	}
	if err := SendWorkTaskCancel(ctx, state, "cross-process-specialist", "subagent-001"); err == nil || !strings.Contains(err.Error(), "HTTP 409") {
		t.Fatalf("completed cancellation replay was accepted: %v", err)
	}
	if _, err := io.WriteString(stdin, "release\n"); err != nil {
		t.Fatal(err)
	}
	_ = stdin.Close()
	if !scanner.Scan() || scanner.Text() != "DONE" {
		t.Fatalf("parent did not continue: %q, %v, %s", scanner.Text(), scanner.Err(), stderr.String())
	}
	if err := child.Wait(); err != nil {
		t.Fatalf("child failed: %v, %s", err, stderr.String())
	}
	if err := SendWorkTaskCancel(context.Background(), state, "cross-process-specialist", "subagent-001"); err == nil {
		t.Fatal("finished Work run accepted specialist cancellation")
	}
}

func TestWorkSpecialistCancelHelperProcess(t *testing.T) {
	if os.Getenv("GATOR_TASK_CANCEL_HELPER") != "1" {
		return
	}
	state, source := os.Getenv("GATOR_CONTROL_STATE"), os.Getenv("GATOR_CONTROL_SOURCE")
	model := &specialistControlModel{release: make(chan struct{})}
	started := make(chan struct{})
	service := Service{Executor: Executor{Model: model, StateDir: state, Code: func(ctx context.Context, _ CodeRequest) (CodeResult, error) {
		close(started)
		<-ctx.Done()
		return CodeResult{}, ctx.Err()
	}}}
	operation := service.Start(context.Background(), Request{RunID: "cross-process-specialist", SourcePath: source, Objective: "Prepare", Contract: artifact.DefaultContract("report.md"), EnableCode: true})
	<-started
	fmt.Println("READY")
	deadline := time.After(5 * time.Second)
	for {
		task, err := operation.InspectTask("subagent-001")
		if err == nil && task.Status == "cancelled" {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("specialist cancellation was not retained: %v, %+v", err, task)
		case <-time.After(10 * time.Millisecond):
		}
	}
	fmt.Println("CANCELLED")
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	if line != "release\n" {
		t.Fatalf("unexpected release signal %q", line)
	}
	close(model.release)
	for range operation.Events {
	}
	result := <-operation.Done
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	fmt.Println("DONE")
}
