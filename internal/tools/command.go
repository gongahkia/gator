package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/sandbox"
	"github.com/gongahkia/gator/internal/workspace"
)

const (
	defaultCommandTimeout = 2 * time.Minute
	defaultCommandOutput  = 64 * 1024
)

// CommandDecision is the developer's choice for one exploratory command.
type CommandDecision int

const (
	CommandDeny CommandDecision = iota
	CommandAllowOnce
	CommandAllowAlways
)

func (d CommandDecision) String() string {
	switch d {
	case CommandAllowOnce:
		return "allow_once"
	case CommandAllowAlways:
		return "allow_always"
	default:
		return "deny"
	}
}

// CommandMemory is the thread-scoped always-allow list for exact argv.
type CommandMemory struct {
	mu       sync.Mutex
	commands [][]string
}

// NewCommandMemory copies initial argv entries into a shared allowlist.
func NewCommandMemory(initial [][]string) *CommandMemory {
	memory := &CommandMemory{}
	for _, argv := range initial {
		memory.Remember(argv)
	}
	return memory
}

// Allows reports whether argv was previously always-allowed.
func (m *CommandMemory) Allows(argv []string) bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return allowed(m.commands, argv)
}

// Remember records argv for the rest of this thread. Duplicates are ignored.
func (m *CommandMemory) Remember(argv []string) {
	if m == nil || len(argv) == 0 || argv[0] == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if allowed(m.commands, argv) {
		return
	}
	m.commands = append(m.commands, cloneArgv(argv))
}

// Snapshot returns a copy of remembered argv lists.
func (m *CommandMemory) Snapshot() [][]string {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return cloneArgvList(m.commands)
}

// CommandPolicy authorizes verification argv automatically and exploratory
// commands only after an explicit developer decision. The process also follows
// the explicit sandbox policy; strict isolation is the default and sandbox-off
// host access requires a deliberate run-level capability grant.
type CommandPolicy struct {
	Allowed        [][]string
	Prefixes       [][]string
	Remembered     *CommandMemory
	Approve        func(context.Context, []string) (CommandDecision, error)
	OnEvent        agent.EventSink
	Timeout        time.Duration
	MaxOutputBytes int
	Sandbox        sandbox.Policy
	BeforeCommand  func(context.Context, []string, bool) error
}

// RunCommand runs argv without a shell, or a command string via bash/sh, from
// the isolated worktree.
type RunCommand struct {
	Root   workspace.Root
	Policy CommandPolicy
}

// CommandResult is a bounded execution record returned to the model tool.
type CommandResult struct {
	Argv      []string `json:"argv"`
	ExitCode  int      `json:"exit_code"`
	Output    string   `json:"output"`
	Truncated bool     `json:"truncated"`
	TimedOut  bool     `json:"timed_out"`
}

func (t RunCommand) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{
		Name:        "run_command",
		Description: "Run a process in the isolated worktree. Provide exactly one of argv (executed without a shell) or command (run with bash -lc, or sh -c if bash is unavailable). The process follows the run's sandbox policy: strict isolation and denied network are the defaults; sandbox-off host access is an explicit developer capability grant. Required verification argv runs immediately. Any other command waits for developer approval. Required verification commands must still succeed before you complete.",
		Parameters:  schema(`{"type":"object","additionalProperties":false,"properties":{"argv":{"type":"array","items":{"type":"string"},"minItems":1},"command":{"type":"string"}}}`),
	}
}

func (t RunCommand) Execute(ctx context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	var arguments struct {
		Argv    []string `json:"argv"`
		Command string   `json:"command"`
	}
	if err := decodeArguments(raw, &arguments); err != nil {
		return agent.ToolResult{}, err
	}
	argv, err := resolveCommandArgv(arguments.Argv, arguments.Command)
	if err != nil {
		return agent.ToolResult{}, err
	}
	autoAllowed := t.autoAllowed(argv)
	if t.Policy.BeforeCommand != nil {
		if err := t.Policy.BeforeCommand(ctx, cloneArgv(argv), autoAllowed); err != nil {
			return agent.ToolResult{}, err
		}
	}
	if !autoAllowed {
		if err := t.approve(ctx, argv); err != nil {
			return agent.ToolResult{}, err
		}
	}
	result, err := runWorktreeCommand(ctx, t.Root, t.Policy, argv)
	if err != nil {
		return agent.ToolResult{}, err
	}
	encoded, encodeErr := success(result)
	if encodeErr != nil {
		return agent.ToolResult{}, encodeErr
	}
	return agent.ToolResult{Content: encoded}, nil
}

func (t RunCommand) autoAllowed(argv []string) bool {
	return allowed(t.Policy.Allowed, argv) || t.Policy.Remembered.Allows(argv) || PrefixAllows(t.Policy.Prefixes, argv)
}

func (t RunCommand) approve(ctx context.Context, argv []string) error {
	t.emit(agent.Event{
		Kind:     agent.EventCommandApprovalRequested,
		ToolCall: &agent.ToolCall{Name: "run_command"},
		Text:     strings.Join(argv, " "),
		Argv:     cloneArgv(argv),
	})
	if t.Policy.Approve == nil {
		t.emitResolved(argv, CommandDeny)
		return errors.New("command is not allowed by policy; exploratory commands require developer approval")
	}
	decision, err := t.Policy.Approve(ctx, argv)
	if err != nil {
		t.emitResolved(argv, CommandDeny)
		return err
	}
	t.emitResolved(argv, decision)
	switch decision {
	case CommandAllowOnce:
		return nil
	case CommandAllowAlways:
		t.Policy.Remembered.Remember(argv)
		return nil
	default:
		return fmt.Errorf("command %q denied by developer", strings.Join(argv, " "))
	}
}

func (t RunCommand) emit(event agent.Event) {
	if t.Policy.OnEvent != nil {
		t.Policy.OnEvent(event)
	}
}

func (t RunCommand) emitResolved(argv []string, decision CommandDecision) {
	t.emit(agent.Event{
		Kind:     agent.EventCommandApprovalResolved,
		ToolCall: &agent.ToolCall{Name: "run_command"},
		Text:     decision.String(),
		Argv:     cloneArgv(argv),
	})
}

func resolveCommandArgv(argv []string, command string) ([]string, error) {
	hasArgv := len(argv) > 0
	hasCommand := strings.TrimSpace(command) != ""
	if hasArgv && hasCommand {
		return nil, errors.New("provide argv or command, not both")
	}
	if hasCommand {
		return shellArgv(command)
	}
	if !hasArgv || argv[0] == "" {
		return nil, errors.New("command argv or command string is required")
	}
	return cloneArgv(argv), nil
}

func shellArgv(command string) ([]string, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return nil, errors.New("command string is required")
	}
	if bash, err := exec.LookPath("bash"); err == nil {
		return []string{bash, "-lc", command}, nil
	}
	if sh, err := exec.LookPath("sh"); err == nil {
		return []string{sh, "-c", command}, nil
	}
	return nil, errors.New("neither bash nor sh is available")
}

// runWorktreeCommand executes argv with cwd set to the worktree. It never
// invokes a shell itself; callers that need a shell must pass bash/sh argv.
func runWorktreeCommand(ctx context.Context, root workspace.Root, policy CommandPolicy, argv []string) (CommandResult, error) {
	if len(argv) == 0 || argv[0] == "" {
		return CommandResult{}, errors.New("command argv is required")
	}
	timeout := policy.Timeout
	if timeout <= 0 {
		timeout = defaultCommandTimeout
	}
	commandContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	output := &limitedBuffer{limit: positiveOr(policy.MaxOutputBytes, defaultCommandOutput)}
	prepared, err := sandbox.Prepare(commandContext, sandbox.Request{Dir: root.Path(), Argv: argv, Policy: policy.Sandbox})
	if err != nil {
		return CommandResult{}, fmt.Errorf("prepare sandboxed command: %w", err)
	}
	defer prepared.Cleanup()
	command := prepared.Command
	command.Stdout = output
	command.Stderr = output
	err = command.Run()
	exitCode := 0
	if err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			exitCode = exitError.ExitCode()
		} else if errors.Is(commandContext.Err(), context.DeadlineExceeded) {
			exitCode = -1
		} else {
			return CommandResult{}, fmt.Errorf("start command: %w", err)
		}
	}
	return CommandResult{Argv: cloneArgv(argv), ExitCode: exitCode, Output: output.String(), Truncated: output.truncated, TimedOut: errors.Is(commandContext.Err(), context.DeadlineExceeded)}, nil
}

// RunDeveloperSetup executes an argv that the developer supplied directly as
// run setup, rather than an argv suggested by a model. It still applies the
// exact worktree sandbox, environment, timeout, and output bounds in policy;
// callers must not expose it to model or untrusted-client input because it
// deliberately bypasses the exploratory-command approval callback.
func RunDeveloperSetup(ctx context.Context, root workspace.Root, policy CommandPolicy, argv []string) (CommandResult, error) {
	return runWorktreeCommand(ctx, root, policy, argv)
}

func allowed(allowlist [][]string, argv []string) bool {
	for _, permitted := range allowlist {
		if len(permitted) != len(argv) {
			continue
		}
		matches := true
		for index := range argv {
			if permitted[index] != argv[index] {
				matches = false
				break
			}
		}
		if matches {
			return true
		}
	}
	return false
}

// MergeArgvLists concatenates argv lists and drops exact duplicates.
func MergeArgvLists(lists ...[][]string) [][]string {
	var merged [][]string
	for _, list := range lists {
		for _, argv := range list {
			if len(argv) == 0 || argv[0] == "" || allowed(merged, argv) {
				continue
			}
			merged = append(merged, cloneArgv(argv))
		}
	}
	return merged
}

func cloneArgv(argv []string) []string {
	return append([]string(nil), argv...)
}

func cloneArgvList(list [][]string) [][]string {
	if len(list) == 0 {
		return nil
	}
	cloned := make([][]string, 0, len(list))
	for _, argv := range list {
		cloned = append(cloned, cloneArgv(argv))
	}
	return cloned
}

type limitedBuffer struct {
	mu        sync.Mutex
	buffer    bytes.Buffer
	limit     int
	truncated bool
}

func (b *limitedBuffer) Write(value []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	remaining := b.limit - b.buffer.Len()
	if remaining <= 0 {
		b.truncated = true
		return len(value), nil
	}
	if len(value) > remaining {
		_, _ = b.buffer.Write(value[:remaining])
		b.truncated = true
		return len(value), nil
	}
	_, _ = b.buffer.Write(value)
	return len(value), nil
}

func (b *limitedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}
