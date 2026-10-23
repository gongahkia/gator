package workhome

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/jobs"
)

// Check is a read-only projection of a schedule and its observed execution.
// NextDue is a schedule time, never a promise that an offline host will run it.
type Check struct {
	CoverageGap                                          jobs.CoverageGap
	Unsuccessful                                         int
	ID, GoalID, Title, Outcome, State, Issue, NextAction string
	LastSuccess, LastAttempt, NextDue                    time.Time
	Enabled                                              bool
	Definition                                           jobs.Definition
	ChecksToday, ModelRequests                           int
	Tokens                                               int64
	DurationSeconds                                      float64
}

func ReadChecks(stateDir string, supervisor bool, now time.Time) ([]Check, error) {
	store, err := jobs.Open(stateDir)
	if err != nil {
		return nil, err
	}
	goals, err := goal.Open(stateDir)
	if err != nil {
		return nil, err
	}
	definitions, err := store.List()
	if err != nil {
		return nil, err
	}
	availability, err := store.Availability()
	if err != nil {
		return nil, err
	}
	checks := make([]Check, 0, len(definitions))
	for _, definition := range definitions {
		policy, _ := definition.PolicyDigest()
		check := Check{ID: definition.ID, GoalID: definition.GoalID, Title: definition.Name, Outcome: definition.Objective, Enabled: definition.Enabled, Definition: definition, State: "waiting for first check", NextAction: "Review results in /inbox"}
		check.CoverageGap = jobs.LatestCoverageGap(availability, definition.CreatedAt)
		active := true
		if definition.GoalID != "" {
			record, loadErr := goals.Load(definition.GoalID)
			if loadErr != nil {
				return nil, loadErr
			}
			check.Title = record.Title
			active = record.Status == goal.Active
		}
		history, historyErr := store.History(definition.ID, 0)
		if historyErr != nil {
			return nil, historyErr
		}
		if len(history) > 0 {
			check.Unsuccessful = jobs.ConsecutiveUnsuccessfulChecks(history, history[0])
		}
		for i, attempt := range history {
			if i == 0 {
				check.LastAttempt = attempt.StartedAt
				check.State = attempt.Status
				if attempt.Status == "failed" || attempt.Status == "interrupted" {
					check.State, check.Issue, check.NextAction = "couldn't check", attempt.Error, "Review access and model setup, then retry the check"
				}
				if attempt.Status == "limited" {
					check.State, check.Issue, check.NextAction = "daily limit", attempt.Summary, "Model checks resume on the next UTC day; edit limits or pause this check"
					if attempt.LimitReason == "shared" {
						check.State = "shared limit"
						check.NextAction = "Open /usage to review or resume shared requests; concurrency limits clear when other requests finish"
					}
				}
				if attempt.NotificationPolicySHA256 != "" && attempt.NotificationPolicySHA256 != policy {
					check.State = "settings changed; waiting for check"
				}
			}
			if check.LastSuccess.IsZero() && attempt.NotificationPolicySHA256 == policy && (attempt.Status == "completed" || attempt.Status == "unchanged") {
				check.LastSuccess = attempt.FinishedAt
			}
			if sameDay(attempt.StartedAt, now.UTC()) {
				if len(attempt.Runs) > 0 {
					check.ChecksToday++
				}
				for _, run := range attempt.Runs {
					if run.BundlePath == "" {
						continue
					}
					bundle, bundleErr := artifact.OpenBundle(filepath.Clean(run.BundlePath))
					if bundleErr != nil {
						continue
					}
					check.ModelRequests += bundle.Manifest.Usage.ModelRequests
					check.Tokens += bundle.Manifest.Usage.InputTokens + bundle.Manifest.Usage.OutputTokens
				}
				if attempt.FinishedAt.After(attempt.StartedAt) {
					check.DurationSeconds += attempt.FinishedAt.Sub(attempt.StartedAt).Seconds()
				}
			}
		}
		check.ChecksToday, err = store.DailyChecks(definition.ID, now)
		if err != nil {
			return nil, err
		}
		if definition.Expired(now) {
			check.State, check.NextAction = "ended", "Review a new end date to continue this check"
		} else if !definition.Enabled || !active {
			check.State, check.NextAction = "paused", "Resume this check when ready"
			if !active {
				check.State, check.NextAction = "goal paused", "Resume this goal in Following"
			}
		} else if !supervisor {
			check.State, check.NextAction = "offline", "Start the local supervisor"
		} else {
			schedule, scheduleErr := jobs.Schedule(definition)
			if scheduleErr != nil {
				return nil, scheduleErr
			}
			check.NextDue = schedule.Next(now)
			if definition.Expired(check.NextDue) {
				check.NextDue = time.Time{}
			}
		}
		checks = append(checks, check)
	}
	return checks, nil
}

func sameDay(a, b time.Time) bool {
	a = a.In(b.Location())
	return a.Year() == b.Year() && a.YearDay() == b.YearDay()
}

func (c Check) HealthLine() string {
	last := "never"
	if !c.LastSuccess.IsZero() {
		last = c.LastSuccess.Local().Format("Jan 2 15:04")
	}
	next := "not running"
	if !c.NextDue.IsZero() {
		next = c.NextDue.Local().Format("Jan 2 15:04")
	}
	return fmt.Sprintf("%s · last successful check: %s · next due: %s", c.State, last, next)
}
