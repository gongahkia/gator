package workapp

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/followup"
	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/jobs"
	"github.com/gongahkia/gator/internal/uisnapshot"
	"github.com/gongahkia/gator/internal/workhome"
)

func TestCommitmentLegacyTimingUsesExactReviewedControllerAndReturnsToCheck(t *testing.T) {
	check := cardCheck(t)
	check.Definition.GoalWatch = false
	digest, _ := check.Definition.PolicyDigest()
	reads := 0
	m := New(Config{BridgeMode: true, CurrentFolder: t.TempDir(), ReadChecks: func() ([]workhome.Check, bool, error) { return []workhome.Check{check}, false, nil }, Controls: func(_ context.Context, family, action string, args []string) (ControlScreen, error) {
		reads++
		if family != "check-timing" || action != "status" || len(args) != 2 || args[0] != check.ID || args[1] != digest {
			t.Fatalf("retargeted timing controller: %s %s %v", family, action, args)
		}
		return ControlScreen{Panel: uisnapshot.Panel{Kind: "controls", Title: "Check timing", ActionIdentity: digest}}, nil
	}})
	m.conversation = "work-original"
	next, read := m.ApplyBridgeCheck(BridgeCheckAction{RequestID: "edit-legacy", ID: check.ID, Digest: digest, Action: "edit"})
	next, controls := next.(Model).Update(read())
	m = next.(Model)
	if m.bridgeCheckResult.Error != "" || controls == nil || !m.controlBusy || m.checkDetail.ID != check.ID {
		t.Fatal("legacy edit did not open its own controller")
	}
	next, _ = m.Update(controls())
	m = next.(Model)
	if reads != 1 || m.BridgeState().Panel.Navigation != "following" || m.BridgeState().Panel.BackLabel != "Back to check details" {
		t.Fatal("timing editor lost Following navigation")
	}
	m, refresh := productKey(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.section != "followup" || m.checkDetail.ID != check.ID || m.conversation != "work-original" || m.controlReturnFollowup || refresh == nil {
		t.Fatal("Back lost the original commitment or conversation")
	}
}

func TestCommitmentSetupUsesTheFreshGoalWithoutRetargetingTheConversation(t *testing.T) {
	current, target := t.TempDir(), t.TempDir()
	record := goal.Record{ID: "goal-selected", Title: "Same title", Context: "Track accessibility changes", SourcePath: target, ConversationID: "work-goal", Status: goal.Active}
	writes := 0
	m := New(Config{BridgeMode: true, CurrentFolder: current, Goals: []goal.Record{{ID: record.ID, SourcePath: "/stale/source"}},
		ReadChecks: func() ([]workhome.Check, bool, error) { return nil, false, nil },
		ListGoals: func() ([]goal.Record, error) {
			return []goal.Record{{ID: "goal-other", Title: record.Title, SourcePath: current, Status: goal.Active}, record}, nil
		},
		SaveFollowup: func(followup.Draft) (goal.Record, jobs.Definition, error) {
			writes++
			return goal.Record{}, jobs.Definition{}, nil
		},
	})
	m.conversation, m.goalID = "work-current", "goal-current"
	beforeSource := m.source
	next, command := m.ApplyBridgeCheck(BridgeCheckAction{RequestID: "setup-1", ID: record.ID, Action: "setup"})
	if command == nil {
		t.Fatal("missing setup read")
	}
	next, _ = next.(Model).Update(command())
	m = next.(Model)
	if m.followupForm == nil || m.followupForm.Review || m.followupSaving || m.bridgeCheckResult.Error != "" {
		t.Fatalf("setup failed: %+v", m.bridgeCheckResult)
	}
	draft := m.followupForm.Draft
	if draft.GoalID != record.ID || draft.SourcePath != target || draft.Title != record.Title || draft.Outcome != record.Context || draft.OriginConversationID != record.ConversationID {
		t.Fatalf("setup used a different goal or source: %+v", draft)
	}
	if writes != 0 || m.source != beforeSource || m.conversation != "work-current" || m.goalID != "goal-current" {
		t.Fatal("opening setup saved or retargeted the original conversation")
	}
}

func TestCommitmentSetupRejectsUnavailableGoalsAndAlwaysClearsBusyState(t *testing.T) {
	for _, scenario := range []string{"missing", "paused", "read failed", "unavailable"} {
		t.Run(scenario, func(t *testing.T) {
			m := New(Config{BridgeMode: true, CurrentFolder: t.TempDir(), ReadChecks: func() ([]workhome.Check, bool, error) { return nil, false, nil },
				ListGoals: func() ([]goal.Record, error) {
					if scenario == "read failed" {
						return nil, errors.New("goal store unavailable")
					}
					if scenario == "missing" {
						return nil, nil
					}
					return []goal.Record{{ID: "goal-selected", Status: goal.Paused}}, nil
				}, SaveFollowup: func(followup.Draft) (goal.Record, jobs.Definition, error) {
					t.Fatal("setup saved a check")
					return goal.Record{}, jobs.Definition{}, nil
				},
			})
			if scenario == "unavailable" {
				m.config.SaveFollowup = nil
			}
			next, command := m.ApplyBridgeCheck(BridgeCheckAction{RequestID: "setup-2", ID: "goal-selected", Action: "setup"})
			if command != nil {
				next, _ = next.(Model).Update(command())
			}
			m = next.(Model)
			if m.followupSaving || m.followupForm != nil || m.bridgeCheckResult.Error == "" {
				t.Fatalf("unsafe or stuck setup: %+v", m.bridgeCheckResult)
			}
		})
	}
}

func cardCheck(t *testing.T) workhome.Check {
	t.Helper()
	draft, err := followup.New(t.TempDir(), "goal-"+strings.Repeat("a", 24))
	if err != nil {
		t.Fatal(err)
	}
	draft.Title, draft.Outcome = "Venue booking", "Keep the booking on track"
	definition, err := draft.Definition(draft.GoalID, nil)
	if err != nil {
		t.Fatal(err)
	}
	return workhome.Check{ID: definition.ID, GoalID: definition.GoalID, Title: definition.Name, Outcome: definition.Objective, Definition: definition, Enabled: true, State: "waiting for first check", NextAction: "Review results in Inbox", NextDue: time.Now().Add(time.Hour)}
}

func TestCommitmentControlsInspectAndEditExactCheck(t *testing.T) {
	check := cardCheck(t)
	other := check
	other.ID, other.Definition.ID = "job-other", "job-other"
	for _, action := range []string{"inspect", "edit"} {
		t.Run(action, func(t *testing.T) {
			m := New(Config{BridgeMode: true, CurrentFolder: t.TempDir(), ReadChecks: func() ([]workhome.Check, bool, error) { return []workhome.Check{other, check}, true, nil }})
			digest, _ := check.Definition.PolicyDigest()
			next, command := m.ApplyBridgeCheck(BridgeCheckAction{RequestID: "exact-1", ID: check.ID, Digest: digest, Action: action})
			if command == nil || !next.(Model).followupSaving {
				t.Fatal("missing async inspection")
			}
			next, _ = next.(Model).Update(command())
			m = next.(Model)
			if m.followupSaving || m.checkDetail == nil || m.checkDetail.ID != check.ID || m.bridgeCheckResult.Error != "" {
				t.Fatalf("wrong check: %+v", m.bridgeCheckResult)
			}
			if action == "edit" && (m.followupForm == nil || m.followupForm.Draft.JobID != check.ID || m.followupForm.Draft.ExpectedDigest != digest || m.followupForm.Review) {
				t.Fatal("edit did not use the canonical unapproved draft")
			}
			state := m.BridgeState()
			if !state.SupervisorRunning || len(state.Checks) != 2 || state.Checks[1].ID != check.ID || state.Checks[1].Digest != digest || state.Checks[1].Outcome != check.Outcome || state.Checks[1].NextDue == "" || state.Checks[1].Timezone != "UTC" || !state.Checks[1].Editable {
				t.Fatalf("incomplete projection: %+v", state.Checks)
			}
		})
	}
}

func TestCommitmentControlsRecheckIdentityAndAgreement(t *testing.T) {
	check := cardCheck(t)
	digest, _ := check.Definition.PolicyDigest()
	for _, scenario := range []string{"stale", "missing", "goal paused", "read failed", "write failed", "legacy edit", "busy"} {
		t.Run(scenario, func(t *testing.T) {
			reads, writes := 0, 0
			current := check
			if scenario == "stale" {
				current.Definition.Objective = "Changed elsewhere"
			}
			if scenario == "goal paused" {
				current.State = "goal paused"
			}
			if scenario == "legacy edit" {
				current.Definition.GoalWatch = false
				digest, _ = current.Definition.PolicyDigest()
			} else {
				digest, _ = check.Definition.PolicyDigest()
			}
			m := New(Config{BridgeMode: true, ReadChecks: func() ([]workhome.Check, bool, error) {
				reads++
				if scenario == "missing" {
					return nil, true, nil
				}
				if scenario == "read failed" {
					return nil, false, errors.New("source store unavailable")
				}
				return []workhome.Check{current}, true, nil
			}, SetCheckEnabled: func(id string, enabled bool, expected string) error {
				writes++
				if id != check.ID || !enabled || expected != digest {
					t.Fatal("authority or target changed")
				}
				return errors.New("disk unavailable")
			}})
			m.running = scenario == "busy"
			action := "resume"
			if scenario == "legacy edit" {
				action = "edit"
			}
			next, command := m.ApplyBridgeCheck(BridgeCheckAction{RequestID: "reject-1", ID: check.ID, Digest: digest, Action: action})
			if command != nil {
				next, _ = next.(Model).Update(command())
			}
			m = next.(Model)
			if m.bridgeCheckResult == nil || m.bridgeCheckResult.RequestID != "reject-1" || m.bridgeCheckResult.Error == "" || m.followupSaving || m.followupForm != nil {
				t.Fatalf("rejection not acknowledged: %+v", m.bridgeCheckResult)
			}
			want := 0
			if scenario == "write failed" {
				want = 1
			}
			if writes != want || scenario == "busy" && reads != 0 {
				t.Fatalf("unexpected read/write %d/%d", reads, writes)
			}
		})
	}
}

func TestCommitmentPauseAcknowledgesSavedChangeEvenIfRefreshFails(t *testing.T) {
	check := cardCheck(t)
	digest, _ := check.Definition.PolicyDigest()
	reads, writes := 0, 0
	m := New(Config{BridgeMode: true, ReadChecks: func() ([]workhome.Check, bool, error) {
		reads++
		if reads > 1 {
			return nil, false, errors.New("health unavailable")
		}
		return []workhome.Check{check}, true, nil
	}, SetCheckEnabled: func(id string, enabled bool, expected string) error {
		writes++
		if enabled {
			t.Fatal("pause resumed check")
		}
		return nil
	}})
	next, command := m.ApplyBridgeCheck(BridgeCheckAction{RequestID: "partial-1", ID: check.ID, Digest: digest, Action: "pause"})
	next, _ = next.(Model).Update(command())
	m = next.(Model)
	if writes != 1 || m.followupSaving || !strings.Contains(m.bridgeCheckResult.Error, "check paused, but") || m.checksIssue == "" {
		t.Fatalf("lost partial success: %+v", m.bridgeCheckResult)
	}
}
