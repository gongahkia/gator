package main

import (
	"time"

	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/jobs"
)

// Continue the latest successfully checked policy, never an obsolete policy or
// an older completion hidden behind a failure. Attempts remain the durable log.
func goalContinuationDue(history []jobs.Attempt, current jobs.Attempt, policy string, now time.Time) bool {
	for _, previous := range history {
		if previous.ID == current.ID || previous.Status == "running" || previous.Status == "unchanged" {
			continue
		}
		return previous.Status == "completed" && previous.NotificationPolicySHA256 == policy &&
			previous.WatchDecision != nil && !previous.WatchDecision.NextCheckAt.IsZero() &&
			!previous.WatchDecision.NextCheckAt.After(now)
	}
	return false
}

// Do not spend the daily allowance repeating one nominal next action forever.
// New observed input resets this bound; owner progress remains owner-reported.
func guardStalledContinuation(history []jobs.Attempt, current jobs.Attempt) jobs.Attempt {
	d := current.WatchDecision
	if d == nil || d.Continuation != "ready" || current.InputSHA256 == "" {
		return current
	}
	count := 1
	for _, previous := range history {
		if previous.ID == current.ID || previous.Status == "running" || previous.Status == "unchanged" {
			continue
		}
		p := previous.WatchDecision
		if previous.Status != "completed" || previous.NotificationPolicySHA256 != current.NotificationPolicySHA256 || previous.InputSHA256 != current.InputSHA256 || p == nil || p.Continuation != "ready" || p.NextStepID != d.NextStepID {
			break
		}
		count++
		if count == 3 {
			copy := *d
			copy.Status, copy.Continuation, copy.NextCheckAt, copy.Urgent = "review", "blocked", time.Time{}, false
			copy.Summary = "Repeated checks have not advanced the selected step; review the blocker."
			copy.Rationale = "Three checks requested the same next step with unchanged inputs. Automatic continuation is held until new input arrives."
			copy.NextStep = "Review the retained outputs and update the plan or source before resuming."
			current.WatchDecision, current.WatchAttention, current.Urgent, current.Summary = &copy, true, false, copy.Summary
			current.ResultSHA256 = "" // This hold must not be suppressed as an old result.
			break
		}
	}
	return current
}

func checkedGoalContinuation(result jobs.WatchDecision, proposed goalWatchDecision, plan goal.Plan, definition jobs.Definition, now time.Time, dependencies ...map[string]goal.StepStatus) jobs.WatchDecision {
	if proposed.Continuation == "" && proposed.NextCheckAt.IsZero() && proposed.NextStepID == "" {
		return result
	}
	invalid := func(reason string) jobs.WatchDecision {
		result.Status, result.Urgent = "review", false
		result.Rationale = "Continuation rejected: " + reason
		result.NextStep = "Review the current plan and this check before continuing."
		return result
	}
	if result.Status == "review" {
		return result
	}
	switch proposed.Continuation {
	case "ready", "waiting", "blocked", "idle":
	default:
		return invalid("unknown continuation state")
	}
	wake := proposed.NextCheckAt.UTC()
	if proposed.Continuation == "idle" && !wake.IsZero() {
		return invalid("idle checks cannot request a wake time")
	}
	if proposed.Continuation == "ready" {
		if proposed.NextStepID == "" || proposed.NextStep == "" {
			return invalid("ready work needs a plan step and concrete action")
		}
		if wake.IsZero() {
			wake = now.Add(time.Duration(max(5, definition.PollMinutes)) * time.Minute)
		}
	}
	if proposed.Continuation == "waiting" && wake.IsZero() {
		return invalid("waiting needs an explicit wake time")
	}
	if !wake.IsZero() && (wake.Before(now.Add(5*time.Minute)) || wake.After(now.Add(7*24*time.Hour)) || !definition.EndsAt.IsZero() && wake.After(definition.EndsAt)) {
		return invalid("wake time is outside the allowed window")
	}
	if proposed.NextStepID != "" {
		found := false
		states := map[string]goal.StepStatus{}
		for _, step := range plan.Steps {
			states[step.ID] = step.Status
		}
		if len(dependencies) > 0 {
			for id, value := range dependencies[0] {
				states[id] = value
			}
		}
		for _, step := range plan.Steps {
			if step.ID != proposed.NextStepID {
				continue
			}
			found = true
			if proposed.Continuation == "ready" {
				if step.Status == goal.StepDone || step.Status == goal.StepBlocked {
					return invalid("the selected step is completed or blocked")
				}
				for _, dependency := range step.DependsOn {
					if states[dependency] != goal.StepDone {
						return invalid("the selected step has unresolved dependencies")
					}
				}
			}
		}
		if !found {
			return invalid("the selected step no longer exists")
		}
	}
	result.Continuation, result.NextCheckAt, result.NextStepID = proposed.Continuation, wake, proposed.NextStepID
	return result
}
