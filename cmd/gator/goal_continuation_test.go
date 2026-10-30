package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/jobs"
)

func TestGoalContinuationWakeAndStall(t *testing.T) {
	now := time.Now().UTC()
	plan := goal.Plan{Steps: []goal.Step{{ID: "step-a", Status: goal.StepPending}}}
	proposed := goalWatchDecision{Continuation: "ready", NextStepID: "step-a", NextStep: "Draft the outline"}
	d := checkedGoalContinuation(jobs.WatchDecision{Status: "quiet"}, proposed, plan, jobs.Definition{PollMinutes: 15}, now)
	if d.Continuation != "ready" || !d.NextCheckAt.Equal(now.Add(15*time.Minute)) {
		t.Fatalf("continuation: %#v", d)
	}
	prior := jobs.Attempt{ID: "prior", Status: "completed", InputSHA256: "input", NotificationPolicySHA256: "policy", WatchDecision: &d}
	quiet := jobs.Attempt{ID: "quiet", Status: "unchanged"}
	if goalContinuationDue([]jobs.Attempt{quiet, prior}, jobs.Attempt{}, "policy", now) {
		t.Fatal("woke early")
	}
	if !goalContinuationDue([]jobs.Attempt{quiet, prior}, jobs.Attempt{}, "policy", d.NextCheckAt) {
		t.Fatal("lost wake across unchanged attempts")
	}
	if goalContinuationDue([]jobs.Attempt{prior}, jobs.Attempt{}, "changed-policy", d.NextCheckAt) {
		t.Fatal("obsolete policy resumed")
	}
	current := prior
	current.ID = "current"
	older := prior
	older.ID = "older"
	held := guardStalledContinuation([]jobs.Attempt{prior, quiet, older}, current)
	if !held.WatchAttention || held.WatchDecision.Continuation != "blocked" || !held.WatchDecision.NextCheckAt.IsZero() {
		t.Fatalf("stalled loop: %#v", held)
	}
	current.InputSHA256 = "new-input"
	if guardStalledContinuation([]jobs.Attempt{prior, older}, current).WatchDecision.Continuation != "ready" {
		t.Fatal("new evidence did not reset bound")
	}
}

func TestGoalContinuationRejectsInvalidAuthorityAndTiming(t *testing.T) {
	now := time.Now()
	plan := goal.Plan{Steps: []goal.Step{{ID: "a", Status: goal.StepPending, DependsOn: []string{"b"}}, {ID: "b", Status: goal.StepPending}}}
	for _, proposed := range []goalWatchDecision{
		{Continuation: "ready", NextStepID: "a", NextStep: "Start blocked work"},
		{Continuation: "ready", NextStepID: "missing", NextStep: "Invent work"},
		{Continuation: "waiting"},
		{Continuation: "waiting", NextCheckAt: now.Add(time.Minute)},
		{Continuation: "waiting", NextCheckAt: now.Add(8 * 24 * time.Hour)},
		{Continuation: "idle", NextCheckAt: now.Add(time.Hour)},
	} {
		d := checkedGoalContinuation(jobs.WatchDecision{Status: "quiet"}, proposed, plan, jobs.Definition{}, now)
		if d.Status != "review" || !d.NextCheckAt.IsZero() {
			t.Fatalf("invalid continuation accepted: %#v", d)
		}
	}
}

func TestGoalContinuationReopensDurableWakeAfterQuietPoll(t *testing.T) {
	state := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", state)
	goals, err := goal.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	record, err := goals.Add("Waiting approval", "", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	plan, err := goals.InitializePlan(record.ID, "Approve budget", []string{"Approval"})
	if err != nil {
		t.Fatal(err)
	}
	if err := goalCommand([]string{"watch", record.ID, "--task", "Wait for reported approval"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	store, err := jobs.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	definitions, err := store.List()
	if err != nil || len(definitions) != 1 {
		t.Fatalf("definitions: %v %v", definitions, err)
	}
	definition := definitions[0]
	input, err := localJobInputHash(definition, state)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for index, status := range []string{"completed", "unchanged"} {
		attempt, err := store.Begin(definition, now.Add(time.Duration(index-2)*time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		attempt.Status, attempt.FinishedAt = status, time.Now().UTC()
		attempt.InputSHA256, attempt.NotificationPolicySHA256 = input, jobNotificationPolicyHash(definition)
		if status == "completed" {
			attempt.WatchDecision = &jobs.WatchDecision{Status: "quiet", Continuation: "waiting", NextStepID: plan.Steps[0].ID, NextCheckAt: now.Add(-time.Minute)}
		}
		if err := store.Record(attempt); err != nil {
			t.Fatal(err)
		}
	}
	store, err = jobs.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	var view bytes.Buffer
	if err := renderGoalActivity(state, record.ID, &view); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(view.String(), "retained continuation: waiting") {
		t.Fatalf("lost retained wake: %s", view.String())
	}
	calls := 0
	execute := func(context.Context, jobs.Definition, string, ...string) (jobResult, error) {
		calls++
		return jobResult{Status: "completed", WatchDecision: &jobs.WatchDecision{Status: "quiet", Continuation: "idle"}}, nil
	}
	current, err := store.Begin(definition, now)
	if err != nil {
		t.Fatal(err)
	}
	current = executeJobContextWith(context.Background(), definition, current, state, false, execute)
	if calls != 1 || current.Status != "completed" {
		t.Fatalf("wake skipped: calls=%d attempt=%#v", calls, current)
	}
	if err := store.Record(current); err != nil {
		t.Fatal(err)
	}
	current, err = store.Begin(definition, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	current = executeJobContextWith(context.Background(), definition, current, state, false, execute)
	if calls != 1 || current.Status != "unchanged" {
		t.Fatalf("old wake replayed: calls=%d status=%s", calls, current.Status)
	}
}
