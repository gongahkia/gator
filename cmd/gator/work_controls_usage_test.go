package main

import (
	"encoding/json"
	"testing"
	"time"

	sharedusage "github.com/gongahkia/gator/internal/usage"
)

func TestUsageDisplayUsesSharedAccountingAndKeepsReviewedPolicy(t *testing.T) {
	root := t.TempDir()
	store, err := sharedusage.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	policy := sharedusage.Policy{DailyRequests: 100, DailyTokens: 10000, ConcurrentRequests: 2}
	if err = store.SavePolicy(policy); err != nil {
		t.Fatal(err)
	}
	finished, err := store.Reserve(time.Now(), "background", "test", 60)
	if err != nil {
		t.Fatal(err)
	}
	cost := 0.001
	if err = finished.Finish(40, true, true, &cost); err != nil {
		t.Fatal(err)
	}
	active, err := store.Reserve(time.Now(), "chat", "test", 100)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = active.Finish(100, false, false, nil) })
	screen, err := usageControl(root, "status", nil)
	if err != nil {
		t.Fatal(err)
	}
	u := screen.Panel.Usage
	if u == nil || u.Requests != 2 || u.Tokens != 140 || u.ActiveRequests != 1 || u.FailedRequests != 1 || u.EstimatedRequests != 1 || u.KnownCostUSD != nil {
		t.Fatalf("display lost real accounting: %+v", u)
	}
	if u.DailyRequestLimit != 100 || u.DailyTokenLimit != 10000 || u.ConcurrentLimit != 2 || u.Paused {
		t.Fatalf("display lost policy: %+v", u)
	}
	reset, err := time.Parse(time.RFC3339, u.ResetAt)
	if err != nil || reset.Format("15:04:05Z07:00") != "00:00:00Z" || reset.Format("2006-01-02") <= u.Day {
		t.Fatalf("invalid UTC reset: %s (%v)", u.ResetAt, err)
	}
	if len(u.Categories) != 7 || u.Categories[0].Name != "chat" || u.Categories[0].Requests != 1 || u.Categories[2].Name != "background" || u.Categories[2].Requests != 1 || u.Categories[6].Name != "voice" || u.Categories[6].Requests != 0 {
		t.Fatalf("incorrect category breakdown: %+v", u.Categories)
	}
	if len(screen.Actions) != 3 || !screen.Actions[0].Review || !screen.Actions[1].Review {
		t.Fatalf("usage edits must retain separate reviews: %+v", screen.Actions)
	}
	if err = active.Finish(100, true, false, &cost); err != nil {
		t.Fatal(err)
	}
	screen, err = usageControl(root, "status", nil)
	if err != nil || screen.Panel.Usage.KnownCostUSD == nil || *screen.Panel.Usage.KnownCostUSD != cost*2 {
		t.Fatalf("complete provider-reported cost not reflected: %v", err)
	}
	// A changed policy must still reject an old desktop review.
	policy.Paused = true
	if err = store.SavePolicy(policy); err != nil {
		t.Fatal(err)
	}
	if _, err = usageControl(root, "pause", screen.Actions[1].Args); err == nil {
		t.Fatal("stale policy review was accepted")
	}
	screen, err = usageControl(root, "status", nil)
	if err != nil || !screen.Panel.Usage.Paused {
		t.Fatalf("paused policy not reflected: %v", err)
	}
	// Zero remains uncapped in the wire contract, rather than a synthetic quota.
	previous, _ := json.Marshal(policy)
	screen, err = usageControl(root, "limits", []string{string(previous), "0", "0", "0"})
	if err != nil || screen.Panel.Usage.DailyRequestLimit != 0 || screen.Panel.Usage.DailyTokenLimit != 0 || screen.Panel.Usage.ConcurrentLimit != 0 {
		t.Fatalf("uncapped policy not reflected: %v", err)
	}
}
