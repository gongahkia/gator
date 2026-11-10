package workapp

import (
	"errors"
	"fmt"
	"testing"
)

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
