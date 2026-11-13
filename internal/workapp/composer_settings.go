package workapp

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/worksession"
)

// ComposerModel contains display metadata, never endpoint or credential values.
type ComposerModel struct {
	Provider    string `json:"provider"`
	Model       string `json:"model"`
	Label       string `json:"label"`
	Unavailable string `json:"unavailable,omitempty"`
}

type ComposerSettings struct {
	ID         string          `json:"id"`
	Effort     string          `json:"effort"`
	Models     []ComposerModel `json:"models"`
	ModelIssue string          `json:"model_issue,omitempty"`
	Issue      string          `json:"issue,omitempty"`
	Disabled   string          `json:"disabled,omitempty"`
	Readiness  ModelReadiness  `json:"readiness"`
	Budget     ComposerBudget  `json:"budget"`
}

// These are configuration checks, not a claim that a remote or local endpoint
// is online. No inference request is made while projecting the composer.
type ModelProcessing struct {
	Kind        string `json:"kind"`
	Destination string `json:"destination,omitempty"`
	Description string `json:"description"`
}

type ModelReadiness struct {
	State      string          `json:"state"`
	Issue      string          `json:"issue,omitempty"`
	Processing ModelProcessing `json:"processing"`
}

type ComposerBudget struct {
	WorkSteps int `json:"work_steps"`
	CodeSteps int `json:"code_steps"`
}

type ComposerAction struct {
	ID        string `json:"id"`
	RequestID string `json:"request_id"`
	Action    string `json:"action"`
	Provider  string `json:"provider,omitempty"`
	Model     string `json:"model,omitempty"`
	Effort    string `json:"effort,omitempty"`
}

type ComposerResult struct {
	RequestID string `json:"request_id"`
	Error     string `json:"error,omitempty"`
}

func (m Model) composerSettings() ComposerSettings {
	state := ComposerSettings{Effort: effortName(m.options.MaxSteps), Models: append([]ComposerModel{}, m.composerModels...), ModelIssue: m.composerModelIssue(), Issue: m.composerModelsIssue}
	state.Readiness = ModelReadiness{State: "configured", Processing: m.modelStatus.Processing, Issue: state.ModelIssue}
	if state.Readiness.Processing.Kind == "" {
		state.Readiness.Processing = ModelProcessing{Kind: "unknown", Description: "Choose a model to inspect where its requests are processed."}
	}
	if m.modelStatus.Provider == "" {
		state.Readiness.State = "needs_model"
		if state.ModelIssue != "" {
			state.Readiness.State = "needs_setup"
		} else {
			state.Readiness.Issue = "Choose a local model or connect a model provider. Your draft stays here during setup."
		}
	} else if state.ModelIssue != "" {
		state.Readiness.State = "needs_setup"
	}
	state.Budget = ComposerBudget{WorkSteps: m.options.MaxSteps, CodeSteps: m.options.Code.MaxSteps}
	if m.preferencesIssue != "" {
		state.Issue, state.Disabled = m.preferencesIssue, m.preferencesIssue
	} else if m.running || len(m.queue) > 0 {
		state.Disabled = "Available after current and queued work finishes."
	} else if m.models != nil || m.launcher || m.section != "" || m.bridgeExternal != nil || m.pendingBundleAction != nil || m.controlBusy || m.recallBusy {
		state.Disabled = "Return to the conversation to change these settings."
	}
	data, _ := json.Marshal([]any{m.source, m.conversation, m.modelStatus.Provider, m.modelStatus.Model, state.Effort, state.Models, state.ModelIssue, state.Issue, state.Disabled, state.Readiness, state.Budget})
	state.ID = fmt.Sprintf("%x", sha256.Sum256(data))
	return state
}

func (m Model) composerModelIssue() string {
	if m.modelStatus.Issue != "" {
		return m.modelStatus.Issue
	}
	if m.config.ComposerModels == nil || m.modelStatus.Provider == "" {
		return ""
	}
	for _, choice := range m.composerModels {
		if choice.Provider == m.modelStatus.Provider && choice.Model == m.modelStatus.Model {
			return choice.Unavailable
		}
	}
	return "Configure models in Settings before sending a message."
}

// ApplyComposerSettings reuses canonical defaults and task budgets. It never
// alters a running request, queued work, conversation identity or draft.
func (m Model) ApplyComposerSettings(action ComposerAction) (tea.Model, tea.Cmd) {
	m.composerResult = &ComposerResult{RequestID: action.RequestID}
	fail := func(text string) (tea.Model, tea.Cmd) { m.composerResult.Error = text; return m, nil }
	if err := m.refreshConversationPreferences(); err != nil {
		return fail(err.Error())
	}
	state := m.composerSettings()
	if action.RequestID == "" || len(action.RequestID) > 64 || action.ID != state.ID {
		return fail("These settings changed. Review the current selection and try again.")
	}
	if state.Disabled != "" {
		return fail(state.Disabled)
	}
	switch action.Action {
	case "model":
		if action.Effort != "" || m.config.SelectComposerModel == nil {
			return fail("Model selection is unavailable.")
		}
		// Re-read configuration at the action boundary so removed providers and
		// credentials cannot be selected using an old display snapshot.
		m.refreshModelStatus()
		if m.composerSettings().ID != action.ID {
			return fail("Available models changed. Review the current list and try again.")
		}
		found := false
		for _, choice := range m.composerModels {
			if choice.Provider == action.Provider && choice.Model == action.Model && choice.Unavailable == "" {
				found = true
				break
			}
		}
		if !found {
			return fail("This model is no longer available. Open Manage models to configure it.")
		}
		if err := m.config.SelectComposerModel(action.Provider, action.Model); err != nil {
			return fail("Could not select model: " + singleLine(err.Error()))
		}
		m.refreshModelStatus()
		m.firstRun = m.modelStatus.Provider == ""
		if m.modelStatus.Provider != action.Provider || m.modelStatus.Model != action.Model {
			return fail("Could not confirm the saved model selection. Inspect the current model settings before submitting work.")
		}
		m.preferencesRevision++
	case "effort":
		if action.Provider != "" || action.Model != "" {
			return fail("Invalid work effort selection.")
		}
		if err := m.setWorkEffort(action.Effort); err != nil {
			return fail(err.Error())
		}
	default:
		return fail("Unknown composer setting.")
	}
	return m, nil
}

func (m *Model) setWorkEffort(value string) error {
	value = strings.ToLower(value)
	budget, err := worksession.WorkEffortBudget(value)
	if err != nil {
		return err
	}
	if m.retainsRequests() {
		if m.config.BeforeLocalCommand != nil {
			release, err := m.config.BeforeLocalCommand()
			if err != nil {
				return err
			}
			defer release()
		}
		prior, err := m.config.SessionStore.LoadPreferences(m.conversation)
		if err != nil {
			return err
		}
		next := prior
		next.WorkEffort = &value
		if err := m.config.SessionStore.SavePreferences(m.conversation, next, &prior); err != nil {
			return err
		}
	}
	m.options.MaxSteps, m.options.Code.MaxSteps = budget.MaxSteps, budget.CodeMaxSteps
	m.preferencesIssue = ""
	m.preferencesRevision++
	return nil
}
