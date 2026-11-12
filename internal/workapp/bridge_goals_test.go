package workapp

import (
	"errors"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/jobs"
	"github.com/gongahkia/gator/internal/workhome"
	"github.com/gongahkia/gator/internal/workrun"
)

func goalControlModel(t *testing.T) (Model, goal.Store, goal.Record) {
	t.Helper()
	store, err := goal.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Add("Venue planning", "Choose an accessible venue", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	plan, err := store.InitializePlan(record.ID, record.Context, []string{"Check capacity"})
	if err != nil {
		t.Fatal(err)
	}
	m := New(Config{BridgeMode: true, Goals: []goal.Record{record}, ListGoals: store.List,
		ReadGoalReview: func(id string) (goal.Record, goal.Plan, error) {
			record, err := store.Load(id)
			return record, plan, err
		},
		SetGoalStatusReviewed: store.SetStatusReviewed,
		ReadChecks: func() ([]workhome.Check, bool, error) {
			return []workhome.Check{{ID: "job-fixture", GoalID: record.ID, Enabled: true, State: "offline", Definition: jobs.Definition{ID: "job-fixture"}}}, false, nil
		},
	})
	return m, store, record
}

func TestBridgeGoalInspectPauseResumeRetainIdentityAndPlan(t *testing.T) {
	m, store, record := goalControlModel(t)
	for _, action := range []string{"inspect", "pause", "resume"} {
		current, _ := store.Load(record.ID)
		digest, _ := current.ReviewDigest()
		value := BridgeGoalAction{RequestID: "review-" + action, ID: record.ID, Action: action, Digest: digest}
		if action == "inspect" {
			value.Digest = ""
		}
		next, command := m.ApplyBridgeGoal(value)
		m = next.(Model)
		if command == nil || !m.bridgeGoalBusy || m.followupSaving != (action != "inspect") {
			t.Fatal("goal control did not start", m.bridgeGoalResult)
		}
		next, _ = m.Update(command())
		m = next.(Model)
		result := m.BridgeState().GoalResult
		if result == nil || result.Error != "" || result.RequestID != value.RequestID || result.Review == nil || result.Review.ID != record.ID {
			t.Fatalf("result=%+v", result)
		}
		if action == "inspect" && !strings.Contains(strings.Join(result.Review.Lines, "\n"), "Check capacity") {
			t.Fatal("retained plan was not inspected")
		}
		want := goal.Active
		if action == "pause" {
			want = goal.Paused
		}
		saved, _ := store.Load(record.ID)
		if saved.Status != want || saved.Context != record.Context || saved.SourcePath != record.SourcePath {
			t.Fatal("wrong goal outcome", saved)
		}
		if !m.BridgeState().FollowingGoals[0].Controllable || result.Review.EnabledChecks != 1 {
			t.Fatal("lost controls or check coverage")
		}
	}
}

func TestBridgeGoalRejectsStaleBusyMissingAndFailedMutations(t *testing.T) {
	for _, scenario := range []string{"stale", "busy", "missing", "write failed", "refresh failed"} {
		t.Run(scenario, func(t *testing.T) {
			m, store, record := goalControlModel(t)
			digest, _ := record.ReviewDigest()
			switch scenario {
			case "stale":
				_, _ = store.SetContext(record.ID, "New outcome to preserve")
			case "busy":
				m.running = true
			case "missing":
				record.ID = "goal-" + strings.Repeat("f", 24)
			case "write failed":
				m.config.SetGoalStatusReviewed = func(string, goal.Status, string) (goal.Record, error) {
					return goal.Record{}, errors.New("read-only goal store")
				}
			case "refresh failed":
				m.config.ReadChecks = func() ([]workhome.Check, bool, error) { return nil, false, errors.New("check health unavailable") }
			}
			next, command := m.ApplyBridgeGoal(BridgeGoalAction{RequestID: "review-1", ID: record.ID, Action: "pause", Digest: digest})
			m = next.(Model)
			if command != nil {
				next, _ = m.Update(command())
				m = next.(Model)
			}
			result := m.BridgeState().GoalResult
			if result == nil || result.Error == "" || m.followupSaving {
				t.Fatal("failed action presented as successful", result)
			}
			if scenario == "refresh failed" {
				saved, _ := store.Load(record.ID)
				if saved.Status != goal.Paused || !strings.Contains(result.Error, "saved status changed") {
					t.Fatal("partial success was hidden", result)
				}
			} else if scenario != "missing" {
				saved, _ := store.Load(record.ID)
				if saved.Status != goal.Active {
					t.Fatal("rejected control changed status", saved)
				}
			}
		})
	}
}

func TestBridgeGoalInspectionKeepsRunningWorkAndOtherControlOwnership(t *testing.T) {
	m, _, record := goalControlModel(t)
	m.running = true
	m.operation = &workrun.Operation{RunID: "original-running-task"}
	guidance := m.bridgeGuidanceID()
	next, command := m.ApplyBridgeGoal(BridgeGoalAction{RequestID: "read-running", ID: record.ID, Action: "inspect"})
	m = next.(Model)
	if command == nil || !m.bridgeGoalBusy || m.followupSaving || !m.running {
		t.Fatal("inspection took execution ownership")
	}
	// An independent control may start while the read is in progress. Finishing
	// this read must not clear that control's busy state.
	m.followupSaving = true
	next, _ = m.Update(command())
	m = next.(Model)
	if !m.running || m.operation.RunID != "original-running-task" || m.bridgeGuidanceID() != guidance || !m.followupSaving || m.bridgeGoalBusy || m.bridgeGoalResult.Error != "" {
		t.Fatal("inspection changed active ownership", m.bridgeGoalResult)
	}
}

func TestBridgeGoalDoesNotConfirmARecordChangedAgainAfterSaving(t *testing.T) {
	m, store, record := goalControlModel(t)
	digest, _ := record.ReviewDigest()
	m.config.SetGoalStatusReviewed = func(id string, status goal.Status, expected string) (goal.Record, error) {
		saved, err := store.SetStatusReviewed(id, status, expected)
		if err != nil {
			return saved, err
		}
		_, err = store.SetStatus(id, goal.Active)
		return saved, err
	}
	next, command := m.ApplyBridgeGoal(BridgeGoalAction{RequestID: "pause-raced", ID: record.ID, Action: "pause", Digest: digest})
	m = next.(Model)
	next, _ = m.Update(command())
	m = next.(Model)
	if m.bridgeGoalResult == nil || !strings.Contains(m.bridgeGoalResult.Error, "changed again") || m.bridgeGoalResult.Review != nil {
		t.Fatal("older saved status presented as current", m.bridgeGoalResult)
	}
	if m.BridgeState().FollowingGoals[0].Status != "active" {
		t.Fatal("lost the actual current status")
	}
}

func TestBridgeGoalRetainsCanonicalUppercaseHexIdentity(t *testing.T) {
	m, store, _ := goalControlModel(t)
	record, err := store.AddWithID("goal-"+strings.Repeat("A", 24), "Existing imported goal", "Keep its original identity", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	digest, _ := record.ReviewDigest()
	next, command := m.ApplyBridgeGoal(BridgeGoalAction{RequestID: "existing-goal", ID: record.ID, Action: "pause", Digest: digest})
	m = next.(Model)
	if command == nil {
		t.Fatal("rejected a canonical goal identity", m.bridgeGoalResult)
	}
	next, _ = m.Update(command())
	m = next.(Model)
	if m.bridgeGoalResult.Error != "" || m.bridgeGoalResult.Review.ID != record.ID {
		t.Fatal("changed the retained goal identity", m.bridgeGoalResult)
	}
	saved, err := store.Load(record.ID)
	if err != nil || saved.Status != goal.Paused {
		t.Fatal(saved, err)
	}
}
