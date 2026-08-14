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
	"github.com/gongahkia/gator/internal/model/openai"
	gatorrun "github.com/gongahkia/gator/internal/run"
)

const usage = `Gator — native, inspectable coding agent

Usage:
  gator help
  gator doctor
  gator run [--model MODEL] [--max-steps N] --verify 'argv ...' TASK

Commands:
  doctor    report local prerequisites and suggested verification commands
  run       propose a tested patch in an isolated Git worktree

Run requires OPENAI_API_KEY. --verify is repeatable; each listed command is
allowed for the run and must pass before Gator accepts completion.`

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "gator:", err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		_, err := fmt.Fprintln(out, usage)
		return err
	}

	switch args[0] {
	case "doctor":
		return doctor(out)
	case "run":
		return runTask(args[1:], out)
	default:
		return fmt.Errorf("unknown command %q; run 'gator help'", args[0])
	}
}

func doctor(out io.Writer) error {
	workingDirectory, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	_, err = os.Stat(".git")
	gitStatus := "not detected"
	if err == nil {
		gitStatus = "detected"
	}
	apiKeyStatus := "missing"
	if os.Getenv("OPENAI_API_KEY") != "" {
		apiKeyStatus = "set"
	}
	if _, err := fmt.Fprintf(out, "Repository: %s\nOpenAI API key: %s\n", gitStatus, apiKeyStatus); err != nil {
		return err
	}
	for _, suggestion := range suggestedVerificationCommands(workingDirectory) {
		if _, err := fmt.Fprintf(out, "Suggested verification: %s\n", suggestion); err != nil {
			return err
		}
	}
	if gitStatus != "detected" {
		_, err := fmt.Fprintln(out, "Run Gator from a Git checkout.")
		return err
	}
	return nil
}

func runTask(arguments []string, out io.Writer) error {
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	modelName := flags.String("model", modelFromEnvironment(), "OpenAI Responses model")
	maxSteps := flags.Int("max-steps", 24, "maximum model turns")
	var verification verificationFlags
	flags.Var(&verification, "verify", "required verification command as a whitespace-separated argv")
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
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return errors.New("OPENAI_API_KEY is required; run 'gator doctor' to check setup")
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	if _, err := fmt.Fprintf(out, "Gator\n  model: %s\n  task: %s\n", *modelName, task); err != nil {
		return err
	}
	executor := gatorrun.Executor{Model: openai.Responses{APIKey: apiKey, Model: *modelName}}
	outcome, err := executor.Execute(context.Background(), gatorrun.Request{
		RepositoryPath: workingDirectory,
		Task:           task,
		MaxSteps:       *maxSteps,
		Verification:   verification,
		OnEvent: func(event agent.Event) {
			printEvent(out, event)
		},
	})
	if outcome.Worktree.Path != "" {
		if _, writeErr := fmt.Fprintf(out, "\nReview worktree: %s\n", outcome.Worktree.Path); writeErr != nil && err == nil {
			err = writeErr
		}
	}
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "\n%s\n", outcome.Result.FinalText)
	return err
}

func printEvent(out io.Writer, event agent.Event) {
	switch event.Kind {
	case agent.EventTurnStarted:
		_, _ = fmt.Fprintf(out, "\n[%02d] thinking\n", event.Step)
	case agent.EventText:
		_, _ = fmt.Fprintf(out, "[%02d] agent: %s\n", event.Step, event.Text)
	case agent.EventToolCalled:
		_, _ = fmt.Fprintf(out, "[%02d] tool → %s\n", event.Step, event.ToolCall.Name)
	case agent.EventToolFinished:
		if event.ToolError == "" {
			_, _ = fmt.Fprintf(out, "[%02d] tool ✓ %s\n", event.Step, event.ToolCall.Name)
		} else {
			_, _ = fmt.Fprintf(out, "[%02d] tool ! %s: %s\n", event.Step, event.ToolCall.Name, event.ToolError)
		}
	case agent.EventCompletionBlocked:
		_, _ = fmt.Fprintf(out, "[%02d] evidence required: %s\n", event.Step, event.Text)
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

func modelFromEnvironment() string {
	if model := os.Getenv("GATOR_MODEL"); model != "" {
		return model
	}
	return openai.DefaultModel()
}

func suggestedVerificationCommands(directory string) []string {
	var suggestions []string
	if fileExists(directory, "go.mod") {
		suggestions = append(suggestions, "go test ./...")
	}
	if fileExists(directory, "package.json") {
		suggestions = append(suggestions, "npm test")
	}
	if fileExists(directory, "pyproject.toml") {
		suggestions = append(suggestions, "pytest")
	}
	if fileExists(directory, "Cargo.toml") {
		suggestions = append(suggestions, "cargo test")
	}
	return suggestions
}

func fileExists(directory, name string) bool {
	info, err := os.Stat(directory + string(os.PathSeparator) + name)
	return err == nil && !info.IsDir()
}
