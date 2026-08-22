package run

import (
	"context"
	"time"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/extension"
	"github.com/gongahkia/gator/internal/hooks"
	"github.com/gongahkia/gator/internal/lsp"
	"github.com/gongahkia/gator/internal/mcp"
	"github.com/gongahkia/gator/internal/sandbox"
	"github.com/gongahkia/gator/internal/tools"
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
	// Scopes identify repository-relative files or directories the task is
	// focused on. They select directory guidance and declarative project rules.
	Scopes          []string
	Profile         string
	AllowedCommands [][]string
	Approve         func(context.Context, []string) (tools.CommandDecision, error)
	System          string
	StateDir        string
	OnEvent         agent.EventSink
	// Steering is available only to native model runs. It is deliberately
	// transient: queued prompts remain a TUI concern and do not resume after a
	// process restart.
	Steering   <-chan string
	ThreadID   string
	BaseCommit string
	// BaseRef selects the initial Git revision for a new run. It is resolved to
	// BaseCommit before the worktree is created and is not reused on resume.
	BaseRef          string
	CopyIgnoredFiles bool
	// Scouts are bounded read-only subagent assignments. Each runs in its own
	// detached worktree before the primary writer receives their evidence.
	Scouts     []string
	ForkedFrom string
	// ForceCompaction requests a model-generated summary of older retained
	// messages before this turn. It is meaningful for resume and fork flows.
	ForceCompaction bool
	// DisableWriterDelegation is an internal recursion guard for a child writer
	// run. It is not a developer-facing capability: only the primary native
	// agent may create a writer worktree.
	DisableWriterDelegation bool
	Images                  []agent.Image
	Attachments             []agent.Attachment
	Mode                    Mode
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
	Worktree       worktree.Worktree
	StatePath      string
	ThreadID       string
	Result         agent.Result
	Events         []agent.Event
	ScoutWorktrees []worktree.Worktree
}

// Executor combines the provider-independent loop with an isolated worktree.
type Executor struct {
	Model      agent.Model
	Now        func() time.Time
	StateDir   string
	Extensions extension.Resolver
	HookTrusts []hooks.Trust
	LSPTrusts  []lsp.Trust
	MCPTrusts  []mcp.Trust
	// MCPCredentials is the private Gator credential store used only for an
	// exact configured Streamable HTTP MCP resource. A zero store leaves remote
	// servers unauthenticated rather than reading another application's state.
	MCPCredentials auth.Store
	Sandbox        sandbox.Policy
}
