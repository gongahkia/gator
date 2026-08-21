package tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/agent"
)

type activityPhase uint8

const (
	activityPreparing activityPhase = iota
	activityThinking
	activityResponding
	activityTool
	activityInspecting
	activityVerifying
	activityAwaitingApproval
	activityFinishing
	activityComplete
	activityFailed
	activityCancelled
)

type runActivity struct {
	phase        activityPhase
	detail       string
	turn         int
	phaseStarted time.Time
	lastActivity time.Time
}

type verificationPhase uint8

const (
	verificationPending verificationPhase = iota
	verificationRunning
	verificationPassed
	verificationFailed
)

type verificationStatus struct {
	argv      []string
	phase     verificationPhase
	startedAt time.Time
	endedAt   time.Time
	detail    string
}

type diffStats struct {
	ready     bool
	files     int
	additions int
	deletions int
}

func (m *Model) beginRunActivity(commands [][]string) {
	now := time.Now()
	m.activity = runActivity{
		phase:        activityPreparing,
		detail:       "creating an isolated worktree",
		phaseStarted: now,
		lastActivity: now,
	}
	m.verificationStatus = make([]verificationStatus, 0, len(commands))
	for _, command := range commands {
		if len(command) == 0 {
			continue
		}
		argv := append([]string(nil), command...)
		m.verificationStatus = append(m.verificationStatus, verificationStatus{argv: argv, phase: verificationPending})
	}
}

func (m *Model) finishRunActivity(err error, cancelled bool) {
	now := time.Now()
	if cancelled {
		m.setActivity(activityCancelled, "cancellation completed; the worktree remains available for review", now)
		return
	}
	if err != nil {
		m.setActivity(activityFailed, compact(err.Error(), 160), now)
		return
	}
	m.setActivity(activityComplete, "run completed; review the retained worktree before handoff", now)
}

func (m *Model) observeActivity(event agent.Event) {
	at := event.At
	if at.IsZero() {
		at = time.Now()
	}
	if event.Step > m.activity.turn {
		m.activity.turn = event.Step
	}
	switch event.Kind {
	case agent.EventTurnStarted:
		m.setActivity(activityThinking, "waiting for the next model response", at)
	case agent.EventTextDelta, agent.EventText:
		m.setActivity(activityResponding, "writing the response", at)
	case agent.EventToolCalled:
		if index := m.verificationForCall(event.ToolCall); index >= 0 {
			m.verificationStatus[index].phase = verificationRunning
			m.verificationStatus[index].startedAt = at
			m.verificationStatus[index].detail = "running"
			m.setActivity(activityVerifying, "running "+strings.Join(m.verificationStatus[index].argv, " "), at)
			return
		}
		if event.ToolCall != nil && (event.ToolCall.Name == "git_status" || event.ToolCall.Name == "git_diff") {
			m.setActivity(activityInspecting, "inspecting the isolated worktree with "+event.ToolCall.Name, at)
			return
		}
		if event.ToolCall != nil {
			m.setActivity(activityTool, "running "+event.ToolCall.Name, at)
			return
		}
		m.setActivity(activityTool, "running a tool", at)
	case agent.EventToolFinished:
		if index := m.verificationForCall(event.ToolCall); index >= 0 {
			status := &m.verificationStatus[index]
			status.endedAt = at
			if event.ToolError == "" {
				status.phase = verificationPassed
				status.detail = "passed"
				m.setActivity(activityThinking, "verification passed; continuing the run", at)
			} else {
				status.phase = verificationFailed
				status.detail = compact(event.ToolError, 140)
				m.setActivity(activityFailed, "verification failed", at)
			}
			return
		}
		if event.ToolError == "" {
			m.setActivity(activityThinking, "tool completed; waiting for the next model response", at)
		} else {
			m.setActivity(activityThinking, "tool failed; waiting for the next model response", at)
		}
	case agent.EventCompletionBlocked:
		m.setActivity(activityInspecting, "completion needs evidence: "+compact(event.Text, 120), at)
	case agent.EventSteeringApplied:
		m.setActivity(activityThinking, "applying your steering instruction", at)
	case agent.EventCommandApprovalRequested:
		m.setActivity(activityAwaitingApproval, "awaiting approval: "+compact(event.Text, 120), at)
	case agent.EventCommandApprovalResolved:
		m.setActivity(activityThinking, "command approval "+compact(event.Text, 40), at)
	case agent.EventRunFinished:
		m.setActivity(activityFinishing, "preparing the final result", at)
	}
}

func (m *Model) setActivity(phase activityPhase, detail string, at time.Time) {
	if m.activity.phase != phase || m.activity.phaseStarted.IsZero() {
		m.activity.phaseStarted = at
	}
	m.activity.phase = phase
	m.activity.detail = detail
	m.activity.lastActivity = at
}

func (m Model) verificationForCall(call *agent.ToolCall) int {
	if call == nil || call.Name != "run_command" {
		return -1
	}
	argv := argvForToolCall(*call)
	for index, status := range m.verificationStatus {
		if sameArgv(status.argv, argv) {
			return index
		}
	}
	return -1
}

func argvForToolCall(call agent.ToolCall) []string {
	var arguments struct {
		Argv []string `json:"argv"`
	}
	if json.Unmarshal(call.Arguments, &arguments) != nil {
		return nil
	}
	return arguments.Argv
}

func sameArgv(first, second []string) bool {
	if len(first) != len(second) {
		return false
	}
	for index := range first {
		if first[index] != second[index] {
			return false
		}
	}
	return true
}

func summarizeDiff(diff string) diffStats {
	stats := diffStats{ready: true}
	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			stats.files++
		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			stats.additions++
		case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
			stats.deletions++
		}
	}
	return stats
}

func (m Model) activityPhaseLabel() string {
	switch m.activity.phase {
	case activityThinking:
		return "thinking"
	case activityResponding:
		return "responding"
	case activityTool:
		return "running tool"
	case activityInspecting:
		return "inspecting"
	case activityVerifying:
		return "verifying"
	case activityAwaitingApproval:
		return "awaiting command approval"
	case activityFinishing:
		return "finishing"
	case activityComplete:
		return "complete"
	case activityFailed:
		return "stopped"
	case activityCancelled:
		return "cancelled"
	default:
		return "preparing"
	}
}

func (m Model) activityDuration(now time.Time) string {
	if m.activity.phaseStarted.IsZero() {
		return ""
	}
	duration := now.Sub(m.activity.phaseStarted).Round(time.Second)
	if duration < 0 {
		duration = 0
	}
	return duration.String()
}

func (m Model) lastActivityAge(now time.Time) string {
	if m.activity.lastActivity.IsZero() {
		return ""
	}
	duration := now.Sub(m.activity.lastActivity).Round(time.Second)
	if duration < 0 {
		duration = 0
	}
	return duration.String() + " ago"
}

func verificationPhaseLabel(phase verificationPhase) string {
	switch phase {
	case verificationRunning:
		return "running"
	case verificationPassed:
		return "passed"
	case verificationFailed:
		return "failed"
	default:
		return "pending"
	}
}

func (m Model) verificationSummary() string {
	if len(m.verificationStatus) == 0 {
		return "verification not configured"
	}
	passed := 0
	for _, status := range m.verificationStatus {
		if status.phase == verificationFailed {
			return "verification failed"
		}
		if status.phase == verificationPassed {
			passed++
		}
	}
	if passed == len(m.verificationStatus) {
		return "verification passed"
	}
	return fmt.Sprintf("verification %d/%d complete", passed, len(m.verificationStatus))
}
