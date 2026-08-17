package run

import (
	"time"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/worktree"
)

// Request configures a single coding task. Every listed verification argv is
// both exposed to the agent and required to succeed before completion.
type Request struct {
	RepositoryPath string
	Task           string
	Provider       string
	Model          string
	BaseURL        string
	RunID          string
	MaxSteps       int
	Verification   [][]string
	System         string
	StateDir       string
	OnEvent        agent.EventSink
	// Steering is available only to native model runs. It is deliberately
	// transient: queued prompts remain a TUI concern and do not resume after a
	// process restart.
	Steering    <-chan string
	ThreadID    string
	BaseCommit  string
	ForkedFrom  string
	Images      []agent.Image
	Attachments []agent.Attachment
	Mode        Mode
}

// Mode controls the native agent tool surface for a turn. Plan mode is
// intentionally enforced by omitting mutation and command tools.
type Mode uint8

const (
	ExecuteMode Mode = iota
	PlanMode
)

func (m Mode) String() string {
	switch m {
	case ExecuteMode:
		return "execute"
	case PlanMode:
		return "plan"
	default:
		return "unknown"
	}
}

// Outcome preserves the reviewable artifacts of a completed or failed run.
type Outcome struct {
	Worktree  worktree.Worktree
	StatePath string
	ThreadID  string
	Result    agent.Result
	Events    []agent.Event
}

// Executor combines the provider-independent loop with an isolated worktree.
type Executor struct {
	Model    agent.Model
	Now      func() time.Time
	StateDir string
}
