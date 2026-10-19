package workhome

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
	"unicode"

	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/inbox"
	"github.com/gongahkia/gator/internal/suggestion"
	"github.com/gongahkia/gator/internal/usage"
	"github.com/gongahkia/gator/internal/workhistory"
	"github.com/gongahkia/gator/internal/workrun"
)

type OverviewItem struct {
	ID         string    `json:"id"`
	Kind       string    `json:"kind"`
	Target     string    `json:"target,omitempty"`
	GoalID     string    `json:"goal_id,omitempty"`
	SourcePath string    `json:"source_path,omitempty"`
	DecisionID int       `json:"decision_id,omitempty"`
	Title      string    `json:"title"`
	Detail     string    `json:"detail"`
	At         time.Time `json:"at,omitempty"`
}
type OverviewSection struct {
	ID      string         `json:"id"`
	Title   string         `json:"title"`
	Summary string         `json:"summary"`
	Total   int            `json:"total"`
	Items   []OverviewItem `json:"items"`
}
type Overview struct {
	At       time.Time         `json:"at"`
	Since    time.Time         `json:"since"`
	Reviewed bool              `json:"reviewed"`
	Sections []OverviewSection `json:"sections"`
}

func concise(value string) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, value)
	runes := []rune(strings.Join(strings.Fields(value), " "))
	if len(runes) > 240 {
		return string(runes[:240]) + "…"
	}
	return string(runes)
}

// ReadOverview joins durable product facts without any model or connector call.
// Failed reads become explicit blockers, never empty or healthy sections.
func ReadOverview(ctx context.Context, stateDir string, supervisor bool, now time.Time) (Overview, error) {
	view := Overview{At: now.UTC(), Since: now.Add(-24 * time.Hour).UTC(), Sections: []OverviewSection{
		{ID: "changed", Title: "What changed", Items: []OverviewItem{}},
		{ID: "needs", Title: "What needs me", Items: []OverviewItem{}},
		{ID: "next", Title: "What happens next", Items: []OverviewItem{}},
		{ID: "blocked", Title: "What is blocked", Items: []OverviewItem{}},
	}}
	var seen struct{ At time.Time }
	data, err := os.ReadFile(filepath.Join(stateDir, "gator", "overview", "seen.json"))
	if err == nil {
		err = json.Unmarshal(data, &seen)
		if err == nil && !seen.At.IsZero() {
			view.Since = seen.At
			view.Reviewed = true
		}
	}
	add := func(section int, item OverviewItem) {
		item.Title = concise(item.Title)
		item.Detail = concise(item.Detail)
		view.Sections[section].Items = append(view.Sections[section].Items, item)
	}
	issue := func(area string, err error) {
		add(3, OverviewItem{ID: "read-" + area, Kind: "refresh", Title: "Could not read " + area, Detail: err.Error()})
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		issue("overview review time", err)
	}
	gs, err := goal.Open(stateDir)
	var goals []goal.Record
	if err == nil {
		goals, err = gs.List()
	}
	if err != nil {
		issue("goals", err)
	}
	for _, g := range goals {
		if err := ctx.Err(); err != nil {
			return Overview{}, err
		}
		if g.Status != goal.Active {
			continue
		}
		sourceInfo, sourceErr := os.Stat(g.SourcePath)
		sourceAvailable := sourceErr == nil && sourceInfo.IsDir()
		if !sourceAvailable {
			add(3, OverviewItem{ID: "source-" + g.ID, Kind: "source", SourcePath: g.SourcePath, Target: g.ID, GoalID: g.ID, Title: g.Title + ": source unavailable", Detail: "Review the source folder before continuing this goal."})
		}
		plan, err := gs.LoadPlan(g.ID)
		if err != nil {
			issue("plan for "+g.Title, err)
			continue
		}
		if !plan.NextReviewAt.IsZero() {
			section := 2
			if !plan.NextReviewAt.After(now) {
				section = 1
			}
			add(section, OverviewItem{ID: "review-" + g.ID, Kind: "plan", Target: g.ID, GoalID: g.ID, Title: "Review " + g.Title, Detail: "Scheduled goal review", At: plan.NextReviewAt})
		}
		nextAdded := false
		for _, step := range plan.Steps {
			if step.Status == goal.StepDone {
				continue
			}
			section := 2
			if step.Status == goal.StepBlocked {
				section = 3
			} else if !step.DueAt.IsZero() && !step.DueAt.After(now) {
				section = 1
			} else if step.DueAt.IsZero() && nextAdded {
				continue
			}
			add(section, OverviewItem{ID: "step-" + g.ID + "-" + step.ID, Kind: "plan", Target: g.ID, GoalID: g.ID, Title: g.Title + ": " + step.Title, Detail: "Step " + string(step.Status), At: step.DueAt})
			if section == 2 {
				nextAdded = true
			}
		}
		if sourceAvailable && !nextAdded && plan.NextReviewAt.IsZero() {
			add(2, OverviewItem{ID: "continue-" + g.ID, Kind: "goal", Target: g.ID, GoalID: g.ID, Title: "Continue " + g.Title, Detail: "Active goal · choose or review its next step"})
		}
	}
	is, err := inbox.Open(stateDir)
	var entries []inbox.Entry
	if err == nil {
		entries, err = is.List(false, 0)
	}
	if err != nil {
		issue("follow-up results", err)
	}
	for _, entry := range entries {
		item := OverviewItem{ID: "inbox-" + entry.ID, Kind: "inbox", Target: entry.ID, GoalID: entry.GoalID, Title: entry.Title, Detail: entry.Summary, At: entry.CreatedAt}
		if entry.CreatedAt.After(view.Since) {
			add(0, item)
		}
		if entry.ReadAt.IsZero() {
			item.Detail = "Unread · " + item.Detail
			add(1, item)
		}
	}
	ss, err := suggestion.Open(stateDir)
	var proposals []suggestion.Record
	if err == nil {
		proposals, err = ss.List("pending")
	}
	if err != nil {
		issue("proposed changes", err)
	}
	for _, p := range proposals {
		add(1, OverviewItem{ID: "suggestion-" + p.ID, Kind: "suggestion", Target: p.ID, GoalID: p.GoalID, Title: "Review proposed " + p.Kind + ": " + p.Title, Detail: "Not active until you review and accept it", At: p.CreatedAt})
	}
	hs, err := workhistory.Open(stateDir)
	var history []workhistory.Record
	if err == nil {
		history, err = hs.List(0)
	}
	if err != nil {
		issue("Work history", err)
	}
	for _, run := range history {
		if run.Status != workhistory.Running && run.FinishedAt.After(view.Since) {
			add(0, OverviewItem{ID: "work-" + run.ID, Kind: "work", Target: run.ID, Title: run.ObjectiveSummary, Detail: "Work " + string(run.Status), At: run.FinishedAt})
		}
	}
	activity, err := workrun.ReadActivity(stateDir)
	if err != nil {
		issue("running Work", err)
	}
	for _, run := range activity.Runs {
		if run.LiveStatus != "active" {
			add(3, OverviewItem{ID: "interrupted-" + run.ID, Kind: "work", Target: run.ID, Title: run.Objective, Detail: "Interrupted Work · no live process confirmed", At: run.StartedAt})
			continue
		}
		if len(run.PendingDecisions) == 0 {
			add(2, OverviewItem{ID: "running-" + run.ID, Kind: "work", Target: run.ID, Title: run.Objective, Detail: "Work is running", At: run.StartedAt})
		}
		for _, decision := range run.PendingDecisions {
			if decision.LiveStatus != "active" {
				add(3, OverviewItem{ID: fmt.Sprintf("stale-decision-%s-%d", run.ID, decision.ID), Kind: "work", Target: run.ID, Title: run.Objective, Detail: "A retained decision has no confirmed live review. Inspect the Work record.", At: run.StartedAt})
				continue
			}
			add(1, OverviewItem{ID: fmt.Sprintf("decision-%s-%d", run.ID, decision.ID), Kind: "decision", Target: run.ID, DecisionID: decision.ID, Title: run.Objective, Detail: "Waiting for your " + decision.Kind + " response; open the exact review", At: run.StartedAt})
		}
	}
	checks, err := ReadChecks(stateDir, supervisor, now)
	if err != nil {
		issue("scheduled checks", err)
	}
	enabled := 0
	for _, check := range checks {
		if check.Enabled && check.State != "goal paused" {
			enabled++
		}
		item := OverviewItem{ID: "check-" + check.ID, Kind: "check", Target: check.ID, GoalID: check.GoalID, Title: check.Title, Detail: check.HealthLine(), At: check.NextDue}
		if check.Issue != "" || check.State == "couldn't check" || check.State == "daily limit" || check.State == "shared limit" {
			item.Detail = check.Issue + " · " + check.NextAction
			add(3, item)
		}
		if !check.NextDue.IsZero() {
			add(2, item)
		}
	}
	if enabled > 0 && !supervisor {
		add(3, OverviewItem{ID: "supervisor", Kind: "recovery", Title: "Background checks are offline", Detail: fmt.Sprintf("%d enabled checks need this computer and its supervisor. No continuous monitoring confirmed.", enabled)})
	}
	ledger, err := usage.Open(stateDir)
	if err == nil {
		v, readErr := ledger.Snapshot(now)
		if readErr != nil {
			err = readErr
		} else {
			reason := ""
			switch {
			case v.Policy.Paused:
				reason = "New model and embedding requests are paused"
			case v.Policy.DailyRequests > 0 && v.Requests >= v.Policy.DailyRequests:
				reason = "Shared daily request allowance is exhausted"
			case v.Policy.DailyTokens > 0 && v.Tokens >= v.Policy.DailyTokens:
				reason = "Shared token admission allowance is exhausted"
			case v.Policy.ConcurrentRequests > 0 && v.ActiveRequests >= v.Policy.ConcurrentRequests:
				reason = "All shared concurrent request slots are occupied"
			}
			if reason != "" {
				add(3, OverviewItem{ID: "usage", Kind: "usage", Title: reason, Detail: "Review shared usage; existing per-check limits also apply."})
			}
		}
	}
	if err != nil {
		issue("shared usage", err)
	}
	// Restore receipts remain actionable when external projects were unavailable.
	if data, err := os.ReadFile(filepath.Join(stateDir, "gator", "data-control", "restored.json")); err == nil && len(data) > 0 && !restoreReviewDone(stateDir) {
		// Missing source folders are detected above for active goals. The receipt
		// points to mappings and recovery guidance for historical-only sources too.
		add(2, OverviewItem{ID: "restored", Kind: "data", Title: "Review this restored profile", Detail: "Set up providers, map missing sources, and review paused checks before enabling them."})
	}
	for i := range view.Sections {
		section := &view.Sections[i]
		sort.SliceStable(section.Items, func(a, b int) bool {
			x, y := section.Items[a], section.Items[b]
			if x.At.Equal(y.At) {
				return x.ID < y.ID
			}
			if i == 2 {
				if x.At.IsZero() {
					return false
				}
				if y.At.IsZero() {
					return true
				}
				return x.At.Before(y.At)
			}
			return x.At.After(y.At)
		})
		section.Total = len(section.Items)
		empty := []string{"No new retained results", "Nothing waiting for your review", "No scheduled next action", "No blockers found in the available records"}
		section.Summary = empty[i]
		if section.Total > 0 {
			section.Summary = fmt.Sprintf("%d · %s", section.Total, section.Items[0].Title)
		}
		if len(section.Items) > 50 {
			section.Items = section.Items[:50]
		}
	}
	return view, ctx.Err()
}

func MarkOverviewSeen(stateDir string, at time.Time) error {
	if at.IsZero() || at.After(time.Now().Add(time.Minute)) {
		return errors.New("invalid overview review time")
	}
	root := filepath.Join(stateDir, "gator", "overview")
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	info, err := os.Lstat(root)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("overview state must be an ordinary directory")
	}
	lock, err := os.OpenFile(filepath.Join(root, ".lock"), os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	var prior struct{ At time.Time }
	if data, err := os.ReadFile(filepath.Join(root, "seen.json")); err == nil {
		if err = json.Unmarshal(data, &prior); err != nil {
			return err
		}
	}
	if prior.At.After(at) {
		return nil
	}
	data, _ := json.Marshal(struct{ At time.Time }{at.UTC()})
	file, err := os.CreateTemp(root, ".seen-")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err = file.Chmod(0600); err == nil {
		_, err = file.Write(data)
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(file.Name(), filepath.Join(root, "seen.json"))
}

func restoreReviewDone(root string) bool {
	_, err := os.Stat(filepath.Join(root, "gator", "data-control", "restore-reviewed.json"))
	return err == nil
}
