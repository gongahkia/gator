package workapp

import (
	"errors"
	"fmt"
	"github.com/gongahkia/gator/internal/worksession"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEffortActionRejectsAStalePreferenceSnapshotAndKeepsDraft(t *testing.T) {
	config, store := requestConfig(t, func(string, string, string, RunOptions) RunResult {
		t.Fatal("settings started Work")
		return RunResult{}
	})
	m := New(config)
	m.input = "Keep this draft"
	before := m.composerSettings()
	high := "high"
	if err := store.SavePreferences(m.conversation, worksession.Preferences{WorkEffort: &high}, nil); err != nil {
		t.Fatal(err)
	}
	next, _ := m.ApplyComposerSettings(ComposerAction{ID: before.ID, RequestID: "stale-effort", Action: "effort", Effort: "low"})
	m = next.(Model)
	if m.composerResult.Error == "" || m.options.MaxSteps != 48 || m.options.Code.MaxSteps != 32 || m.input != "Keep this draft" {
		t.Fatal("stale setting overwrote canonical effort or draft", m.composerResult)
	}
	p, _ := store.LoadPreferences(m.conversation)
	if p.WorkEffort == nil || *p.WorkEffort != "high" {
		t.Fatal("stale choice was persisted")
	}
}

func TestEffortCannotSaveWhenPrivateDataGuardRejectsTheChange(t *testing.T) {
	config, store := requestConfig(t, func(string, string, string, RunOptions) RunResult {
		t.Fatal("settings started Work")
		return RunResult{}
	})
	m := New(config)
	m.config.BeforeLocalCommand = func() (func(), error) { return nil, errors.New("private data is being restored") }
	if err := m.setWorkEffort("high"); err == nil {
		t.Fatal("unguarded effort update succeeded")
	}
	p, err := store.LoadPreferences(m.conversation)
	if err != nil {
		t.Fatal(err)
	}
	if p.WorkEffort != nil || m.options.MaxSteps != 24 || m.preferencesRevision != 0 {
		t.Fatal("failed guard changed persisted settings or reported success")
	}
}

func TestUnreadableEffortPreferenceCannotReportSavedOrAdmitWork(t *testing.T) {
	root := t.TempDir()
	store, err := worksession.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	chat, err := store.CreateChat(t.TempDir(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	config := Config{CurrentFolder: chat.SourcePath, StartConversationID: chat.ID, Conversations: []worksession.Conversation{chat}, SessionStore: &store, Run: func(string, string, string, RunOptions) RunResult {
		t.Fatal("corrupt preferences admitted Work")
		return RunResult{}
	}}
	m := New(config)
	m.input = "Keep this draft"
	before := m.composerSettings()
	path := filepath.Join(root, "gator", "conversations", chat.ID, "preferences.json")
	if err := os.WriteFile(path, []byte(`{"work_effort":"unsupported"}`), 0600); err != nil {
		t.Fatal(err)
	}
	next, _ := m.ApplyComposerSettings(ComposerAction{ID: before.ID, RequestID: "failed-effort", Action: "effort", Effort: "high"})
	m = next.(Model)
	if m.composerResult.Error == "" || m.options.MaxSteps != 24 || m.input != "Keep this draft" {
		t.Fatal("failed save reported success or changed draft")
	}
	next, run := m.SubmitBridgeUploads(BridgeUploadSubmission{RequestID: "blocked-effort-request", Context: "conversation:" + chat.ID, Text: "A task"})
	m = next.(Model)
	if run != nil || m.bridgeUploadResult.Error == "" || m.composerSettings().Disabled == "" {
		t.Fatal("unreadable preferences silently used stale limits")
	}
}

func TestComposerProjectsReadinessAndActualBudgetsWithoutClaimingConnectivity(t *testing.T) {
	status := ModelStatus{}
	m := New(Config{ModelStatus: func() (ModelStatus, error) { return status, nil }, ComposerModels: func() ([]ComposerModel, error) {
		return []ComposerModel{{Provider: "custom", Model: "first"}}, nil
	}})
	if state := m.BridgeState().Composer; state.Readiness.State != "needs_model" || state.Readiness.Issue == "" || state.Budget.WorkSteps != 24 || state.Budget.CodeSteps != 16 {
		t.Fatalf("first task readiness = %#v", state)
	}
	status = ModelStatus{Provider: "custom", Model: "first", Issue: "Connect the missing API key", Processing: ModelProcessing{Kind: "remote", Destination: "service.example", Description: "Task context goes to the provider"}}
	m.refreshModelStatus()
	before := m.composerSettings()
	if before.Readiness.State != "needs_setup" || before.ModelIssue != status.Issue || before.Readiness.Processing != status.Processing {
		t.Fatalf("missing setup = %#v", before)
	}
	status.Issue = ""
	m.refreshModelStatus()
	after := m.composerSettings()
	if after.Readiness.State != "configured" || after.Readiness.Issue != "" || after.ID == before.ID {
		t.Fatalf("configured model / stale control identity = %#v", after)
	}
	if err := m.setWorkEffort("high"); err != nil {
		t.Fatal(err)
	}
	if state := m.composerSettings(); state.Budget.WorkSteps != 48 || state.Budget.CodeSteps != 32 {
		t.Fatalf("effort budget = %#v", state.Budget)
	}
}

func TestComposerRetainsConfigurationReadFailureInsteadOfOfferingWork(t *testing.T) {
	m := New(Config{ModelStatus: func() (ModelStatus, error) { return ModelStatus{}, errors.New("private store unavailable") }})
	state := m.composerSettings()
	if state.Readiness.State != "needs_setup" || state.Readiness.Issue == "" {
		t.Fatalf("configuration failure = %#v", state)
	}
}

func TestComposerBoundsLargeCatalogAndKeepsCurrentChoice(t *testing.T) {
	choices := make([]ComposerModel, 300)
	for i := range choices {
		choices[i] = ComposerModel{Provider: "fixture", Model: fmt.Sprintf("model-%d", i)}
	}
	m := New(Config{SelectedModel: func() (string, string, error) { return "fixture", "model-299", nil }, ComposerModels: func() ([]ComposerModel, error) { return choices, nil }})
	state := m.composerSettings()
	if len(state.Models) != 256 || state.Models[255].Model != "model-299" || state.Issue == "" {
		t.Fatal("large model catalog was not bounded with the active selection retained")
	}
	if choices[255].Model != "model-255" {
		t.Fatal("projection mutated the catalog owner's slice")
	}
}

func TestComposerUsesExistingSelectionAndBudgetsWithoutChangingDraft(t *testing.T) {
	provider, name := "custom", "first"
	m := New(Config{
		SelectedModel: func() (string, string, error) { return provider, name, nil },
		ComposerModels: func() ([]ComposerModel, error) {
			return []ComposerModel{{Provider: "custom", Model: "first", Label: "First"}, {Provider: "custom", Model: "second", Label: "Second"}}, nil
		},
		SelectComposerModel: func(p, n string) error { provider, name = p, n; return nil },
	})
	m.input, m.conversation, m.source = "unsent text", "conversation", "/source"
	apply := func(action ComposerAction) {
		action.ID, action.RequestID = m.composerSettings().ID, "request"
		next, _ := m.ApplyComposerSettings(action)
		m = next.(Model)
		if m.composerResult.Error != "" {
			t.Fatal(m.composerResult.Error)
		}
	}
	apply(ComposerAction{Action: "model", Provider: "custom", Model: "second"})
	if name != "second" || m.modelStatus.Model != "second" {
		t.Fatal("selection did not reach canonical defaults")
	}
	apply(ComposerAction{Action: "effort", Effort: "high"})
	if m.options.MaxSteps != 48 || m.options.Code.MaxSteps != 32 {
		t.Fatal("incorrect work budgets")
	}
	apply(ComposerAction{Action: "effort", Effort: "standard"})
	if m.options.MaxSteps != 24 || m.options.Code.MaxSteps != 16 || name != "second" {
		t.Fatal("reset changed the model or failed to restore default budgets")
	}
	if m.input != "unsent text" || m.conversation != "conversation" || m.source != "/source" || len(m.messages) != 0 {
		t.Fatal("composer action changed conversation or added transcript clutter")
	}
}

func TestComposerRejectsStaleBusyAndUnavailableChoices(t *testing.T) {
	for _, scenario := range []string{"stale", "running", "queued", "panel", "missing", "unavailable", "save-failure", "bad-effort"} {
		t.Run(scenario, func(t *testing.T) {
			called := false
			m := New(Config{ComposerModels: func() ([]ComposerModel, error) {
				return []ComposerModel{{Provider: "custom", Model: "first"}, {Provider: "custom", Model: "locked", Unavailable: "Setup required"}}, nil
			}, SelectComposerModel: func(_, _ string) error { called = true; return errors.New("could not save") }})
			action := ComposerAction{RequestID: "test", Action: "model", Provider: "custom", Model: "first"}
			switch scenario {
			case "running":
				m.running = true
			case "queued":
				m.queue = []queuedRun{{id: 1, prompt: "queued"}}
			case "panel":
				m.section = "effort"
			case "missing":
				action.Model = "not-listed"
			case "unavailable":
				action.Model = "locked"
			case "bad-effort":
				action = ComposerAction{RequestID: "test", Action: "effort", Effort: "max"}
			}
			action.ID = m.composerSettings().ID
			if scenario == "stale" {
				action.ID = "old"
			}
			next, _ := m.ApplyComposerSettings(action)
			updated := next.(Model)
			if updated.composerResult.Error == "" {
				t.Fatal("invalid setting accepted")
			}
			if called != (scenario == "save-failure") {
				t.Fatal("invalid action reached persistent model selection")
			}
			if updated.options.MaxSteps != m.options.MaxSteps {
				t.Fatal("rejected action changed work effort")
			}
		})
	}
}
