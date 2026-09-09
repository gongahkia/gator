package worktui

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/tools"
	"github.com/gongahkia/gator/internal/workrun"
)

type liveModel struct{ step int }

func (m *liveModel) Complete(context.Context, agent.TurnRequest) (agent.Turn, error) {
	m.step++
	if m.step == 1 {
		return agent.Turn{ToolCalls: []agent.ToolCall{{ID: "children", Name: "delegate_agents", Arguments: json.RawMessage(`{"tasks":[{"agent":"code","task":"first"},{"agent":"code","task":"second"}]}`)}}}, nil
	}
	if m.step == 2 {
		return agent.Turn{ToolCalls: []agent.ToolCall{{ID: "write", Name: "write_artifact", Arguments: json.RawMessage(`{"path":"report.md","content":"approved"}`)}}}, nil
	}
	return agent.Turn{Text: "Complete"}, nil
}
func TestRunningTUIQueuesApprovalsWithoutLosingControl(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	service := workrun.Service{Executor: workrun.Executor{Model: &liveModel{}, StateDir: t.TempDir(), Code: func(ctx context.Context, r workrun.CodeRequest) (workrun.CodeResult, error) {
		decision, err := r.Approve(ctx, []string{r.Task})
		if decision != tools.CommandAllowOnce {
			t.Error("approval lost")
		}
		return workrun.CodeResult{Summary: "approved"}, err
	}}}
	operation := service.Start(ctx, workrun.Request{SourcePath: t.TempDir(), Objective: "Run bounded assignments", Contract: artifact.DefaultContract("report.md")})
	defer func() {
		operation.Cancel()
		for range operation.Events {
		}
		<-operation.Done
	}()
	model := New(Config{CurrentFolder: t.TempDir()})
	model.home = false
	model.launcher = false
	model.running = true
	model.operation = operation
	model.cancel = operation.Cancel
	var first, second workrun.Interaction
	select {
	case first = <-operation.Interactions:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	select {
	case second = <-operation.Interactions:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	updated, _ := model.Update(first)
	model = updated.(Model)
	updated, _ = model.Update(second)
	model = updated.(Model)
	if model.interaction.ID != first.ID || len(model.pendingInteractions) != 1 {
		t.Fatal("parallel approval replaced active preview")
	}
	model.input = "future task"
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if len(model.queue) != 1 {
		t.Fatal("future turn was not queued")
	}
	model.input = "/approve"
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if model.interaction == nil || model.interaction.ID != second.ID {
		t.Fatal("second approval inaccessible")
	}
	model.input = "/approve"
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if model.interaction != nil {
		t.Fatal("approval remained pending")
	}
	for range operation.Events {
	}
	completed := <-operation.Done
	if completed.Err != nil || completed.Outcome.RevisionID == "" {
		t.Fatalf("completion: %v", completed.Err)
	}
}
