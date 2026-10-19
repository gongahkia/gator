package main

import (
	"bytes"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/workhistory"
)

func TestPilotCommandRetainsOnlyObservedUsability(t *testing.T) {
	state := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", state)
	history, _ := workhistory.Open(state)
	now := time.Now().UTC()
	if _, err := history.Start(workhistory.Start{ID: "work-pilot-command", Objective: "Review project change", Mode: action.Draft, ExternalActions: action.Forbid, StartedAt: now.Add(-time.Second)}); err != nil {
		t.Fatal(err)
	}
	if _, err := history.Finish("work-pilot-command", workhistory.Finish{Succeeded: true, FinishedAt: now, VerificationStatus: workhistory.VerificationUnavailable}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	args := []string{"record", "--work", "work-pilot-command", "--draft", "light-edits", "--corrections", "2", "--setup-seconds", "90", "--repair-seconds", "0", "--unassisted", "false"}
	if err := pilotCommand(args, &out); err != nil {
		t.Fatal(err)
	}
	report, err := readPilotReport(state, "")
	if err != nil {
		t.Fatal(err)
	}
	if report.SetupSeconds != 90 || report.SetupObservations != 1 || report.RepairObservations != 1 || report.ResumeObservations != 0 || report.NeededHelp != 1 || report.RecoverySuccesses != 0 || report.KnownCostUSD != nil {
		t.Fatalf("unmeasured outcomes became observations: %#v", report)
	}
	if err := pilotCommand([]string{"record", "--work", "work-pilot-command", "--draft", "usable"}, &out); err == nil {
		t.Fatal("missing correction count was silently accepted")
	}
}
