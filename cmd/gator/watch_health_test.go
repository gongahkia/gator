package main

import (
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/inbox"
	"github.com/gongahkia/gator/internal/jobs"
)

func TestWatchHealthDeliveryDeduplicatesAcrossRestartAndRetainsQuietRecovery(t *testing.T) {
	state := t.TempDir()
	store, err := jobs.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	d, err := store.Save(jobs.Definition{Name: "Project", GoalID: "goal-0123456789abcdef01234567", OriginConversationID: "origin", GoalWatch: true, ChangeOnly: true, RefreshSnapshot: true, Missed: "run_once", Schedule: "* * * * *", SourcePath: t.TempDir(), Objective: "Check deadline", Mode: action.Inspect, Contract: artifact.InspectionContract()})
	if err != nil {
		t.Fatal(err)
	}
	mailbox, _ := inbox.Open(state)
	now := time.Now().UTC().Add(-time.Hour)
	policy, _ := d.PolicyDigest()
	complete := func(status string, index int) (jobs.Attempt, inbox.Entry) {
		t.Helper()
		a, err := store.Begin(d, now.Add(time.Duration(index)*time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		a.Status, a.Error, a.Summary, a.FinishedAt, a.NotificationPolicySHA256 = status, "source unavailable", "request limit", a.StartedAt.Add(time.Second), policy
		if err := store.Record(a); err != nil {
			t.Fatal(err)
		}
		e, err := recordJobInbox(state, d, a)
		if err != nil {
			t.Fatal(err)
		}
		return a, e
	}
	first, e := complete("failed", 0)
	if e.ID == "" {
		t.Fatal("first failure missing")
	}
	if !claimJobNotification(state, e.ID) || claimJobNotification(state, e.ID) {
		t.Fatal("notification handoff not deduplicated")
	}
	if _, err := mailbox.Receipt("origin", e.ID, "resolve", 0, time.Now()); err != nil {
		t.Fatal(err)
	}
	// Reopen the store to simulate a new supervisor process.
	store, err = jobs.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	_, repeated := complete("interrupted", 1)
	if repeated.ID != "" {
		t.Fatal("same incident interrupted again", repeated)
	}
	recovered, recovery := complete("unchanged", 2)
	if recovery.Kind != "job_health" || recovery.Status != "recovered" || !strings.Contains(recovery.Summary, "2 unsuccessful") || recovery.CanRate() {
		t.Fatal("quiet recovery absent or scored as alert", recovery)
	}
	if _, err := recordJobInbox(state, d, recovered); err != nil {
		t.Fatal("recovery retry not idempotent", err)
	}
	entries, err := mailbox.ForConversation("origin", 0)
	if err != nil || len(entries) != 2 {
		t.Fatal("incident events duplicated", entries, err)
	}
	retained, err := mailbox.Load(inbox.JobUpdateID(d.ID, first.ID))
	if err != nil || retained.ResolvedAt.IsZero() || !retained.ReadAt.IsZero() {
		t.Fatal("health changed user receipts", retained, err)
	}
	_, newIncident := complete("interrupted", 3)
	if newIncident.ID == "" || newIncident.ID == e.ID {
		t.Fatal("new post-recovery failure was suppressed")
	}
	history, _ := store.History(d.ID, 0)
	if len(history) != 4 {
		t.Fatal("immutable attempts lost")
	}
}
