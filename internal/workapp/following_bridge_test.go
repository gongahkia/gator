package workapp

import (
	"errors"
	"testing"

	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/workhome"
)

func TestFollowingKeepsSavedGoalsSeparateFromCheckHealth(t *testing.T) {
	m := New(Config{BridgeMode: true, Goals: []goal.Record{{ID: "goal-one", Title: "Saved intention", Status: goal.Active}, {ID: "goal-paused", Status: goal.Paused}, {ID: "goal-done", Status: goal.Completed}}})
	state := m.BridgeState()
	if len(state.FollowingGoals) != 2 || len(state.Checks) != 0 {
		t.Fatalf("saved goal implied monitoring: %+v", state)
	}
	next, _ := m.Update(checksRefreshed{Checks: []workhome.Check{{GoalID: "goal-one", Title: "Actual check", State: "paused"}}})
	m = next.(Model)
	next, _ = m.Update(checksRefreshed{Err: errors.New("unreadable schedule")})
	state = next.(Model).BridgeState()
	if state.ChecksIssue == "" || len(state.Checks) != 1 || state.Checks[0].GoalID != "goal-one" || state.Checks[0].State != "paused" {
		t.Fatalf("lost health failure or last observed state: %+v", state)
	}
	next, _ = next.(Model).Update(checksRefreshed{Checks: nil})
	state = next.(Model).BridgeState()
	if state.ChecksIssue != "" || len(state.Checks) != 0 {
		t.Fatal("successful refresh retained stale health")
	}
	m.config.ListGoals = func() ([]goal.Record, error) { return nil, errors.New("unavailable") }
	m.refreshBridgeFollowing()
	if state = m.BridgeState(); state.FollowingIssue == "" || len(state.FollowingGoals) != 0 {
		t.Fatal("presented stale goals as current")
	}
}
