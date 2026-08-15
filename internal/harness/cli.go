// Package harness delegates an isolated worktree to a vendor-supported coding
// CLI that is already authenticated on the developer's machine.
package harness

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gongahkia/gator/internal/agent"
)

const maxOutputBytes = 2 * 1024 * 1024

// Provider identifies a vendor CLI that can use its own supported login flow.
type Provider string

const (
	Codex   Provider = "codex"
	Claude  Provider = "claude"
	Copilot Provider = "copilot"
	Cursor  Provider = "cursor"
)

// Request is one delegated agent run inside Gator's already-isolated worktree.
// A CLI harness owns its own model/tool loop; Gator owns worktree creation,
// final verification, journaling, and review presentation.
type Request struct {
	Root     string
	Task     string
	System   string
	Model    string
	MaxSteps int
	OnEvent  agent.EventSink
}

// Result is the vendor CLI's final report. Output is bounded and kept only in
// memory; Gator's event journal deliberately never persists it.
type Result struct {
	FinalText string
	Output    string
}

// Runner executes a delegated harness.
type Runner interface {
	Run(context.Context, Request) (Result, error)
}

// CLI invokes a locally installed vendor command. CommandPath and Execute are
// injectable for deterministic tests; normal use leaves both zero-valued.
type CLI struct {
	Provider    Provider
	CommandPath string
	Execute     ExecuteFunc
}

// ExecuteFunc runs one process and forwards complete stdout/stderr lines while
// retaining at most maxOutputBytes from each stream.
type ExecuteFunc func(context.Context, string, []string, string, func(string, bool)) (processResult, error)

type processResult struct {
	Stdout string
	Stderr string
	Code   int
}

// New returns a CLI harness for a supported provider. It only validates that
// the binary exists; authentication stays entirely inside the vendor CLI.
func New(provider Provider) (CLI, error) {
	if !supported(provider) {
		return CLI{}, fmt.Errorf("unsupported CLI harness provider %q", provider)
	}
	path, err := exec.LookPath(binary(provider))
	if err != nil {
		return CLI{}, fmt.Errorf("%s CLI is not installed or not on PATH", provider)
	}
	return CLI{Provider: provider, CommandPath: path}, nil
}

// Run executes the selected CLI in the Gator-created worktree. It never reads
// another tool's auth files or passes their credentials to a network endpoint.
func (c CLI) Run(ctx context.Context, request Request) (Result, error) {
	if !supported(c.Provider) {
		return Result{}, fmt.Errorf("unsupported CLI harness provider %q", c.Provider)
	}
	if strings.TrimSpace(request.Root) == "" {
		return Result{}, errors.New("CLI harness worktree root is required")
	}
	if strings.TrimSpace(request.Task) == "" {
		return Result{}, errors.New("CLI harness task is required")
	}
	path := c.CommandPath
	if path == "" {
		var err error
		path, err = exec.LookPath(binary(c.Provider))
		if err != nil {
			return Result{}, fmt.Errorf("%s CLI is not installed or not on PATH", c.Provider)
		}
	}
	arguments, cleanup, err := c.arguments(request)
	if err != nil {
		return Result{}, err
	}
	defer cleanup()
	emit(request.OnEvent, agent.Event{Kind: agent.EventHarnessStarted, Step: 1, Text: string(c.Provider)})
	run := c.Execute
	if run == nil {
		run = execute
	}
	result, err := run(ctx, path, arguments, request.Root, func(line string, stderr bool) {
		if text := streamedText(c.Provider, line, stderr); text != "" {
			emit(request.OnEvent, agent.Event{Kind: agent.EventTextDelta, Step: 1, Text: text})
		}
	})
	output := result.Stdout
	if result.Stderr != "" {
		if output != "" {
			output += "\n"
		}
		output += result.Stderr
	}
	if err != nil {
		emit(request.OnEvent, agent.Event{Kind: agent.EventHarnessFinished, Step: 1, ToolError: err.Error()})
		return Result{Output: output}, fmt.Errorf("%s CLI: %w", c.Provider, err)
	}
	if result.Code != 0 {
		detail := compactOutput(output)
		if detail == "" {
			detail = "no diagnostic output"
		}
		err := fmt.Errorf("%s CLI exited with status %d: %s", c.Provider, result.Code, detail)
		emit(request.OnEvent, agent.Event{Kind: agent.EventHarnessFinished, Step: 1, ToolError: err.Error()})
		return Result{Output: output}, err
	}
	final := finalText(c.Provider, result.Stdout)
	if final == "" {
		final = "Delegated " + string(c.Provider) + " CLI run completed."
	}
	emit(request.OnEvent, agent.Event{Kind: agent.EventHarnessFinished, Step: 1, Text: string(c.Provider)})
	return Result{FinalText: final, Output: output}, nil
}

func (c CLI) arguments(request Request) ([]string, func(), error) {
	prompt := prompt(request)
	model := strings.TrimSpace(request.Model)
	switch c.Provider {
	case Codex:
		arguments := []string{"exec", "--ephemeral", "--json", "--color", "never", "--sandbox", "workspace-write", "--approve-for-me", "-C", request.Root}
		if model != "" {
			arguments = append(arguments, "--model", model)
		}
		arguments = append(arguments, prompt)
		return arguments, func() {}, nil
	case Claude:
		arguments := []string{"-p", "--bare", "--no-session-persistence", "--output-format", "stream-json", "--verbose", "--include-partial-messages", "--permission-mode", "auto", "--max-turns", fmt.Sprint(maxSteps(request.MaxSteps))}
		if model != "" {
			arguments = append(arguments, "--model", model)
		}
		arguments = append(arguments, prompt)
		return arguments, func() {}, nil
	case Copilot:
		arguments := []string{"--prompt", prompt, "--stream", "off", "--silent", "--allow-all-tools", "--deny-tool=shell(git push)", "--disable-builtin-mcps", "--disallow-temp-dir", "--no-custom-instructions"}
		if model != "" {
			arguments = append(arguments, "--model", model)
		}
		return arguments, func() {}, nil
	case Cursor:
		arguments := []string{"--print", "--force", "--output-format", "stream-json"}
		if model != "" {
			arguments = append(arguments, "--model", model)
		}
		arguments = append(arguments, prompt)
		return arguments, func() {}, nil
	default:
		return nil, nil, fmt.Errorf("unsupported CLI harness provider %q", c.Provider)
	}
}

func prompt(request Request) string {
	var builder strings.Builder
	builder.WriteString("You are working as a delegated coding agent inside a Gator-created isolated Git worktree. Implement the developer task below. Work only inside the current worktree. Do not commit, push, change Git remotes, modify global configuration, or access credentials. Inspect the repository before editing and leave a reviewable patch. Gator runs the required verification commands after you finish.\n\nDeveloper task:\n")
	builder.WriteString(request.Task)
	if strings.TrimSpace(request.System) != "" {
		builder.WriteString("\n\nProject instructions:\n")
		builder.WriteString(request.System)
	}
	return builder.String()
}

func maxSteps(value int) int {
	if value > 0 {
		return value
	}
	return 24
}

func supported(provider Provider) bool {
	switch provider {
	case Codex, Claude, Copilot, Cursor:
		return true
	default:
		return false
	}
}

func binary(provider Provider) string {
	switch provider {
	case Cursor:
		return "cursor-agent"
	default:
		return string(provider)
	}
}

func execute(ctx context.Context, command string, arguments []string, directory string, onLine func(string, bool)) (processResult, error) {
	process := exec.CommandContext(ctx, command, arguments...)
	process.Dir = directory
	stdout, err := process.StdoutPipe()
	if err != nil {
		return processResult{}, fmt.Errorf("open CLI stdout: %w", err)
	}
	stderr, err := process.StderrPipe()
	if err != nil {
		return processResult{}, fmt.Errorf("open CLI stderr: %w", err)
	}
	if err := process.Start(); err != nil {
		return processResult{}, fmt.Errorf("start CLI: %w", err)
	}
	var stdoutBuffer, stderrBuffer boundedBuffer
	var group sync.WaitGroup
	var scanErr error
	var scanErrMu sync.Mutex
	read := func(reader io.Reader, destination *boundedBuffer, isStderr bool) {
		defer group.Done()
		scanner := bufio.NewScanner(reader)
		scanner.Buffer(make([]byte, 64*1024), maxOutputBytes)
		for scanner.Scan() {
			line := scanner.Text()
			destination.WriteString(line + "\n")
			onLine(line, isStderr)
		}
		if err := scanner.Err(); err != nil {
			scanErrMu.Lock()
			if scanErr == nil {
				scanErr = err
			}
			scanErrMu.Unlock()
		}
	}
	group.Add(2)
	go read(stdout, &stdoutBuffer, false)
	go read(stderr, &stderrBuffer, true)
	err = process.Wait()
	group.Wait()
	result := processResult{Stdout: stdoutBuffer.String(), Stderr: stderrBuffer.String()}
	if scanErr != nil {
		return result, fmt.Errorf("read CLI output: %w", scanErr)
	}
	if err == nil {
		return result, nil
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		result.Code = exitError.ExitCode()
		return result, nil
	}
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	return result, err
}

type boundedBuffer struct {
	mu        sync.Mutex
	buffer    bytes.Buffer
	truncated bool
}

func (b *boundedBuffer) WriteString(value string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	remaining := maxOutputBytes - b.buffer.Len()
	if remaining <= 0 {
		b.truncated = true
		return
	}
	if len(value) > remaining {
		b.buffer.WriteString(value[:remaining])
		b.truncated = true
		return
	}
	b.buffer.WriteString(value)
}

func (b *boundedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	value := b.buffer.String()
	if b.truncated {
		value += "\n[CLI output truncated]"
	}
	return value
}

func streamedText(provider Provider, line string, stderr bool) string {
	if stderr {
		return strings.TrimSpace(line)
	}
	var event map[string]json.RawMessage
	if json.Unmarshal([]byte(line), &event) != nil {
		return strings.TrimSpace(line)
	}
	if provider == Claude {
		var envelope struct {
			Type  string `json:"type"`
			Event struct {
				Delta struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"delta"`
			} `json:"event"`
		}
		if json.Unmarshal([]byte(line), &envelope) == nil && envelope.Type == "stream_event" && envelope.Event.Delta.Type == "text_delta" {
			return envelope.Event.Delta.Text
		}
	}
	if provider == Cursor {
		var envelope struct {
			Type    string `json:"type"`
			Message struct {
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal([]byte(line), &envelope) == nil && envelope.Type == "assistant" {
			var text strings.Builder
			for _, content := range envelope.Message.Content {
				if content.Type == "text" {
					text.WriteString(content.Text)
				}
			}
			return text.String()
		}
	}
	return ""
}

func finalText(provider Provider, output string) string {
	var final string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var event any
		if json.Unmarshal([]byte(line), &event) == nil {
			if text := findText(event, provider); text != "" {
				final = text
			}
			continue
		}
		final = line
	}
	return final
}

func findText(value any, provider Provider) string {
	object, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	if provider == Claude {
		if result, ok := object["result"].(string); ok {
			return result
		}
	}
	if item, ok := object["item"].(map[string]any); ok {
		if itemType, _ := item["type"].(string); strings.Contains(itemType, "message") {
			if text, ok := item["text"].(string); ok {
				return text
			}
			if text, ok := item["content"].(string); ok {
				return text
			}
		}
	}
	for _, key := range []string{"result", "text", "content", "message"} {
		if text, ok := object[key].(string); ok {
			return text
		}
	}
	return ""
}

func compactOutput(output string) string {
	output = strings.Join(strings.Fields(output), " ")
	if len(output) <= 500 {
		return output
	}
	return output[:499] + "…"
}

func emit(sink agent.EventSink, event agent.Event) {
	if sink == nil {
		return
	}
	if event.At.IsZero() {
		event.At = time.Now()
	}
	sink(event)
}

// RelativePath returns a path in the worktree for diagnostics without leaking
// the current user's home directory from a vendor CLI error.
func RelativePath(root, path string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil || strings.HasPrefix(relative, "..") {
		return filepath.Base(path)
	}
	return relative
}
