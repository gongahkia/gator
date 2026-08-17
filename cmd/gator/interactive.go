package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/extension"
	"github.com/gongahkia/gator/internal/journal"
	gatorrun "github.com/gongahkia/gator/internal/run"
	"github.com/gongahkia/gator/internal/tui"
)

func interactive() error {
	return interactiveWithOptions(interactiveOptions{})
}

type interactiveOptions struct {
	RepositoryPath    string
	ResumeStatePath   string
	ForkStatePath     string
	StartInRecent     bool
	RecentAll         bool
	AllowNoRepository bool
}

func interactiveWithOptions(options interactiveOptions) error {
	repository := options.RepositoryPath
	workingDirectory, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	if repository == "" {
		repository, err = gitRepositoryRoot(workingDirectory)
		if err != nil {
			if !options.AllowNoRepository {
				return errors.New("interactive mode must start inside a Git checkout; run 'gator doctor' for setup")
			}
			repository = workingDirectory
		}
	}
	inputInfo, err := os.Stdin.Stat()
	if err != nil {
		return fmt.Errorf("inspect terminal input: %w", err)
	}
	if inputInfo.Mode()&os.ModeCharDevice == 0 {
		return errors.New("interactive mode requires a terminal; use 'gator run' for scripts")
	}
	outputInfo, err := os.Stdout.Stat()
	if err != nil {
		return fmt.Errorf("inspect terminal output: %w", err)
	}
	if outputInfo.Mode()&os.ModeCharDevice == 0 {
		return errors.New("interactive mode requires a terminal; use 'gator run' for scripts")
	}
	terminal, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return errors.New("interactive mode requires a controlling terminal; use 'gator run' for scripts")
	}
	if err := terminal.Close(); err != nil {
		return fmt.Errorf("close terminal check: %w", err)
	}
	provider, err := providerNameFromEnvironment()
	if err != nil {
		return err
	}
	settings, err := loadSettings()
	if err != nil {
		return err
	}
	extensionResolver, err := extension.DefaultResolver(settings)
	if err != nil {
		return err
	}
	extensions, err := extensionResolver.Load(repository)
	if err != nil {
		return fmt.Errorf("load extensions: %w", err)
	}
	loadedCommands, err := extensions.Commands()
	if err != nil {
		return err
	}
	extensionCommands := make([]tui.ExtensionCommand, 0, len(loadedCommands))
	for _, command := range loadedCommands {
		extensionCommands = append(extensionCommands, tui.ExtensionCommand{Name: command.Name, Description: command.Description, Prompt: command.Prompt})
	}
	stateDir, err := journal.ResolveStateDir(os.Getenv("GATOR_STATE_DIR"))
	if err != nil {
		return err
	}
	application := tui.New(tui.Config{
		RepositoryPath:    repository,
		Provider:          provider,
		Model:             modelFromProviderName(provider),
		BaseURL:           os.Getenv("GATOR_BASE_URL"),
		StateDir:          stateDir,
		Verification:      parseSuggestedVerification(suggestedVerificationCommands(repository)),
		ResumeStatePath:   options.ResumeStatePath,
		ForkStatePath:     options.ForkStatePath,
		StartInRecent:     options.StartInRecent,
		RecentAll:         options.RecentAll,
		CustomProviders:   settings.CustomProviders,
		ExtensionCommands: extensionCommands,
		Theme:             settings.Theme,
		NewExecutor: func(provider, modelName, baseURL string) (gatorrun.Executor, error) {
			return newExecutor(provider, modelName, baseURL)
		},
		BeginOAuthLogin: func(provider string) (tui.OAuthLogin, error) {
			return beginTUIOAuthLogin(provider)
		},
		NewConnectCommand: func(provider string) (*exec.Cmd, error) {
			return exec.Command(os.Args[0], "connect", provider), nil
		},
		NewDelegateCommand: func(runtime, task, modelName string, verification [][]string, repository string) (*exec.Cmd, error) {
			arguments := []string{"delegate", runtime, "run"}
			if strings.TrimSpace(modelName) != "" {
				arguments = append(arguments, "--model", modelName)
			}
			for _, command := range verification {
				arguments = append(arguments, "--verify", strings.Join(command, " "))
			}
			arguments = append(arguments, "--", task)
			process := exec.Command(os.Args[0], arguments...)
			process.Dir = repository
			return process, nil
		},
		SetTheme: saveTheme,
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
