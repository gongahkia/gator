package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/model"
	gatorrun "github.com/gongahkia/gator/internal/run"
	"github.com/gongahkia/gator/internal/tools"
)

func runTask(arguments []string, out io.Writer) error {
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	defaults, err := configuredDefaults()
	if err != nil {
		return err
	}
	defaultProvider := defaults.Provider
	if configured := os.Getenv("GATOR_PROVIDER"); configured != "" {
		defaultProvider = configured
	}
	providerName := flags.String("provider", defaultProvider, "model provider")
	defaultModel := defaults.Model
	if configured := os.Getenv("GATOR_MODEL"); configured != "" {
		defaultModel = configured
	}
	modelName := flags.String("model", defaultModel, "model name")
	baseURL := flags.String("base-url", os.Getenv("GATOR_BASE_URL"), "provider API base URL override")
	maxSteps := flags.Int("max-steps", 24, "maximum model turns")
	var verification verificationFlags
	flags.Var(&verification, "verify", "required verification command as a whitespace-separated argv")
	var allowedCommands verificationFlags
	flags.Var(&allowedCommands, "allow-command", "pre-approve an exact worktree argv for this run")
	trustCommands := flags.Bool("trust-commands", false, "auto-approve exploratory worktree commands (unsafe; not a sandbox)")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	task := strings.TrimSpace(strings.Join(flags.Args(), " "))
	if task == "" {
		return errors.New("run task is required")
	}
	if len(verification) == 0 {
		return errors.New("at least one --verify command is required; run 'gator doctor' for suggestions")
	}
	resolvedProvider, resolvedModel, err := resolveConfiguredProvider(*providerName, *modelName)
	if err != nil {
		return err
	}
	executor, err := newExecutor(resolvedProvider, resolvedModel, *baseURL)
	if err != nil {
		return err
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	if _, err := fmt.Fprintf(out, "Gator\n  provider: %s\n  model: %s\n  task: %s\n", resolvedProvider, displayModel(resolvedModel), task); err != nil {
		return err
	}
	printer := eventPrinter{out: out}
	outcome, err := executor.Execute(context.Background(), gatorrun.Request{
		RepositoryPath:  workingDirectory,
		Task:            task,
		Provider:        resolvedProvider,
		Model:           resolvedModel,
		BaseURL:         *baseURL,
		MaxSteps:        *maxSteps,
		Verification:    verification,
		AllowedCommands: allowedCommands,
		Approve:         cliCommandApprover(*trustCommands, out),
		OnEvent:         printer.Print,
	})
	if outcome.Worktree.Path != "" {
		if _, writeErr := fmt.Fprintf(out, "\nReview worktree: %s\n", outcome.Worktree.Path); writeErr != nil && err == nil {
			err = writeErr
		}
	}
	if outcome.StatePath != "" {
		if _, writeErr := fmt.Fprintf(out, "Run record: %s\n", outcome.StatePath); writeErr != nil && err == nil {
			err = writeErr
		}
	}
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "\n%s\n", outcome.Result.FinalText)
	return err
}

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
	case agent.EventCompletionBlocked:
		_, _ = fmt.Fprintf(p.out, "[%02d] evidence required: %s\n", event.Step, event.Text)
	}
}

func cliCommandApprover(trust bool, out io.Writer) func(context.Context, []string) (tools.CommandDecision, error) {
	if trust {
		return func(context.Context, []string) (tools.CommandDecision, error) {
			return tools.CommandAllowOnce, nil
		}
	}
	if !stdinIsTTY() {
		return nil
	}
	return func(_ context.Context, argv []string) (tools.CommandDecision, error) {
		_, _ = fmt.Fprintf(os.Stderr, "Gator: approve worktree command %q?\n  y allow once  a always this argv  n deny\n> ", strings.Join(argv, " "))
		var line string
		if _, err := fmt.Fscanln(os.Stdin, &line); err != nil {
			return tools.CommandDeny, err
		}
		switch strings.TrimSpace(strings.ToLower(line)) {
		case "y", "yes":
			return tools.CommandAllowOnce, nil
		case "a", "always":
			return tools.CommandAllowAlways, nil
		default:
			return tools.CommandDeny, nil
		}
	}
}

func stdinIsTTY() bool {
	info, err := os.Stdin.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
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
