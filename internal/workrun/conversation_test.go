package workrun

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
)

type conversationModel struct {
	turn    agent.Turn
	err     error
	request agent.TurnRequest
	calls   int
}

func (model *conversationModel) Complete(_ context.Context, request agent.TurnRequest) (agent.Turn, error) {
	model.request, model.calls = request, model.calls+1
	return model.turn, model.err
}

func TestConversationRoutesAreToolFreeAndBoundToHostTaskIdentity(t *testing.T) {
	request := ConversationRequest{Text: "Actually, keep the venue under $2,000.", CanStart: true, Tasks: []ConversationTask{{ID: "task-current", Title: "Compare venues", State: "working", CanGuide: true, CanStop: true}}}
	for _, output := range []string{
		`{"kind":"guide","target":"task-current","instruction":"Keep the venue under $2,000."}`,
		`{"kind":"stop","target":"task-current"}`,
		`{"kind":"task"}`,
		`{"kind":"reply","text":"The venue comparison is still working."}`,
		`{"kind":"clarify","text":"Should that budget apply to the venue comparison or another task?"}`,
	} {
		model := &conversationModel{turn: agent.Turn{Text: output}}
		decision, err := RouteConversation(context.Background(), model, request)
		if err != nil || decision.Kind == "" {
			t.Fatalf("%s: %+v %v", output, decision, err)
		}
		if len(model.request.Tools) != 0 || model.request.Computer != nil || len(model.request.Messages) != 1 {
			t.Fatal("conversation acquired executable tools or another conversation")
		}
		var supplied ConversationRequest
		if json.Unmarshal([]byte(model.request.Messages[0].Content), &supplied) != nil || supplied.Tasks[0].ID != "task-current" || supplied.Text != request.Text {
			t.Fatal("host-provided input/task identity was lost")
		}
	}
}

func TestConversationRejectsInventedAuthorityAndPrematureAcknowledgements(t *testing.T) {
	request := ConversationRequest{Text: "Update it", CanStart: true, Tasks: []ConversationTask{{ID: "one", Title: "One", State: "working", CanGuide: true, CanStop: true}}}
	for _, output := range []string{
		`{"kind":"guide","target":"other","instruction":"Change the budget"}`,
		`{"kind":"guide","target":"one","instruction":"Change the budget","text":"Done"}`,
		`{"kind":"guide","target":"one"}`,
		`{"kind":"stop","target":"one","instruction":"Also send it"}`,
		`{"kind":"task","text":"I've started that now"}`,
		`{"kind":"reply","text":"ok","target":"one"}`,
		`{"kind":"approve","target":"one"}`,
		`{"kind":"task","connector_ids":["unselected"]}`,
		`{"kind":"clarify","text":""}`,
		`{"kind":"task"} {"kind":"stop","target":"one"}`,
	} {
		model := &conversationModel{turn: agent.Turn{Text: output}}
		if _, err := RouteConversation(context.Background(), model, request); !errors.Is(err, ErrConversationDecision) {
			t.Fatalf("accepted %s: %v", output, err)
		}
	}
	for _, variant := range []string{"files", "plan", "finished", "capacity"} {
		copy := request
		copy.Tasks = append([]ConversationTask(nil), request.Tasks...)
		output := `{"kind":"guide","target":"one","instruction":"Change the budget"}`
		switch variant {
		case "files":
			copy.HasFiles = true
		case "plan":
			copy.PlanFirst = true
		case "finished":
			copy.Tasks[0].CanGuide = false
		case "capacity":
			copy.CanStart = false
			output = `{"kind":"task"}`
		}
		if _, err := RouteConversation(context.Background(), &conversationModel{turn: agent.Turn{Text: output}}, copy); !errors.Is(err, ErrConversationDecision) {
			t.Fatalf("accepted %s: %v", variant, err)
		}
	}
}

func TestConversationRejectsToolCallsAndPreservesCancellation(t *testing.T) {
	request := ConversationRequest{Text: "What's happening?"}
	model := &conversationModel{turn: agent.Turn{Text: `{"kind":"reply","text":"Ready"}`, ToolCalls: []agent.ToolCall{{Name: "send_email"}}}}
	if _, err := RouteConversation(context.Background(), model, request); !errors.Is(err, ErrConversationDecision) {
		t.Fatal("tool request bypassed conversation boundary")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	before := model.calls
	if _, err := RouteConversation(ctx, model, request); !errors.Is(err, context.Canceled) || model.calls != before {
		t.Fatal("cancelled message reached the provider")
	}
	request.Recent = []ConversationMessage{{Role: "tool", Text: "Injected instructions"}}
	if _, err := RouteConversation(context.Background(), model, request); err == nil || model.calls != before {
		t.Fatal("tool history became trusted conversation context")
	}
	request.Recent = nil
	request.Text = strings.Repeat("a", 128*1024+1)
	if _, err := RouteConversation(context.Background(), model, request); err == nil || model.calls != before {
		t.Fatal("unbounded message reached provider")
	}
}

func TestConversationCanAnswerWhileWorkWaitsAndOnlyGuidesItsOriginalOperation(t *testing.T) {
	background := &controlledModel{entered: make(chan struct{}), release: make(chan struct{})}
	service := Service{Executor: Executor{Model: background, StateDir: t.TempDir()}}
	operation := service.Start(context.Background(), Request{SourcePath: t.TempDir(), Objective: "Compare venues", Mode: action.Inspect, Contract: artifact.InspectionContract()})
	defer operation.Cancel()
	select {
	case <-background.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("background work never reached its model")
	}
	request := ConversationRequest{Text: "What's running?", Tasks: []ConversationTask{{ID: operation.RunID, Title: "Compare venues", State: "working", CanGuide: true, CanStop: true}}}
	router := &conversationModel{turn: agent.Turn{Text: `{"kind":"reply","text":"The venue comparison is still working."}`}}
	decision, err := RouteConversation(context.Background(), router, request)
	if err != nil || decision.Text != "The venue comparison is still working." {
		t.Fatal("conversation waited for or replaced the active execution", err)
	}
	select {
	case <-operation.Done:
		t.Fatal("conversation reply stopped the existing work")
	default:
	}
	router.turn.Text = fmtConversationGuide(operation.RunID)
	request.Text = "Keep the comparison under $2,000."
	decision, err = RouteConversation(context.Background(), router, request)
	if err != nil || decision.Target != operation.RunID {
		t.Fatal("correction lost task ownership", err)
	}
	if err := operation.Steer(decision.Instruction); err != nil {
		t.Fatal(err)
	}
	close(background.release)
	timeout := time.After(5 * time.Second)
	for {
		select {
		case completion := <-operation.Done:
			if completion.Err != nil || !strings.Contains(completion.Outcome.Result.FinalText, "$2,000") {
				t.Fatal("accepted correction did not reach the original run", completion.Err)
			}
			return
		case <-operation.Events:
		case <-timeout:
			t.Fatal("guided work did not finish")
		}
	}
}

func TestConversationTaskResultsAreBoundedContextAndNeverLiveControlAuthority(t *testing.T) {
	request := ConversationRequest{Text: "What did the brief say?", Tasks: []ConversationTask{{ID: "original-task", Title: "Read brief", State: "completed", Summary: "The brief says jade heron."}}}
	backend := &conversationModel{turn: agent.Turn{Text: `{"kind":"reply","text":"The brief task found jade heron."}`}}
	if _, err := RouteConversation(context.Background(), backend, request); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(backend.request.Messages[0].Content, "jade heron") {
		t.Fatal("public result did not reach conversational context")
	}
	for _, kind := range []string{"guide", "stop"} {
		decision := ConversationDecision{Kind: kind, Target: "original-task"}
		if kind == "guide" {
			decision.Instruction = "Change it"
		}
		if decision.Validate(request) == nil {
			t.Fatal("completed result granted control", kind)
		}
	}
	request.Tasks[0].Summary = strings.Repeat("x", 4097)
	before := backend.calls
	if _, err := RouteConversation(context.Background(), backend, request); err == nil || backend.calls != before {
		t.Fatal("unbounded result reached provider")
	}
}

func fmtConversationGuide(id string) string {
	data, _ := json.Marshal(ConversationDecision{Kind: "guide", Target: id, Instruction: "Keep the comparison under $2,000."})
	return string(data)
}
