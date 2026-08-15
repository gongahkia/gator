// Package agent defines the provider-independent coding-agent runtime.
package agent

import (
	"context"
	"encoding/json"
	"time"
)

// Role identifies the source of a conversation item.
type Role string

const (
	RoleSystem Role = "system"
	RoleUser   Role = "user"
	RoleAgent  Role = "agent"
	RoleTool   Role = "tool"
)

// Message is the normalized conversation history passed to a model adapter.
// Tool messages carry a call ID so adapters can preserve provider correlation.
type Message struct {
	Role         Role            `json:"role"`
	Content      string          `json:"content"`
	ToolCalls    []ToolCall      `json:"tool_calls,omitempty"`
	ToolCallID   string          `json:"tool_call_id,omitempty"`
	ToolName     string          `json:"tool_name,omitempty"`
	ProviderData json.RawMessage `json:"provider_data,omitempty"`
}

// ToolDefinition describes one tool available to the model. Parameters is a
// JSON Schema object, kept raw so the core is not coupled to one provider SDK.
type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// ToolCall is a model request to invoke one tool.
type ToolCall struct {
	ID         string          `json:"id"`
	ProviderID string          `json:"provider_id,omitempty"`
	Name       string          `json:"name"`
	Arguments  json.RawMessage `json:"arguments"`
}

// TurnRequest is one model turn. Implementations must not mutate Messages.
type TurnRequest struct {
	System   string           `json:"system"`
	Messages []Message        `json:"messages"`
	Tools    []ToolDefinition `json:"tools"`
}

// Turn is a completed model response. Text may accompany tool calls; the loop
// records it, invokes calls, then requests another turn.
type Turn struct {
	Text         string          `json:"text"`
	ToolCalls    []ToolCall      `json:"tool_calls"`
	ProviderData json.RawMessage `json:"provider_data,omitempty"`
}

// Model is the sole provider-facing interface used by the runtime.
type Model interface {
	Complete(context.Context, TurnRequest) (Turn, error)
}

// StreamingModel is an optional extension for adapters that can surface
// incremental text while preserving the same completed-turn contract.
type StreamingModel interface {
	CompleteStream(context.Context, TurnRequest, func(string)) (Turn, error)
}

// Tool executes one typed model request. Tools are responsible for parsing and
// validating their own arguments at their boundary.
type Tool interface {
	Definition() ToolDefinition
	Execute(context.Context, json.RawMessage) (ToolResult, error)
}

// ToolResult is serialized to the model as the result of one named call.
type ToolResult struct {
	Content string `json:"content"`
}

// EventKind names a durable, user-visible occurrence within a run.
type EventKind string

const (
	EventTurnStarted       EventKind = "turn_started"
	EventTextDelta         EventKind = "text_delta"
	EventText              EventKind = "text"
	EventToolCalled        EventKind = "tool_called"
	EventToolFinished      EventKind = "tool_finished"
	EventCompletionBlocked EventKind = "completion_blocked"
	EventHarnessStarted    EventKind = "harness_started"
	EventHarnessFinished   EventKind = "harness_finished"
	EventRunFinished       EventKind = "run_finished"
)

// Event is intentionally structured so the UI, journal, and tests observe the
// same behavior. Details is never used for unbounded raw model transcripts.
type Event struct {
	Kind      EventKind `json:"kind"`
	At        time.Time `json:"at"`
	Step      int       `json:"step"`
	ToolCall  *ToolCall `json:"tool_call,omitempty"`
	ToolError string    `json:"tool_error,omitempty"`
	Text      string    `json:"text,omitempty"`
}

// EventSink receives events in their exact execution order.
type EventSink func(Event)

// RunOptions defines one bounded autonomous run.
type RunOptions struct {
	Task            string
	System          string
	InitialMessages []Message
	MaxSteps        int
	OnEvent         EventSink
	// CompletionCheck may require concrete evidence, such as a diff inspection
	// or a named verifier, before a final response is accepted.
	CompletionCheck func([]Message) error
}

// Result is the terminal state of a completed agent loop.
type Result struct {
	FinalText string
	Messages  []Message
	Steps     int
}
