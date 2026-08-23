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
	MethodApprove      = "approve"
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
	Task             string     `json:"task,omitempty"`
	Provider         string     `json:"provider,omitempty"`
	Model            string     `json:"model,omitempty"`
	BaseURL          string     `json:"base_url,omitempty"`
	Verify           [][]string `json:"verify,omitempty"`
	Scopes           []string   `json:"scopes,omitempty"`
	Profile          string     `json:"profile,omitempty"`
	BaseRef          string     `json:"base_ref,omitempty"`
	CopyIgnoredFiles bool       `json:"copy_ignored_files,omitempty"`
	Scouts           []string   `json:"scouts,omitempty"`
	MaxSteps         int        `json:"max_steps,omitempty"`
	Mode             string     `json:"mode,omitempty"`
	StatePath        string     `json:"state_path,omitempty"`
	ThreadID         string     `json:"thread_id,omitempty"`
	RunID            string     `json:"run_id,omitempty"`
	Message          string     `json:"message,omitempty"`
	Decision         string     `json:"decision,omitempty"`
	All              bool       `json:"all,omitempty"`
	Compact          bool       `json:"compact,omitempty"`
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
// arguments and tool output stay out of the protocol except for command
// approval requests, which must show the argv a parent is asked to allow.
type Event struct {
	Kind      string   `json:"kind"`
	Step      int      `json:"step"`
	Tool      string   `json:"tool,omitempty"`
	Text      string   `json:"text,omitempty"`
	ToolError string   `json:"tool_error,omitempty"`
	Argv      []string `json:"argv,omitempty"`
}

// Error is an actionable request failure. Codes are stable enough for clients
// to branch on, while Message remains written for humans.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ValidID reports whether value is a safe request correlation ID. Keeping the
// rule in the public protocol package lets every transport reject malformed
// IDs before it allocates per-request state or constructs an event URL.
func ValidID(value string) bool {
	if len(value) == 0 || len(value) > 128 {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || character == '.' || character == '_' || character == '-' {
			continue
		}
		return false
	}
	return true
}
