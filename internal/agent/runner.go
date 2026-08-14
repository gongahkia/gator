package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const defaultMaxSteps = 24

// Runner executes a bounded tool-use loop. It deliberately runs tool calls in
// response order: this makes filesystem effects, events, and replay behavior
// deterministic until we have a policy for safe parallelism.
type Runner struct {
	Model Model
	Tools []Tool
	Now   func() time.Time
}

// Run performs a task until the model produces a final text response or the
// configured turn limit is reached.
func (r Runner) Run(ctx context.Context, options RunOptions) (Result, error) {
	if r.Model == nil {
		return Result{}, errors.New("agent model is required")
	}
	if strings.TrimSpace(options.Task) == "" {
		return Result{}, errors.New("agent task is required")
	}

	maxSteps := options.MaxSteps
	if maxSteps == 0 {
		maxSteps = defaultMaxSteps
	}
	if maxSteps < 1 {
		return Result{}, fmt.Errorf("max steps must be positive, got %d", maxSteps)
	}

	now := r.Now
	if now == nil {
		now = time.Now
	}
	tools, definitions, err := indexTools(r.Tools)
	if err != nil {
		return Result{}, err
	}
	messages := []Message{{Role: RoleUser, Content: options.Task}}

	for step := 1; step <= maxSteps; step++ {
		r.emit(options.OnEvent, Event{Kind: EventTurnStarted, At: now(), Step: step})
		turn, err := r.Model.Complete(ctx, TurnRequest{
			System:   options.System,
			Messages: append([]Message(nil), messages...),
			Tools:    definitions,
		})
		if err != nil {
			return Result{Messages: messages, Steps: step}, fmt.Errorf("model turn %d: %w", step, err)
		}

		if turn.Text != "" {
			messages = append(messages, Message{Role: RoleAgent, Content: turn.Text})
			r.emit(options.OnEvent, Event{Kind: EventText, At: now(), Step: step, Text: turn.Text})
		}
		if len(turn.ToolCalls) == 0 {
			if strings.TrimSpace(turn.Text) == "" {
				return Result{Messages: messages, Steps: step}, fmt.Errorf("model turn %d returned neither text nor tool calls", step)
			}
			r.emit(options.OnEvent, Event{Kind: EventRunFinished, At: now(), Step: step, Text: turn.Text})
			return Result{FinalText: turn.Text, Messages: messages, Steps: step}, nil
		}

		for _, call := range turn.ToolCalls {
			r.emit(options.OnEvent, Event{Kind: EventToolCalled, At: now(), Step: step, ToolCall: cloneCall(call)})
			result, toolErr := executeTool(ctx, tools, call)
			content := result.Content
			if toolErr != nil {
				content = encodeToolFailure(toolErr)
			}
			messages = append(messages, Message{
				Role:       RoleTool,
				Content:    content,
				ToolCallID: call.ID,
				ToolName:   call.Name,
			})
			event := Event{Kind: EventToolFinished, At: now(), Step: step, ToolCall: cloneCall(call)}
			if toolErr != nil {
				event.ToolError = toolErr.Error()
			}
			r.emit(options.OnEvent, event)
		}
	}

	return Result{Messages: messages, Steps: maxSteps}, fmt.Errorf("agent stopped after reaching the %d-step limit", maxSteps)
}

func indexTools(tools []Tool) (map[string]Tool, []ToolDefinition, error) {
	indexed := make(map[string]Tool, len(tools))
	definitions := make([]ToolDefinition, 0, len(tools))
	for _, tool := range tools {
		if tool == nil {
			return nil, nil, errors.New("agent tool must not be nil")
		}
		definition := tool.Definition()
		if strings.TrimSpace(definition.Name) == "" {
			return nil, nil, errors.New("agent tool name is required")
		}
		if _, exists := indexed[definition.Name]; exists {
			return nil, nil, fmt.Errorf("duplicate agent tool %q", definition.Name)
		}
		indexed[definition.Name] = tool
		definitions = append(definitions, definition)
	}
	return indexed, definitions, nil
}

func executeTool(ctx context.Context, tools map[string]Tool, call ToolCall) (ToolResult, error) {
	if strings.TrimSpace(call.ID) == "" {
		return ToolResult{}, errors.New("tool call id is required")
	}
	tool, ok := tools[call.Name]
	if !ok {
		return ToolResult{}, fmt.Errorf("tool %q is not available", call.Name)
	}
	if !json.Valid(call.Arguments) {
		return ToolResult{}, fmt.Errorf("tool %q received invalid JSON arguments", call.Name)
	}
	result, err := tool.Execute(ctx, call.Arguments)
	if err != nil {
		return ToolResult{}, fmt.Errorf("tool %q: %w", call.Name, err)
	}
	return result, nil
}

func encodeToolFailure(err error) string {
	payload, marshalErr := json.Marshal(struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}{OK: false, Error: err.Error()})
	if marshalErr != nil {
		return `{"ok":false,"error":"tool execution failed"}`
	}
	return string(payload)
}

func cloneCall(call ToolCall) *ToolCall {
	clone := call
	clone.Arguments = append(json.RawMessage(nil), call.Arguments...)
	return &clone
}

func (r Runner) emit(sink EventSink, event Event) {
	if sink != nil {
		sink(event)
	}
}
