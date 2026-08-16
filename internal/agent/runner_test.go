package agent

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestRunnerCompletesToolUseLoop(t *testing.T) {
	model := &scriptedModel{turns: []Turn{
		{Text: "I will inspect the project.", ToolCalls: []ToolCall{{ID: "call-1", Name: "read_file", Arguments: json.RawMessage(`{"path":"README.md"}`)}}},
		{Text: "The feature is ready for review."},
	}}
	tool := &recordingTool{result: ToolResult{Content: `{"ok":true,"content":"# Demo"}`}}
	var events []Event
	runner := Runner{
		Model: model,
		Tools: []Tool{tool},
		Now:   fixedClock(),
	}

	result, err := runner.Run(context.Background(), RunOptions{
		Task:     "Add the demo feature",
		MaxSteps: 4,
		OnEvent:  func(event Event) { events = append(events, event) },
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.FinalText != "The feature is ready for review." {
		t.Fatalf("final text = %q", result.FinalText)
	}
	if result.Steps != 2 {
		t.Fatalf("steps = %d, want 2", result.Steps)
	}
	if got := tool.arguments; string(got) != `{"path":"README.md"}` {
		t.Fatalf("tool arguments = %s", got)
	}
	if got := eventKinds(events); !reflect.DeepEqual(got, []EventKind{
		EventTurnStarted, EventText, EventToolCalled, EventToolFinished,
		EventTurnStarted, EventText, EventRunFinished,
	}) {
		t.Fatalf("event kinds = %v", got)
	}
	if got := model.requests[1].Messages[len(model.requests[1].Messages)-1]; got.Role != RoleTool || got.ToolCallID != "call-1" {
		t.Fatalf("second turn did not include correlated tool result: %#v", got)
	}
}

func TestRunnerLetsModelRecoverFromToolFailure(t *testing.T) {
	model := &scriptedModel{turns: []Turn{
		{ToolCalls: []ToolCall{{ID: "call-1", Name: "read_file", Arguments: json.RawMessage(`{}`)}}},
		{Text: "That path was invalid; I need developer input."},
	}}
	tool := &recordingTool{err: errors.New("path must be inside the worktree")}
	var events []Event
	runner := Runner{Model: model, Tools: []Tool{tool}, Now: fixedClock()}

	result, err := runner.Run(context.Background(), RunOptions{
		Task:     "Read the secret file",
		MaxSteps: 2,
		OnEvent:  func(event Event) { events = append(events, event) },
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.FinalText == "" {
		t.Fatal("run returned no final text")
	}
	if got := model.requests[1].Messages[len(model.requests[1].Messages)-1].Content; got != `{"ok":false,"error":"tool \"read_file\": path must be inside the worktree"}` {
		t.Fatalf("tool failure message = %q", got)
	}
	if events[2].ToolError != "tool \"read_file\": path must be inside the worktree" {
		t.Fatalf("tool error event = %q", events[2].ToolError)
	}
}

func TestRunnerRejectsInvalidRunConfiguration(t *testing.T) {
	tests := []struct {
		name    string
		runner  Runner
		options RunOptions
		want    string
	}{
		{name: "missing model", options: RunOptions{Task: "feature"}, want: "model is required"},
		{name: "missing task", runner: Runner{Model: &scriptedModel{}}, want: "task is required"},
		{name: "invalid step limit", runner: Runner{Model: &scriptedModel{}}, options: RunOptions{Task: "feature", MaxSteps: -1}, want: "max steps"},
		{name: "duplicate tool", runner: Runner{Model: &scriptedModel{}, Tools: []Tool{&recordingTool{}, &recordingTool{}}}, options: RunOptions{Task: "feature"}, want: "duplicate"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := test.runner.Run(context.Background(), test.options)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("run error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestRunnerStopsAtStepLimit(t *testing.T) {
	model := &scriptedModel{turns: []Turn{{ToolCalls: []ToolCall{{ID: "call-1", Name: "read_file", Arguments: json.RawMessage(`{}`)}}}}}
	runner := Runner{Model: model, Tools: []Tool{&recordingTool{}}, Now: fixedClock()}

	result, err := runner.Run(context.Background(), RunOptions{Task: "feature", MaxSteps: 1})
	if err == nil || !strings.Contains(err.Error(), "step limit") {
		t.Fatalf("run error = %v, want step-limit error", err)
	}
	if result.Steps != 1 {
		t.Fatalf("steps = %d, want 1", result.Steps)
	}
}

func TestRunnerRequiresCompletionEvidence(t *testing.T) {
	model := &scriptedModel{turns: []Turn{
		{Text: "The feature is done."},
		{ToolCalls: []ToolCall{{ID: "call-1", Name: "read_file", Arguments: json.RawMessage(`{}`)}}},
		{Text: "The feature is done and reviewed."},
	}}
	var events []Event
	runner := Runner{Model: model, Tools: []Tool{&recordingTool{}}, Now: fixedClock()}

	result, err := runner.Run(context.Background(), RunOptions{
		Task:     "implement a feature",
		MaxSteps: 3,
		OnEvent:  func(event Event) { events = append(events, event) },
		CompletionCheck: func(messages []Message) error {
			for _, message := range messages {
				if message.Role == RoleTool && message.ToolName == "read_file" {
					return nil
				}
			}
			return errors.New("inspect the changed files")
		},
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.Steps != 3 || result.FinalText != "The feature is done and reviewed." {
		t.Fatalf("result = %#v", result)
	}
	if events[2].Kind != EventCompletionBlocked || !strings.Contains(events[2].Text, "inspect the changed files") {
		t.Fatalf("completion-block event = %#v", events[2])
	}
}

func TestRunnerUsesProvidedHistory(t *testing.T) {
	model := &scriptedModel{turns: []Turn{{Text: "Resumed and complete."}}}
	runner := Runner{Model: model, Now: fixedClock()}
	history := []Message{{Role: RoleUser, Content: "Original task"}, {Role: RoleTool, ToolCallID: "call-1", ToolName: "read_file", Content: `{"ok":true}`}, {Role: RoleUser, Content: "Continue"}}

	result, err := runner.Run(context.Background(), RunOptions{Task: "Continue", InitialMessages: history, MaxSteps: 1})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := model.requests[0].Messages; !reflect.DeepEqual(got, history) {
		t.Fatalf("history = %#v, want %#v", got, history)
	}
	if result.Messages[0].Content != "Original task" {
		t.Fatalf("result messages = %#v", result.Messages)
	}
}

func TestRunnerAddsPendingSteeringBeforeTheNextModelTurn(t *testing.T) {
	steering := make(chan string, 2)
	steering <- "Focus on the failing verification first."
	steering <- "Do not change public APIs."
	model := &scriptedModel{turns: []Turn{{Text: "Understood."}}}
	var events []Event
	runner := Runner{Model: model, Now: fixedClock()}

	result, err := runner.Run(context.Background(), RunOptions{
		Task:     "Implement the feature",
		MaxSteps: 1,
		Steering: steering,
		OnEvent:  func(event Event) { events = append(events, event) },
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.FinalText != "Understood." {
		t.Fatalf("result = %#v", result)
	}
	messages := model.requests[0].Messages
	if len(messages) != 3 || messages[1].Role != RoleUser || !strings.Contains(messages[1].Content, "failing verification") || !strings.Contains(messages[2].Content, "public APIs") {
		t.Fatalf("request messages = %#v", messages)
	}
	if got := eventKinds(events); !reflect.DeepEqual(got, []EventKind{EventSteeringApplied, EventTurnStarted, EventText, EventRunFinished}) {
		t.Fatalf("event kinds = %v", got)
	}
}

func TestRunnerSupersedesACompletedResponseWhenSteeredDuringModelGeneration(t *testing.T) {
	steering := make(chan string, 1)
	model := &scriptedModel{
		turns: []Turn{{Text: "Obsolete response."}, {Text: "Adjusted response."}},
		onComplete: func(call int) {
			if call == 1 {
				steering <- "Prefer the safer approach."
			}
		},
	}
	var events []Event
	runner := Runner{Model: model, Now: fixedClock()}

	result, err := runner.Run(context.Background(), RunOptions{Task: "Implement the feature", MaxSteps: 2, Steering: steering, OnEvent: func(event Event) { events = append(events, event) }})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.FinalText != "Adjusted response." {
		t.Fatalf("result = %#v", result)
	}
	if got := eventKinds(events); !reflect.DeepEqual(got, []EventKind{EventTurnStarted, EventSteeringApplied, EventTurnStarted, EventText, EventRunFinished}) {
		t.Fatalf("event kinds = %v", got)
	}
	messages := model.requests[1].Messages
	if len(messages) != 2 || messages[0].Role != RoleUser || messages[1].Role != RoleUser || !strings.Contains(messages[1].Content, "safer approach") {
		t.Fatalf("second-turn messages = %#v", messages)
	}
}

func TestRunnerSkipsPendingToolCallsBeforeApplyingSteering(t *testing.T) {
	steering := make(chan string, 1)
	model := &scriptedModel{
		turns: []Turn{
			{ToolCalls: []ToolCall{
				{ID: "call-1", Name: "read_file", Arguments: json.RawMessage(`{}`)},
				{ID: "call-2", Name: "read_file", Arguments: json.RawMessage(`{}`)},
			}},
			{Text: "Adjusted plan complete."},
		},
	}
	tool := &recordingTool{onExecute: func(call int) {
		if call == 1 {
			steering <- "Do not read more files; summarize the safer next step."
		}
	}}
	var events []Event
	runner := Runner{Model: model, Tools: []Tool{tool}, Now: fixedClock()}

	result, err := runner.Run(context.Background(), RunOptions{Task: "Implement the feature", MaxSteps: 2, Steering: steering, OnEvent: func(event Event) { events = append(events, event) }})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.FinalText != "Adjusted plan complete." || tool.calls != 1 {
		t.Fatalf("result = %#v, tool calls = %d", result, tool.calls)
	}
	if got := eventKinds(events); !reflect.DeepEqual(got, []EventKind{EventTurnStarted, EventToolCalled, EventToolFinished, EventToolFinished, EventSteeringApplied, EventTurnStarted, EventText, EventRunFinished}) {
		t.Fatalf("event kinds = %v", got)
	}
	messages := model.requests[1].Messages
	if len(messages) != 5 || messages[1].Role != RoleAgent || messages[2].Role != RoleTool || messages[3].Role != RoleTool || messages[4].Role != RoleUser || !strings.Contains(messages[3].Content, "skipped after developer steering") || !strings.Contains(messages[4].Content, "Do not read more files") {
		t.Fatalf("second-turn messages = %#v", messages)
	}
}

func TestRunnerForwardsStreamingTextWithoutDuplicateFinalEvent(t *testing.T) {
	model := streamingModel{turn: Turn{Text: "hello world"}, deltas: []string{"hello ", "world"}}
	var events []Event
	runner := Runner{Model: model, Now: fixedClock()}

	result, err := runner.Run(context.Background(), RunOptions{Task: "say hello", MaxSteps: 1, OnEvent: func(event Event) { events = append(events, event) }})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.FinalText != "hello world" {
		t.Fatalf("result = %#v", result)
	}
	if got := eventKinds(events); !reflect.DeepEqual(got, []EventKind{EventTurnStarted, EventTextDelta, EventTextDelta, EventRunFinished}) {
		t.Fatalf("event kinds = %v", got)
	}
}

type scriptedModel struct {
	turns      []Turn
	requests   []TurnRequest
	onComplete func(int)
}

func (m *scriptedModel) Complete(_ context.Context, request TurnRequest) (Turn, error) {
	m.requests = append(m.requests, request)
	if len(m.turns) == 0 {
		return Turn{}, errors.New("unexpected model call")
	}
	turn := m.turns[0]
	m.turns = m.turns[1:]
	if m.onComplete != nil {
		m.onComplete(len(m.requests))
	}
	return turn, nil
}

type recordingTool struct {
	arguments json.RawMessage
	result    ToolResult
	err       error
	calls     int
	onExecute func(int)
}

type streamingModel struct {
	turn   Turn
	deltas []string
}

func (m streamingModel) Complete(_ context.Context, _ TurnRequest) (Turn, error) {
	return Turn{}, errors.New("non-streaming completion should not be called")
}

func (m streamingModel) CompleteStream(_ context.Context, _ TurnRequest, onDelta func(string)) (Turn, error) {
	for _, delta := range m.deltas {
		onDelta(delta)
	}
	return m.turn, nil
}

func (t *recordingTool) Definition() ToolDefinition {
	return ToolDefinition{Name: "read_file", Description: "read a file", Parameters: json.RawMessage(`{"type":"object"}`)}
}

func (t *recordingTool) Execute(_ context.Context, arguments json.RawMessage) (ToolResult, error) {
	t.calls++
	t.arguments = append(t.arguments[:0], arguments...)
	if t.onExecute != nil {
		t.onExecute(t.calls)
	}
	return t.result, t.err
}

func fixedClock() func() time.Time {
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	return func() time.Time { return now }
}

func eventKinds(events []Event) []EventKind {
	kinds := make([]EventKind, 0, len(events))
	for _, event := range events {
		kinds = append(kinds, event.Kind)
	}
	return kinds
}
