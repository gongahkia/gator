package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/sandbox"
	"github.com/gongahkia/gator/internal/workrun"
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
	"github.com/gongahkia/gator/internal/model"
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
	settingsStore, err := config.DefaultStore()
	if err != nil {
		return err
	}
	localModels := newLocalModelManager(settingsStore)
	defer localModels.Close()
	application := worktui.New(worktui.Config{
		Models: workModelPanel(settingsStore, stateDir, localModels),
		SelectedModel: func() (string, string, error) {
			current, err := settingsStore.Load()
			return current.Defaults.Provider, current.Defaults.Model, err
		},
		Live:          true,
		CurrentFolder: workingDirectory, Conversations: conversations, Jobs: definitions, Inbox: inboxEntries,
		StartConversationID: startConversationID,
		FirstRun:            settings.Defaults.Provider == "" && len(conversations) == 0,
		ProviderCommand:     workTUIProviderCommand,
		ProviderChoices:     workTUIProviderChoices,
		CompleteSetup:       selectWorkOnboardingProvider,
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
	final, err := program.Run()
	if application, ok := final.(worktui.Model); ok {
		application.Close()
	}
	return err
}

func workTUIProviderCommand(action, provider string) *exec.Cmd {
	arguments := []string{action, provider}
	if action == "setup" {
		arguments[0] = "connect"
	} else if action == "login" {
		parsed, err := model.ParseProvider(provider)
		if err == nil && model.SupportsAPIKeyLogin(parsed) {
			arguments = append(arguments, "--prompt")
		}
	}
	command := exec.Command(os.Args[0], arguments...)
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	return command
}

func workTUIProviderChoices(action string) []string {
	if action == "setup" {
		return []string{"openai", "anthropic", "gemini"}
	}
	choices := make([]string, 0, len(model.Names()))
	for _, name := range model.Names() {
		provider, err := model.ParseProvider(name)
		if err != nil {
			continue
		}
		include := false
		switch action {
		case "connect":
			include = model.SupportsAPIKeyLogin(provider) && model.APIKeyEnvironment(provider) != ""
			switch provider {
			case model.Codex, model.Claude, model.Copilot, model.KimiCoding, model.XAI, model.OpenRouter, model.Radius:
				include = true
			}
		case "login":
			include = provider != model.Claude && (model.RequiresOAuthLogin(provider) || model.SupportsOAuthLogin(provider) || model.SupportsAPIKeyLogin(provider))
		case "logout":
			include = true
		}
		if include {
			choices = append(choices, name)
		}
	}
	return choices
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
	ctx := options.Context
	if ctx == nil {
		ctx = context.Background()
	}
	request := workrun.Request{SourcePath: source, ConversationID: conversationID, Objective: prompt, MaxSteps: options.MaxSteps, Mode: action.Draft, Contract: artifact.DefaultContract("report.md"), OnEvent: options.OnEvent, Steering: options.Steering}
	parse := func(values []string) ([][]string, error) {
		var result verificationFlags
		for _, v := range values {
			if err := result.Set(v); err != nil {
				return nil, err
			}
		}
		return result, nil
	}
	var err error
	request.Code = workrun.CodePolicy{MaxSteps: options.Code.MaxSteps, Scopes: options.Code.Scopes, Profile: options.Code.Profile, Sandbox: sandbox.Policy{Mode: sandbox.Mode(options.Code.Sandbox), Network: sandbox.Network(options.Code.Network)}, Capabilities: options.Code.Capabilities, BrowserSession: options.Code.BrowserSession}
	for _, pair := range []struct {
		values []string
		target *[][]string
	}{{options.Code.Verification, &request.Code.Verification}, {options.Code.Setup, &request.Code.Setup}, {options.Code.AllowedCommands, &request.Code.AllowedCommands}, {options.Code.AllowedCommandPrefixes, &request.Code.AllowedCommandPrefixes}} {
		*pair.target, err = parse(pair.values)
		if err != nil {
			return worktui.RunResult{Error: err.Error()}
		}
	}
	if len(request.Code.Verification) == 0 {
		request.Code.Verification = parseSuggestedVerification(suggestedVerificationCommands(source))
	}
	var images, documents attachmentFlags
	for _, path := range options.Attachments {
		if attachment.IsImage(path) {
			images = append(images, path)
		} else {
			documents = append(documents, path)
		}
	}
	request.Images, request.Attachments, err = loadPromptAttachments(source, images, documents)
	if err != nil {
		return worktui.RunResult{Error: err.Error()}
	}
	service, err := configuredWorkService("", "", stateDir, &request)
	if err != nil {
		return worktui.RunResult{Error: err.Error()}
	}
	var outcome workrun.Outcome
	if options.OnOperation != nil {
		operation := service.Start(ctx, request)
		options.OnOperation(operation)
		for range operation.Events {
		} // events also arrive through the caller's sink.
		completion := <-operation.Done
		outcome, err = completion.Outcome, completion.Err
	} else {
		outcome, err = service.Execute(ctx, request)
	}
	result := worktui.RunResult{ConversationID: outcome.ConversationID, RevisionID: outcome.RevisionID, SnapshotID: outcome.SnapshotID, FinalText: outcome.Result.FinalText, OutputPath: outcome.Work.Output.Path()}
	if err != nil {
		result.Error = err.Error()
	}
	return result
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
