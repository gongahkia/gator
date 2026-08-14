package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/journal"
	"github.com/gongahkia/gator/internal/model/openai"
	gatorrun "github.com/gongahkia/gator/internal/run"
	"github.com/gongahkia/gator/internal/tui"
)

const usage = `Gator — native, inspectable coding agent

Usage:
  gator
  gator tui
  gator help
  gator doctor
  gator run [--model MODEL] [--max-steps N] --verify 'argv ...' TASK
  gator resume [--max-steps N] RUN_RECORD_PATH TASK

Commands:
  tui       open the interactive terminal application (the default command)
  doctor    report local prerequisites and suggested verification commands
  run       propose a tested patch in an isolated Git worktree
  resume    continue a retained worktree from its local run record

Run requires OPENAI_API_KEY. --verify is repeatable; each listed command is
allowed for the run and must pass before Gator accepts completion.`

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "gator:", err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	if len(args) == 0 || args[0] == "tui" {
		return interactive()
	}
	if args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		_, err := fmt.Fprintln(out, usage)
		return err
	}

	switch args[0] {
	case "doctor":
		return doctor(out)
	case "run":
		return runTask(args[1:], out)
	case "resume":
		return resumeTask(args[1:], out)
	default:
		return fmt.Errorf("unknown command %q; run 'gator help'", args[0])
	}
}

func interactive() error {
	workingDirectory, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	repository, err := gitRepositoryRoot(workingDirectory)
	if err != nil {
		return errors.New("interactive mode must start inside a Git checkout; run 'gator doctor' for setup")
	}
	inputInfo, err := os.Stdin.Stat()
	if err != nil {
		return fmt.Errorf("inspect terminal input: %w", err)
	}
	if inputInfo.Mode()&os.ModeCharDevice == 0 {
		return errors.New("interactive mode requires a terminal; use 'gator run' for scripts")
	}
	application := tui.New(tui.Config{
		RepositoryPath: repository,
		Model:          modelFromEnvironment(),
		Verification:   parseSuggestedVerification(suggestedVerificationCommands(repository)),
		APIKey:         os.Getenv("OPENAI_API_KEY"),
		NewExecutor: func(model string) gatorrun.Executor {
			return gatorrun.Executor{Model: openai.Responses{APIKey: os.Getenv("OPENAI_API_KEY"), Model: model}}
		},
	})
	program := tea.NewProgram(application, tea.WithAltScreen())
	_, err = program.Run()
	return err
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

func resumeTask(arguments []string, out io.Writer) error {
	flags := flag.NewFlagSet("resume", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	maxSteps := flags.Int("max-steps", 0, "maximum model turns for this continuation")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if len(flags.Args()) < 2 {
		return errors.New("resume requires a run record path and a continuation task")
	}
	statePath := flags.Arg(0)
	continuation := strings.TrimSpace(strings.Join(flags.Args()[1:], " "))
	if continuation == "" {
		return errors.New("resume task is required")
	}
	session, err := journal.LoadSession(statePath)
	if err != nil {
		return err
	}
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return errors.New("OPENAI_API_KEY is required; run 'gator doctor' to check setup")
	}
	modelName := session.Model
	if modelName == "" {
		modelName = modelFromEnvironment()
	}
	if _, err := fmt.Fprintf(out, "Gator resume\n  model: %s\n  task: %s\n", modelName, continuation); err != nil {
		return err
	}
	printer := eventPrinter{out: out}
	executor := gatorrun.Executor{Model: openai.Responses{APIKey: apiKey, Model: modelName}}
	outcome, err := executor.Resume(context.Background(), session, statePath, continuation, gatorrun.Request{
		MaxSteps: *maxSteps,
		OnEvent:  printer.Print,
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

func doctor(out io.Writer) error {
	workingDirectory, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	repository, err := gitRepositoryRoot(workingDirectory)
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
	suggestionDirectory := workingDirectory
	if gitStatus == "detected" {
		suggestionDirectory = repository
	}
	for _, suggestion := range suggestedVerificationCommands(suggestionDirectory) {
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
	printer := eventPrinter{out: out}
	executor := gatorrun.Executor{Model: openai.Responses{APIKey: apiKey, Model: *modelName}}
	outcome, err := executor.Execute(context.Background(), gatorrun.Request{
		RepositoryPath: workingDirectory,
		Task:           task,
		Model:          *modelName,
		MaxSteps:       *maxSteps,
		Verification:   verification,
		OnEvent:        printer.Print,
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
	info, err := os.Stat(filepath.Join(directory, name))
	return err == nil && !info.IsDir()
}

func gitRepositoryRoot(directory string) (string, error) {
	command := exec.Command("git", "-C", directory, "rev-parse", "--show-toplevel")
	output, err := command.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}
