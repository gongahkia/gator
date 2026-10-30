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
	"github.com/gongahkia/gator/internal/jobs"
	"github.com/gongahkia/gator/internal/knowledge"
	"github.com/gongahkia/gator/internal/model"
	"github.com/gongahkia/gator/internal/suggestion"
	"github.com/gongahkia/gator/internal/workrun"
)

// Explicitly opt-in, synthetic state only; keep unsuccessful reports too.
func TestLiveAgentContinuity(t *testing.T) {
	reportPath := os.Getenv("GATOR_LIVE_CONTINUITY_REPORT")
	if reportPath == "" {
		t.Skip("set GATOR_LIVE_CONTINUITY_REPORT, GATOR_LIVE_MODEL and OPENAI_API_KEY")
	}
	if os.Getenv("OPENAI_API_KEY") == "" || os.Getenv("GATOR_LIVE_MODEL") == "" {
		t.Fatal("live configuration missing")
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
	budget := &agent.Budget{Limits: agent.Limits{ModelRequests: 80, Tokens: 250000}}
	backend := agent.WithBudget(configured.Model, budget)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	type phase struct {
		Name         string              `json:"name"`
		Objective    string              `json:"objective"`
		Response     string              `json:"response"`
		Artifact     string              `json:"artifact,omitempty"`
		Decision     *jobs.WatchDecision `json:"decision,omitempty"`
		Tools        []string            `json:"tools"`
		ToolErrors   []string            `json:"tool_errors,omitempty"`
		Proposals    []suggestion.Record `json:"proposals,omitempty"`
		Milliseconds int64               `json:"milliseconds"`
		Error        string              `json:"error,omitempty"`
	}
	report := struct {
		Model        string              `json:"model"`
		Started      time.Time           `json:"started_at"`
		Phases       []phase             `json:"phases"`
		Checks       map[string]bool     `json:"checks"`
		Usage        agent.Usage         `json:"usage"`
		Continuation *jobs.WatchDecision `json:"continuation,omitempty"`
	}{Model: os.Getenv("GATOR_LIVE_MODEL"), Started: time.Now().UTC(), Checks: map[string]bool{}}
	t.Cleanup(func() {
		report.Usage = budget.Usage()
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			t.Error(err)
			return
		}
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
	goals, err := goal.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	notes, err := knowledge.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	proposals, err := suggestion.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	project, err := goals.Add("Community workshop", "Only track reported changes; use native local plans.", "")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := goals.InitializePlan(project.ID, "Prepare a workshop", []string{"Weekly preparation", "Deliver workshop"})
	if err != nil {
		t.Fatal(err)
	}
	approval, err := goals.Add("Budget review", "Wait for my approval; do not infer it.", "")
	if err != nil {
		t.Fatal(err)
	}
	approvalPlan, err := goals.InitializePlan(approval.ID, "Approve workshop budget", []string{"Approve budget"})
	if err != nil {
		t.Fatal(err)
	}
	note, err := notes.AddNoteWithRefs("Workshop date", "The workshop date is 2026-11-24. This was recorded from my initial plan.", "", []string{"user:initial-plan"})
	if err != nil {
		t.Fatal(err)
	}
	due, _ := time.Parse(time.RFC3339, "2026-11-27T10:00:00+08:00")
	plan, err = goals.ApplyPlanChange(project.ID, goal.PlanChange{Kind: "update_step", ExpectedRevision: plan.Revision, StepID: plan.Steps[1].ID, DueAt: due})
	if err != nil {
		t.Fatal(err)
	}
	run := func(name, objective string, acceptKind string) phase {
		t.Log("Live phase:", name)
		p := phase{Name: name, Objective: objective}
		started := time.Now()
		outcome, err := (workrun.Service{Executor: workrun.Executor{Model: backend, StateDir: state}}).Execute(ctx, workrun.Request{SourcePath: project.SourcePath, Objective: objective, ForegroundApp: true, DisableBrowser: true, DisableDelegation: true, DisableChatHistory: true, Mode: action.Inspect, Contract: artifact.InspectionContract(), Limits: agent.Limits{ModelRequests: 12, WallSeconds: 120}, OnEvent: func(e agent.Event) {
			if e.ToolError != "" {
				p.ToolErrors = append(p.ToolErrors, e.ToolError)
			}
			if e.Kind == agent.EventToolCalled && e.ToolCall != nil {
				p.Tools = append(p.Tools, e.ToolCall.Name)
			}
		}})
		p.Response = outcome.Result.FinalText
		if err != nil {
			p.Error = err.Error()
		}
		p.Proposals, err = proposals.List("pending")
		if err != nil {
			t.Fatal(err)
		}
		if acceptKind != "" {
			check(name+": exactly one intended proposal", len(p.Proposals) == 1 && p.Proposals[0].Kind == acceptKind && p.Proposals[0].GoalID == project.ID)
			if len(p.Proposals) == 1 && p.Proposals[0].Kind == acceptKind && p.Proposals[0].GoalID == project.ID {
				if err := suggestionCommand([]string{"accept", p.Proposals[0].ID}, &bytes.Buffer{}); err != nil {
					p.Error = err.Error()
				}
			}
		}
		p.Milliseconds = time.Since(started).Milliseconds()
		report.Phases = append(report.Phases, p)
		if p.Error != "" {
			t.Fatal(name + ": " + p.Error)
		}
		return p
	}
	run("weekly-block", "In Community workshop, schedule the existing Weekly preparation step every Monday from 09:00 to 10:00 Asia/Singapore, beginning 2026-11-02. Use a weekly recurring native plan block, interval 1, with no deadline. Keep the step pending and propose exactly one change for review. Do not create a reminder or duplicate step.", "plan")
	plan, err = goals.LoadPlan(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	check("weekly recurrence persisted", plan.Steps[0].Recurrence != nil && plan.Steps[0].Recurrence.Frequency == "weekly" && plan.Steps[0].Recurrence.Timezone == "Asia/Singapore" && plan.Steps[0].StartsAt.Equal(time.Date(2026, 11, 2, 1, 0, 0, 0, time.UTC)) && plan.Steps[0].EndsAt.Equal(time.Date(2026, 11, 2, 2, 0, 0, 0, time.UTC)))
	run("complete-one-occurrence", "I completed only the 2026-11-02 Weekly preparation occurrence in Community workshop. Propose that occurrence as done. Keep the recurring series pending and all future occurrences unchanged. Exactly one plan proposal, no other changes.", "plan")
	plan, err = goals.LoadPlan(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	check("occurrence done leaves series pending", plan.Steps[0].Status == goal.StepPending && len(plan.Steps[0].Occurrences) == 1 && plan.Steps[0].Occurrences[0].Date == "2026-11-02" && plan.Steps[0].Occurrences[0].Status == "done")
	run("cross-project-dependency", "Community workshop's Deliver workshop step depends on the Approve budget step in my separate Budget review project. Inspect both plans and propose exactly that dependency. Preserve all dates and statuses. Do not create new steps or infer budget approval.", "plan")
	plan, err = goals.LoadPlan(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	ref := approval.ID + "/" + approvalPlan.Steps[0].ID
	check("cross-project dependency persisted", len(plan.Steps[1].DependsOn) == 1 && plan.Steps[1].DependsOn[0] == ref)
	states, err := goals.DependencyStates(plan)
	if err != nil {
		t.Fatal(err)
	}
	check("external pending approval remains blocker", states[ref] == goal.StepPending)
	before := plan.Revision
	conflict := run("conflicting-date-review", "Compare my saved Workshop date note with the Community workshop Deliver workshop deadline. They disagree. Inspect both, then retain exactly one conflict review finding with the exact source IDs, revisions and quotes. Do not choose a true date, update either source, or make any other proposal.", "")
	check("conflict finding retained", len(conflict.Proposals) == 1 && conflict.Proposals[0].Kind == "conflict" && len(conflict.Proposals[0].ConflictSources) == 2)
	after, _ := goals.LoadPlan(project.ID)
	sameNote, _ := notes.LoadNote(note.ID)
	check("conflict detection preserves both sources", after.Revision == before && sameNote.Revision() == note.Revision())
	if len(conflict.Proposals) == 1 && conflict.Proposals[0].Kind == "conflict" {
		// A later user report makes the proposal stale, so acceptance must fail.
		_, err = goals.ApplyPlanChange(project.ID, goal.PlanChange{Kind: "update_step", ExpectedRevision: after.Revision, StepID: after.Steps[1].ID, DueAt: due.AddDate(0, 0, 1)})
		if err != nil {
			t.Fatal(err)
		}
		err = suggestionCommand([]string{"accept", conflict.Proposals[0].ID}, &bytes.Buffer{})
		check("stale conflict acceptance refused", err != nil && strings.Contains(err.Error(), "changed"))
		if err := suggestionCommand([]string{"reject", conflict.Proposals[0].ID}, &bytes.Buffer{}); err != nil {
			t.Fatal(err)
		}
	}
	// Use the production watch request, output validator and durable attempt path.
	store, err := jobs.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	definition, err := store.Save(jobs.Definition{Name: "Budget check", SourcePath: approval.SourcePath, GoalID: approval.ID, GoalWatch: true, PollMinutes: 15, Schedule: "*/15 * * * *", RefreshSnapshot: true, Missed: "run_once", Contract: artifact.InspectionContract(), Timezone: "UTC", Mode: action.Inspect, ChangeOnly: true, Objective: "Inspect the budget plan. There is no new user approval. Stay quiet and wait for the owner; do not propose changes. Retain a waiting continuation to check again in thirty minutes, selecting the existing approval step. Do not infer completion.", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := store.Begin(definition, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	t.Log("Live phase: retained-watch-continuation")
	p := phase{Name: "retained-watch-continuation", Objective: definition.Objective}
	started := time.Now()
	attempt = executeJobContextWith(ctx, definition, attempt, state, false, func(ctx context.Context, d jobs.Definition, dir string, ids ...string) (jobResult, error) {
		return runJobProcessWith(ctx, d, dir, func(ctx context.Context, _, _, _ string, req workrun.Request) (workrun.Outcome, error) {
			req.DisableBrowser, req.DisableDelegation, req.DisableChatHistory = true, true, true
			req.Limits = agent.Limits{ModelRequests: 12, WallSeconds: 120}
			req.OnEvent = func(e agent.Event) {
				if e.ToolError != "" {
					p.ToolErrors = append(p.ToolErrors, e.ToolError)
				}
				if e.Kind == agent.EventToolCalled && e.ToolCall != nil {
					p.Tools = append(p.Tools, e.ToolCall.Name)
				}
			}
			outcome, err := (workrun.Service{Executor: workrun.Executor{Model: backend, StateDir: state}}).Execute(ctx, req)
			p.Response = outcome.Result.FinalText
			return outcome, err
		}, ids...)
	})
	p.Milliseconds = time.Since(started).Milliseconds()
	p.Error = attempt.Error
	report.Phases = append(report.Phases, p)
	report.Continuation = attempt.WatchDecision
	check("live watch waiting without inferred completion", attempt.Status == "completed" && attempt.WatchDecision != nil && attempt.WatchDecision.Continuation == "waiting" && attempt.WatchDecision.Status == "quiet" && !attempt.WatchDecision.NextCheckAt.IsZero())
	if err := store.Record(attempt); err != nil {
		t.Fatal(err)
	}
	reopened, err := jobs.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	history, err := reopened.History(definition.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	policy := jobNotificationPolicyHash(definition)
	if attempt.WatchDecision != nil && !attempt.WatchDecision.NextCheckAt.IsZero() {
		check("wake survives reopening store", goalContinuationDue(history, jobs.Attempt{}, policy, attempt.WatchDecision.NextCheckAt))
	}
	approvalPlan, _ = goals.LoadPlan(approval.ID)
	check("watch did not mark approval done", approvalPlan.Steps[0].Status == goal.StepPending)

	// Seed a previously validated ready decision, then exercise its actual draft
	// execution through the unchanged-input gate and normal Work artifact checks.
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "workshop.txt"), []byte("Purpose: teach residents basic home composting.\nAgenda: introduction, hands-on mixing demo, questions.\nUnknown: venue capacity and budget approval. No approval has been given.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	drafting, err := goals.Add("Composting brief", "Draft only; never send or assume approval.", source)
	if err != nil {
		t.Fatal(err)
	}
	draftPlan, err := goals.InitializePlan(drafting.ID, "Prepare a clear workshop brief", []string{"Draft brief"})
	if err != nil {
		t.Fatal(err)
	}
	contract := artifact.DefaultContract("brief.md")
	draftDefinition, err := store.Save(jobs.Definition{Name: "Draft continuation", SourcePath: source, GoalID: drafting.ID, GoalWatch: true, PollMinutes: 15, Schedule: "*/15 * * * *", Timezone: "UTC", Mode: action.Draft, Contract: contract, ChangeOnly: true, RefreshSnapshot: true, Missed: "run_once", Enabled: true, Objective: "Carry out the retained ready next action: read workshop.txt and draft brief.md, a concise workshop brief with headings Purpose, Agenda, and Open questions. Preserve the unknown venue capacity and unapproved budget. Check the artifact before reporting. Do not send anything, propose plan changes, or mark the owner's step done. Once drafted, use a blocked continuation with no wake while the owner reviews the draft."})
	if err != nil {
		t.Fatal(err)
	}
	prior, err := store.Begin(draftDefinition, time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	decision := checkedGoalContinuation(jobs.WatchDecision{Status: "quiet"}, goalWatchDecision{Continuation: "ready", NextStepID: draftPlan.Steps[0].ID, NextStep: "Read workshop.txt and create brief.md for owner review."}, draftPlan, draftDefinition, time.Now().Add(-time.Hour))
	prior.WatchDecision = &decision
	prior.InputSHA256, err = localJobInputHash(draftDefinition, state)
	if err != nil {
		t.Fatal(err)
	}
	prior.NotificationPolicySHA256 = jobNotificationPolicyHash(draftDefinition)
	prior.Status, prior.FinishedAt = "completed", time.Now().UTC()
	if err := store.Record(prior); err != nil {
		t.Fatal(err)
	}
	draftAttempt, err := store.Begin(draftDefinition, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	p = phase{Name: "ready-continuation-produces-reviewed-draft", Objective: draftDefinition.Objective}
	started = time.Now()
	t.Log("Live phase:", p.Name)
	calls := 0
	draftAttempt = executeJobContextWith(ctx, draftDefinition, draftAttempt, state, false, func(ctx context.Context, d jobs.Definition, dir string, ids ...string) (jobResult, error) {
		calls++
		return runJobProcessWith(ctx, d, dir, func(ctx context.Context, _, _, _ string, req workrun.Request) (workrun.Outcome, error) {
			req.DisableBrowser, req.DisableDelegation, req.DisableChatHistory = true, true, true
			req.Limits = agent.Limits{ModelRequests: 12, WallSeconds: 120}
			req.OnEvent = func(e agent.Event) {
				if e.ToolError != "" {
					p.ToolErrors = append(p.ToolErrors, e.ToolError)
				}
				if e.Kind == agent.EventToolCalled && e.ToolCall != nil {
					p.Tools = append(p.Tools, e.ToolCall.Name)
				}
			}
			outcome, err := (workrun.Service{Executor: workrun.Executor{Model: backend, StateDir: state}}).Execute(ctx, req)
			p.Response = outcome.Result.FinalText
			return outcome, err
		}, ids...)
	})
	p.Milliseconds = time.Since(started).Milliseconds()
	p.Error = draftAttempt.Error
	p.Decision = draftAttempt.WatchDecision
	data, readErr := os.ReadFile(filepath.Join(draftAttempt.BundlePath, "output", "brief.md"))
	p.Artifact = string(data)
	report.Phases = append(report.Phases, p)
	check("ready wake executes through unchanged-input gate", calls == 1 && draftAttempt.Status == "completed")
	check("ready continuation creates checked draft artifact", readErr == nil && strings.Contains(string(data), "Purpose") && strings.Contains(string(data), "Agenda") && strings.Contains(string(data), "Open questions"))
	check("draft continuation holds for owner review", draftAttempt.WatchDecision != nil && draftAttempt.WatchDecision.Continuation == "blocked" && draftAttempt.WatchDecision.NextCheckAt.IsZero())
	draftPlan, err = goals.LoadPlan(drafting.ID)
	if err != nil {
		t.Fatal(err)
	}
	check("draft does not infer owner task completion", draftPlan.Steps[0].Status == goal.StepPending)
	if err := store.Record(draftAttempt); err != nil {
		t.Fatal(err)
	}
	t.Log(fmt.Sprintf("Live continuity checks: %d; requests: %d", len(report.Checks), budget.Usage().ModelRequests))
}
