package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/knowledge"
	"github.com/gongahkia/gator/internal/model"
	"github.com/gongahkia/gator/internal/suggestion"
	"github.com/gongahkia/gator/internal/workrun"
)

// Opt-in only: consumes real provider requests, always against a fresh synthetic
// profile. Regular CI never invokes the provider. Reports contain synthetic data.
func TestLiveInternalAgentLifecycle(t *testing.T) {
	reportPath := os.Getenv("GATOR_LIVE_INTERNAL_REPORT")
	if reportPath == "" {
		t.Skip("set GATOR_LIVE_INTERNAL_REPORT, GATOR_LIVE_MODEL and OPENAI_API_KEY for a bounded live run")
	}
	if os.Getenv("OPENAI_API_KEY") == "" || os.Getenv("GATOR_LIVE_MODEL") == "" {
		t.Fatal("live provider configuration missing")
	}
	if _, err := os.Stat(reportPath); !os.IsNotExist(err) {
		t.Fatal("report path must be new")
	}
	state := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", state)
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	configured, err := model.New(model.Config{Provider: model.OpenAI, Model: os.Getenv("GATOR_LIVE_MODEL"), BaseURL: os.Getenv("GATOR_LIVE_ENDPOINT")})
	if err != nil {
		t.Fatal(err)
	}
	budget := &agent.Budget{Limits: agent.Limits{ModelRequests: 100, Tokens: 300000}}
	backend := agent.WithBudget(configured.Model, budget)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	type phase struct {
		Name         string              `json:"name"`
		Objective    string              `json:"objective"`
		Response     string              `json:"response"`
		Tools        []string            `json:"tools"`
		ToolErrors   []string            `json:"tool_errors,omitempty"`
		Proposals    []suggestion.Record `json:"proposals,omitempty"`
		Milliseconds int64               `json:"milliseconds"`
		Usage        agent.Usage         `json:"usage_cumulative"`
		Error        string              `json:"error,omitempty"`
	}
	report := struct {
		Model     string          `json:"model"`
		Started   time.Time       `json:"started_at"`
		Phases    []phase         `json:"phases"`
		Checks    map[string]bool `json:"checks"`
		Usage     agent.Usage     `json:"usage"`
		NoteBody  string          `json:"note_body"`
		Plan      goal.Plan       `json:"plan"`
		Baselines []phase         `json:"decision_baselines"`
	}{Model: os.Getenv("GATOR_LIVE_MODEL"), Started: time.Now().UTC(), Checks: map[string]bool{}}
	t.Cleanup(func() {
		report.Usage = budget.Usage()
		data, _ := json.MarshalIndent(report, "", "  ")
		if err := os.MkdirAll(filepath.Dir(reportPath), 0700); err != nil {
			t.Error(err)
			return
		}
		if err := os.WriteFile(reportPath, append(data, '\n'), 0600); err != nil {
			t.Error(err)
		}
	})
	check := func(name string, ok bool) {
		report.Checks[name] = ok
		if !ok {
			t.Errorf("live check failed: %s", name)
		}
	}
	notes, _ := knowledge.Open(state)
	goals, _ := goal.Open(state)
	queue, _ := suggestion.Open(state)
	note, err := notes.AddNoteWithRefs("Community workshop project", "Target date: 2026-11-20. Status: proposed; budget unapproved.", "", []string{"user:initial-plan"})
	if err != nil {
		t.Fatal(err)
	}
	record, err := goals.Add("Community workshop", "Track only what I report; no connected accounts.", "")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := goals.InitializePlan(record.ID, "Prepare a workshop", []string{"Budget approval", "Workshop outline"})
	if err != nil {
		t.Fatal(err)
	}
	firstID, secondID := plan.Steps[0].ID, plan.Steps[1].ID
	run := func(name, objective, scope string, accept bool) phase {
		t.Log("Live phase:", name)
		current := phase{Name: name, Objective: objective}
		started := time.Now()
		outcome, err := (workrun.Service{Executor: workrun.Executor{Model: backend, StateDir: state}}).Execute(ctx, workrun.Request{SourcePath: record.SourcePath, GoalID: scope, Objective: objective, ForegroundApp: scope == "", DisableBrowser: true, DisableDelegation: true, DisableChatHistory: true, Mode: action.Inspect, Contract: artifact.InspectionContract(), Limits: agent.Limits{ModelRequests: 12, WallSeconds: 120}, OnEvent: func(event agent.Event) {
			if event.ToolError != "" {
				current.ToolErrors = append(current.ToolErrors, event.ToolError)
			}
			if event.Kind == agent.EventToolCalled && event.ToolCall != nil {
				current.Tools = append(current.Tools, event.ToolCall.Name)
			}
		}})
		current.Response = outcome.Result.FinalText
		if err != nil {
			current.Error = err.Error()
			t.Errorf("%s: %v", name, err)
		}
		pending, err := queue.List("pending")
		if err != nil {
			t.Fatal(err)
		}
		current.Proposals = pending
		if accept {
			for _, proposal := range pending {
				if proposal.Kind == "note" && proposal.NoteID == note.ID || proposal.Kind == "plan" && proposal.GoalID == record.ID {
					if err := suggestionCommand([]string{"accept", proposal.ID}, &bytes.Buffer{}); err != nil {
						t.Errorf("%s acceptance: %v", name, err)
						current.Error = err.Error()
					}
				} else {
					t.Errorf("%s proposed an unexpected mutation: %s", name, proposal.Kind)
				}
			}
		}
		current.Milliseconds = time.Since(started).Milliseconds()
		current.Usage = budget.Usage()
		report.Phases = append(report.Phases, current)
		if current.Error != "" {
			t.Fatal("stopping live workflow after execution failure; partial report retained")
		}
		return current
	}
	run("correct-existing-note", "Find my saved Community workshop project note. Update that existing note: the target date is now 2026-11-24 and it remains a proposal with unapproved budget. Keep its identity and source history; do not create a duplicate or store project content as profile memory. Prepare the exact update for my review.", "", true)
	updated, _ := notes.LoadNote(note.ID)
	all, _ := notes.ListNotes("")
	report.NoteBody = updated.Body
	check("original note updated without duplicate", len(all) == 1 && strings.Contains(updated.Body, "2026-11-24") && !strings.Contains(updated.Body, "2026-11-20"))
	check("original note source retained", strings.Contains(strings.Join(updated.SourceRefs, " "), "user:initial-plan"))
	start := time.Now().UTC().Truncate(time.Minute).Add(24 * time.Hour)
	end := start.Add(time.Hour)
	due := end.Add(24 * time.Hour)
	run("schedule-native-step", fmt.Sprintf("In my Community workshop goal, I have started Budget approval. Set that existing step to doing, with a work block from %s to %s and deadline %s. Record this as my reported update. Propose the change for review; do not create a new goal, reminder or connector.", start.Format(time.RFC3339), end.Format(time.RFC3339), due.Format(time.RFC3339)), "", true)
	plan, _ = goals.LoadPlan(record.ID)
	check("chat scheduled exact existing step", plan.Steps[0].ID == firstID && plan.Steps[0].Status == goal.StepDoing && plan.Steps[0].StartsAt.Equal(start) && plan.Steps[0].EndsAt.Equal(end) && plan.Steps[0].DueAt.Equal(due))
	run("add-native-dependency", fmt.Sprintf("For my existing Workshop outline step in Community workshop, it depends on Budget approval. Schedule the outline from %s to %s. Keep its current pending status and propose this update for review. An overlap is intentional for this test; point it out but don't move either block.", start.Add(30*time.Minute).Format(time.RFC3339), end.Add(time.Hour).Format(time.RFC3339)), "", true)
	plan, _ = goals.LoadPlan(record.ID)
	agenda, _ := goals.Agenda("", time.Now(), time.Now().Add(7*24*time.Hour))
	check("dependency preserved on original step", plan.Steps[1].ID == secondID && len(plan.Steps[1].DependsOn) == 1 && plan.Steps[1].DependsOn[0] == firstID)
	check("agenda reports dependency and overlap", len(agenda.Items) == 2 && len(agenda.Items[0].Conflicts) > 0 && len(agenda.Items[1].BlockedBy) > 0)
	fresh := run("fresh-chat-recall", "In a fresh chat, read my saved Community workshop project note and local agenda. Tell me the target date, unfinished work, unresolved dependency and scheduling overlap. Do not change anything or imply the budget is approved.", "", false)
	check("fresh chat recalls corrected date", strings.Contains(fresh.Response, "24") && !strings.Contains(fresh.Response, "November 20") && !strings.Contains(fresh.Response, "2026-11-20"))
	check("read-only recall queued no changes", len(fresh.Proposals) == 0)
	run("reported-completion", "Update Community workshop: I have now completed Budget approval. Mark only that existing step done, with my update as evidence, for review. Workshop outline is still pending; do not change or reschedule it.", "", true)
	plan, _ = goals.LoadPlan(record.ID)
	agenda, _ = goals.Agenda("", time.Now(), time.Now().Add(7*24*time.Hour))
	check("completion only affects reported step", plan.Steps[0].Status == goal.StepDone && plan.Steps[1].Status == goal.StepPending && len(agenda.Items) == 1 && len(agenda.Items[0].BlockedBy) == 0)
	// Two live decision cases share exact plan state with a text-only baseline.
	// These assess decisions; they do not claim an end-to-end baseline can persist data.
	for _, overdue := range []bool{false, true} {
		if overdue {
			plan, err = goals.ApplyPlanChange(record.ID, goal.PlanChange{Kind: "update_step", ExpectedRevision: plan.Revision, StepID: secondID, DueAt: time.Now().Add(-time.Hour)})
			if err != nil {
				t.Fatal(err)
			}
		}
		name := "quiet-check"
		if overdue {
			name = "overdue-check"
		}
		task := "Check this local goal's current plan. Notify only if an unfinished step is overdue now. Future blocks and already completed steps alone are not alerts. Treat statuses as last reported. Inspect the current plan; do not propose changes or claim actions. Return only JSON with attention (boolean), summary and evidence (array of exact 'goal plan step STEP_ID' references)."
		decision := run(name, task, record.ID, false)
		parsed, valid := parseGoalWatchDecision(decision.Response)
		check(name+" decision", valid && *parsed.Attention == overdue)
		check(name+" has no mutations", len(decision.Proposals) == 0)
		if overdue {
			check("overdue alert cites the actual pending step", valid && strings.Contains(strings.Join(parsed.Evidence, " "), secondID))
		}
		data, _ := json.Marshal(plan)
		started := time.Now()
		turn, err := backend.Complete(ctx, agent.TurnRequest{System: "Answer from this synthetic user-maintained plan. Current time: " + time.Now().Format(time.RFC3339), Messages: []agent.Message{{Role: agent.RoleUser, Content: task + "\nPlan: " + string(data)}}})
		baseline := phase{Name: name, Objective: task, Response: turn.Text, Milliseconds: time.Since(started).Milliseconds(), Usage: budget.Usage()}
		if err != nil {
			baseline.Error = err.Error()
			t.Error(err)
		}
		report.Baselines = append(report.Baselines, baseline)
	}
	report.Plan, _ = goals.LoadPlan(record.ID)
	check("live checks did not complete overdue work", report.Plan.Steps[1].Status == goal.StepPending)
}
