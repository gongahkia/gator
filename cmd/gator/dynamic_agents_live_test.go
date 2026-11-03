package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/model/openai"
	"github.com/gongahkia/gator/internal/orchestrator"
	"github.com/gongahkia/gator/internal/workrun"
)

// Opt-in synthetic acceptance. The same cumulative competitive-evaluation
// transport reserves money before dispatch; no network search or real accounts.
func TestLiveDynamicAgents(t *testing.T) {
	path := os.Getenv("GATOR_LIVE_DYNAMIC_AGENTS_REPORT")
	if path == "" {
		t.Skip("opt-in budgeted dynamic specialist evaluation")
	}
	if os.Getenv("OPENAI_API_KEY") == "" {
		t.Fatal("provider key unavailable")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	state := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", state)
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	type phase struct {
		Name          string
		Milliseconds  int64
		Tools, Errors []string
		Answer        string
		Tasks         []orchestrator.Task
		Usage         agent.Usage
		Error         string
	}
	report := struct {
		Phases []phase
		Checks map[string]bool
		Spend  projectEvalSpend
	}{Checks: map[string]bool{}}
	defer func() {
		data, _ := os.ReadFile(filepath.Join(filepath.Dir(path), "budget.json"))
		_ = json.Unmarshal(data, &report.Spend)
		e := json.NewEncoder(f)
		e.SetIndent("", "  ")
		if err := e.Encode(report); err != nil {
			t.Error(err)
		}
	}()
	check := func(name string, ok bool) {
		t.Helper()
		report.Checks[name] = ok
		if !ok {
			t.Error(name)
		}
	}
	backend := projectEvalModel{openai.Responses{APIKey: os.Getenv("OPENAI_API_KEY"), Model: "gpt-5.6", BaseURL: "https://us.api.openai.com/v1/responses", Client: &http.Client{Transport: projectEvalTransport{directory: filepath.Dir(path)}, Timeout: 60 * time.Second}}}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	run := func(name, sourceText, objective string) phase {
		t.Helper()
		t.Log("Live specialist phase:", name)
		source := t.TempDir()
		if err := os.WriteFile(filepath.Join(source, "project.md"), []byte(sourceText), 0600); err != nil {
			t.Fatal(err)
		}
		p := phase{Name: name}
		start := time.Now()
		runID := "work-live-dynamic-" + name
		out, err := (workrun.Executor{Model: backend, StateDir: state}).Execute(ctx, workrun.Request{
			RunID: runID, SourcePath: source, Objective: objective, Mode: action.Inspect, Contract: artifact.InspectionContract(), MaxSteps: 8, ForegroundApp: true, DisableBrowser: true, DisableChatHistory: true,
			Limits: agent.Limits{ModelRequests: 12, Tokens: 120000, WallSeconds: 110},
			OnEvent: func(e agent.Event) {
				if e.Kind == agent.EventToolCalled && e.ToolCall != nil {
					p.Tools = append(p.Tools, e.ToolCall.Name)
				}
				if e.ToolError != "" {
					p.Errors = append(p.Errors, e.ToolError)
				}
			},
		})
		p.Milliseconds, p.Answer, p.Usage = time.Since(start).Milliseconds(), out.Result.FinalText, out.Manifest.Usage
		if err != nil {
			p.Error = err.Error()
			t.Errorf("%s: %v", name, err)
		}
		p.Tasks, err = orchestrator.ReadTasks(filepath.Join(state, "gator", "tasks", runID))
		if err != nil {
			t.Error(err)
		}
		report.Phases = append(report.Phases, p)
		return p
	}
	first := run("create", "# Community site\nOriginal client review: 20 November 2026.\nConfirmed update: review moved to 18 November 2026.\nUnresolved blocker: venue photographs.\n", "Create a specialist called deadline_researcher for checking project deadlines and contradictory dated claims in the selected local files. Save it for future chats. Use it now to investigate the actual client review deadline and unresolved blocker. Wait for its findings, then give me a concise answer with source references. This is a local read-only investigation; do not schedule anything or contact anyone.")
	check("created specialist actually completed", len(first.Tasks) == 1 && first.Tasks[0].Role == "deadline_researcher" && first.Tasks[0].Status == "completed")
	if len(first.Tasks) == 1 {
		check("definition captured with execution identity", first.Tasks[0].Profile != nil && first.Tasks[0].Profile.Saved && first.Tasks[0].Profile.Definition.Base == "source_researcher" && first.Tasks[0].Configuration.DefinitionSHA256 == first.Tasks[0].Profile.Digest)
		check("worker inspected selected source", strings.Contains(strings.Join(first.Tasks[0].Result.InspectedPaths, ","), "source/project.md"))
	}
	check("manager returned dated findings", strings.Contains(first.Answer, "18 November") && strings.Contains(strings.ToLower(first.Answer), "photo"))
	store, _ := orchestrator.NewProfileStore(state)
	profiles, err := store.List()
	check("explicit reusable profile persisted", err == nil && len(profiles) == 1 && profiles[0].Name == "deadline_researcher")
	if err != nil || len(profiles) != 1 {
		return
	}
	second := run("reuse", "# Different project\nConfirmed client review: 21 November 2026.\nBlockers: none reported.\n", "Use my saved deadline_researcher specialist on this newly selected project. Investigate the review deadline and blockers, wait for its findings, then answer briefly with source references. Reuse the saved definition without redefining it. Do not schedule anything or contact anyone.")
	check("saved specialist executed in new conversation", len(second.Tasks) == 1 && second.Tasks[0].Role == "deadline_researcher" && second.Tasks[0].Status == "completed")
	check("reuse did not redefine specialist", !strings.Contains(strings.Join(second.Tools, ","), "create_agent"))
	check("new source used without stale prior date", strings.Contains(second.Answer, "21 November") && !strings.Contains(second.Answer, "18 November"))
}
