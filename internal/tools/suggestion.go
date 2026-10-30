package tools

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gongahkia/gator/internal/knowledge"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/suggestion"
)

// SuggestChange can only queue a reviewed candidate. It has no direct write
// path to active knowledge or a goal plan.
type SuggestChange struct {
	Notes       *knowledge.Store
	ExcludedIDs []string
	Store       suggestion.Store
	GoalID      string
	WorkID      string
	SourcePath  string
}

func (SuggestChange) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{Name: "suggest_change", Description: "Queue a new ongoing goal, note, reusable skill, typed goal-plan change, or recurring goal schedule for user review. It does not activate the suggestion. A goal proposal needs title, context in body, goal_outcome, and goal_steps; optional cron/timezone/task schedules read-only follow-up only after acceptance. use_current_source attaches the host-selected Work source, never a model-supplied path. For notes include exact source references when available. To update an existing note, read it with knowledge_read then supply note_id, expected_note_revision, title and the complete replacement body. Existing source references are preserved; do not create a duplicate note for a correction. Schedule suggestions require five-field cron, IANA timezone, and a bounded task; use change_only only for tasks independent of time passing.", Parameters: json.RawMessage(`{"type":"object","required":["kind"],"properties":{"kind":{"type":"string","enum":["goal","note","skill","plan","schedule"]},"title":{"type":"string","maxLength":256},"body":{"type":"string","maxLength":24576},"goal_outcome":{"type":"string","maxLength":2048},"goal_steps":{"type":"array","minItems":1,"maxItems":8,"items":{"type":"string","maxLength":512}},"use_current_source":{"type":"boolean"},"source_refs":{"type":"array","maxItems":15,"items":{"type":"string","maxLength":512}},"note_id":{"type":"string","maxLength":29},"expected_note_revision":{"type":"string","maxLength":64},"skill_id":{"type":"string","maxLength":64},"description":{"type":"string","maxLength":512},"plan_change":{"type":"object"},"cron":{"type":"string","maxLength":256},"task":{"type":"string","maxLength":8192},"timezone":{"type":"string","maxLength":128},"change_only":{"type":"boolean"}},"additionalProperties":false}`)}
}

func (t SuggestChange) Execute(_ context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	var args struct {
		NoteID               string          `json:"note_id"`
		ExpectedNoteRevision string          `json:"expected_note_revision"`
		Kind                 string          `json:"kind"`
		Title                string          `json:"title"`
		Body                 string          `json:"body"`
		GoalOutcome          string          `json:"goal_outcome"`
		GoalSteps            []string        `json:"goal_steps"`
		UseCurrentSource     bool            `json:"use_current_source"`
		SourceRefs           []string        `json:"source_refs"`
		SkillID              string          `json:"skill_id"`
		Description          string          `json:"description"`
		PlanChange           goal.PlanChange `json:"plan_change"`
		Cron                 string          `json:"cron"`
		Task                 string          `json:"task"`
		Timezone             string          `json:"timezone"`
		ChangeOnly           bool            `json:"change_only"`
	}
	if err := decodeArguments(raw, &args); err != nil {
		return agent.ToolResult{}, err
	}
	if args.Kind != "goal" && args.Kind != "note" && args.Kind != "skill" && args.Kind != "plan" && args.Kind != "schedule" {
		return agent.ToolResult{}, errors.New("suggestion kind must be goal, note, skill, plan, or schedule")
	}
	if args.Kind == "goal" && t.GoalID != "" {
		return agent.ToolResult{}, errors.New("new goal suggestions require the main conversation, not an existing goal")
	}
	if (args.Kind == "plan" || args.Kind == "schedule") && t.GoalID == "" {
		return agent.ToolResult{}, errors.New("plan and schedule suggestions require an active goal")
	}
	goalID := t.GoalID
	if args.NoteID != "" {
		if args.Kind != "note" || t.Notes == nil || slices.Contains(t.ExcludedIDs, args.NoteID) {
			return agent.ToolResult{}, errors.New("note is not available for updates in this run")
		}
		note, err := t.Notes.LoadNote(args.NoteID)
		if err != nil {
			return agent.ToolResult{}, err
		}
		if note.GoalID != "" && note.GoalID != t.GoalID {
			return agent.ToolResult{}, errors.New("note is outside this goal")
		}
		if args.ExpectedNoteRevision == "" || note.Revision() != args.ExpectedNoteRevision {
			return agent.ToolResult{}, errors.New("note changed; read it again before suggesting an update")
		}
		goalID = note.GoalID
	}
	if args.Kind == "skill" {
		goalID = ""
	}
	if args.Kind == "plan" && strings.TrimSpace(args.Body) != "" {
		return agent.ToolResult{}, errors.New("plan changes must use plan_change, not body text")
	}
	goalSource := ""
	if args.Kind == "goal" && args.UseCurrentSource {
		if t.SourcePath == "" {
			return agent.ToolResult{}, errors.New("this Work run has no selected source directory")
		}
		var err error
		goalSource, err = filepath.Abs(t.SourcePath)
		if err != nil {
			return agent.ToolResult{}, err
		}
		goalSource, err = filepath.EvalSymlinks(goalSource)
		if err != nil {
			return agent.ToolResult{}, err
		}
	}
	record, err := t.Store.Create(suggestion.Record{NoteID: args.NoteID, ExpectedNoteRevision: args.ExpectedNoteRevision, Kind: args.Kind, Title: args.Title, Body: args.Body, GoalSourcePath: goalSource, GoalOutcome: args.GoalOutcome, GoalSteps: args.GoalSteps, SourceRefs: args.SourceRefs, SkillID: args.SkillID, Description: args.Description, GoalID: goalID, WorkID: t.WorkID, PlanChange: args.PlanChange, Cron: args.Cron, Task: args.Task, Timezone: args.Timezone, ChangeOnly: args.ChangeOnly})
	if err != nil {
		return agent.ToolResult{}, err
	}
	content, err := success(map[string]any{"id": record.ID, "status": record.Status, "kind": record.Kind, "review": "gator suggestion show " + record.ID + "; gator suggestion accept " + record.ID})
	return agent.ToolResult{Content: content}, err
}
