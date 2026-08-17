package main

import (
	"context"
	"crypto/rand"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/model"
	"github.com/gongahkia/gator/internal/worktree"
)

const delegateUsage = `Usage:
  gator delegate codex login [--device]
  gator delegate codex status
  gator delegate codex run [--model MODEL] --verify 'argv ...' TASK
  gator delegate copilot login [--host URL]
  gator delegate copilot run [--model MODEL] --verify 'argv ...' TASK
  gator delegate claude run [--model MODEL] --verify 'argv ...' TASK
  gator delegate kimi login
  gator delegate kimi run [--model MODEL] --verify 'argv ...' TASK
  gator delegate opencode login --provider PROVIDER [--method METHOD]
  gator delegate opencode status
  gator delegate opencode run [--model PROVIDER/MODEL] --verify 'argv ...' TASK
  gator delegate external run --task TASK --verify 'argv ...' -- COMMAND [ARG ...]

Delegated runtimes use their own installed CLI and credential store. Gator keeps
the worktree and verification boundary, but the delegated CLI owns its agent
tools, sandbox, approvals, session state, and account authentication.

Codex, Copilot, and Kimi login invoke their vendor CLI. OpenCode owns the
provider login it starts. Claude runs in --bare API-key mode; Gator never
offers Claude.ai subscription login.`

func delegate(arguments []string, out io.Writer) error {
	if len(arguments) < 2 {
		return errors.New(delegateUsage)
	}
	runtime, action := arguments[0], arguments[1]
	switch runtime {
	case "codex":
		switch action {
		case "login":
			return delegateCodexLogin(arguments[2:], out)
		case "status":
			if len(arguments) != 2 {
				return errors.New("usage: gator delegate codex status")
			}
			return runDelegateCommand(context.Background(), delegateProgram("codex"), []string{"login", "status"}, "", out, nil)
		case "run":
			return delegateCodexRun(arguments[2:], out)
		}
	case "claude":
		switch action {
		case "login":
			return errors.New("Gator does not offer Claude.ai login. Anthropic requires third-party products to use an API key unless separately approved; set ANTHROPIC_API_KEY or run 'gator connect claude'")
		case "run":
			return delegateClaudeRun(arguments[2:], out)
		}
	case "copilot":
		switch action {
		case "login":
			return delegateCopilotLogin(arguments[2:], out)
		case "run":
			return delegateCopilotRun(arguments[2:], out)
		}
	case "kimi":
		switch action {
		case "login":
			return delegateKimiLogin(arguments[2:], out)
		case "run":
			return delegateKimiRun(arguments[2:], out)
		}
	case "opencode":
		switch action {
		case "login":
			return delegateOpenCodeLogin(arguments[2:], out)
		case "status":
			if len(arguments) != 2 {
				return errors.New("usage: gator delegate opencode status")
			}
			return runDelegateCommand(context.Background(), delegateProgram("opencode"), []string{"providers", "list"}, "", out, nil)
		case "run":
			return delegateOpenCodeRun(arguments[2:], out)
		}
	case "external":
		if action == "run" {
			return delegateExternalRun(arguments[2:], out)
		}
	}
	return errors.New(delegateUsage)
}

func delegateCodexLogin(arguments []string, out io.Writer) error {
	flags := flag.NewFlagSet("delegate codex login", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	device := flags.Bool("device", false, "use Codex device authorization")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if len(flags.Args()) != 0 {
		return errors.New("usage: gator delegate codex login [--device]")
	}
	command := []string{"login"}
	if *device {
		command = append(command, "--device-auth")
	}
	return runDelegateCommand(context.Background(), delegateProgram("codex"), command, "", out, os.Stdin)
}

func delegateCodexRun(arguments []string, out io.Writer) error {
	options, task, err := parseDelegatedRunFlags("delegate codex run", arguments)
	if err != nil {
		return err
	}
	return runDelegatedTask(context.Background(), "codex", task, options, out, func(ctx context.Context, directory string, writer io.Writer) error {
		arguments := []string{"exec", "--sandbox", "workspace-write", "--approve-for-me", "--cd", directory}
		if options.model != "" {
			arguments = append(arguments, "--model", options.model)
		}
		arguments = append(arguments, "--", task)
		return runDelegateCommand(ctx, delegateProgram("codex"), arguments, directory, writer, nil)
	})
}

func delegateClaudeRun(arguments []string, out io.Writer) error {
	options, task, err := parseDelegatedRunFlags("delegate claude run", arguments)
	if err != nil {
		return err
	}
	environment, err := delegatedClaudeEnvironment()
	if err != nil {
		return err
	}
	return runDelegatedTask(context.Background(), "claude", task, options, out, func(ctx context.Context, directory string, writer io.Writer) error {
		arguments := []string{"--bare", "--print", "--permission-mode", "acceptEdits", "--output-format", "text"}
		if options.model != "" {
			arguments = append(arguments, "--model", options.model)
		}
		arguments = append(arguments, "--", task)
		return runDelegateCommand(ctx, delegateProgram("claude"), arguments, directory, writer, nil, environment)
	})
}

func delegatedClaudeEnvironment() ([]string, error) {
	if strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY")) != "" {
		return os.Environ(), nil
	}
	credentials, err := gatorCredentials()
	if err != nil {
		return nil, err
	}
	credential, found, err := credentials.Read(string(model.Anthropic))
	if err != nil {
		return nil, fmt.Errorf("read Gator Anthropic credential: %w", err)
	}
	if found && credential.IsAPIKey() && strings.TrimSpace(credential.Key) != "" {
		return append(os.Environ(), "ANTHROPIC_API_KEY="+credential.Key), nil
	}
	return nil, errors.New("an Anthropic API key is required for delegated Claude Code runs; set ANTHROPIC_API_KEY or run 'gator connect claude'. Gator deliberately does not use Claude.ai OAuth or Claude Code's stored credentials")
}

func delegateCopilotLogin(arguments []string, out io.Writer) error {
	flags := flag.NewFlagSet("delegate copilot login", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	host := flags.String("host", "", "GitHub host URL")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if len(flags.Args()) != 0 {
		return errors.New("usage: gator delegate copilot login [--host URL]")
	}
	command := []string{"login"}
	if value := strings.TrimSpace(*host); value != "" {
		command = append(command, "--host", value)
	}
	return runDelegateCommand(context.Background(), delegateProgram("copilot"), command, "", out, os.Stdin)
}

func delegateCopilotRun(arguments []string, out io.Writer) error {
	options, task, err := parseDelegatedRunFlags("delegate copilot run", arguments)
	if err != nil {
		return err
	}
	return runDelegatedTask(context.Background(), "copilot", task, options, out, func(ctx context.Context, directory string, writer io.Writer) error {
		command := []string{"--prompt", task, "--allow-all-tools", "--no-ask-user"}
		if options.model != "" {
			command = append(command, "--model", options.model)
		}
		return runDelegateCommand(ctx, delegateProgram("copilot"), command, directory, writer, nil)
	})
}

func delegateKimiLogin(arguments []string, out io.Writer) error {
	if len(arguments) != 0 {
		return errors.New("usage: gator delegate kimi login")
	}
	return runDelegateCommand(context.Background(), delegateProgram("kimi"), []string{"login"}, "", out, os.Stdin)
}

func delegateKimiRun(arguments []string, out io.Writer) error {
	options, task, err := parseDelegatedRunFlags("delegate kimi run", arguments)
	if err != nil {
		return err
	}
	return runDelegatedTask(context.Background(), "kimi", task, options, out, func(ctx context.Context, directory string, writer io.Writer) error {
		command := []string{"--auto", "--prompt", task, "--output-format", "text"}
		if options.model != "" {
			command = append(command, "--model", options.model)
		}
		return runDelegateCommand(ctx, delegateProgram("kimi"), command, directory, writer, nil)
	})
}

func delegateOpenCodeLogin(arguments []string, out io.Writer) error {
	flags := flag.NewFlagSet("delegate opencode login", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	provider := flags.String("provider", "", "OpenCode provider ID or name")
	method := flags.String("method", "", "OpenCode login method label")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if len(flags.Args()) != 0 || strings.TrimSpace(*provider) == "" {
		return errors.New("usage: gator delegate opencode login --provider PROVIDER [--method METHOD]")
	}
	command := []string{"providers", "login", "--provider", strings.TrimSpace(*provider)}
	if value := strings.TrimSpace(*method); value != "" {
		command = append(command, "--method", value)
	}
	return runDelegateCommand(context.Background(), delegateProgram("opencode"), command, "", out, os.Stdin)
}

func delegateOpenCodeRun(arguments []string, out io.Writer) error {
	options, task, err := parseDelegatedRunFlags("delegate opencode run", arguments)
	if err != nil {
		return err
	}
	return runDelegatedTask(context.Background(), "opencode", task, options, out, func(ctx context.Context, directory string, writer io.Writer) error {
		command := []string{"run", "--dir", directory, "--auto"}
		if options.model != "" {
			command = append(command, "--model", options.model)
		}
		command = append(command, task)
		return runDelegateCommand(ctx, delegateProgram("opencode"), command, directory, writer, nil)
	})
}

func delegateExternalRun(arguments []string, out io.Writer) error {
	flags := flag.NewFlagSet("delegate external run", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	task := flags.String("task", "", "task supplied to the external harness")
	var verification verificationFlags
	flags.Var(&verification, "verify", "required verification command as a whitespace-separated argv")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if strings.TrimSpace(*task) == "" {
		return errors.New("delegate external run requires --task TASK")
	}
	command := flags.Args()
	if len(command) > 0 && command[0] == "--" {
		command = command[1:]
	}
	if len(command) == 0 {
		return errors.New("delegate external run requires a command after --")
	}
	if len(verification) == 0 {
		return errors.New("at least one --verify command is required for a delegated run")
	}
	options := delegatedRunOptions{verification: verification}
	return runDelegatedTask(context.Background(), "external", *task, options, out, func(ctx context.Context, directory string, writer io.Writer) error {
		resolved := make([]string, len(command))
		for index, value := range command {
			switch value {
			case "{task}":
				resolved[index] = *task
			case "{worktree}":
				resolved[index] = directory
			default:
				resolved[index] = value
			}
		}
		environment := append(os.Environ(), "GATOR_TASK="+*task, "GATOR_WORKTREE="+directory)
		return runDelegateCommand(ctx, resolved[0], resolved[1:], directory, writer, nil, environment)
	})
}

type delegatedRunOptions struct {
	model        string
	verification verificationFlags
}

func parseDelegatedRunFlags(name string, arguments []string) (delegatedRunOptions, string, error) {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	model := flags.String("model", "", "delegated runtime model")
	var verification verificationFlags
	flags.Var(&verification, "verify", "required verification command as a whitespace-separated argv")
	if err := flags.Parse(arguments); err != nil {
		return delegatedRunOptions{}, "", err
	}
	task := strings.TrimSpace(strings.Join(flags.Args(), " "))
	if task == "" {
		return delegatedRunOptions{}, "", errors.New(name + " requires a task")
	}
	if len(verification) == 0 {
		return delegatedRunOptions{}, "", errors.New("at least one --verify command is required for a delegated run")
	}
	return delegatedRunOptions{model: strings.TrimSpace(*model), verification: verification}, task, nil
}

func runDelegatedTask(ctx context.Context, runtime, task string, options delegatedRunOptions, out io.Writer, launch func(context.Context, string, io.Writer) error) error {
	workingDirectory, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	runID, err := newDelegatedRunID(runtime)
	if err != nil {
		return err
	}
	isolation, err := worktree.Create(ctx, workingDirectory, runID)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Delegated runtime: %s\nWorktree: %s\nTask: %s\n\n", runtime, isolation.Path, task); err != nil {
		return err
	}
	if err := launch(ctx, isolation.Path, out); err != nil {
		return fmt.Errorf("%s delegated run failed; inspect retained worktree %s: %w", runtime, isolation.Path, err)
	}
	for _, command := range options.verification {
		if _, err := fmt.Fprintf(out, "\n[verify] %s\n", strings.Join(command, " ")); err != nil {
			return err
		}
		if err := runDelegateCommand(ctx, command[0], command[1:], isolation.Path, out, nil); err != nil {
			return fmt.Errorf("verification failed in retained worktree %s: %w", isolation.Path, err)
		}
	}
	_, err = fmt.Fprintf(out, "\nDelegated run complete. Review worktree: %s\n", isolation.Path)
	return err
}

func runDelegateCommand(ctx context.Context, program string, arguments []string, directory string, out io.Writer, input io.Reader, environment ...[]string) error {
	command := exec.CommandContext(ctx, program, arguments...)
	command.Dir = directory
	command.Stdin = input
	command.Stdout = out
	command.Stderr = out
	if len(environment) > 0 {
		command.Env = environment[0]
	}
	if err := command.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return fmt.Errorf("%s is not installed or is not on PATH", filepath.Base(program))
		}
		return fmt.Errorf("run %s: %w", filepath.Base(program), err)
	}
	return nil
}

func delegateProgram(runtime string) string {
	variable := "GATOR_" + strings.ToUpper(strings.ReplaceAll(runtime, "-", "_")) + "_COMMAND"
	if override := strings.TrimSpace(os.Getenv(variable)); override != "" {
		return override
	}
	return runtime
}

func newDelegatedRunID(runtime string) (string, error) {
	var suffix [5]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return "", fmt.Errorf("generate delegated run id: %w", err)
	}
	return runtime + "-" + time.Now().UTC().Format("20060102-150405") + fmt.Sprintf("-%x", suffix), nil
}
