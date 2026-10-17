package main

import (
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/inbox"
	"github.com/gongahkia/gator/internal/jobs"
	"github.com/gongahkia/gator/internal/orchestrator"
	"github.com/gongahkia/gator/internal/suggestion"
)

// renderGoalActivity is the terminal's read-only view of one continuing goal.
// All sections are drawn from durable state so the view survives a restart.
func renderGoalActivity(stateDir, goalID string, out io.Writer) error {
	goals, err := goal.Open(stateDir)
	if err != nil {
		return err
	}
	record, err := goals.Load(goalID)
	if err != nil {
		return err
	}
	plan, err := goals.LoadPlan(goalID)
	if err != nil {
		return err
	}
	jobStore, err := jobs.Open(stateDir)
	if err != nil {
		return err
	}
	definitions, err := jobStore.List()
	if err != nil {
		return err
	}
	inboxStore, err := inbox.Open(stateDir)
	if err != nil {
		return err
	}
	entries, err := inboxStore.List(true, 0)
	if err != nil {
		return err
	}
	suggestionStore, err := suggestion.Open(stateDir)
	if err != nil {
		return err
	}
	suggestions, err := suggestionStore.List("pending")
	if err != nil {
		return err
	}
	var view bytes.Buffer
	fmt.Fprintf(&view, "%s  %s\nStatus: %s\n", record.ID, activityLine(record.Title, 120), record.Status)
	fmt.Fprintf(&view, "Goal desktop policy: %s (if globally enabled); quiet hours: %s\n", goalDesktopStatus(record.Notifications), goalQuietStatus(record.Notifications))
	if plan.Outcome != "" {
		fmt.Fprintf(&view, "Outcome: %s\n", activityLine(plan.Outcome, 160))
	}
	if !plan.NextReviewAt.IsZero() {
		fmt.Fprintf(&view, "Next plan review: %s\n", plan.NextReviewAt.Local().Format(time.RFC3339))
	}
	activeSteps := 0
	for _, step := range plan.Steps {
		if step.Status == goal.StepDone {
			continue
		}
		activeSteps++
		if activeSteps > 3 {
			continue
		}
		fmt.Fprintf(&view, "Step: %s [%s] %s", step.ID, step.Status, activityLine(step.Title, 100))
		if !step.DueAt.IsZero() {
			fmt.Fprintf(&view, " (due %s)", step.DueAt.Local().Format(time.RFC3339))
		}
		view.WriteByte('\n')
	}
	if activeSteps > 3 {
		fmt.Fprintf(&view, "... and %d more open steps; gator goal plan %s\n", activeSteps-3, goalID)
	}
	view.WriteString("\nChecks:\n")
	jobCount := 0
	now := time.Now()
	for _, definition := range definitions {
		if definition.GoalID != goalID {
			continue
		}
		jobCount++
		if jobCount > 8 {
			continue
		}
		kind := "schedule"
		if definition.GoalWatch {
			kind = "watch"
		}
		state := "disabled"
		if definition.Enabled && record.Status == goal.Active {
			state = "enabled"
		} else if definition.Enabled {
			state = "goal " + string(record.Status)
		}
		fmt.Fprintf(&view, "  %s %s (%s): %s\n", kind, definition.ID, state, activityLine(definition.Objective, 100))
		for _, readiness := range goalConnectorReadiness(definition) {
			fmt.Fprintf(&view, "    source: %s\n", activityLine(readiness, 180))
		}
		if definition.Enabled && record.Status == goal.Active {
			schedule, err := jobs.Schedule(definition)
			if err != nil {
				return err
			}
			fmt.Fprintf(&view, "    next: %s\n", schedule.Next(now).Format(time.RFC3339))
		}
		history, err := jobStore.History(definition.ID, 0)
		if err != nil {
			return err
		}
		if len(history) > 0 {
			last := history[0]
			fmt.Fprintf(&view, "    last: %s %s", last.ScheduledAt.Local().Format(time.RFC3339), last.Status)
			if last.Error != "" {
				fmt.Fprintf(&view, " — %s", activityLine(last.Error, 140))
			} else if last.Summary != "" {
				fmt.Fprintf(&view, " — %s", activityLine(last.Summary, 140))
			}
			view.WriteByte('\n')
			changes := last.WatchChanges
			if len(changes) == 0 {
				for _, prior := range history[1:] {
					if prior.Status == "completed" && len(prior.WatchChanges) > 0 {
						changes = prior.WatchChanges
						fmt.Fprintf(&view, "    last detected change: %s\n", prior.ScheduledAt.Local().Format(time.RFC3339))
						break
					}
				}
			}
			for index, change := range changes {
				if index == 3 {
					fmt.Fprintf(&view, "    ... and %d more changed items\n", len(changes)-3)
					break
				}
				fmt.Fprintf(&view, "    changed: %s %s %s\n", change.Kind, activityLine(change.Source, 60), activityLine(change.ID, 80))
			}
			if len(last.Runs) > 0 {
				run := last.Runs[len(last.Runs)-1]
				runID := run.RunID
				if runID == fmt.Sprintf("%s-try-%d", last.ID, run.Try) && filepath.Base(runID) == runID && !strings.ContainsAny(runID, "\\/") {
					tasks, err := orchestrator.ReadTasks(filepath.Join(stateDir, "gator", "tasks", runID))
					if err != nil {
						view.WriteString("    specialist checkpoints unavailable; inspect the Work run.\n")
					} else if len(tasks) > 0 {
						for index, task := range tasks {
							if index == 3 {
								fmt.Fprintf(&view, "    ... and %d more specialist tasks\n", len(tasks)-3)
								break
							}
							fmt.Fprintf(&view, "    specialist: %s %s [%s]\n", activityLine(task.ID, 40), activityLine(task.Role, 60), activityLine(task.Status, 30))
						}
						fmt.Fprintf(&view, "    inspect: gator work tasks %s\n", runID)
					}
				}
			}
		}
	}
	if jobCount == 0 {
		view.WriteString("  None. Add one with gator goal schedule or gator goal watch.\n")
	} else if jobCount > 8 {
		fmt.Fprintf(&view, "  ... and %d more; gator goal schedules %s\n", jobCount-8, goalID)
	}
	if jobCount > 0 {
		fmt.Fprintf(&view, "  Live source check: gator goal access %s (reads selected watch sources without changing watch state).\n", goalID)
		if err := jobSupervisorRequest("health", io.Discard); err != nil {
			view.WriteString("  Supervisor: offline; checks resume when its per-user service or foreground process runs.\n")
		} else {
			view.WriteString("  Supervisor: running.\n")
		}
	}
	view.WriteString("\nNeeds review:\n")
	count := 0
	for _, entry := range entries {
		if entry.GoalID != goalID {
			continue
		}
		count++
		if count <= 5 {
			fmt.Fprintf(&view, "  inbox %s [%s] %s\n", entry.ID, entry.Status, activityLine(entry.Summary, 140))
		}
	}
	for _, proposal := range suggestions {
		if proposal.GoalID != goalID {
			continue
		}
		count++
		if count <= 5 {
			fmt.Fprintf(&view, "  suggestion %s [%s] %s\n", proposal.ID, proposal.Kind, activityLine(suggestionLabel(proposal), 140))
		}
	}
	if count == 0 {
		view.WriteString("  Nothing pending.\n")
	} else if count > 5 {
		fmt.Fprintf(&view, "  ... and %d more; gator job inbox and gator suggestion list\n", count-5)
	}
	fmt.Fprintf(&view, "\nControl: gator goal chat %s | gator goal pause %s | gator goal resume %s | gator goal schedules %s | gator goal notifications %s\n", goalID, goalID, goalID, goalID, goalID)
	_, err = out.Write(view.Bytes())
	return err
}

func activityLine(value string, limit int) string {
	var clean strings.Builder
	for _, letter := range value {
		if unicode.IsControl(letter) {
			clean.WriteRune(' ')
		} else {
			clean.WriteRune(letter)
		}
	}
	value = strings.Join(strings.Fields(clean.String()), " ")
	letters := []rune(value)
	if len(letters) > limit {
		return string(letters[:limit-1]) + "…"
	}
	return value
}
