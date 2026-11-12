package workapp

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/goal"
)

type BridgeGoalAction struct {
	RequestID string `json:"request_id"`
	ID        string `json:"id"`
	Action    string `json:"action"`
	Digest    string `json:"digest,omitempty"`
}
type BridgeGoalReview struct {
	ID            string   `json:"id"`
	Digest        string   `json:"digest"`
	Title         string   `json:"title"`
	Status        string   `json:"status"`
	Lines         []string `json:"lines"`
	EnabledChecks int      `json:"enabled_checks"`
}
type BridgeGoalResult struct {
	RequestID string            `json:"request_id"`
	Error     string            `json:"error,omitempty"`
	Review    *BridgeGoalReview `json:"review,omitempty"`
}
type bridgeGoalDone struct {
	Action     BridgeGoalAction
	Record     goal.Record
	Plan       goal.Plan
	Goals      []goal.Record
	GoalsError error
	Checks     checksRefreshed
	Err        error
}

var bridgeGoalID = regexp.MustCompile(`^goal-[a-fA-F0-9]{24}$`)

func (m Model) ApplyBridgeGoal(action BridgeGoalAction) (tea.Model, tea.Cmd) {
	m.bridgeGoalResult = nil
	fail := func(text string) (tea.Model, tea.Cmd) {
		m.bridgeGoalResult = &BridgeGoalResult{RequestID: action.RequestID, Error: text}
		return m, nil
	}
	if !bridgeCheckRequest.MatchString(action.RequestID) || !bridgeGoalID.MatchString(action.ID) || action.Action != "inspect" && action.Action != "pause" && action.Action != "resume" {
		return fail("Invalid goal control.")
	}
	if action.Action == "inspect" && action.Digest != "" || action.Action != "inspect" && !bridgeCheckDigest.MatchString(action.Digest) {
		return fail("Review the current goal before changing its status.")
	}
	if m.config.ReadGoalReview == nil || m.config.ListGoals == nil || m.config.ReadChecks == nil || action.Action != "inspect" && m.config.SetGoalStatusReviewed == nil {
		return fail("Goal controls are unavailable.")
	}
	if m.bridgeGoalBusy || m.running && action.Action != "inspect" || m.followupSaving || m.controlBusy || m.bridgeExternal != nil || m.followupForm != nil {
		return fail("Finish the current operation before managing this goal.")
	}
	m.bridgeGoalBusy = true
	if action.Action != "inspect" {
		m.followupSaving = true
	}
	read, change, list, checks := m.config.ReadGoalReview, m.config.SetGoalStatusReviewed, m.config.ListGoals, m.config.ReadChecks
	return m, func() tea.Msg {
		done := bridgeGoalDone{Action: action}
		if action.Action == "inspect" {
			done.Record, done.Plan, done.Err = read(action.ID)
		} else {
			status := goal.Paused
			if action.Action == "resume" {
				status = goal.Active
			}
			done.Record, done.Err = change(action.ID, status, action.Digest)
		}
		done.Goals, done.GoalsError = list()
		done.Checks.Checks, done.Checks.Running, done.Checks.Err = checks()
		return done
	}
}

func (m Model) finishBridgeGoal(value bridgeGoalDone) (tea.Model, tea.Cmd) {
	m.bridgeGoalBusy = false
	if value.Action.Action != "inspect" {
		m.followupSaving = false
	}
	if value.GoalsError != nil {
		m.bridgeFollowingIssue = "Goal status could not be refreshed: " + value.GoalsError.Error()
	} else {
		m.config.Goals, m.bridgeFollowingIssue = value.Goals, ""
	}
	next, _ := m.Update(value.Checks)
	m = next.(Model)
	m.bridgeGoalResult = &BridgeGoalResult{RequestID: value.Action.RequestID}
	if value.Err != nil {
		m.bridgeGoalResult.Error = value.Err.Error()
		return m, nil
	}
	if value.Record.ID != value.Action.ID {
		m.bridgeGoalResult.Error = "The selected goal was not confirmed."
		return m, nil
	}
	if value.Action.Action == "pause" && value.Record.Status != goal.Paused || value.Action.Action == "resume" && value.Record.Status != goal.Active {
		m.bridgeGoalResult.Error = "The requested goal status was not confirmed. Refresh Following before retrying."
		return m, nil
	}
	if value.GoalsError != nil || value.Checks.Err != nil {
		if value.Action.Action == "inspect" {
			m.bridgeGoalResult.Error = "Goal read, but Following could not refresh. Inspect recovery before relying on its check status."
		} else {
			m.bridgeGoalResult.Error = fmt.Sprintf("Goal %s, but Following could not refresh. Its saved status changed; refresh before any further change.", value.Record.Status)
		}
		return m, nil
	}
	digest, err := value.Record.ReviewDigest()
	if err != nil {
		m.bridgeGoalResult.Error = err.Error()
		return m, nil
	}
	found := false
	for _, record := range value.Goals {
		if record.ID != value.Record.ID {
			continue
		}
		current, err := record.ReviewDigest()
		found = err == nil && current == digest
		break
	}
	if !found {
		m.bridgeGoalResult.Error = "This goal changed again before confirmation. Inspect its current state before retrying."
		return m, nil
	}
	if value.Action.Action == "inspect" && value.Plan.GoalID != value.Record.ID {
		m.bridgeGoalResult.Error = "The retained plan did not match the selected goal."
		return m, nil
	}
	review := &BridgeGoalReview{ID: value.Record.ID, Digest: digest, Title: value.Record.Title, Status: string(value.Record.Status)}
	review.Lines = []string{"Goal: " + value.Record.ID, "Status: " + string(value.Record.Status), "Outcome: " + value.Record.Context, "Workspace: " + value.Record.SourcePath}
	if strings.TrimSpace(value.Plan.Outcome) != "" {
		review.Lines = append(review.Lines, "Plan outcome: "+value.Plan.Outcome)
	}
	for _, step := range value.Plan.Steps {
		review.Lines = append(review.Lines, fmt.Sprintf("%s · %s", step.Status, step.Title))
		if !step.DueAt.IsZero() {
			review.Lines = append(review.Lines, "Due: "+step.DueAt.Format(time.RFC3339))
		}
		if !step.StartsAt.IsZero() {
			review.Lines = append(review.Lines, "Scheduled: "+step.StartsAt.Format(time.RFC3339)+" to "+step.EndsAt.Format(time.RFC3339))
		}
		if len(step.DependsOn) > 0 {
			review.Lines = append(review.Lines, "Depends on: "+strings.Join(step.DependsOn, ", "))
		}
		for _, evidence := range step.Evidence {
			review.Lines = append(review.Lines, "Evidence: "+evidence)
		}
		if repeat := step.Recurrence; repeat != nil {
			review.Lines = append(review.Lines, fmt.Sprintf("Repeats: %s every %d · %s", repeat.Frequency, repeat.Interval, repeat.Timezone))
			if !repeat.Until.IsZero() {
				review.Lines = append(review.Lines, "Repeats until: "+repeat.Until.Format(time.RFC3339))
			}
		}
		for _, occurrence := range step.Occurrences {
			review.Lines = append(review.Lines, fmt.Sprintf("Occurrence: %s · %s", occurrence.Date, occurrence.Status))
		}
	}
	if !value.Plan.NextReviewAt.IsZero() {
		review.Lines = append(review.Lines, "Next plan review: "+value.Plan.NextReviewAt.Format(time.RFC3339))
	}
	for _, check := range m.checks {
		if check.GoalID != value.Record.ID {
			continue
		}
		if check.Enabled {
			review.EnabledChecks++
		}
		review.Lines = append(review.Lines, fmt.Sprintf("Check: %s · %s · %s", check.ID, check.Title, check.State), "Next: "+check.NextAction)
		review.Lines = append(review.Lines, "Timing: "+check.Definition.Schedule+" · "+check.Definition.Timezone)
		if !check.NextDue.IsZero() {
			review.Lines = append(review.Lines, "Next check: "+check.NextDue.Format(time.RFC3339))
		}
		if !check.LastSuccess.IsZero() {
			review.Lines = append(review.Lines, "Last successful check: "+check.LastSuccess.Format(time.RFC3339))
		}
	}
	if review.EnabledChecks == 0 {
		review.Lines = append(review.Lines, "No enabled recurring check is attached. A saved goal alone is not automatic monitoring.")
	}
	review.Lines = append(review.Lines, "Checks require the local service, an awake computer and accessible sources. A check already running may finish.")
	m.bridgeGoalResult.Review = review
	return m, nil
}
