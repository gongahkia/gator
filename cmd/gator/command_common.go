package main

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/model"
)

// configuredDefaults is shared by Work and the temporary machine-facing
// transports. It contains no execution or session lifecycle behavior.
func configuredDefaults() (config.Defaults, error) {
	store, err := config.DefaultStore()
	if err != nil {
		return config.Defaults{}, err
	}
	settings, err := store.Load()
	if err != nil {
		return config.Defaults{}, err
	}
	defaults := settings.Defaults
	if defaults.Provider == "" {
		defaults.Provider = string(model.OpenAI)
	}
	return defaults, nil
}

// eventPrinter is the shared terminal rendering of observable Work events.
type eventPrinter struct {
	out           io.Writer
	streamingText bool
}

func (p *eventPrinter) Print(event agent.Event) {
	if event.Kind != agent.EventTextDelta && p.streamingText {
		_, _ = fmt.Fprintln(p.out)
		p.streamingText = false
	}
	switch event.Kind {
	case agent.EventTurnStarted:
		_, _ = fmt.Fprintf(p.out, "\n[%02d] thinking\n", event.Step)
	case agent.EventTextDelta:
		if !p.streamingText {
			_, _ = fmt.Fprintf(p.out, "[%02d] agent: ", event.Step)
			p.streamingText = true
		}
		_, _ = fmt.Fprint(p.out, event.Text)
	case agent.EventText:
		_, _ = fmt.Fprintf(p.out, "[%02d] agent: %s\n", event.Step, event.Text)
	case agent.EventToolCalled:
		_, _ = fmt.Fprintf(p.out, "[%02d] tool → %s\n", event.Step, event.ToolCall.Name)
	case agent.EventToolFinished:
		if event.ToolError == "" {
			_, _ = fmt.Fprintf(p.out, "[%02d] tool ✓ %s\n", event.Step, event.ToolCall.Name)
		} else {
			_, _ = fmt.Fprintf(p.out, "[%02d] tool ! %s: %s\n", event.Step, event.ToolCall.Name, event.ToolError)
		}
	case agent.EventCommandApprovalRequested:
		_, _ = fmt.Fprintf(p.out, "[%02d] command approval: %s\n", event.Step, strings.Join(event.Argv, " "))
	case agent.EventCommandApprovalResolved:
		_, _ = fmt.Fprintf(p.out, "[%02d] command %s\n", event.Step, event.Text)
	case agent.EventHook:
		_, _ = fmt.Fprintf(p.out, "[%02d] hook %s\n", event.Step, event.Text)
	case agent.EventSubagent:
		_, _ = fmt.Fprintf(p.out, "[%02d] subagent %s\n", event.Step, event.Text)
	case agent.EventTerminal:
		_, _ = fmt.Fprintf(p.out, "[%02d] terminal %s\n", event.Step, event.Text)
	case agent.EventWorktreeSetup:
		_, _ = fmt.Fprintf(p.out, "[%02d] worktree setup %s\n", event.Step, event.Text)
	case agent.EventCompletionBlocked:
		_, _ = fmt.Fprintf(p.out, "[%02d] evidence required: %s\n", event.Step, event.Text)
	}
}

type verificationFlags [][]string

func (v *verificationFlags) String() string {
	commands := make([]string, 0, len(*v))
	for _, command := range *v {
		commands = append(commands, strings.Join(command, " "))
	}
	return strings.Join(commands, ", ")
}

func (v *verificationFlags) Set(value string) error {
	argv := strings.Fields(value)
	if len(argv) == 0 {
		return errors.New("verification command must not be empty")
	}
	*v = append(*v, argv)
	return nil
}

type stringFlags []string

func (v *stringFlags) String() string { return strings.Join(*v, ", ") }

func (v *stringFlags) Set(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return errors.New("scope must not be empty")
	}
	*v = append(*v, value)
	return nil
}

func parseSuggestedVerification(commands []string) [][]string {
	result := make([][]string, 0, len(commands))
	for _, command := range commands {
		if argv := strings.Fields(command); len(argv) > 0 {
			result = append(result, argv)
		}
	}
	return result
}

func valueOrDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}
