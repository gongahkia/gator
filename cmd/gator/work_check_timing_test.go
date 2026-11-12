package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/jobs"
)

func timingFixture(t *testing.T) (string, jobs.Store, jobs.Definition) {
	t.Helper()
	root, source := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "brief.txt"), []byte("Retain the sealed source\n"), 0600); err != nil {
		t.Fatal(err)
	}
	store, err := jobs.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	definition, err := store.Save(jobs.Definition{ID: "job-original", Name: "Same title", Enabled: false, SourcePath: source, Objective: "Review the exact retained workspace", Schedule: "15 8 * * *", Timezone: "UTC", Missed: "skip",
		OriginConversationID: "work-original", Mode: action.Draft, Contract: artifact.DefaultContract("report.md"),
		ConnectorIDs: []string{"mail-one", "legacy-mail"}, OAuthAccountPins: map[string]string{"mail-one": strings.Repeat("b", 64)}, OAuthRefreshPins: map[string]string{"legacy-mail": strings.Repeat("c", 64)},
		Provider: "retained-provider", Model: "retained-model", WebOrigins: []string{"https://example.com"},
		Limits: agent.Limits{ModelRequests: 7, Tokens: 32000, WallSeconds: 180}, MaxSteps: 19, MaxChecksPerDay: 12, AlertWhen: "Only a changed deadline", EndsAt: time.Date(2030, 12, 1, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	return root, store, definition
}

func TestCheckTimingReviewedSavePreservesCanonicalAgreementAndBookkeeping(t *testing.T) {
	root, store, original := timingFixture(t)
	peer := original
	peer.ID = "job-peer"
	if _, err := store.Save(peer); err != nil {
		t.Fatal(err)
	}
	peerPath := filepath.Join(store.Root(), "definitions", peer.ID+".json")
	peerBefore, _ := os.ReadFile(peerPath)
	digest, _ := original.PolicyDigest()
	args := []string{original.ID, digest, "weekdays", "09:30", "Asia/Singapore"}
	review, err := checkTimingControl(context.Background(), root, "preview-timing", args)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(review.Panel)
	for _, text := range []string{original.ID, "30 9 * * 1-5", "Asia/Singapore", "Retained account pin: mail-one", "Enabled state retained: false", "retained-provider/retained-model"} {
		if !strings.Contains(string(encoded), text) {
			t.Fatalf("missing exact review %q: %s", text, encoded)
		}
	}
	unchanged, _ := store.Load(original.ID)
	if !reflect.DeepEqual(unchanged, original) {
		t.Fatal("preview mutated the agreement")
	}
	// The scheduler may advance this counter without invalidating policy review.
	unchanged.LastScheduledAt = time.Date(2026, 11, 12, 1, 0, 0, 0, time.UTC)
	if _, err := store.Save(unchanged); err != nil {
		t.Fatal(err)
	}
	screen, err := checkTimingControl(context.Background(), root, review.Actions[0].Action, review.Actions[0].Args)
	if err != nil || screen.Panel.NoticeTone != "success" {
		t.Fatalf("save: %v %+v", err, screen.Panel)
	}
	reopened, _ := jobs.Open(root)
	saved, _ := reopened.Load(original.ID)
	if saved.Schedule != "30 9 * * 1-5" || saved.Timezone != "Asia/Singapore" || saved.LastScheduledAt != unchanged.LastScheduledAt {
		t.Fatalf("wrong timing/bookkeeping: %+v", saved)
	}
	saved.Schedule, saved.Timezone, saved.UpdatedAt, saved.LastScheduledAt = original.Schedule, original.Timezone, original.UpdatedAt, original.LastScheduledAt
	if !reflect.DeepEqual(saved, original) {
		t.Fatalf("timing edit replaced retained authority or frozen inputs\nbefore=%+v\nafter=%+v", original, saved)
	}
	peerAfter, _ := os.ReadFile(peerPath)
	if string(peerAfter) != string(peerBefore) {
		t.Fatal("peer check changed")
	}
	if _, err := checkTimingControl(context.Background(), root, "save-timing", args); err == nil {
		t.Fatal("old review replayed after the policy change")
	}
}

func TestCheckTimingRejectsInvalidAndStaleChangesWithoutWrites(t *testing.T) {
	root, store, original := timingFixture(t)
	digest, _ := original.PolicyDigest()
	path := filepath.Join(store.Root(), "definitions", original.ID+".json")
	before, _ := os.ReadFile(path)
	for _, values := range [][]string{{}, {"daily", "9:30", "UTC"}, {"daily", "24:10", "UTC"}, {"daily", "09:30", "Imaginary/Zone"}, {"daily", "09:30", "UTC", "extra"}, {"hourly", "60", "UTC"}, {"hourly", "x", "UTC"}, {"custom", "0 9 30 2 *", "UTC"}, {"custom", "0 0 9 * * *", "UTC"}, {"missed", "replay-all"}, {"end", "2020-01-01T10:00:00Z"}, {"end", "2030-11-12"}, {"source", "/replacement"}} {
		args := append([]string{original.ID, digest}, values...)
		if _, err := checkTimingControl(context.Background(), root, "preview-timing", args); err == nil {
			t.Fatalf("invalid change accepted: %v", values)
		}
		if _, err := checkTimingControl(context.Background(), root, "save-timing", args); err == nil {
			t.Fatalf("invalid save accepted: %v", values)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := checkTimingControl(ctx, root, "save-timing", []string{original.ID, digest, "daily", "09:30", "UTC"}); err == nil {
		t.Fatal("cancelled save accepted")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("validation or cancellation wrote the definition")
	}
	args := []string{original.ID, digest, "daily", "09:30", "UTC"}
	if _, err := checkTimingControl(context.Background(), root, "preview-timing", args); err != nil {
		t.Fatal(err)
	}
	original.Objective = "Changed from another terminal"
	if _, err := store.Save(original); err != nil {
		t.Fatal(err)
	}
	if _, err := checkTimingControl(context.Background(), root, "save-timing", args); err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("stale review accepted: %v", err)
	}
	saved, _ := store.Load(original.ID)
	if saved.Objective != original.Objective || saved.Schedule != original.Schedule {
		t.Fatal("stale edit replaced a concurrent change")
	}
	if _, err := checkTimingControl(context.Background(), root, "status", []string{"job-missing", digest}); err == nil {
		t.Fatal("missing check opened")
	}
}

func TestCheckTimingChoicesUseCanonicalTimezoneAndCatchUpSemantics(t *testing.T) {
	_, _, prior := timingFixture(t)
	now := time.Date(2030, 3, 8, 15, 0, 0, 0, time.UTC)
	for _, item := range []struct {
		values                 []string
		schedule, zone, missed string
	}{
		{[]string{"daily", "09:30", "America/New_York"}, "30 9 * * *", "America/New_York", "skip"},
		{[]string{"weekdays", "10:05", "Asia/Singapore"}, "5 10 * * 1-5", "Asia/Singapore", "skip"},
		{[]string{"hourly", "17", "UTC"}, "17 * * * *", "UTC", "skip"},
		{[]string{"custom", "0 8 * * 0", "UTC"}, "0 8 * * 0", "UTC", "skip"},
		{[]string{"missed", "run_once"}, prior.Schedule, prior.Timezone, "run_once"},
		{[]string{"end", ""}, prior.Schedule, prior.Timezone, "skip"},
	} {
		next, err := changedCheckTiming(prior, item.values, now)
		if err != nil || next.Schedule != item.schedule || next.Timezone != item.zone || next.Missed != item.missed || next.Enabled {
			t.Fatalf("wrong timing: %v %+v %v", item.values, next, err)
		}
		if item.values[0] == "end" && !next.EndsAt.IsZero() {
			t.Fatal("blank end date was not cleared")
		}
	}
	dst, _ := changedCheckTiming(prior, []string{"daily", "09:30", "America/New_York"}, now)
	schedule, _ := jobs.Schedule(dst)
	first, second := schedule.Next(now), schedule.Next(schedule.Next(now))
	if first.Hour() != 14 || second.Hour() != 13 {
		t.Fatalf("local daily time did not cross daylight savings correctly: %v %v", first, second)
	}
}
