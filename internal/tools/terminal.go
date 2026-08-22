package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/terminal"
)

// TerminalTools exposes the persistent terminal task surface for a single
// Execute-mode run. The manager owns process cleanup; these tools enforce the
// same developer approval policy as exploratory commands.
func TerminalTools(manager *terminal.Manager, policy CommandPolicy) []agent.Tool {
	if manager == nil {
		return nil
	}
	return []agent.Tool{
		terminalStart{manager: manager, policy: policy},
		terminalRead{manager: manager},
		terminalWrite{manager: manager, policy: policy},
		terminalList{manager: manager},
		terminalStop{manager: manager},
	}
}

type terminalStart struct {
	manager *terminal.Manager
	policy  CommandPolicy
}

func (t terminalStart) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{
		Name:        "terminal_start",
		Description: "Start a persistent pseudo-terminal task in the isolated worktree. Provide exactly one of argv (no shell) or command (bash -lc, or sh -c if bash is unavailable). Starting a terminal task follows the strict sandbox/network policy and requires developer approval unless this exact argv was already allowed for the thread. The task is automatically cancelled when the run ends and has bounded lifetime and scrollback. Use terminal_read for output, terminal_write for separately approved input, terminal_list for state, and terminal_stop to cancel it.",
		Parameters:  schema(`{"type":"object","additionalProperties":false,"properties":{"argv":{"type":"array","items":{"type":"string"},"minItems":1},"command":{"type":"string"}}}`),
	}
}

func (t terminalStart) Execute(ctx context.Context, raw json.RawMessage) (agent.ToolResult, error) {
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
	if err := authorizeTerminalStart(ctx, t.policy, argv); err != nil {
		return agent.ToolResult{}, err
	}
	task, err := t.manager.Start(ctx, argv)
	if err != nil {
		return agent.ToolResult{}, err
	}
	encoded, err := success(task)
	if err != nil {
		return agent.ToolResult{}, err
	}
	return agent.ToolResult{Content: encoded}, nil
}

type terminalRead struct{ manager *terminal.Manager }

func (t terminalRead) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{
		Name:        "terminal_read",
		Description: "Read the next bounded increment of a persistent terminal task's output. Pass the prior next cursor to continue without repeating output. The response reports if old output was dropped from bounded scrollback.",
		Parameters:  schema(`{"type":"object","additionalProperties":false,"required":["id"],"properties":{"id":{"type":"string","minLength":1},"cursor":{"type":"integer","minimum":0}}}`),
	}
}

func (t terminalRead) Execute(_ context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	var arguments struct {
		ID     string `json:"id"`
		Cursor int64  `json:"cursor"`
	}
	if err := decodeArguments(raw, &arguments); err != nil {
		return agent.ToolResult{}, err
	}
	result, err := t.manager.Read(strings.TrimSpace(arguments.ID), arguments.Cursor)
	if err != nil {
		return agent.ToolResult{}, err
	}
	encoded, err := success(result)
	if err != nil {
		return agent.ToolResult{}, err
	}
	return agent.ToolResult{Content: encoded}, nil
}

type terminalWrite struct {
	manager *terminal.Manager
	policy  CommandPolicy
}

func (t terminalWrite) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{
		Name:        "terminal_write",
		Description: "Send one bounded UTF-8 input fragment to a running persistent terminal task. Every distinct input requires developer approval; the approval event identifies the task and a digest but never logs the input itself. Include a line terminator when the program needs one.",
		Parameters:  schema(`{"type":"object","additionalProperties":false,"required":["id","input"],"properties":{"id":{"type":"string","minLength":1},"input":{"type":"string","minLength":1,"maxLength":16384}}}`),
	}
}

func (t terminalWrite) Execute(ctx context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	var arguments struct {
		ID    string `json:"id"`
		Input string `json:"input"`
	}
	if err := decodeArguments(raw, &arguments); err != nil {
		return agent.ToolResult{}, err
	}
	id := strings.TrimSpace(arguments.ID)
	if id == "" {
		return agent.ToolResult{}, errors.New("terminal task id is required")
	}
	if arguments.Input == "" {
		return agent.ToolResult{}, errors.New("terminal input is required")
	}
	if err := authorizeTerminalInput(ctx, t.policy, id, []byte(arguments.Input)); err != nil {
		return agent.ToolResult{}, err
	}
	task, err := t.manager.Write(id, []byte(arguments.Input))
	if err != nil {
		return agent.ToolResult{}, err
	}
	encoded, err := success(task)
	if err != nil {
		return agent.ToolResult{}, err
	}
	return agent.ToolResult{Content: encoded}, nil
}

type terminalList struct{ manager *terminal.Manager }

func (t terminalList) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{
		Name:        "terminal_list",
		Description: "List persistent terminal tasks and their current exit state without returning terminal output.",
		Parameters:  schema(`{"type":"object","additionalProperties":false}`),
	}
}

func (t terminalList) Execute(_ context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	var arguments struct{}
	if err := decodeArguments(raw, &arguments); err != nil {
		return agent.ToolResult{}, err
	}
	encoded, err := success(t.manager.List())
	if err != nil {
		return agent.ToolResult{}, err
	}
	return agent.ToolResult{Content: encoded}, nil
}

type terminalStop struct{ manager *terminal.Manager }

func (t terminalStop) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{
		Name:        "terminal_stop",
		Description: "Cancel a persistent terminal task. Stopping is always permitted because it only reduces the task's authority. Use terminal_list or terminal_read afterwards to observe its final state.",
		Parameters:  schema(`{"type":"object","additionalProperties":false,"required":["id"],"properties":{"id":{"type":"string","minLength":1}}}`),
	}
}

func (t terminalStop) Execute(_ context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	var arguments struct {
		ID string `json:"id"`
	}
	if err := decodeArguments(raw, &arguments); err != nil {
		return agent.ToolResult{}, err
	}
	task, err := t.manager.Stop(strings.TrimSpace(arguments.ID))
	if err != nil {
		return agent.ToolResult{}, err
	}
	encoded, err := success(task)
	if err != nil {
		return agent.ToolResult{}, err
	}
	return agent.ToolResult{Content: encoded}, nil
}

func authorizeTerminalStart(ctx context.Context, policy CommandPolicy, argv []string) error {
	if policy.BeforeCommand != nil {
		if err := policy.BeforeCommand(ctx, cloneArgv(argv), false); err != nil {
			return err
		}
	}
	if policy.Remembered.Allows(argv) {
		return nil
	}
	return approveTerminal(ctx, policy, "terminal_start", argv, strings.Join(argv, " "))
}

func authorizeTerminalInput(ctx context.Context, policy CommandPolicy, id string, input []byte) error {
	digest := sha256.Sum256(input)
	argv := []string{"terminal_write", id, hex.EncodeToString(digest[:])}
	if policy.Remembered.Allows(argv) {
		return nil
	}
	return approveTerminal(ctx, policy, "terminal_write", argv, fmt.Sprintf("terminal %s input (%d bytes, sha256:%s)", id, len(input), hex.EncodeToString(digest[:8])))
}

func approveTerminal(ctx context.Context, policy CommandPolicy, toolName string, argv []string, description string) error {
	emitTerminalApproval(policy, toolName, argv, description, "")
	if policy.Approve == nil {
		emitTerminalApproval(policy, toolName, argv, CommandDeny.String(), CommandDeny.String())
		return errors.New("terminal task operation requires developer approval")
	}
	decision, err := policy.Approve(ctx, cloneArgv(argv))
	if err != nil {
		emitTerminalApproval(policy, toolName, argv, CommandDeny.String(), CommandDeny.String())
		return err
	}
	emitTerminalApproval(policy, toolName, argv, decision.String(), decision.String())
	switch decision {
	case CommandAllowOnce:
		return nil
	case CommandAllowAlways:
		policy.Remembered.Remember(argv)
		return nil
	default:
		return fmt.Errorf("terminal task operation %q denied by developer", description)
	}
}

func emitTerminalApproval(policy CommandPolicy, toolName string, argv []string, text, resolved string) {
	if policy.OnEvent == nil {
		return
	}
	kind := agent.EventCommandApprovalRequested
	if resolved != "" {
		kind = agent.EventCommandApprovalResolved
	}
	policy.OnEvent(agent.Event{Kind: kind, ToolCall: &agent.ToolCall{Name: toolName}, Text: text, Argv: cloneArgv(argv)})
}
