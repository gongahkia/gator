package workrun

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/tools"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type controlledModel struct {
	entered chan struct{}
	release chan struct{}
	calls   int
}

func (m *controlledModel) Complete(ctx context.Context, r agent.TurnRequest) (agent.Turn, error) {
	m.calls++
	if m.calls == 1 {
		close(m.entered)
		select {
		case <-m.release:
		case <-ctx.Done():
			return agent.Turn{}, ctx.Err()
		}
	}
	return agent.Turn{Text: r.Messages[len(r.Messages)-1].Content}, nil
}
func TestServiceSteersAndCancelsRunningOperation(t *testing.T) {
	for _, cancelRun := range []bool{false, true} {
		t.Run(map[bool]string{true: "cancel", false: "steer"}[cancelRun], func(t *testing.T) {
			model := &controlledModel{entered: make(chan struct{}), release: make(chan struct{})}
			service := Service{Executor: Executor{Model: model, StateDir: t.TempDir()}}
			operation := service.Start(context.Background(), Request{SourcePath: t.TempDir(), Objective: "inspect", Mode: action.Inspect, Contract: artifact.InspectionContract()})
			<-model.entered
			if cancelRun {
				operation.Cancel()
			} else {
				if err := operation.Steer("Remember amber"); err != nil {
					t.Fatal(err)
				}
				close(model.release)
			}
			timeout := time.After(3 * time.Second)
			for {
				select {
				case result := <-operation.Done:
					if result.Outcome.RevisionID == "" {
						t.Fatal("no inspectable revision")
					}
					if cancelRun && result.Err == nil {
						t.Fatal("cancellation lost")
					}
					if !cancelRun && (result.Err != nil || !strings.Contains(result.Outcome.Result.FinalText, "amber")) {
						t.Fatalf("%+v %v", result.Outcome.Result, result.Err)
					}
					return
				case <-operation.Events:
				case <-timeout:
					t.Fatal("operation did not stop")
				}
			}
		})
	}
}

func TestServiceApprovalIsPendingUntilExplicitResponse(t *testing.T) {
	state := t.TempDir()
	model := &scriptedModel{turns: []agent.Turn{{ToolCalls: []agent.ToolCall{{ID: "child", Name: "delegate_agents", Arguments: json.RawMessage(`{"tasks":[{"agent":"code","task":"approval"}]}`)}}}, {ToolCalls: []agent.ToolCall{{ID: "write", Name: "write_artifact", Arguments: json.RawMessage(`{"path":"report.md","content":"approval resolved"}`)}}}, {Text: "done"}}}
	executed := make(chan struct{})
	service := Service{Executor: Executor{Model: model, StateDir: state, Code: func(ctx context.Context, request CodeRequest) (CodeResult, error) {
		decision, err := request.Approve(ctx, []string{"reviewed-command"})
		if err != nil {
			return CodeResult{}, err
		}
		if decision != tools.CommandAllowOnce {
			return CodeResult{}, errors.New("denied")
		}
		close(executed)
		return CodeResult{Summary: "Approved command received"}, nil
	}}}
	operation := service.Start(context.Background(), Request{RunID: "approval-run", SourcePath: t.TempDir(), Objective: "prepare", Contract: artifact.DefaultContract("report.md")})
	defer operation.Cancel()
	interaction := <-operation.Interactions
	select {
	case <-executed:
		t.Fatal("ran before approval")
	default:
	}
	payload, err := os.ReadFile(filepath.Join(state, "gator", "interactions", "approval-run", "1.json"))
	if err != nil || !strings.Contains(string(payload), "pending") {
		t.Fatalf("pending state: %s %v", payload, err)
	}
	if err := operation.Respond(interaction.ID, true); err != nil {
		t.Fatal(err)
	}
	for range operation.Events {
	}
	result := <-operation.Done
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	select {
	case <-executed:
	default:
		t.Fatal("approved command not released")
	}
	if err := operation.Respond(interaction.ID, true); err == nil {
		t.Fatal("approval replay accepted")
	}
}
