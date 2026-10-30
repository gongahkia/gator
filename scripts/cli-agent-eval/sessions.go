package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/cliagent"
	"github.com/gongahkia/gator/internal/model"
	"github.com/gongahkia/gator/internal/orchestrator"
	"github.com/gongahkia/gator/internal/workrun"
)

// This evaluation uses a live manager and a live native worker, exercising the
// normal Work service and tool routing. It verifies native identity plus exact
// remembered facts, rather than accepting the manager's claim of completion.
func runSessionManager(ctx context.Context, cfg cliagent.Config, root, source, original string) error {
	backend, err := model.New(model.Config{Provider: model.OpenAI, Model: cfg.Model, BaseURL: strings.TrimRight(cfg.BaseURL, "/") + "/responses"})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	state := filepath.Join(root, "state")
	service := workrun.Service{Executor: workrun.Executor{Model: backend.Model, StateDir: state}}
	objective := fmt.Sprintf(`Test native conversation continuity using %s only. First call start_agent with this exact task: "Read project.md. Remember the codeword capybara-924 in this conversation. Report the project owner and deadline and confirm the codeword. Do not edit files." Wait for completion. Then call start_agent with continue pointing to that completed task and this exact task: "Without reading any files, what codeword did I ask you to remember? Who owns the project and what is its deadline?" Do not paste prior answers, the owner, deadline, or codeword into this second assignment. Wait for completion and report both results. Do not call another specialist or create files.`, cfg.Role())
	request := workrun.Request{SourcePath: source, RunID: "session-manager", Objective: objective, Mode: action.Inspect, Contract: artifact.InspectionContract(), ExternalAgents: []cliagent.Config{cfg}, MaxSteps: 16, Limits: agent.Limits{ModelRequests: 20, Tokens: 100000}}
	start := time.Now()
	_, runErr := service.Execute(ctx, request)
	tasks, taskErr := orchestrator.ReadTasks(filepath.Join(state, "gator", "tasks", "session-manager"))
	checks := map[string]bool{"work_succeeded": runErr == nil, "two_worker_turns": taskErr == nil && len(tasks) == 2}
	type evidence struct{ ID, Previous, SessionID, NativeThread, Assignment, Summary, Status string }
	items := []evidence{}
	var first, second *orchestrator.Task
	for i := range tasks {
		t := &tasks[i]
		thread := ""
		if t.Native != nil {
			thread = t.Native.ThreadID
		}
		items = append(items, evidence{t.ID, t.Previous, t.Result.SessionID, thread, t.SelectedInput, t.Result.Summary, t.Status})
		if t.Previous == "" {
			first = t
		} else {
			second = t
		}
	}
	checks["same_native_conversation"] = first != nil && second != nil && first.Result.SessionID != "" && first.Result.SessionID == second.Result.SessionID && first.Native != nil && second.Native != nil && first.Native.ThreadID == second.Native.ThreadID
	checks["followup_not_rehydrated"] = second != nil && !strings.Contains(second.SelectedInput, "capybara-924") && !strings.Contains(second.SelectedInput, "Mira") && !strings.Contains(second.SelectedInput, "2030-05-16")
	checks["remembered_facts"] = second != nil && second.Status == "completed" && recalledFixture(second.Result.Summary)
	current, readErr := os.ReadFile(filepath.Join(source, "project.md"))
	checks["original_unchanged"] = readErr == nil && string(current) == original
	passed := true
	for _, value := range checks {
		passed = passed && value
	}
	diagnostic := ""
	if err := errors.Join(runErr, taskErr); err != nil {
		diagnostic = strings.ReplaceAll(err.Error(), os.Getenv(cfg.APIKeyEnv), "[redacted]")
	}
	report := map[string]any{"kind": cfg.Kind, "model": cfg.Model, "evaluation": "live-manager-native-session-v1", "at": time.Now().UTC().Format(time.RFC3339), "passed": passed, "checks": checks, "tasks": items, "seconds": time.Since(start).Seconds(), "error": diagnostic}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		return err
	}
	if !passed {
		return errors.New("live manager/native continuity evaluation failed")
	}
	return nil
}

func recalledFixture(text string) bool {
	date := regexp.MustCompile(`(?i)\b(?:2030-05-16|May\s+16,?\s+2030|16\s+May\s+2030)\b`)
	return strings.Contains(text, "capybara-924") && strings.Contains(text, "Mira") && date.MatchString(text)
}
