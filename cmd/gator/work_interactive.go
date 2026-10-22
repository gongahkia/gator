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
	"time"

	"github.com/atotto/clipboard"
	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/delivery"
	"github.com/gongahkia/gator/internal/followup"
	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/hostproc"
	"github.com/gongahkia/gator/internal/inbox"
	"github.com/gongahkia/gator/internal/jobs"
	"github.com/gongahkia/gator/internal/learning"
	"github.com/gongahkia/gator/internal/sandbox"
	"github.com/gongahkia/gator/internal/state"
	"github.com/gongahkia/gator/internal/uisnapshot"
	sharedusage "github.com/gongahkia/gator/internal/usage"
	"github.com/gongahkia/gator/internal/workapp"
	"github.com/gongahkia/gator/internal/workhistory"
	"github.com/gongahkia/gator/internal/workhome"
	"github.com/gongahkia/gator/internal/workrun"
	"github.com/gongahkia/gator/internal/worksession"
	"github.com/gongahkia/gator/internal/workspace"
)

func workInteractive() error {
	return workInteractiveConversation("")
}

func workInteractiveConversation(startConversationID string) error {
	return workInteractiveConversationForGoal(startConversationID, "")
}

func workInteractiveGoal(goalID string) error {
	return workInteractiveConversationForGoal("", goalID)
}

func workInteractiveConversationForGoal(startConversationID, goalID string) error {
	workingDirectory, err := os.Getwd()
	if err != nil {
		return err
	}
	originalWorkingDirectory := workingDirectory
	backend := os.Getenv("GATOR_UI_BACKEND") == "1"
	if !backend {
		return errors.New(desktopOnlyMessage)
	}
	stateDir, err := state.ResolveDir(os.Getenv("GATOR_STATE_DIR"))
	if err != nil {
		return err
	}
	goalStore, err := goal.Open(stateDir)
	if err != nil {
		return err
	}
	var goalTitle string
	if goalID != "" {
		currentGoal, loadErr := goalStore.Load(goalID)
		if loadErr != nil {
			return loadErr
		}
		if currentGoal.Status != goal.Active {
			return fmt.Errorf("goal %s is %s; resume it before chatting", goalID, currentGoal.Status)
		}
		workingDirectory = currentGoal.SourcePath
		goalTitle = currentGoal.Title
		startConversationID = currentGoal.ConversationID
	}
	sessions, err := worksession.Open(stateDir)
	if err != nil {
		return err
	}
	history, err := workhistory.Open(stateDir)
	if err != nil {
		return err
	}
	deliveryStore, err := delivery.Open(stateDir)
	if err != nil {
		return err
	}
	learningStore, err := learning.Open(stateDir)
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
	supervisorRunning := false
	for _, definition := range definitions {
		if definition.Enabled {
			supervisorRunning = jobSupervisorRequest("health", io.Discard) == nil
			break
		}
	}
	inboxStore, err := inbox.Open(stateDir)
	if err != nil {
		return err
	}
	inboxEntries, err := inboxStore.List(false, 20)
	if err != nil {
		return err
	}
	goals, err := goalStore.List()
	if err != nil {
		return err
	}
	attention, err := workhome.Read(stateDir, goals, inboxEntries)
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
	application := workapp.New(workapp.Config{
		Models: workModelPanel(settingsStore, stateDir, localModels),
		SelectedModel: func() (string, string, error) {
			current, err := settingsStore.Load()
			return current.Defaults.Provider, current.Defaults.Model, err
		},
		ModelStatus: func() (workapp.ModelStatus, error) {
			return currentWorkModelStatus(settingsStore, stateDir)
		},
		Live:          true,
		BridgeMode:    backend,
		InitialTitle:  goalTitle,
		GoalID:        goalID,
		CurrentFolder: workingDirectory, Conversations: conversations, Jobs: definitions, SupervisorRunning: supervisorRunning, Inbox: inboxEntries, Goals: goals, Attention: attention,
		RefreshAttention: func() (workhome.Attention, error) {
			freshGoals, err := goalStore.List()
			if err != nil {
				return workhome.Attention{}, err
			}
			freshInbox, err := inboxStore.List(false, 20)
			if err != nil {
				return workhome.Attention{}, err
			}
			return workhome.Read(stateDir, freshGoals, freshInbox)
		},
		ListGoals: goalStore.List,
		SelectGoal: func(id string) (workapp.GoalState, error) {
			record, err := goalStore.Load(id)
			if err != nil {
				return workapp.GoalState{}, err
			}
			if record.Status != goal.Active {
				return workapp.GoalState{}, fmt.Errorf("goal %s is %s", id, record.Status)
			}
			goalID = id
			return workapp.GoalState{Title: record.Title, SourcePath: record.SourcePath, ConversationID: record.ConversationID}, nil
		},
		ClearGoal: func() { goalID = "" },
		ResetPrivateState: func() (workapp.GoalState, error) {
			goalID = ""
			source := originalWorkingDirectory
			if info, err := os.Stat(source); err != nil || !info.IsDir() {
				source = filepath.Join(stateDir, "gator", "empty-workspace")
				if err = os.MkdirAll(source, 0700); err != nil {
					return workapp.GoalState{}, err
				}
			}
			return workapp.GoalState{SourcePath: source, Title: "Work in " + filepath.Base(source)}, nil
		},
		PlanAction: func(command string) (string, error) {
			if goalID == "" {
				return "", errors.New("select an active goal with /goals before using /plan")
			}
			return workTUIPlanAction(goalStore, goalID, command)
		},
		PlanResult: func(command string) (*uisnapshot.Result, error) { return workPlanResult(goalStore, goalID, command) },
		ActivityAction: func() (string, error) {
			var output bytes.Buffer
			err := renderWorkTUIActivity(stateDir, goalID, &output)
			return strings.TrimSpace(output.String()), err
		},
		ActivityResult: func() (*uisnapshot.Result, error) { return workActivityResult(stateDir, goalID) },
		NotificationAction: func(arguments []string) (string, error) {
			if goalID == "" {
				return "", errors.New("select an active goal with /goals before using /notifications")
			}
			var output bytes.Buffer
			err := goalNotificationsCommand(goalStore, append([]string{goalID}, arguments...), &output)
			return strings.TrimSpace(output.String()), err
		},
		SuggestionAction: func(arguments []string) (string, error) {
			var output bytes.Buffer
			err := suggestionCommand(arguments, &output)
			return strings.TrimSpace(output.String()), err
		},
		MemoryAction: func(arguments []string) (string, error) {
			if goalID != "" && len(arguments) > 0 && (arguments[0] == "add" || arguments[0] == "list" || arguments[0] == "search") {
				hasGoal := false
				for _, argument := range arguments {
					if argument == "--goal" || strings.HasPrefix(argument, "--goal=") {
						hasGoal = true
						break
					}
				}
				if !hasGoal {
					arguments = append([]string{arguments[0], "--goal", goalID}, arguments[1:]...)
				}
			}
			var output bytes.Buffer
			err := memoryCommand(arguments, &output)
			return strings.TrimSpace(output.String()), err
		},
		MemoryResult: func(arguments []string) (*uisnapshot.Result, error) {
			return workMemoryResult(stateDir, goalID, arguments)
		},
		RecallStatus:    func(ctx context.Context) (workapp.RecallStatus, error) { return workRecallStatus(ctx, stateDir) },
		ConfigureRecall: configureWorkRecall,
		InstallRecall:   installWorkRecall,
		ReadContext: func(workID, activeGoal string) (workapp.ContextState, error) {
			return readWorkContext(stateDir, workID, activeGoal)
		},
		ChangeKnowledge: func(id, action, text, activeGoal string) error {
			return changeWorkKnowledge(stateDir, id, action, text, activeGoal)
		},
		SaveFollowup:       func(draft followup.Draft) (goal.Record, jobs.Definition, error) { return saveFollowup(stateDir, draft) },
		ReadChecks:         func() ([]workhome.Check, bool, error) { return readFollowupChecks(stateDir) },
		StartSupervisor:    func() error { return startLocalSupervisor(stateDir) },
		PilotAction:        workPilotResult,
		Controls:           workControls(stateDir),
		ReadOverview:       func(ctx context.Context) (workhome.Overview, error) { return readEverydayOverview(ctx, stateDir) },
		ControlCommand:     func(args []string) *exec.Cmd { return overviewTerminalCommand(stateDir, args) },
		BeforeLocalCommand: func() (func(), error) { return state.Guard(stateDir, false) },
		NoteAction:         func(command string) (string, error) { return workTUINoteAction(command, goalID) },
		NoteResult:         func(command string) (*uisnapshot.Result, error) { return workNoteResult(goalID, command) },
		SkillAction:        workTUISkillAction,
		SkillResult:        workSkillResult,
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
		ConnectorAction: workTUIConnectorAction,
		ConnectorCommand: func(args []string) *exec.Cmd {
			command := workTUIConnectorCommand(args)
			if command != nil {
				command.Env = append(os.Environ(), "GATOR_STATE_DIR="+stateDir)
			}
			return command
		},
		BundleAction: workTUIBundleAction(stateDir),
		ValidateUploads: func(source string, paths, uploads []string) error {
			_, _, err := loadWorkPromptAttachments(source, paths, uploads)
			return err
		},
		ReviewBundle: func(path string) (*uisnapshot.Result, error) { return workReviewResult(stateDir, path) },
		ListConversations: func() ([]worksession.Conversation, error) {
			return sessions.List(50)
		},
		LoadConversation: func(conversationID string) (workapp.ConversationState, error) {
			return loadWorkTUIConversation(sessions, conversationID)
		},
		LoadConversationOptions: func(conversationID string) (workapp.RunOptions, error) {
			return loadWorkTUIConversationOptions(sessions, conversationID)
		},
		StartConversationID: startConversationID,
		PrimaryConversation: sessions.Primary,
		BindPrimaryConversation: func(id string) error {
			release, err := state.Guard(stateDir, false)
			if err != nil {
				return err
			}
			defer release()
			return sessions.BindPrimary(id)
		},
		FirstRun: settings.Defaults.Provider == "" && len(conversations) == 0,
		JobAction: func(arguments []string) (string, error) {
			release, err := state.Guard(stateDir, false)
			if err != nil {
				return "", err
			}
			defer release()
			return workTUIJobAction(arguments)
		},
		RefreshJobs: jobStore.List,
		InboxAction: func(arguments []string) (string, error) {
			var output bytes.Buffer
			err := inboxCommand(arguments, &output)
			return strings.TrimSpace(output.String()), err
		},
		RefreshInbox: func() ([]inbox.Entry, error) {
			return inboxStore.List(false, 20)
		},
		Run: func(source, conversationID, prompt string, options workapp.RunOptions) workapp.RunResult {
			release, err := state.Guard(stateDir, false)
			if err != nil {
				return workapp.RunResult{Error: err.Error()}
			}
			defer release()
			if goalID != "" {
				currentGoal, loadErr := goalStore.Load(goalID)
				if loadErr != nil {
					return workapp.RunResult{Error: loadErr.Error()}
				}
				if currentGoal.Status != goal.Active {
					return workapp.RunResult{Error: fmt.Sprintf("goal %s is %s", goalID, currentGoal.Status)}
				}
				if source != currentGoal.SourcePath || (conversationID != "" && currentGoal.ConversationID != "" && conversationID != currentGoal.ConversationID) {
					return workapp.RunResult{Error: "this chat is bound to its goal workspace and conversation; use gator to work elsewhere"}
				}
				result := runInteractiveWorkGoal(currentGoal.SourcePath, currentGoal.ConversationID, prompt, stateDir, options, currentGoal)
				if result.ConversationID != "" {
					if _, attachErr := goalStore.AttachConversation(goalID, result.ConversationID); attachErr != nil && result.Error == "" {
						result.Error = attachErr.Error()
					}
				}
				return result
			}
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
		HistoryStore:  &history,
		DeliveryStore: &deliveryStore,
		LearningStore: &learningStore,
		SessionStore:  &sessions,
		FeedbackAction: func(workID string, arguments []string) (string, error) {
			return workFeedbackTUIAction(stateDir, workID, arguments)
		},
		Inspect:           inspectWorkTUITopic,
		DoctorResult:      func() (*uisnapshot.Result, error) { return doctorResult(nil) },
		Copy:              clipboard.WriteAll,
		Theme:             settings.Theme,
		SetTheme:          saveTheme,
		StatusLine:        settings.TUI.StatusLine,
		StatusLineEnabled: settings.TUI.StatusLineEnabled,
		SetStatusLine:     saveWorkStatusLine(settingsStore),
	})
	return runUIBridge(application)
}

func workTUIConnectorAction(arguments []string) (string, error) {
	var output bytes.Buffer
	err := connectorCommandWithIO(arguments, os.Stdin, &output)
	return strings.TrimSpace(output.String()), err
}

func workTUIJobAction(arguments []string) (string, error) {
	var output bytes.Buffer
	err := jobCommand(arguments, &output)
	return strings.TrimSpace(output.String()), err
}

func workTUIConnectorCommand(arguments []string) *exec.Cmd {
	command := hostproc.Command(os.Args[0], append([]string{"provider", "connector"}, arguments...)...)
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	return command
}

func runInteractiveWork(source, conversationID, prompt, stateDir string, options workapp.RunOptions) workapp.RunResult {
	return runInteractiveWorkGoal(source, conversationID, prompt, stateDir, options, goal.Record{})
}

func runInteractiveWorkGoal(source, conversationID, prompt, stateDir string, options workapp.RunOptions, currentGoal goal.Record) workapp.RunResult {
	release, guardErr := state.Guard(stateDir, false)
	if guardErr != nil {
		return workapp.RunResult{Error: guardErr.Error()}
	}
	defer release()
	requestedAt := time.Now()
	ctx := options.Context
	if ctx == nil {
		ctx = context.Background()
	}
	ctx = sharedusage.WithState(sharedusage.WithCategory(ctx, "chat"), stateDir)
	mode, paths, decisionOrigin, decisionReason, err := resolveInteractiveWork(ctx, prompt, options)
	if err != nil {
		return workapp.RunResult{Error: err.Error()}
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
			return workapp.RunResult{Error: err.Error()}
		}
	}
	request := workrun.Request{
		RequestedAt:        requestedAt,
		PreparationContext: ctx,
		SourcePath:         source, ConversationID: conversationID, Objective: prompt,
		MaxSteps: options.MaxSteps, Mode: mode, Contract: contract,
		DecisionOrigin: decisionOrigin, DecisionReason: decisionReason,
		ExcludedKnowledgeIDs:    append([]string(nil), options.ExcludedKnowledgeIDs...),
		GoalContext:             goalContext(currentGoal),
		GoalID:                  currentGoal.ID,
		ConnectorIDs:            append([]string(nil), options.ConnectorIDs...),
		WebOrigins:              append([]string(nil), options.WebOrigins...),
		IgnoredInstructionPaths: append([]string(nil), options.IgnoredInstructionPaths...),
		RefreshSource:           options.RefreshSource || currentGoal.ID != "",
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
			return workapp.RunResult{Error: err.Error()}
		}
	}
	if len(request.Code.Verification) == 0 {
		request.Code.Verification = parseSuggestedVerification(suggestedVerificationCommands(source))
	}
	request.Images, request.Attachments, err = loadWorkPromptAttachments(source, options.Attachments, options.Uploads)
	if err != nil {
		return workapp.RunResult{Error: err.Error()}
	}
	service, err := configuredWorkService("", "", stateDir, &request)
	if err != nil {
		return workapp.RunResult{Error: err.Error()}
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
	result := workapp.RunResult{Activity: outcome.Activity, WorkID: outcome.Manifest.RunID, ConversationID: outcome.ConversationID, RevisionID: outcome.RevisionID, SnapshotID: outcome.SnapshotID, FinalText: outcome.Result.FinalText}
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

// resolveInteractiveWork spends one model request only when explicit controls
// and clear language leave the desired output uncertain. A failed classifier
// degrades to inspection; it cannot select act or connected capabilities.
func resolveInteractiveWork(ctx context.Context, prompt string, options workapp.RunOptions) (action.Mode, []string, string, string, error) {
	mode, paths, err := inferInteractiveWork(prompt, options)
	if err != nil {
		return "", nil, "", "", err
	}
	if !ambiguousInteractivePrompt(prompt, options) {
		origin := "auto"
		if options.Mode != "" && options.Mode != "auto" || len(options.Artifacts) > 0 {
			origin = "explicit"
		}
		return mode, paths, origin, "Selected from explicit controls or clear request wording.", nil
	}
	defaults, err := configuredDefaults()
	if err != nil {
		return action.Inspect, nil, "auto", "Classification unavailable; using inspect mode.", nil
	}
	provider, modelName, err := resolveConfiguredProvider(defaults.Provider, defaults.Model)
	if err != nil {
		return action.Inspect, nil, "auto", "Classification unavailable; using inspect mode.", nil
	}
	backend, err := nativeWorkModel(provider, modelName, "")
	if err != nil {
		return action.Inspect, nil, "auto", "Classification unavailable; using inspect mode.", nil
	}
	routeCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	turn, err := backend.Complete(sharedusage.WithCategory(routeCtx, "routing"), agent.TurnRequest{
		System:   `Classify the user's immediate request for the Gator desktop personal agent. Return one JSON object only: {"mode":"inspect"|"draft","artifact":"optional basename.ext"}. Inspect answers questions, including questions about Gator and its settings. Draft creates a file. Never choose act, a connector, a goal, or a schedule. If uncertain, choose inspect.`,
		Messages: []agent.Message{{Role: agent.RoleUser, Content: prompt}},
	})
	if err != nil || len(turn.ToolCalls) > 0 || len(turn.Text) > 2048 {
		return action.Inspect, nil, "auto", "Classification unavailable; using inspect mode.", nil
	}
	var route struct {
		Mode     string `json:"mode"`
		Artifact string `json:"artifact"`
	}
	if json.Unmarshal([]byte(strings.TrimSpace(turn.Text)), &route) != nil {
		return action.Inspect, nil, "auto", "Classification unavailable; using inspect mode.", nil
	}
	if route.Mode != "draft" {
		return action.Inspect, nil, "model", "Ambiguous request classified as a question.", nil
	}
	path := strings.TrimSpace(route.Artifact)
	if path == "" || filepath.Base(path) != path || artifact.ValidateOutputPath(path) != nil {
		path = inferArtifactPaths(strings.ToLower(prompt))[0]
	}
	return action.Draft, []string{path}, "model", "Ambiguous request classified as a deliverable.", nil
}

func ambiguousInteractivePrompt(prompt string, options workapp.RunOptions) bool {
	if options.Mode != "" && options.Mode != "auto" || len(options.Artifacts) > 0 {
		return false
	}
	lower := strings.ToLower(strings.TrimSpace(prompt))
	if strings.HasSuffix(lower, "?") || containsAny(lower, "write ", "create ", "draft ", "produce ", "generate ", "prepare ", "build ", "reconcile ", "implement ", "fix ", "refactor ", "design ", ".md", ".json", ".csv", ".docx", ".xlsx", ".pdf", ".pptx", ".svg", ".html") {
		return false
	}
	if len(options.PreviousArtifacts) > 0 && containsAny(lower, "revise", "rewrite", "update it", "edit it", "tighten", "improve it") {
		return false
	}
	return !containsAny(lower, "what ", "why ", "how ", "when ", "where ", "who ", "explain ", "summarize ")
}

func inferInteractiveWork(prompt string, options workapp.RunOptions) (action.Mode, []string, error) {
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
	documentWork := containsAny(lower, "report", "brief", "memo", "document", "spreadsheet", "workbook", "csv", "json", "pdf", "docx", "presentation", "slide", "deck", "pptx", "poster", "svg", "html", "web page", "webpage", "dashboard")
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
		case ".md", ".txt", ".json", ".csv", ".yaml", ".yml", ".docx", ".xlsx", ".pdf", ".pptx", ".svg", ".html", ".htm":
			candidate = filepath.ToSlash(filepath.Base(candidate))
			if artifact.ValidateOutputPath(candidate) == nil {
				return []string{candidate}
			}
		}
	}
	switch {
	case containsAny(prompt, "presentation", "slide deck", "slides", "pptx", "deck"):
		return []string{"deck.pptx"}
	case containsAny(prompt, "poster", "svg", "vector graphic"):
		return []string{"design.svg"}
	case containsAny(prompt, "html", "web page", "webpage", "dashboard", "prototype"):
		return []string{"prototype.html"}
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

func summarizeWorkBundle(path string) (workapp.BundleSummary, error) {
	bundle, err := artifact.OpenBundle(path)
	if err != nil {
		return workapp.BundleSummary{}, err
	}
	if err := artifact.VerifyBundle(bundle); err != nil {
		return workapp.BundleSummary{}, err
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
	summary := workapp.BundleSummary{Path: bundle.Path, Status: string(bundle.Manifest.Status)}
	for _, file := range bundle.Manifest.Artifacts {
		summary.Artifacts = append(summary.Artifacts, workapp.ArtifactSummary{
			Path: file.Path, MediaType: file.MediaType, Bytes: file.Bytes, Valid: valid[file.Path],
		})
	}
	for _, candidate := range bundle.Manifest.Candidates {
		summary.Candidates = append(summary.Candidates, workapp.CandidateSummary{
			ID: candidate.ID, PatchPath: candidate.PatchPath, Status: candidate.Status,
			ChangedPaths: append([]string(nil), candidate.ChangedPaths...),
		})
	}
	projectWorkBundleDetails(bundle, &summary)
	return summary, nil
}

func workTUIBundleAction(stateDir string) func(workapp.BundleActionRequest) (string, error) {
	return func(request workapp.BundleActionRequest) (string, error) {
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

func loadWorkTUIConversationOptions(store worksession.Store, conversationID string) (workapp.RunOptions, error) {
	conversation, err := store.Load(conversationID)
	if err != nil {
		return workapp.RunOptions{}, err
	}
	options := workapp.RunOptions{
		Mode: "auto", MaxSteps: 24,
		Code: workapp.CodeOptions{MaxSteps: 16, Sandbox: "strict", Network: "deny"},
	}
	if conversation.HeadRevision == "" {
		return options, nil
	}
	revision, err := store.LoadRevision(conversationID, conversation.HeadRevision)
	if err != nil {
		return workapp.RunOptions{}, err
	}
	if revision.Replay == nil || len(revision.Replay.Configuration) == 0 {
		return options, nil
	}
	var configuration struct {
		Contract            artifact.Contract
		Mode                action.Mode
		Code                workrun.CodePolicy
		Connectors          []string
		WebOrigins          []string
		IgnoredInstructions []string
		MaxSteps            int
	}
	if err := json.Unmarshal(revision.Replay.Configuration, &configuration); err != nil {
		return workapp.RunOptions{}, fmt.Errorf("decode retained Work settings: %w", err)
	}
	if configuration.MaxSteps > 0 {
		options.MaxSteps = configuration.MaxSteps
	}
	for _, requirement := range configuration.Contract.Artifacts {
		options.PreviousArtifacts = append(options.PreviousArtifacts, requirement.Path)
	}
	options.ConnectorIDs = append([]string(nil), configuration.Connectors...)
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
func loadWorkTUIConversation(store worksession.Store, conversationID string) (workapp.ConversationState, error) {
	conversation, err := store.Load(conversationID)
	if err != nil {
		return workapp.ConversationState{}, err
	}
	state := workapp.ConversationState{
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
		return workapp.ConversationState{}, err
	}
	state.Messages = make([]workapp.TranscriptMessage, 0, len(lineage)*3)
	for _, revision := range lineage {
		state.Messages = append(state.Messages, workapp.TranscriptMessage{Role: "You", Text: revision.Objective, Inputs: append([]string(nil), revision.InputNames...)})
		activity := revision.Activity.Clone()
		if strings.TrimSpace(revision.FinalText) != "" {
			state.Messages = append(state.Messages, workapp.TranscriptMessage{Activity: activity, Role: "Gator", Text: revision.FinalText})
		} else if revision.Status != string(artifact.Completed) {
			state.Messages = append(state.Messages, workapp.TranscriptMessage{
				Activity: activity, Role: "Gator", Text: "This retained Work revision ended with status " + revision.Status + ".",
			})
		}
		bundle, bundleErr := summarizeWorkBundle(revision.BundlePath)
		if bundleErr != nil {
			// Keep restoration usable when an older bundle has been removed or no
			// longer verifies, while making the missing review evidence explicit.
			state.Messages = append(state.Messages, workapp.TranscriptMessage{
				Role: "Gator", Text: "The retained deliverables for revision " + revision.ID + " cannot be displayed: " + bundleErr.Error(),
			})
			continue
		}
		state.Messages = append(state.Messages, workapp.TranscriptMessage{Role: "Deliverables", Bundle: &bundle})
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
	stateDir, err := state.ResolveDir(os.Getenv("GATOR_STATE_DIR"))
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
