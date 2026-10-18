// Package workhome collects the small, read-only state needed to guide a
// person to the next useful action when they open Gator.
package workhome

import (
	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/inbox"
	"github.com/gongahkia/gator/internal/suggestion"
	"github.com/gongahkia/gator/internal/workhistory"
	"github.com/gongahkia/gator/internal/workrun"
)

type Attention struct {
	UnreadResults      int
	PendingSuggestions int
	ActiveGoals        int
	ActiveRuns         int
	RecentWork         bool
}

type Action struct {
	Label   string
	Command string
}

// Read uses the same durable stores as the individual CLI commands. Counts
// are navigation hints, not claims that a result or proposal is correct.
func Read(stateDir string, goals []goal.Record, entries []inbox.Entry) (Attention, error) {
	var attention Attention
	for _, record := range goals {
		if record.Status == goal.Active {
			attention.ActiveGoals++
		}
	}
	for _, entry := range entries {
		if entry.ReadAt.IsZero() {
			attention.UnreadResults++
		}
	}
	proposals, err := suggestion.Open(stateDir)
	if err != nil {
		return Attention{}, err
	}
	pending, err := proposals.List("pending")
	if err != nil {
		return Attention{}, err
	}
	attention.PendingSuggestions = len(pending)
	activity, err := workrun.ReadActivity(stateDir)
	if err != nil {
		return Attention{}, err
	}
	for _, run := range activity.Runs {
		if run.LiveStatus == "active" {
			attention.ActiveRuns++
		}
	}
	history, err := workhistory.Open(stateDir)
	if err != nil {
		return Attention{}, err
	}
	recent, err := history.List(1)
	if err != nil {
		return Attention{}, err
	}
	attention.RecentWork = len(recent) > 0
	return attention, nil
}

// Next lists the shortest useful routes before the user starts another task.
func (a Attention) Next() []Action {
	var actions []Action
	if a.PendingSuggestions > 0 {
		actions = append(actions, Action{Label: "Review proposed changes", Command: "/suggestions"})
	}
	if a.UnreadResults > 0 {
		actions = append(actions, Action{Label: "Read follow-up results", Command: "/inbox"})
	}
	if a.ActiveRuns > 0 {
		actions = append(actions, Action{Label: "Check running work", Command: "/activity"})
	}
	if a.ActiveGoals > 0 {
		actions = append(actions, Action{Label: "Continue an ongoing goal", Command: "/goals"})
	}
	if a.RecentWork {
		actions = append(actions, Action{Label: "Review recent work", Command: "/history"})
	}
	if len(actions) == 0 {
		return []Action{{Label: "Start with a task", Command: "type below"}}
	}
	return actions
}
