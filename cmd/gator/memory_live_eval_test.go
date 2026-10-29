package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/knowledge"
	"github.com/gongahkia/gator/internal/memory"
	"github.com/gongahkia/gator/internal/model"
	"github.com/gongahkia/gator/internal/tools"
	"github.com/gongahkia/gator/internal/workrun"
)

// Explicit opt-in only. The selected provider receives synthetic text and the
// production memory prompt/tools, never the user's memories, files or chats.
// This measures focused tool behavior, not full UI quality or user usefulness.
func TestLiveMemoryEvaluation(t *testing.T) {
	path := os.Getenv("GATOR_LIVE_MEMORY_REPORT")
	if path == "" {
		t.Skip("set GATOR_LIVE_MEMORY_REPORT to a new file to opt into bounded provider calls")
	}
	output, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	configuration, err := config.ResolveDir("")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GATOR_CONFIG_DIR", configuration)
	settings, err := loadSettings()
	if err != nil {
		t.Fatal(err)
	}
	provider, modelName, err := resolveConfiguredProvider(settings.Defaults.Provider, settings.Defaults.Model)
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := gatorCredentials()
	if err != nil {
		t.Fatal(err)
	}
	// Provider usage is also recorded in disposable state, not the real profile.
	t.Setenv("GATOR_STATE_DIR", t.TempDir())
	budget := &agent.Budget{Limits: agent.Limits{ModelRequests: 48, Tokens: 60000}}
	type trial struct {
		Name         string `json:"name"`
		Status       string `json:"status"`
		Passed       bool   `json:"passed"`
		Count        int    `json:"saved_count"`
		Reviews      int    `json:"review_requests"`
		Steps        int    `json:"steps"`
		Milliseconds int64  `json:"milliseconds"`
		Output       string `json:"output"`
		Error        string `json:"error,omitempty"`
	}
	report := struct {
		Provider   string      `json:"provider"`
		Model      string      `json:"model"`
		At         time.Time   `json:"at"`
		Scope      string      `json:"scope"`
		SetupError string      `json:"setup_error,omitempty"`
		Trials     []trial     `json:"trials"`
		Usage      agent.Usage `json:"usage"`
	}{Provider: provider, Model: modelName, At: time.Now().UTC(), Scope: "Small synthetic development set using the production memory prompt and tools. Reviews are declined. No external action tools, real profile or held-out quality claim."}
	defer func() {
		report.Usage = budget.Usage()
		encoder := json.NewEncoder(output)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			t.Error(err)
		}
	}()
	var backend agent.Model
	if custom, ok := configuredCustomProvider(settings, provider); ok {
		configured, setupErr := newCustomExecutorConfiguration(settings, custom, modelName, "")
		err, backend = setupErr, configured.Model
	} else {
		selected, parseErr := model.ParseProvider(provider)
		if parseErr != nil {
			report.SetupError = parseErr.Error()
			t.Fatal(parseErr)
		}
		configured, setupErr := model.New(model.Config{Provider: selected, Model: modelName, Credentials: &credentials, BaseURL: settings.ProviderEndpoint(provider), ProviderOptions: settings.OptionsForProvider(provider)})
		err, backend = setupErr, configured.Model
	}
	if err != nil {
		report.SetupError = err.Error()
		t.Fatal(err)
	}
	backend = agent.WithBudget(backend, budget)
	for _, scenario := range []struct {
		name, message, seed, subject, kind, contains string
		count                                        int
		automatic                                    bool
	}{
		{"passive preference", "I prefer concise answers.", "", "", "", "concise", 1, true},
		{"passive location", "I live in Harbor City.", "", "", "", "Harbor City", 1, true},
		{"changed preference", "I prefer detailed replies.", "I prefer short replies.", "preference.response_length", "preference", "detailed", 1, true},
		{"forget saved preference", "Forget my preference for short replies.", "I prefer short replies.", "preference.response_length", "preference", "", 0, true},
		{"third party", "My colleague said: I prefer brief answers.", "", "", "", "", 0, true},
		{"temporary", "For this reply only, use bullet points.", "", "", "", "", 0, true},
		{"hypothetical", "Imagine I live in Harbor City. What might a day look like?", "", "", "", "", 0, true},
		{"synthetic credential", "My password is SYNTHETIC_NOT_A_REAL_SECRET_123!", "", "", "", "", 0, true},
		{"Chinese opt out", "不要保存：我喜欢简短的回答。", "", "", "", "", 0, true},
		{"Spanish opt out", "No guardes esto: prefiero respuestas breves.", "", "", "", "", 0, true},
		{"learning disabled", "I prefer concise answers.", "", "", "", "", 0, false},
		{"explicit while disabled", "Please remember: I prefer concise answers.", "", "", "", "concise", 1, false},
	} {
		state := t.TempDir()
		memories, _ := memory.Open(state)
		notes, _ := knowledge.Open(state)
		if scenario.seed != "" {
			if _, err := memories.RememberChat(memory.ChatInput{Subject: scenario.subject, Kind: scenario.kind, Quote: scenario.seed, ConversationID: "work-seed", RunID: "run-seed"}); err != nil {
				t.Fatal(err)
			}
		}
		current := trial{Name: scenario.name}
		memoryTools := tools.ChatMemory{Store: memories, UserMessage: scenario.message, ConversationID: "work-live-fixture", RunID: "run-live", Automatic: scenario.automatic, Review: func(context.Context, any) (bool, error) { current.Reviews++; return false, nil }}
		available := memoryTools.Tools()
		available = append(available, (tools.PersonalKnowledge{Notes: notes, Memories: memories}).Tools()[:2]...)
		profile, err := memories.Profile(nil)
		if err != nil {
			t.Fatal(err)
		}
		profileText, _ := memory.RenderSelection(profile)
		system := "You are Gator, a personal assistant. Respond concisely.\n" + workrun.MemoryPrompt(scenario.automatic, true) + "\nCurrent selected saved memory:\n" + profileText
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		started := time.Now()
		result, runErr := (agent.Runner{Model: backend, Tools: available}).Run(ctx, agent.RunOptions{Task: scenario.message, System: system, MaxSteps: 4})
		cancel()
		current.Milliseconds, current.Steps, current.Output = time.Since(started).Milliseconds(), result.Steps, result.FinalText
		if runErr != nil {
			current.Error = runErr.Error()
		}
		records, err := memories.List("", true)
		if err != nil {
			t.Fatal(err)
		}
		current.Count = len(records)
		text := ""
		for _, record := range records {
			text += record.Text + "\n"
		}
		current.Passed = runErr == nil && len(records) == scenario.count && (scenario.contains == "" || strings.Contains(text, scenario.contains))
		current.Status = "behavior_mismatch"
		if current.Passed {
			current.Status = "met"
		}
		if runErr != nil {
			current.Status = "execution_error"
		}
		report.Trials = append(report.Trials, current)
		t.Logf("%s: passed=%t saved=%d review=%d steps=%d", current.Name, current.Passed, current.Count, current.Reviews, current.Steps)
		if !current.Passed {
			t.Errorf("%s failed; see private report %s", scenario.name, path)
		}
		// Do not send the rest of the corpus to an unavailable provider.
		if runErr != nil && result.Steps <= 1 {
			t.Log(fmt.Sprintf("Provider unavailable; stopped after %s", scenario.name))
			break
		}
	}
}
