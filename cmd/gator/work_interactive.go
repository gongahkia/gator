package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/attachment"
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/inbox"
	"github.com/gongahkia/gator/internal/jobs"
	"github.com/gongahkia/gator/internal/journal"
	"github.com/gongahkia/gator/internal/worksession"
	"github.com/gongahkia/gator/internal/worktui"
)

func workInteractive() error {
	return workInteractiveConversation("")
}

func workInteractiveConversation(startConversationID string) error {
	workingDirectory, err := os.Getwd()
	if err != nil {
		return err
	}
	input, err := os.Stdin.Stat()
	if err != nil || input.Mode()&os.ModeCharDevice == 0 {
		return errors.New("interactive Work requires a terminal; use 'gator work' for scripts")
	}
	output, err := os.Stdout.Stat()
	if err != nil || output.Mode()&os.ModeCharDevice == 0 {
		return errors.New("interactive Work requires a terminal; use 'gator work' for scripts")
	}
	stateDir, err := journal.ResolveStateDir(os.Getenv("GATOR_STATE_DIR"))
	if err != nil {
		return err
	}
	sessions, err := worksession.Open(stateDir)
	if err != nil {
		return err
	}
	conversations, err := sessions.List(8)
	if err != nil {
		return err
	}
	if startConversationID != "" {
		found := false
		for _, conversation := range conversations {
			found = found || conversation.ID == startConversationID
		}
		if !found {
			conversation, loadErr := sessions.Load(startConversationID)
			if loadErr != nil {
				return loadErr
			}
			conversations = append([]worksession.Conversation{conversation}, conversations...)
		}
	}
	jobStore, err := jobs.Open(stateDir)
	if err != nil {
		return err
	}
	definitions, err := jobStore.List()
	if err != nil {
		return err
	}
	inboxStore, err := inbox.Open(stateDir)
	if err != nil {
		return err
	}
	inboxEntries, err := inboxStore.List(false, 20)
	if err != nil {
		return err
	}
	settings, err := loadSettings()
	if err != nil {
		return err
	}
	application := worktui.New(worktui.Config{
		CurrentFolder: workingDirectory, Conversations: conversations, Jobs: definitions, Inbox: inboxEntries,
		StartConversationID: startConversationID,
		FirstRun:            settings.Defaults.Provider == "" && len(conversations) == 0,
		SetupCommand: func(provider string) *exec.Cmd {
			command := exec.Command(os.Args[0], "connect", provider)
			command.Stdin = os.Stdin
			command.Stdout = os.Stdout
			command.Stderr = os.Stderr
			return command
		},
		CompleteSetup: selectWorkOnboardingProvider,
		Run: func(source, conversationID, prompt string, options worktui.RunOptions) worktui.RunResult {
			return runInteractiveWork(source, conversationID, prompt, stateDir, options)
		},
		MoveBack: func(conversationID string) (string, error) {
			conversation, err := sessions.Load(conversationID)
			if err != nil {
				return "", err
			}
			revision, err := sessions.LoadRevision(conversationID, conversation.HeadRevision)
			if err != nil {
				return "", err
			}
			if revision.ParentRevisionID == "" {
				return "", errors.New("already at the first revision")
			}
			moved, err := sessions.MoveHead(conversationID, revision.ParentRevisionID)
			if err != nil {
				return "", err
			}
			return "Moved back to " + moved.HeadRevision + ". Your newer branch is still retained.", nil
		},
		MoveForward: func(conversationID string) (string, error) {
			conversation, err := sessions.Load(conversationID)
			if err != nil {
				return "", err
			}
			children, err := sessions.Children(conversationID, conversation.HeadRevision)
			if err != nil {
				return "", err
			}
			if len(children) == 0 {
				return "", errors.New("there is no forward revision")
			}
			if len(children) > 1 {
				var choices []string
				for _, child := range children {
					choices = append(choices, child.ID+" — "+firstLine(child.Objective))
				}
				return "", errors.New("choose a branch from History:\n" + strings.Join(choices, "\n"))
			}
			moved, err := sessions.MoveHead(conversationID, children[0].ID)
			if err != nil {
				return "", err
			}
			return "Moved forward to " + moved.HeadRevision + ".", nil
		},
		MoveToRevision: func(conversationID, revisionID string) (string, error) {
			conversation, err := sessions.Load(conversationID)
			if err != nil {
				return "", err
			}
			children, err := sessions.Children(conversationID, conversation.HeadRevision)
			if err != nil {
				return "", err
			}
			for _, child := range children {
				if child.ID == revisionID {
					moved, moveErr := sessions.MoveHead(conversationID, revisionID)
					if moveErr != nil {
						return "", moveErr
					}
					return "Moved forward to " + moved.HeadRevision + ".", nil
				}
			}
			return "", errors.New("selected revision is not a child of the current head")
		},
		History: func(conversationID string) (string, error) {
			conversation, err := sessions.Load(conversationID)
			if err != nil {
				return "", err
			}
			revisions, err := sessions.Revisions(conversationID)
			if err != nil {
				return "", err
			}
			var lines []string
			for _, revision := range revisions {
				marker := "  "
				if revision.ID == conversation.HeadRevision {
					marker = "→ "
				}
				lines = append(lines, marker+revision.ID+" — "+firstLine(revision.Objective))
			}
			return strings.Join(lines, "\n"), nil
		},
		Inspect:  inspectWorkTUITopic,
		Copy:     clipboard.WriteAll,
		Theme:    settings.Theme,
		SetTheme: saveTheme,
	})
	program := tea.NewProgram(application, tea.WithAltScreen())
	_, err = program.Run()
	return err
}

func selectWorkOnboardingProvider(providerName string) error {
	providerName = strings.ToLower(strings.TrimSpace(providerName))
	if providerName == "claude" {
		providerName = "anthropic"
	}
	provider, modelName, err := resolveConfiguredProvider(providerName, "")
	if err != nil {
		return err
	}
	if strings.TrimSpace(modelName) == "" {
		return fmt.Errorf("provider %q needs an explicit model; configure it from the full model screen", provider)
	}
	store, err := config.DefaultStore()
	if err != nil {
		return err
	}
	settings, err := store.Load()
	if err != nil {
		return err
	}
	settings.Defaults.Provider = provider
	settings.Defaults.Model = modelName
	if err := store.Save(settings); err != nil {
		return fmt.Errorf("save selected Work provider: %w", err)
	}
	return nil
}

func runInteractiveWork(source, conversationID, prompt, stateDir string, options worktui.RunOptions) worktui.RunResult {
	arguments := []string{"work", "--json"}
	if conversationID != "" {
		arguments = append(arguments, "--conversation", conversationID)
	} else {
		arguments = append(arguments, "--source", source)
	}
	if options.MaxSteps > 0 {
		arguments = append(arguments, "--max-steps", fmt.Sprint(options.MaxSteps))
	}
	if options.Code.MaxSteps > 0 {
		arguments = append(arguments, "--code-max-steps", fmt.Sprint(options.Code.MaxSteps))
	}
	for _, path := range options.Attachments {
		flag := "--attach"
		if attachment.IsImage(path) {
			flag = "--image"
		}
		arguments = append(arguments, flag, path)
	}
	for _, value := range options.Code.Verification {
		arguments = append(arguments, "--verify", value)
	}
	for _, value := range options.Code.Scopes {
		arguments = append(arguments, "--scope", value)
	}
	if options.Code.Profile != "" {
		arguments = append(arguments, "--profile", options.Code.Profile)
	}
	for _, value := range options.Code.Setup {
		arguments = append(arguments, "--setup", value)
	}
	for _, value := range options.Code.AllowedCommands {
		arguments = append(arguments, "--allow-command", value)
	}
	for _, value := range options.Code.AllowedCommandPrefixes {
		arguments = append(arguments, "--allow-command-prefix", value)
	}
	arguments = append(arguments, "--sandbox", options.Code.Sandbox, "--network", options.Code.Network)
	for _, value := range options.Code.Capabilities {
		arguments = append(arguments, "--code-capability", value)
	}
	if options.Code.BrowserSession != "" {
		arguments = append(arguments, "--browser-session", options.Code.BrowserSession)
	}
	arguments = append(arguments, "--", prompt)
	command := exec.Command(os.Args[0], arguments...)
	command.Env = append(os.Environ(), "GATOR_STATE_DIR="+stateDir)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	runErr := command.Run()
	var response struct {
		ConversationID string `json:"conversation_id"`
		RevisionID     string `json:"revision_id"`
		SnapshotID     string `json:"snapshot_id"`
		FinalText      string `json:"final_text"`
		OutputPath     string `json:"output_path"`
		Error          string `json:"error"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		return worktui.RunResult{Error: strings.TrimSpace(stderr.String() + " " + stdout.String())}
	}
	if runErr != nil && response.Error == "" {
		response.Error = fmt.Sprintf("%v: %s", runErr, stderr.String())
	}
	return worktui.RunResult{ConversationID: response.ConversationID, RevisionID: response.RevisionID, SnapshotID: response.SnapshotID, FinalText: response.FinalText, OutputPath: response.OutputPath, Error: response.Error}
}

func inspectWorkTUITopic(topic string) (string, error) {
	var output bytes.Buffer
	var err error
	switch topic {
	case "doctor":
		err = doctor(nil, &output)
	case "agents":
		err = agentCommand([]string{"list"}, &output)
	case "settings":
		err = configure([]string{"show"}, &output)
	default:
		err = fmt.Errorf("unknown inspection topic %q", topic)
	}
	return strings.TrimSpace(output.String()), err
}

func unifiedResume(arguments []string, out io.Writer) error {
	if len(arguments) == 0 {
		return workInteractive()
	}
	stateDir, err := journal.ResolveStateDir(os.Getenv("GATOR_STATE_DIR"))
	if err != nil {
		return err
	}
	store, err := worksession.Open(stateDir)
	if err != nil {
		return err
	}
	if _, err := store.Load(arguments[0]); err != nil {
		return fmt.Errorf("load Gator conversation %q: %w", arguments[0], err)
	}
	if len(arguments) == 1 {
		return workInteractiveConversation(arguments[0])
	}
	forwarded := append([]string{"resume", arguments[0]}, arguments[1:]...)
	return workSessionCommand(forwarded, os.Stdin, out, nativeWorkModel)
}
