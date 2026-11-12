package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/jobs"
	"github.com/gongahkia/gator/internal/state"
	"github.com/gongahkia/gator/internal/uisnapshot"
	"github.com/gongahkia/gator/internal/workapp"
)

// Timing edits copy the canonical definition, changing only the selected timing
// fields. In particular, they never rebuild legacy policies as goal-watch drafts.
func checkTimingControl(ctx context.Context, stateDir, action string, args []string) (workapp.ControlScreen, error) {
	if err := ctx.Err(); err != nil {
		return workapp.ControlScreen{}, err
	}
	if len(args) < 2 {
		return workapp.ControlScreen{}, errors.New("refresh Following and select the check to edit")
	}
	if action != "status" && action != "preview-timing" && action != "save-timing" {
		return workapp.ControlScreen{}, errors.New("invalid timing action")
	}
	if action == "status" && len(args) != 2 {
		return workapp.ControlScreen{}, errors.New("select one saved check")
	}
	store, err := jobs.Open(stateDir)
	if err != nil {
		return workapp.ControlScreen{}, err
	}
	prior, err := store.Load(args[0])
	if err != nil {
		return workapp.ControlScreen{}, fmt.Errorf("this check is unavailable; refresh Following: %w", err)
	}
	digest, err := prior.PolicyDigest()
	if err != nil {
		return workapp.ControlScreen{}, err
	}
	if digest != args[1] {
		return workapp.ControlScreen{}, errors.New("this check changed; refresh Following and review its current agreement")
	}
	if prior.GoalWatch {
		return workapp.ControlScreen{}, errors.New("edit this project's polling agreement from Following")
	}
	if action == "status" {
		return checkTimingScreen(prior), nil
	}
	next, err := changedCheckTiming(prior, args[2:], time.Now())
	if err != nil {
		return workapp.ControlScreen{}, err
	}
	if err := ctx.Err(); err != nil {
		return workapp.ControlScreen{}, err
	}
	if action == "save-timing" {
		release, err := state.Guard(stateDir, false)
		if err != nil {
			return workapp.ControlScreen{}, err
		}
		defer release()
		saved, err := store.SaveReviewed(next, digest)
		if err != nil {
			return workapp.ControlScreen{}, err
		}
		screen := checkTimingScreen(saved)
		screen.RefreshChecks = true
		screen.Panel.Notice = "Timing saved. Existing work may finish. The check's enabled state, accounts, sources and permissions are retained. No check or service was started."
		screen.Panel.NoticeTone = "success"
		return screen, nil
	}
	proposed, err := next.PolicyDigest()
	if err != nil {
		return workapp.ControlScreen{}, err
	}
	lines := []string{"Check: " + prior.Name + " · " + prior.ID,
		"Previous schedule: " + prior.Schedule + " · " + prior.Timezone,
		"Proposed schedule: " + next.Schedule + " · " + next.Timezone,
		"Missed checks: " + missedCheckLabel(next.Missed),
		"Ends: " + checkEndLabel(next.EndsAt),
		"Enabled state retained: " + strconv.FormatBool(next.Enabled),
		"An existing check may finish. Future checks use the saved schedule while this computer is awake, the local service is running, and its sources are accessible.",
		"Saving does not run a check, start the service, resume a goal, or grant permissions. Other checks stay unchanged."}
	lines = append(lines, checkTimingNext(next, time.Now())...)
	return workapp.ControlScreen{Panel: uisnapshot.Panel{Kind: "confirmation", Title: "Review check timing", PresentationID: "check-timing-review:" + prior.ID,
		ActionIdentity: digest + ":" + proposed, Blocks: []uisnapshot.Block{{Title: "Exact timing change", Lines: lines}, {Title: "Retained agreement", Lines: checkTimingAgreement(prior), Collapsible: true}}},
		Actions: []workapp.ControlAction{{Key: "Enter", Label: "Save timing", Action: "save-timing", Args: append([]string(nil), args...)}}}, nil
}

func changedCheckTiming(prior jobs.Definition, values []string, now time.Time) (jobs.Definition, error) {
	next := prior
	if len(values) == 0 {
		return next, errors.New("choose the timing change")
	}
	switch values[0] {
	case "daily", "weekdays":
		if len(values) != 3 {
			return next, errors.New("provide a local time and timezone")
		}
		clock, err := time.Parse("15:04", values[1])
		if err != nil || clock.Format("15:04") != values[1] {
			return next, errors.New("use a 24-hour time such as 09:30")
		}
		days := "*"
		if values[0] == "weekdays" {
			days = "1-5"
		}
		next.Schedule, next.Timezone = fmt.Sprintf("%d %d * * %s", clock.Minute(), clock.Hour(), days), values[2]
	case "hourly":
		if len(values) != 3 {
			return next, errors.New("provide the minute of the hour and timezone")
		}
		minute, err := strconv.Atoi(values[1])
		if err != nil || minute < 0 || minute > 59 {
			return next, errors.New("choose a minute from 0 to 59")
		}
		next.Schedule, next.Timezone = fmt.Sprintf("%d * * * *", minute), values[2]
	case "custom":
		if len(values) != 3 {
			return next, errors.New("provide a five-field schedule and timezone")
		}
		next.Schedule, next.Timezone = strings.TrimSpace(values[1]), values[2]
	case "missed":
		if len(values) != 2 || values[1] != "skip" && values[1] != "run_once" {
			return next, errors.New("choose skip or one catch-up check")
		}
		next.Missed = values[1]
	case "end":
		if len(values) != 2 {
			return next, errors.New("provide an end time, or leave it blank")
		}
		next.EndsAt = time.Time{}
		if values[1] != "" {
			end, err := time.Parse(time.RFC3339, values[1])
			if err != nil || !end.After(now) {
				return next, errors.New("choose a future end date with a time and timezone, or leave it blank")
			}
			next.EndsAt = end
		}
	default:
		return next, errors.New("choose a supported timing change")
	}
	if err := next.Validate(); err != nil {
		return next, err
	}
	schedule, err := jobs.Schedule(next)
	if err != nil {
		return next, err
	}
	if schedule.Next(now).IsZero() {
		return next, errors.New("this schedule has no next occurrence; choose a valid date pattern")
	}
	return next, nil
}

func checkTimingScreen(definition jobs.Definition) workapp.ControlScreen {
	digest, _ := definition.PolicyDigest()
	args := []string{definition.ID, digest}
	clock, minute := "09:00", "0"
	parts := strings.Fields(definition.Schedule)
	if len(parts) == 5 {
		m, em := strconv.Atoi(parts[0])
		h, eh := strconv.Atoi(parts[1])
		if em == nil {
			minute = strconv.Itoa(m)
		}
		if em == nil && eh == nil {
			clock = fmt.Sprintf("%02d:%02d", h, m)
		}
	}
	timeField := workapp.ControlField{Label: "Local time (24-hour)", Value: clock, Help: "For example 09:30. The timezone below determines the local day."}
	zoneField := workapp.ControlField{Label: "Timezone", Value: definition.Timezone, Help: "IANA name, for example Asia/Singapore or America/New_York. Local follows this computer's timezone."}
	action := func(label, kind string, fields ...workapp.ControlField) workapp.ControlAction {
		return workapp.ControlAction{Label: label, Action: "preview-timing", Args: append(append([]string(nil), args...), kind), Fields: fields, Preview: true,
			Description: "Review the computed schedule before saving. Accounts, source selection, model, budgets and permissions are retained."}
	}
	end := ""
	if !definition.EndsAt.IsZero() {
		end = definition.EndsAt.Format(time.RFC3339)
	}
	return workapp.ControlScreen{Panel: uisnapshot.Panel{Kind: "controls", Title: "Check timing", Subtitle: definition.Name, ActionIdentity: digest,
		PresentationID: "check-timing:" + definition.ID, Summary: []uisnapshot.Field{{Label: "Schedule", Value: definition.Schedule + " · " + definition.Timezone}, {Label: "Missed checks", Value: missedCheckLabel(definition.Missed)}},
		Blocks: []uisnapshot.Block{{Title: "Next scheduled times", Lines: checkTimingNext(definition, time.Now()), Collapsible: true}, {Title: "Retained agreement", Lines: checkTimingAgreement(definition), Collapsible: true}}},
		Actions: []workapp.ControlAction{
			action("Daily at a local time", "daily", timeField, zoneField),
			action("Weekdays at a local time", "weekdays", timeField, zoneField),
			action("Hourly at a chosen minute", "hourly", workapp.ControlField{Label: "Minute of the hour (0–59)", Value: minute, Kind: "number", Help: "For example 15 runs at 10:15, 11:15, and so on."}, zoneField),
			action("Custom five-field schedule", "custom", workapp.ControlField{Label: "Minute hour day month weekday", Value: definition.Schedule, Help: "For example 30 9 * * 1-5 means weekdays at 09:30. Use * for every value; seconds are not supported."}, zoneField),
			action("Missed-check behavior", "missed", workapp.ControlField{Label: "When this computer returns", Value: definition.Missed, Kind: "select", Options: []uisnapshot.Option{{Value: "skip", Label: "Skip missed checks; wait for the next due time"}, {Value: "run_once", Label: "Run one catch-up check, then follow the schedule"}}}),
			action("Change or remove end date", "end", workapp.ControlField{Label: "End date and time (blank for no end)", Value: end, Kind: "datetime", Help: "Include a timezone, for example 2030-11-12T18:00:00+08:00. An existing check may finish."}),
			{Label: "All recurring checks", Command: "/followup"},
		}}
}

func missedCheckLabel(value string) string {
	if value == "run_once" {
		return "One catch-up check after returning, then the regular schedule"
	}
	return "Skip missed checks and wait for the next scheduled time"
}
func checkEndLabel(value time.Time) string {
	if value.IsZero() {
		return "No end date; pause this check to stop future runs"
	}
	return value.Format(time.RFC3339)
}
func checkTimingNext(definition jobs.Definition, now time.Time) []string {
	if !definition.EndsAt.IsZero() && !definition.EndsAt.After(now) {
		return []string{"Ended. Review a new end date before resuming checks."}
	}
	schedule, err := jobs.Schedule(definition)
	if err != nil {
		return []string{"Next time unavailable: " + err.Error()}
	}
	location, _ := time.LoadLocation(definition.Timezone)
	lines := []string{"Scheduled occurrences; actual checks also require an active goal (if linked), available sources and the local service."}
	if !definition.Enabled {
		lines = append(lines, "Paused. These are schedule occurrences only; resume separately to enable checks.")
	}
	occurrences := 0
	for i := 0; i < 3; i++ {
		next := schedule.Next(now)
		if next.IsZero() || !definition.EndsAt.IsZero() && !next.Before(definition.EndsAt) {
			break
		}
		lines = append(lines, next.In(location).Format("Mon 2 Jan 2006, 15:04 MST (Z07:00)"))
		occurrences++
		now = next
	}
	if occurrences == 0 {
		lines = append(lines, "No scheduled occurrence before the current end date.")
	}
	return lines
}
func checkTimingAgreement(definition jobs.Definition) []string {
	lines := []string{"Check: " + definition.ID, "Outcome: " + definition.Objective, "Workspace: " + definition.SourcePath,
		"Permission mode: " + string(definition.Mode), "Model: " + definition.Provider + "/" + definition.Model,
		fmt.Sprintf("Per attempt: %d model requests · %d tokens · %d seconds · %d steps", definition.Limits.ModelRequests, definition.Limits.Tokens, definition.Limits.WallSeconds, definition.MaxSteps),
		"Sources: " + strings.Join(definition.ConnectorIDs, ", "), "Notification conditions: " + definition.AlertWhen,
		"Source snapshots, account pins, artifact contracts, code/web policies, linked goal and destination conversation stay unchanged.",
		"Sending or publishing still requires separate foreground approval."}
	for _, id := range definition.ConnectorIDs {
		if account := definition.OAuthAccountPins[id]; account != "" {
			lines = append(lines, "Retained account pin: "+id+" · "+account)
		}
	}
	return lines
}
