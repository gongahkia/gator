// Package rpc defines Gator's versioned JSONL automation protocol.
package rpc

import "time"

// Version changes only for incompatible wire-format changes.
const Version = 1

const (
	MethodCapabilities = "capabilities"
	MethodRun          = "run"
	MethodResume       = "resume"
	MethodSteer        = "steer"
	MethodCancel       = "cancel"
	MethodStatus       = "status"
	MethodThreads      = "threads"
)

// Request is one newline-delimited command sent to gator rpc.
type Request struct {
	Version int    `json:"version"`
	ID      string `json:"id"`
	Method  string `json:"method"`
	Params  Params `json:"params,omitempty"`
}

// Params contains the union of supported method arguments. Fields unrelated
// to a method are ignored, allowing clients to use one typed request object.
type Params struct {
	Task      string     `json:"task,omitempty"`
	Provider  string     `json:"provider,omitempty"`
	Model     string     `json:"model,omitempty"`
	BaseURL   string     `json:"base_url,omitempty"`
	Verify    [][]string `json:"verify,omitempty"`
	MaxSteps  int        `json:"max_steps,omitempty"`
	Mode      string     `json:"mode,omitempty"`
	StatePath string     `json:"state_path,omitempty"`
	ThreadID  string     `json:"thread_id,omitempty"`
	RunID     string     `json:"run_id,omitempty"`
	Message   string     `json:"message,omitempty"`
	All       bool       `json:"all,omitempty"`
}

// Message is one newline-delimited server response, event, or error.
type Message struct {
	Version int       `json:"version"`
	Type    string    `json:"type"`
	ID      string    `json:"id,omitempty"`
	Event   *Event    `json:"event,omitempty"`
	Result  any       `json:"result,omitempty"`
	Error   *Error    `json:"error,omitempty"`
	At      time.Time `json:"at"`
}

// Event is the safe, stable projection of a native agent event. Tool
// arguments and tool output stay out of the protocol to avoid accidentally
// exposing source or credential material to a parent process.
type Event struct {
	Kind      string `json:"kind"`
	Step      int    `json:"step"`
	Tool      string `json:"tool,omitempty"`
	Text      string `json:"text,omitempty"`
	ToolError string `json:"tool_error,omitempty"`
}

// Error is an actionable request failure. Codes are stable enough for clients
// to branch on, while Message remains written for humans.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
