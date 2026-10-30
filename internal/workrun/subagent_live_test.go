package workrun

import (
	"context"
	"encoding/json"
	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/model"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Opt-in live smoke test of actual manager routing, not a statistical benchmark.
func TestLiveSubagentManager(t *testing.T) {
	reportPath := os.Getenv("GATOR_LIVE_SUBAGENT_REPORT")
	if reportPath == "" {
		t.Skip("opt-in live evaluation")
	}
	if _, err := os.Stat(reportPath); !os.IsNotExist(err) {
		t.Fatal("report path must be new")
	}
	selected := os.Getenv("GATOR_LIVE_MODEL")
	if selected == "" || os.Getenv("OPENAI_API_KEY") == "" {
		t.Fatal("live model and credential required")
	}
	backend, err := model.New(model.Config{Provider: model.OpenAI, Model: selected, BaseURL: os.Getenv("GATOR_LIVE_ENDPOINT")})
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "project.md"), []byte("# Project Lantern\nOwner: Mira\nDeadline: 2030-05-16\nStatus: draft\n"), 0600); err != nil {
		t.Fatal(err)
	}
	budget := &agent.Budget{Limits: agent.Limits{ModelRequests: 20, Tokens: 100000}}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	outcome, runErr := (Service{Executor: Executor{Model: agent.WithBudget(backend.Model, budget), StateDir: t.TempDir()}}).Execute(ctx, Request{SourcePath: root, Objective: "Use list_agents first. Start two independent source_researcher tasks with start_agent before awaiting either: one reads project.md for the project owner, the other reads project.md for the deadline. Await both to completion, then report the project name, owner and exact deadline. Do not change any files.", Mode: action.Inspect, Contract: artifact.InspectionContract(), MaxSteps: 12})
	completed := 0
	for _, item := range outcome.Manifest.Subagents {
		if item.Agent == "source_researcher" && item.Status == "completed" {
			completed++
		}
	}
	response := outcome.Result.FinalText
	passed := runErr == nil && completed == 2 && strings.Contains(response, "Mira") && strings.Contains(response, "2030-05-16")
	diagnostic := ""
	if runErr != nil {
		diagnostic = runErr.Error()
	}
	report := map[string]any{"model": selected, "passed": passed, "response": response, "subagents": outcome.Manifest.Subagents, "usage": budget.Usage(), "error": diagnostic, "at": time.Now().UTC()}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(reportPath, append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	if !passed {
		t.Fatalf("live delegation failed: %s", diagnostic)
	}
}
