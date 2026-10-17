package workrun

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/orchestrator"
	"github.com/gongahkia/gator/internal/workhistory"
)

// Activity is a bounded projection of currently running Work records and their
// retained specialist and decision checkpoints. It never includes decision
// previews or complete specialist inputs.
type Activity struct {
	Runs []ActivityRun `json:"runs"`
}

type ActivityRun struct {
	ID               string             `json:"id"`
	Objective        string             `json:"objective"`
	LiveStatus       string             `json:"live_status"`
	StartedAt        time.Time          `json:"started_at"`
	Tasks            []ActivityTask     `json:"tasks"`
	PendingDecisions []ActivityDecision `json:"pending_decisions"`
}

type ActivityTask struct {
	ID      string `json:"id"`
	Role    string `json:"role"`
	Status  string `json:"status"`
	Summary string `json:"summary"`
}

type ActivityDecision struct {
	ID         int    `json:"id"`
	Kind       string `json:"kind"`
	LiveStatus string `json:"live_status"`
}

// ReadActivity joins running Work history with retained specialist and
// decision state. A running record with a stopped lease is shown for recovery,
// but its task checkpoints are not proof that those tasks are still executing.
func ReadActivity(stateDir string) (Activity, error) {
	history, err := workhistory.Open(stateDir)
	if err != nil {
		return Activity{}, err
	}
	records, err := history.List(0)
	if err != nil {
		return Activity{}, err
	}
	activity := Activity{Runs: []ActivityRun{}}
	for _, record := range records {
		if record.Status != workhistory.Running {
			continue
		}
		run := ActivityRun{
			ID: record.ID, Objective: record.ObjectiveSummary,
			LiveStatus: record.LiveStatus, StartedAt: record.StartedAt,
			Tasks: []ActivityTask{}, PendingDecisions: []ActivityDecision{},
		}
		tasks, err := orchestrator.ReadTasks(filepath.Join(stateDir, "gator", "tasks", record.ID))
		if err != nil {
			return Activity{}, fmt.Errorf("read specialists for Work run %s: %w", record.ID, err)
		}
		for _, task := range tasks {
			if task.ParentRun != record.ID || task.GlobalID != record.ID+"/"+task.ID {
				return Activity{}, fmt.Errorf("specialist checkpoint does not belong to Work run %s", record.ID)
			}
			run.Tasks = append(run.Tasks, ActivityTask{ID: task.ID, Role: task.Role, Status: task.Status, Summary: activitySummary(task.SelectedInput)})
		}
		slices.SortFunc(run.Tasks, func(a, b ActivityTask) int { return strings.Compare(a.ID, b.ID) })
		decisions, err := ReadPendingInteractionRecordsForRun(stateDir, record.ID)
		if err != nil {
			return Activity{}, fmt.Errorf("read decisions for Work run %s: %w", record.ID, err)
		}
		for _, decision := range decisions {
			if decision.Status == "pending" {
				run.PendingDecisions = append(run.PendingDecisions, ActivityDecision{
					ID: decision.Interaction.ID, Kind: activitySummary(decision.Interaction.Kind), LiveStatus: decision.LiveStatus,
				})
			}
		}
		slices.SortFunc(run.PendingDecisions, func(a, b ActivityDecision) int { return a.ID - b.ID })
		activity.Runs = append(activity.Runs, run)
	}
	slices.SortStableFunc(activity.Runs, func(a, b ActivityRun) int {
		if a.LiveStatus == "active" && b.LiveStatus != "active" {
			return -1
		}
		if a.LiveStatus != "active" && b.LiveStatus == "active" {
			return 1
		}
		return 0 // history.List already orders newest first within each group.
	})
	return activity, nil
}

func activitySummary(value string) string {
	value = strings.TrimSpace(value)
	value, _, _ = strings.Cut(value, "\n")
	characters := []rune(value)
	if len(characters) > 96 {
		return string(characters[:96]) + "…"
	}
	return value
}
