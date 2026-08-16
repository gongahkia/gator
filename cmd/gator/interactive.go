package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
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
	provider, err := providerFromEnvironment()
	if err != nil {
		return err
	}
	stateDir, err := journal.ResolveStateDir(os.Getenv("GATOR_STATE_DIR"))
	if err != nil {
		return err
	}
	application := tui.New(tui.Config{
		RepositoryPath:  repository,
		Provider:        string(provider),
		Model:           modelFromEnvironment(provider),
		BaseURL:         os.Getenv("GATOR_BASE_URL"),
		StateDir:        stateDir,
		Verification:    parseSuggestedVerification(suggestedVerificationCommands(repository)),
		ResumeStatePath: options.ResumeStatePath,
		StartInRecent:   options.StartInRecent,
		RecentAll:       options.RecentAll,
		NewExecutor: func(provider, modelName, baseURL string) (gatorrun.Executor, error) {
			return newExecutor(provider, modelName, baseURL)
		},
		BeginOAuthLogin: func(provider string) (tui.OAuthLogin, error) {
			return beginTUIOAuthLogin(provider)
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
