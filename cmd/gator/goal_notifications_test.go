package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/inbox"
	"github.com/gongahkia/gator/internal/jobs"
)

func TestGoalQuietHoursSuppressDesktopButKeepInbox(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", stateDir)
	goals, err := goal.Open(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	record, err := goals.Add("Plan the offsite", "", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := goalCommand([]string{"notifications", record.ID, "--quiet", "22:00-08:00", "--timezone", "Asia/Singapore"}, &output); err != nil || !strings.Contains(output.String(), "22:00-08:00 Asia/Singapore") {
		t.Fatalf("notification settings = %q, %v", output.String(), err)
	}
	quiet, _ := time.Parse(time.RFC3339, "2026-10-17T16:00:00Z")
	if goalAllowsDesktopNotification(stateDir, record.ID, quiet, false) {
		t.Fatal("quiet goal allowed desktop interruption")
	}
	definition := jobs.Definition{ID: "job-goal-quiet", Name: "Offsite", GoalID: record.ID}
	attempt := jobs.Attempt{Status: "failed", Error: "provider unavailable"}
	entry, err := recordJobInbox(stateDir, definition, attempt)
	if err != nil || entry.ID == "" {
		t.Fatalf("quiet goal lost failure inbox entry: %#v, %v", entry, err)
	}
	inboxStore, err := inbox.Open(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := inboxStore.List(true, 0)
	if err != nil || len(entries) != 1 || entries[0].GoalID != record.ID {
		t.Fatalf("quiet goal inbox = %#v, %v", entries, err)
	}
	if err := goalCommand([]string{"notifications", record.ID, "--desktop", "off", "--clear-quiet"}, &output); err != nil {
		t.Fatal(err)
	}
	loaded, err := goals.Load(record.ID)
	if err != nil || !loaded.Notifications.DesktopMuted || loaded.Notifications.QuietStart != "" {
		t.Fatalf("updated notification policy = %#v, %v", loaded.Notifications, err)
	}
}

func TestGoalUrgentOnlyDesktopSettingKeepsRoutineResultsInInbox(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", stateDir)
	goals, err := goal.Open(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	record, err := goals.Add("Track offsite", "", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := goalCommand([]string{"notifications", record.ID, "--urgent-only", "on"}, &output); err != nil || !strings.Contains(output.String(), "on (urgent only)") {
		t.Fatalf("urgent-only setting = %q, %v", output.String(), err)
	}
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	if goalAllowsDesktopNotification(stateDir, record.ID, now, false) || !goalAllowsDesktopNotification(stateDir, record.ID, now, true) {
		t.Fatal("urgent-only policy did not distinguish routine from urgent")
	}
	definition := jobs.Definition{ID: "job-offsite", Name: "Offsite", GoalID: record.ID}
	entry, err := recordJobInbox(stateDir, definition, jobs.Attempt{Status: "completed", Summary: "Routine update", ResultSHA256: "routine"})
	if err != nil || entry.ID == "" || entry.Urgent {
		t.Fatalf("routine result = %#v, %v", entry, err)
	}
	failure, err := recordJobInbox(stateDir, definition, jobs.Attempt{Status: "failed", Error: "provider unavailable"})
	if err != nil || failure.ID == "" || !failure.Urgent {
		t.Fatalf("failed result = %#v, %v", failure, err)
	}
	mailbox, err := inbox.Open(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := mailbox.List(false, 10)
	if err != nil || len(entries) != 2 {
		t.Fatalf("urgent-only inbox = %#v, %v", entries, err)
	}
	output.Reset()
	if err := inboxCommand([]string{"show", failure.ID}, &output); err != nil || !strings.Contains(output.String(), "urgent: true") {
		t.Fatalf("urgency was not visible in inbox details = %q, %v", output.String(), err)
	}
	output.Reset()
	if err := goalCommand([]string{"notifications", record.ID, "--urgent-only", "off"}, &output); err != nil || goalAllowsDesktopNotification(stateDir, record.ID, now, false) == false {
		t.Fatalf("urgent-only reset = %q, %v", output.String(), err)
	}
}
