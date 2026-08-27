package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/extension"
	"github.com/gongahkia/gator/internal/journal"
	"github.com/gongahkia/gator/internal/lsp"
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
	store, err := config.DefaultStore()
	if err != nil {
		return err
	}
	settings, err := store.Load()
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
	loadedUI, err := extensions.UI()
	if err != nil {
		return err
	}
	extensionCommands := make([]tui.ExtensionCommand, 0, len(loadedCommands))
	for _, command := range loadedCommands {
		extensionCommands = append(extensionCommands, tui.ExtensionCommand{Name: command.Name, Description: command.Description, Prompt: command.Prompt})
	}
	extensionUI := make([]tui.ExtensionUIContribution, 0, len(loadedUI))
	for _, contribution := range loadedUI {
		extensionUI = append(extensionUI, tui.ExtensionUIContribution{ID: contribution.ID, Slot: contribution.Slot, Title: contribution.Title, Description: contribution.Description, Prompt: contribution.Prompt})
	}
	stateDir, err := journal.ResolveStateDir(os.Getenv("GATOR_STATE_DIR"))
	if err != nil {
		return err
	}
	baseURL := strings.TrimSpace(os.Getenv("GATOR_BASE_URL"))
	if baseURL == "" {
		baseURL = settings.ProviderEndpoint(provider)
	}
	lspRegistry := lsp.NewRegistry()
	management, err := newTUIManagementBackend(repository, stateDir, store, lspRegistry)
	if err != nil {
		return fmt.Errorf("configure TUI management: %w", err)
	}
	application := tui.New(tui.Config{
		RepositoryPath:    repository,
		Provider:          provider,
		Model:             modelFromProviderName(provider),
		BaseURL:           baseURL,
		StateDir:          stateDir,
		Verification:      parseSuggestedVerification(suggestedVerificationCommands(repository)),
		ResumeStatePath:   options.ResumeStatePath,
		ForkStatePath:     options.ForkStatePath,
		StartInRecent:     options.StartInRecent,
		RecentAll:         options.RecentAll,
		CustomProviders:   settings.CustomProviders,
		ProviderEndpoints: settings.ProviderEndpoints,
		ProviderOptions:   settings.ProviderOptions,
		ModelAliases:      settings.ModelAliases,
		ExtensionCommands: extensionCommands,
		ExtensionUI:       extensionUI,
		Theme:             settings.Theme,
		Execution:         settings.Execution,
		NewExecutor: func(provider, modelName, baseURL string) (gatorrun.Executor, error) {
			return newExecutor(provider, modelName, baseURL)
		},
		LSPRegistry: lspRegistry,
		BeginOAuthLogin: func(provider string) (tui.OAuthLogin, error) {
			return beginTUIOAuthLogin(provider)
		},
		SaveCloudModel:  saveTUICloudModelConfiguration(store, stateDir),
		ModelManagement: newTUIModelManagementBackend(store, stateDir),
		NewConnectCommand: func(provider string) (*exec.Cmd, error) {
			return exec.Command(os.Args[0], "connect", provider), nil
		},
		NewDelegateCommand: func(runtime, task, modelName string, verification [][]string, repository string) (tui.DelegateCommand, error) {
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
			output := &delegateOutputCapture{}
			writer := io.MultiWriter(os.Stdout, output)
			process.Stdout = writer
			process.Stderr = writer
			return tui.DelegateCommand{Process: process, Output: output.String}, nil
		},
		SetTheme:    saveTheme,
		LocalModels: newLocalModelManager(store),
		Management:  management,
		Doctor:      newTUIDoctorBackend(settings.Execution),
	})
	program := tea.NewProgram(application, tea.WithAltScreen(), tea.WithMouseCellMotion())
	final, runErr := program.Run()
	if model, ok := final.(tui.Model); ok {
		model.Close()
	}
	return runErr
}

const delegateOutputLimit = 6 * 1024

type delegateOutputCapture struct {
	mutex     sync.Mutex
	value     []byte
	truncated bool
}

func (capture *delegateOutputCapture) Write(value []byte) (int, error) {
	capture.mutex.Lock()
	defer capture.mutex.Unlock()
	if len(value) >= delegateOutputLimit {
		capture.value = append(capture.value[:0], value[len(value)-delegateOutputLimit:]...)
		capture.truncated = true
		return len(value), nil
	}
	if overflow := len(capture.value) + len(value) - delegateOutputLimit; overflow > 0 {
		copy(capture.value, capture.value[overflow:])
		capture.value = capture.value[:len(capture.value)-overflow]
		capture.truncated = true
	}
	capture.value = append(capture.value, value...)
	return len(value), nil
}

func (capture *delegateOutputCapture) String() string {
	capture.mutex.Lock()
	defer capture.mutex.Unlock()
	if capture.truncated {
		return "… earlier terminal output omitted …\n" + string(capture.value)
	}
	return string(capture.value)
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
