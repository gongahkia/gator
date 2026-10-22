package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/gongahkia/gator/internal/lifecycle"
	"github.com/gongahkia/gator/internal/state"
	sharedusage "github.com/gongahkia/gator/internal/usage"
	"github.com/gongahkia/gator/internal/workrun"
)

func TestGatorAppUsageChangesPreserveUnspecifiedLimits(t *testing.T) {
	c := appControllerForTest(t)
	store, _ := sharedusage.Open(c.stateDir)
	if err := store.SavePolicy(sharedusage.Policy{DailyRequests: 200, ConcurrentRequests: 3}); err != nil {
		t.Fatal(err)
	}
	mutation, err := c.Prepare(context.Background(), workrun.AppChange{Setting: "usage_limits", Value: json.RawMessage(`{"daily_tokens":500000}`)})
	if err != nil {
		t.Fatal(err)
	}
	if !mutation.Review || !mutation.AfterReply {
		t.Fatal("limit changes must be reviewed and wait for the response")
	}
	if _, err := mutation.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	p, _ := store.Policy()
	if p.DailyRequests != 200 || p.ConcurrentRequests != 3 || p.DailyTokens != 500000 {
		t.Fatalf("unrequested limits changed: %#v", p)
	}
	if _, err := mutation.Apply(context.Background()); err == nil {
		t.Fatal("stale policy update repeated")
	}
	if _, err := c.Prepare(context.Background(), workrun.AppChange{Setting: "usage_limits", Value: json.RawMessage(`{"daily_requests":-1}`)}); err == nil {
		t.Fatal("negative limit accepted")
	}
}

func TestGatorAppRetentionWaitsForAnIdleProfile(t *testing.T) {
	c := appControllerForTest(t)
	mutation, err := c.Prepare(context.Background(), workrun.AppChange{Setting: "retention", Value: json.RawMessage(`{"history_days":30}`)})
	if err != nil {
		t.Fatal(err)
	}
	if !mutation.Review || !mutation.AfterRun {
		t.Fatal("retention must be reviewed and wait until work finishes")
	}
	release, err := state.Guard(c.stateDir, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mutation.Apply(context.Background()); err == nil {
		release()
		t.Fatal("retention changed while work held the profile")
	}
	release()
	if _, err := mutation.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	store, _ := lifecycle.Open(c.stateDir)
	p, _ := store.Policy()
	if p.HistoryDays != 30 || p.KnowledgeDays != 0 || p.SnapshotDays != 0 {
		t.Fatalf("retention=%#v", p)
	}
}
