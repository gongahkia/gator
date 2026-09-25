package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/attachment"
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/delivery"
	"github.com/gongahkia/gator/internal/inbox"
	"github.com/gongahkia/gator/internal/jobs"
	"github.com/gongahkia/gator/internal/journal"
	"github.com/gongahkia/gator/internal/sandbox"
	"github.com/gongahkia/gator/internal/workhistory"
	"github.com/gongahkia/gator/internal/workrun"
	"github.com/gongahkia/gator/internal/worksession"
	"github.com/gongahkia/gator/internal/workspace"
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
	history, err := workhistory.Open(stateDir)
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
		ModelStatus: func() (worktui.ModelStatus, error) {
			return currentWorkModelStatus(settingsStore, stateDir)
		},
		Live:          true,
		CurrentFolder: workingDirectory, Conversations: conversations, Jobs: definitions, Inbox: inboxEntries,
		ResolveSource: func(path string) (string, error) {
			if !filepath.IsAbs(path) {
				path = filepath.Join(workingDirectory, path)
			}
			root, err := workspace.Open(path)
			if err != nil {
				return "", err
			}
			return root.Path(), nil
		},
		ConnectorChoices: func() []string {
			current, loadErr := settingsStore.Load()
			if loadErr != nil {
				return nil
			}
			ids := make([]string, 0, len(current.Connectors))
			for _, descriptor := range current.Connectors {
				ids = append(ids, descriptor.ID)
			}
			return ids
		},
		ConnectorAction:  workTUIConnectorAction,
		ConnectorCommand: workTUIConnectorCommand,
		BundleAction:     workTUIBundleAction(stateDir),
		ListConversations: func() ([]worksession.Conversation, error) {
			return sessions.List(50)
		},
		LoadConversation: func(conversationID string) (worktui.ConversationState, error) {
			return loadWorkTUIConversation(sessions, conversationID)
		},
		LoadConversationOptions: func(conversationID string) (worktui.RunOptions, error) {
			return loadWorkTUIConversationOptions(sessions, conversationID)
		},
		StartConversationID: startConversationID,
		FirstRun:            settings.Defaults.Provider == "" && len(conversations) == 0,
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
			return workHistoryText(history, conversationID)
		},
		LearningAction: func(arguments []string) (string, error) {
			return learningTUIAction(stateDir, arguments)
		},
		FeedbackAction: func(workID string, arguments []string) (string, error) {
			return workFeedbackTUIAction(stateDir, workID, arguments)
		},
		Inspect:       inspectWorkTUITopic,
		Copy:          clipboard.WriteAll,
		Theme:         settings.Theme,
		SetTheme:      saveTheme,
		StatusLine:    settings.TUI.StatusLine,
		SetStatusLine: saveWorkStatusLine(settingsStore),
	})
	program := tea.NewProgram(application, tea.WithAltScreen())
	final, err := program.Run()
	if application, ok := final.(worktui.Model); ok {
		application.Close()
	}
	return err
}

func workTUIConnectorAction(arguments []string) (string, error) {
	var output bytes.Buffer
	err := connectorCommandWithIO(arguments, os.Stdin, &output)
	return strings.TrimSpace(output.String()), err
}

func workTUIConnectorCommand(arguments []string) *exec.Cmd {
	command := exec.Command(os.Args[0], append([]string{"provider", "connector"}, arguments...)...)
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	return command
}

func runInteractiveWork(source, conversationID, prompt, stateDir string, options worktui.RunOptions) worktui.RunResult {
	ctx := options.Context
	if ctx == nil {
		ctx = context.Background()
	}
	mode, paths, err := inferInteractiveWork(prompt, options)
	if err != nil {
		return worktui.RunResult{Error: err.Error()}
	}
	disposition := action.Forbid
	if mode == action.Draft {
		disposition = action.Propose
	} else if mode == action.Act {
		disposition = action.Approve
	}
	var contract artifact.Contract
	if mode == action.Inspect || len(paths) == 0 {
		contract = artifact.InspectionContract()
		contract.ExternalActions = disposition
	} else {
		contract, err = workContract(mode, disposition, artifactFlags(paths), nil)
		if err != nil {
			return worktui.RunResult{Error: err.Error()}
		}
	}
	request := workrun.Request{
		SourcePath: source, ConversationID: conversationID, Objective: prompt,
		MaxSteps: options.MaxSteps, Mode: mode, Contract: contract,
		ConnectorIDs:            append([]string(nil), options.ConnectorIDs...),
		WebOrigins:              append([]string(nil), options.WebOrigins...),
		IgnoredInstructionPaths: append([]string(nil), options.IgnoredInstructionPaths...),
		RefreshSource:           options.RefreshSource,
		RequireCode:             options.RequireCode,
		OnEvent:                 options.OnEvent, Steering: options.Steering,
	}
	parse := func(values []string) ([][]string, error) {
		var result verificationFlags
		for _, v := range values {
			if err := result.Set(v); err != nil {
				return nil, err
			}
		}
		return result, nil
	}
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
	result := worktui.RunResult{ConversationID: outcome.ConversationID, RevisionID: outcome.RevisionID, SnapshotID: outcome.SnapshotID, FinalText: outcome.Result.FinalText}
	if outcome.Work.Path != "" {
		result.OutputPath = outcome.Work.Output.Path()
		if summary, summaryErr := summarizeWorkBundle(outcome.Work.Path); summaryErr == nil {
			result.Bundle = summary
		} else if err == nil {
			err = summaryErr
		}
	}
	if err != nil {
		result.Error = err.Error()
	}
	return result
}

func inferInteractiveWork(prompt string, options worktui.RunOptions) (action.Mode, []string, error) {
	modeName := strings.ToLower(strings.TrimSpace(options.Mode))
	if modeName == "" {
		modeName = "auto"
	}
	paths := append([]string(nil), options.Artifacts...)
	if modeName != "auto" {
		mode := action.Mode(modeName)
		if err := mode.Validate(); err != nil {
			return "", nil, err
		}
		if mode == action.Inspect && len(paths) > 0 {
			return "", nil, errors.New("inspect mode cannot require deliverables; use /artifact clear or /mode draft")
		}
		return mode, paths, nil
	}
	if len(paths) > 0 {
		return action.Draft, paths, nil
	}
	lower := strings.ToLower(prompt)
	if len(options.PreviousArtifacts) > 0 && containsAny(lower, "revise", "rewrite", "update it", "edit it", "tighten", "improve it", "change it", "continue working") {
		return action.Draft, append([]string(nil), options.PreviousArtifacts...), nil
	}
	codeWork := containsAny(lower, "implement ", "fix ", "refactor ", "add a test", "create a test", "write code", "write a script", "add a feature", "add support", "change the code", "update the code", "patch ")
	createWork := containsAny(lower, "write ", "create ", "draft ", "produce ", "generate ", "prepare ", "build ", "reconcile ", "turn this into", "make a ")
	documentWork := containsAny(lower, "report", "brief", "memo", "document", "spreadsheet", "workbook", "csv", "json", "pdf", "docx")
	if !codeWork && !createWork && !documentWork {
		return action.Inspect, nil, nil
	}
	if codeWork && !documentWork {
		return action.Draft, nil, nil
	}
	return action.Draft, inferArtifactPaths(lower), nil
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func inferArtifactPaths(prompt string) []string {
	for _, field := range strings.Fields(prompt) {
		candidate := strings.Trim(field, "`'\".,:;()[]{}")
		switch strings.ToLower(filepath.Ext(candidate)) {
		case ".md", ".txt", ".json", ".csv", ".yaml", ".yml", ".docx", ".xlsx", ".pdf":
			candidate = filepath.ToSlash(filepath.Base(candidate))
			if artifact.ValidateOutputPath(candidate) == nil {
				return []string{candidate}
			}
		}
	}
	switch {
	case strings.Contains(prompt, "reconcile") || strings.Contains(prompt, "workbook"):
		return []string{"checked.xlsx", "memo.md"}
	case strings.Contains(prompt, "spreadsheet"):
		return []string{"workbook.xlsx"}
	case strings.Contains(prompt, "csv"):
		return []string{"results.csv"}
	case strings.Contains(prompt, "json"):
		return []string{"result.json"}
	case strings.Contains(prompt, "pdf"):
		return []string{"report.pdf"}
	case strings.Contains(prompt, "docx") || strings.Contains(prompt, "word document"):
		return []string{"report.docx"}
	case strings.Contains(prompt, "brief"):
		return []string{"brief.md"}
	case strings.Contains(prompt, "memo"):
		return []string{"memo.md"}
	case strings.Contains(prompt, "report"):
		return []string{"report.md"}
	default:
		return []string{"deliverable.md"}
	}
}

func summarizeWorkBundle(path string) (worktui.BundleSummary, error) {
	bundle, err := artifact.OpenBundle(path)
	if err != nil {
		return worktui.BundleSummary{}, err
	}
	if err := artifact.VerifyBundle(bundle); err != nil {
		return worktui.BundleSummary{}, err
	}
	valid := map[string]bool{}
	for _, file := range bundle.Manifest.Artifacts {
		valid[file.Path] = true
	}
	for _, validation := range bundle.Manifest.Validations {
		if !validation.Passed {
			valid[validation.Path] = false
		}
	}
	summary := worktui.BundleSummary{Path: bundle.Path, Status: string(bundle.Manifest.Status)}
	for _, file := range bundle.Manifest.Artifacts {
		summary.Artifacts = append(summary.Artifacts, worktui.ArtifactSummary{
			Path: file.Path, MediaType: file.MediaType, Bytes: file.Bytes, Valid: valid[file.Path],
		})
	}
	for _, candidate := range bundle.Manifest.Candidates {
		summary.Candidates = append(summary.Candidates, worktui.CandidateSummary{
			ID: candidate.ID, PatchPath: candidate.PatchPath, Status: candidate.Status,
			ChangedPaths: append([]string(nil), candidate.ChangedPaths...),
		})
	}
	return summary, nil
}

func workTUIBundleAction(stateDir string) func(worktui.BundleActionRequest) (string, error) {
	return func(request worktui.BundleActionRequest) (string, error) {
		bundle, err := artifact.OpenBundle(request.BundlePath)
		if err != nil {
			return "", err
		}
		if err := artifact.VerifyBundle(bundle); err != nil {
			return "", err
		}
		store, err := delivery.Open(stateDir)
		if err != nil {
			return "", err
		}
		switch request.Action {
		case "preview":
			previews, err := artifact.PreviewBundle(bundle)
			if err != nil {
				return "", err
			}
			sections := make([]string, 0, len(previews)+1)
			for _, preview := range previews {
				section := preview.Path + " · " + preview.Summary
				if preview.Content != "" {
					content := preview.Content
					if len(content) > 4000 {
						content = content[:4000] + "\n…"
					}
					section += "\n" + content
				}
				sections = append(sections, section)
			}
			records, err := store.ListWork(bundle.Manifest.RunID)
			if err != nil {
				return "", err
			}
			for _, record := range records {
				sections = append(sections, formatDeliveryRecord("Changes", record, nil))
			}
			if len(sections) == 0 {
				return "This inspection turn has no deliverable files.", nil
			}
			return strings.Join(sections, "\n\n"), nil
		case "save":
			if !request.Execute {
				plan, err := delivery.PreviewArtifacts(bundle, request.Target, request.Replace)
				if err != nil {
					return "", err
				}
				for _, operation := range plan.Operations {
					if operation.Disposition == artifact.ApplyConflict {
						return "", fmt.Errorf("%s already exists with different contents; review it or use /save --replace", operation.Path)
					}
				}
				return formatApplyPlan("Save verified deliverables", plan), nil
			}
			record, deliveryErr := store.DeliverArtifacts(context.Background(), bundle, request.Target, request.Replace)
			return formatDeliveryRecord("Saved verified deliverables", record, deliveryErr), nil
		case "apply-code":
			if !request.Execute {
				candidate, err := delivery.PreviewCandidate(context.Background(), bundle, request.Target, request.CandidateID)
				if err != nil {
					return "", err
				}
				return fmt.Sprintf("Apply verified candidate %s to %s\nChanged paths: %s", candidate.ID, request.Target, strings.Join(candidate.ChangedPaths, ", ")), nil
			}
			changedPaths := ""
			for _, candidate := range bundle.Manifest.Candidates {
				if candidate.ID == request.CandidateID && candidate.Status == "verified" {
					changedPaths = strings.Join(candidate.ChangedPaths, ", ")
					break
				}
			}
			record, deliveryErr := store.DeliverCandidate(context.Background(), bundle, request.Target, request.CandidateID)
			result := formatDeliveryRecord("Applied verified Code candidate", record, deliveryErr)
			if changedPaths != "" {
				result += "\nChanged paths: " + changedPaths
			}
			if deliveryErr == nil {
				result += "\nReview and commit the checkout changes."
			}
			return result, nil
		case "retry":
			if !request.Execute {
				record, err := store.PreviewRetry(bundle.Manifest.RunID, request.DeliveryID)
				if err != nil {
					return "", err
				}
				return formatDeliveryRecord("Retry local changes", record, nil), nil
			}
			record, deliveryErr := store.Retry(context.Background(), bundle.Manifest.RunID, request.DeliveryID)
			return formatDeliveryRecord("Retried local changes", record, deliveryErr), nil
		default:
			return "", fmt.Errorf("unknown bundle action %q", request.Action)
		}
	}
}

func formatDeliveryRecord(title string, record delivery.Record, deliveryErr error) string {
	if record.ID == "" {
		return title
	}
	lines := []string{title, "Delivery: " + record.ID, "Target: " + record.TargetPath}
	for _, effect := range record.Effects {
		mark := "○"
		switch effect.Status {
		case delivery.Applied:
			mark = "✓"
		case delivery.Failed:
			mark = "!"
		case delivery.Unknown:
			mark = "?"
		case delivery.Superseded:
			mark = "~"
		}
		lines = append(lines, fmt.Sprintf("%s %s · %s", mark, effect.SourcePath, effect.Status))
	}
	if deliveryErr != nil {
		lines = append(lines, "Delivery incomplete: "+deliveryErr.Error())
	}
	return strings.Join(lines, "\n")
}

func formatApplyPlan(title string, plan artifact.ApplyPlan) string {
	lines := []string{title + " to " + plan.Target}
	for _, operation := range plan.Operations {
		lines = append(lines, "  "+string(operation.Disposition)+" "+operation.Path)
	}
	return strings.Join(lines, "\n")
}

func loadWorkTUIConversationOptions(store worksession.Store, conversationID string) (worktui.RunOptions, error) {
	conversation, err := store.Load(conversationID)
	if err != nil {
		return worktui.RunOptions{}, err
	}
	options := worktui.RunOptions{
		Mode: "auto", MaxSteps: 24,
		Code: worktui.CodeOptions{MaxSteps: 16, Sandbox: "strict", Network: "deny"},
	}
	if conversation.HeadRevision == "" {
		return options, nil
	}
	revision, err := store.LoadRevision(conversationID, conversation.HeadRevision)
	if err != nil {
		return worktui.RunOptions{}, err
	}
	if revision.Replay == nil || len(revision.Replay.Configuration) == 0 {
		return options, nil
	}
	var configuration struct {
		Contract            artifact.Contract
		Mode                action.Mode
		RequireCode         bool
		Code                workrun.CodePolicy
		Connectors          []string
		WebOrigins          []string
		IgnoredInstructions []string
		MaxSteps            int
	}
	if err := json.Unmarshal(revision.Replay.Configuration, &configuration); err != nil {
		return worktui.RunOptions{}, fmt.Errorf("decode retained Work settings: %w", err)
	}
	if configuration.MaxSteps > 0 {
		options.MaxSteps = configuration.MaxSteps
	}
	for _, requirement := range configuration.Contract.Artifacts {
		options.PreviousArtifacts = append(options.PreviousArtifacts, requirement.Path)
	}
	options.ConnectorIDs = append([]string(nil), configuration.Connectors...)
	options.RequireCode = configuration.RequireCode
	options.WebOrigins = append([]string(nil), configuration.WebOrigins...)
	options.IgnoredInstructionPaths = append([]string(nil), configuration.IgnoredInstructions...)
	if configuration.Code.MaxSteps > 0 {
		options.Code.MaxSteps = configuration.Code.MaxSteps
	}
	options.Code.Scopes = append([]string(nil), configuration.Code.Scopes...)
	options.Code.Profile = configuration.Code.Profile
	options.Code.Sandbox = string(configuration.Code.Sandbox.Mode)
	options.Code.Network = string(configuration.Code.Sandbox.Network)
	options.Code.Capabilities = append([]string(nil), configuration.Code.Capabilities...)
	options.Code.BrowserSession = configuration.Code.BrowserSession
	return options, nil
}

// loadWorkTUIConversation reconstructs exactly the visible path to a
// conversation's selected head. Revision siblings remain available through
// history navigation, but never leak into the active branch's transcript.
// Bundle cards are created only from a freshly verified manifest.
func loadWorkTUIConversation(store worksession.Store, conversationID string) (worktui.ConversationState, error) {
	conversation, err := store.Load(conversationID)
	if err != nil {
		return worktui.ConversationState{}, err
	}
	state := worktui.ConversationState{
		Title:      conversation.Title,
		SourcePath: conversation.SourcePath,
		RevisionID: conversation.HeadRevision,
		SnapshotID: conversation.SnapshotID,
	}
	if conversation.HeadRevision == "" {
		return state, nil
	}
	lineage, err := store.Lineage(conversationID, conversation.HeadRevision)
	if err != nil {
		return worktui.ConversationState{}, err
	}
	state.Messages = make([]worktui.TranscriptMessage, 0, len(lineage)*3)
	for _, revision := range lineage {
		state.Messages = append(state.Messages, worktui.TranscriptMessage{Role: "You", Text: revision.Objective})
		if strings.TrimSpace(revision.FinalText) != "" {
			state.Messages = append(state.Messages, worktui.TranscriptMessage{Role: "Gator", Text: revision.FinalText})
		} else if revision.Status != string(artifact.Completed) {
			state.Messages = append(state.Messages, worktui.TranscriptMessage{
				Role: "Gator", Text: "This retained Work revision ended with status " + revision.Status + ".",
			})
		}
		bundle, bundleErr := summarizeWorkBundle(revision.BundlePath)
		if bundleErr != nil {
			// Keep restoration usable when an older bundle has been removed or no
			// longer verifies, while making the missing review evidence explicit.
			state.Messages = append(state.Messages, worktui.TranscriptMessage{
				Role: "Gator", Text: "The retained deliverables for revision " + revision.ID + " cannot be displayed: " + bundleErr.Error(),
			})
			continue
		}
		state.Messages = append(state.Messages, worktui.TranscriptMessage{Role: "Deliverables", Bundle: &bundle})
		if revision.ID == conversation.HeadRevision {
			state.LastBundle = bundle
			state.OutputPath = filepath.Join(bundle.Path, "output")
		}
	}
	return state, nil
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
