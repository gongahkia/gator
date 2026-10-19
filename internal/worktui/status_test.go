package worktui

import (
	"errors"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/workrun"
)

func TestWorkStatusShowsSelectedModelAndAccess(t *testing.T) {
	m := New(Config{ModelStatus: func() (ModelStatus, error) {
		return ModelStatus{Provider: "codex", Model: "gpt-5.6", Access: "logged in (Gator OAuth credential)"}, nil
	}})
	status := m.workStatus()
	for _, expected := range []string{
		"when Gator works: on request; no enabled scheduled checks (/jobs)",
		"what Gator may do: auto: inspect or draft; external actions require foreground approval (/mode)",
		"model: codex / gpt-5.6",
		"model access: logged in (Gator OAuth credential)",
		"internal specialists: managed by Gator",
	} {
		if !strings.Contains(status, expected) {
			t.Fatalf("status omitted %q:\n%s", expected, status)
		}
	}
}

func TestWorkStatusShowsCurrentSpecialistsAndApprovalControls(t *testing.T) {
	m := New(Config{})
	m.running = true
	m.operation = &workrun.Operation{RunID: "work-live"}
	m.tasks = map[string]string{
		"subagent-001": "source_researcher: running",
		"subagent-002": "claim_verifier: queued",
		"subagent-003": "artifact_reviewer: completed",
	}
	m.interaction = &workrun.Interaction{ID: 7, Kind: "action", Preview: "private payload"}
	m.pendingInteractions = []workrun.Interaction{{ID: 8, Kind: "connected_read"}}
	status := m.workStatus()
	for _, expected := range []string{
		"current work: running (/cancel); steer with /steer MESSAGE",
		"work run: work-live (inspect from another terminal: gator work decisions work-live)",
		"live specialists: 2 (/tasks, /cancel-task ID, /steer-task ID MESSAGE)",
		"subagent-001 source_researcher: running",
		"subagent-002 claim_verifier: queued",
		"pending approvals: 2 (current: action #7; /approve or /deny)",
	} {
		if !strings.Contains(status, expected) {
			t.Fatalf("live status omitted %q:\n%s", expected, status)
		}
	}
	if strings.Contains(status, "private payload") || strings.Contains(status, "artifact_reviewer") {
		t.Fatalf("status included approval payload or completed specialist: %s", status)
	}
	m.running = false
	if idle := m.workStatus(); !strings.Contains(idle, "current work: idle") || strings.Contains(idle, "pending approvals") {
		t.Fatalf("idle status = %s", idle)
	}
}

func TestWorkStatusShowsUnconfiguredAndSafeErrors(t *testing.T) {
	unconfigured := New(Config{ModelStatus: func() (ModelStatus, error) {
		return ModelStatus{Access: "not configured"}, nil
	}}).workStatus()
	if !strings.Contains(unconfigured, "model: none") || !strings.Contains(unconfigured, "model access: not configured") {
		t.Fatalf("unconfigured status = %q", unconfigured)
	}

	failed := New(Config{ModelStatus: func() (ModelStatus, error) {
		return ModelStatus{}, errors.New("cannot read\nconfiguration")
	}}).workStatus()
	if !strings.Contains(failed, "model: unavailable") || !strings.Contains(failed, "status unavailable: cannot read configuration") {
		t.Fatalf("failed status = %q", failed)
	}
}
