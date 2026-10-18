package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/browser"
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/connector"
	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/knowledge"
	"github.com/gongahkia/gator/internal/learning"
	"github.com/gongahkia/gator/internal/memory"
	"github.com/gongahkia/gator/internal/orchestrator"
	"github.com/gongahkia/gator/internal/snapshot"
	"github.com/gongahkia/gator/internal/workrun"
)

// configuredWorkService assembles trusted adapters; execution stays in workrun.Service.
func configuredWorkService(provider, modelName, stateDir string, request *workrun.Request) (workrun.Service, error) {
	defaults, err := configuredDefaults()
	if err != nil {
		return workrun.Service{}, err
	}
	if provider == "" {
		provider = defaults.Provider
	}
	if modelName == "" {
		modelName = defaults.Model
	}
	provider, modelName, err = resolveConfiguredProvider(provider, modelName)
	if err != nil {
		return workrun.Service{}, err
	}
	backend, err := nativeWorkModel(provider, modelName, "")
	if err != nil {
		return workrun.Service{}, err
	}
	settings, err := loadSettings()
	if err != nil {
		return workrun.Service{}, err
	}
	registry, err := connector.NewRegistry(settings.Connectors)
	if err != nil {
		return workrun.Service{}, err
	}
	credentials, err := gatorCredentials()
	if err != nil {
		return workrun.Service{}, err
	}
	request.OTLPEndpoint = os.Getenv("GATOR_OTLP_ENDPOINT")
	request.Provider = provider + "/" + modelName
	request.DisableBrowser = provider == "gator-local"
	request.StateDir = stateDir
	if err := applyLearningContext(stateDir, request); err != nil {
		return workrun.Service{}, err
	}
	if err := applyMemoryContext(stateDir, request); err != nil {
		return workrun.Service{}, err
	}
	if err := applyNoteContext(stateDir, request); err != nil {
		return workrun.Service{}, err
	}
	if err := applyGoalPlanContext(stateDir, request); err != nil {
		return workrun.Service{}, err
	}
	request.ConnectorPermissions = connector.PermissionSet(settings.ConnectorPermissions)
	request.SnapshotOptions = snapshot.Options{Limits: snapshot.Limits{MaxFiles: settings.Snapshots.MaxFiles, MaxTotal: settings.Snapshots.MaxTotalBytes, MaxFileBytes: settings.Snapshots.MaxFileBytes}, Excludes: settings.Snapshots.Excludes}
	executor := workrun.Executor{Model: backend, StateDir: stateDir, Connectors: connector.Runtime{Registry: registry, Credentials: credentials}}
	if request.BrowserSession != "" {
		store, err := browser.Open(stateDir)
		if err != nil {
			return workrun.Service{}, err
		}
		client, err := browser.NewClient(store, request.BrowserSession)
		if err != nil {
			return workrun.Service{}, err
		}
		session, err := client.Session(context.Background(), request.BrowserSession)
		if err != nil {
			return workrun.Service{}, err
		}
		if len(session.SelectedTabs) == 0 {
			return workrun.Service{}, errors.New("Work browser session has no developer-selected tabs")
		}
		executor.Browser = client
	}
	if native, ok := backend.(*nativeWorkBackend); ok {
		executor.Code = native.codeDelegate(stateDir)
	}
	if err := configureWorkRoles(&executor, settings, stateDir); err != nil {
		return workrun.Service{}, err
	}
	return workrun.Service{Executor: executor, Defaults: request}, nil
}

// applyLearningContext keeps learning selection at the one application
// boundary shared by CLI, TUI, jobs, headless Work, and configured live evals.
// Invalid hand-edited state fails closed rather than entering model context.
func applyLearningContext(stateDir string, request *workrun.Request) error {
	store, err := learning.Open(stateDir)
	if err != nil {
		return err
	}
	records, err := store.Projection(learning.Context{Project: request.SourcePath})
	if err != nil {
		return fmt.Errorf("resolve active learnings: %w", err)
	}
	request.LearningContext = learning.RenderProjection(records)
	return nil
}

func applyMemoryContext(stateDir string, request *workrun.Request) error {
	store, err := memory.Open(stateDir)
	if err != nil {
		return err
	}
	query := request.Objective + " " + request.GoalContext
	records, err := store.Search(query, request.GoalID, 8)
	if err != nil {
		return fmt.Errorf("resolve relevant memories: %w", err)
	}
	request.MemoryContext = memory.Render(records)
	return nil
}

func applyNoteContext(stateDir string, request *workrun.Request) error {
	store, err := knowledge.Open(stateDir)
	if err != nil {
		return err
	}
	notes, err := store.SearchNotes(request.Objective, request.GoalID, 4)
	if err != nil {
		return fmt.Errorf("resolve relevant notes: %w", err)
	}
	if len(notes) == 0 && strings.TrimSpace(request.GoalContext) != "" {
		notes, err = store.SearchNotes(request.GoalContext, request.GoalID, 4)
		if err != nil {
			return fmt.Errorf("resolve goal-context notes: %w", err)
		}
	}
	var lines []string
	remaining := 2400
	for _, note := range notes {
		line := fmt.Sprintf("- [%s] %s (%s, lexical match: %s)", note.ID, safeNoteContext(note.Title), note.MatchScope, strings.Join(note.MatchedTerms, ", "))
		if len(note.SourceRefs) > 0 {
			refs := note.SourceRefs[:min(2, len(note.SourceRefs))]
			line += " [source labels: " + safeNoteContext(strings.Join(refs, ", ")) + "]"
		}
		if note.Excerpt != "" {
			line += ": " + safeNoteContext(note.Excerpt)
		}
		if len(line) > remaining {
			break
		}
		lines = append(lines, line)
		remaining -= len(line)
	}
	request.NoteContext = strings.Join(lines, "\n")
	return nil
}

func safeNoteContext(value string) string {
	clean := strings.Map(func(character rune) rune {
		if unicode.IsControl(character) {
			return ' '
		}
		return character
	}, value)
	return strings.Join(strings.Fields(clean), " ")
}

func applyGoalPlanContext(stateDir string, request *workrun.Request) error {
	request.GoalPlanContext = ""
	if request.GoalID == "" {
		return nil
	}
	store, err := goal.Open(stateDir)
	if err != nil {
		return err
	}
	plan, err := store.LoadPlan(request.GoalID)
	if err != nil {
		return fmt.Errorf("load goal plan: %w", err)
	}
	var lines []string
	if plan.Outcome != "" {
		lines = append(lines, "Outcome: "+plan.Outcome)
	}
	for _, step := range plan.Steps {
		line := fmt.Sprintf("- [%s] %s: %s", step.ID, step.Status, step.Title)
		if !step.DueAt.IsZero() {
			line += " (due " + step.DueAt.Format("2006-01-02T15:04Z07:00") + ")"
		}
		if len(step.Evidence) > 0 {
			line += " [evidence: " + strings.Join(step.Evidence, ", ") + "]"
		}
		if len(strings.Join(lines, "\n"))+len(line) > 8*1024 {
			lines = append(lines, "- Additional steps omitted; inspect the goal plan before deciding.")
			break
		}
		lines = append(lines, line)
	}
	if !plan.NextReviewAt.IsZero() {
		lines = append(lines, "Next review: "+plan.NextReviewAt.Format("2006-01-02T15:04Z07:00"))
	}
	request.GoalPlanContext = strings.Join(lines, "\n")
	return nil
}

func executeConfiguredWork(ctx context.Context, provider, modelName, stateDir string, request workrun.Request) (workrun.Outcome, error) {
	service, err := configuredWorkService(provider, modelName, stateDir, &request)
	if err != nil {
		return workrun.Outcome{}, err
	}
	return service.Execute(ctx, request)
}

func configureWorkRoles(executor *workrun.Executor, settings config.Settings, state string) error {
	executor.RoleConfiguration = map[string]orchestrator.RoleConfiguration{}
	executor.RoleModels = map[string]agent.Model{}
	executor.RoleSteps = map[string]int{}
	for _, role := range settings.WorkRoles {
		executor.RoleSteps[role.Name] = role.MaxSteps
		configuration := orchestrator.RoleConfiguration{Version: role.Version, MaxSteps: role.MaxSteps}
		if role.Provider != "" {
			configuration.Provider = role.Provider + "/" + role.Model
		}
		executor.RoleConfiguration[role.Name] = configuration
		if role.Provider == "" {
			continue
		}
		model, err := nativeWorkModel(role.Provider, role.Model, "")
		if err != nil {
			return err
		}
		if role.Name == "code" {
			executor.Code = model.(*nativeWorkBackend).codeDelegate(state)
		} else {
			executor.RoleModels[role.Name] = model
		}
	}
	return nil
}

func readWorkJSON(path string, value any) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if info.Size() > 1024*1024 {
		return errors.New("Work JSON document exceeds 1 MiB")
	}
	decoder := json.NewDecoder(io.LimitReader(file, 1024*1024+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("expected one bounded JSON document")
	}
	return nil
}
