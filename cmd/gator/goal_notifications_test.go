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
	if goalAllowsDesktopNotification(stateDir, record.ID, quiet) {
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
