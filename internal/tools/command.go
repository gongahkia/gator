package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"sync"
	"time"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/workspace"
)

const (
	defaultCommandTimeout = 2 * time.Minute
	defaultCommandOutput  = 64 * 1024
)

// CommandPolicy is deliberately allowlist-only. A model cannot turn the
// command tool into arbitrary shell execution by altering its arguments.
type CommandPolicy struct {
	Allowed        [][]string
	Timeout        time.Duration
	MaxOutputBytes int
}

// RunCommand invokes an exact argv entry allowed by policy. It never invokes a
// shell and returns nonzero exits as structured results for model recovery.
type RunCommand struct {
	Root   workspace.Root
	Policy CommandPolicy
}

func (t RunCommand) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{
		Name:        "run_command",
		Description: "Run one verification command allowed by the current policy. The argv must exactly match an advertised command.",
		Parameters:  schema(`{"type":"object","additionalProperties":false,"required":["argv"],"properties":{"argv":{"type":"array","items":{"type":"string"},"minItems":1}}}`),
	}
}

func (t RunCommand) Execute(ctx context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	var arguments struct {
		Argv []string `json:"argv"`
	}
	if err := decodeArguments(raw, &arguments); err != nil {
		return agent.ToolResult{}, err
	}
	if len(arguments.Argv) == 0 || arguments.Argv[0] == "" {
		return agent.ToolResult{}, errors.New("command argv is required")
	}
	if !t.allowed(arguments.Argv) {
		return agent.ToolResult{}, fmt.Errorf("command %q is not allowed by policy", arguments.Argv)
	}
	timeout := t.Policy.Timeout
	if timeout <= 0 {
		timeout = defaultCommandTimeout
	}
	commandContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	output := &limitedBuffer{limit: positiveOr(t.Policy.MaxOutputBytes, defaultCommandOutput)}
	command := exec.CommandContext(commandContext, arguments.Argv[0], arguments.Argv[1:]...)
	command.Dir = t.Root.Path()
	command.Stdout = output
	command.Stderr = output
	err := command.Run()
	exitCode := 0
	if err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			exitCode = exitError.ExitCode()
		} else if errors.Is(commandContext.Err(), context.DeadlineExceeded) {
			exitCode = -1
		} else {
			return agent.ToolResult{}, fmt.Errorf("start command: %w", err)
		}
	}
	result, encodeErr := success(struct {
		Argv      []string `json:"argv"`
		ExitCode  int      `json:"exit_code"`
		Output    string   `json:"output"`
		Truncated bool     `json:"truncated"`
		TimedOut  bool     `json:"timed_out"`
	}{
		Argv:      arguments.Argv,
		ExitCode:  exitCode,
		Output:    output.String(),
		Truncated: output.truncated,
		TimedOut:  errors.Is(commandContext.Err(), context.DeadlineExceeded),
	})
	if encodeErr != nil {
		return agent.ToolResult{}, encodeErr
	}
	return agent.ToolResult{Content: result}, nil
}

func (t RunCommand) allowed(argv []string) bool {
	for _, allowed := range t.Policy.Allowed {
		if len(allowed) != len(argv) {
			continue
		}
		matches := true
		for index := range argv {
			if argv[index] != allowed[index] {
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
