package tools

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/suggestion"
)

// SuggestChange can only queue a reviewed candidate. It has no direct write
// path to active knowledge or a goal plan.
type SuggestChange struct {
	Store      suggestion.Store
	GoalID     string
	WorkID     string
	SourcePath string
}

func (SuggestChange) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{Name: "suggest_change", Description: "Queue a new ongoing goal, note, reusable skill, typed goal-plan change, or recurring goal schedule for user review. It does not activate the suggestion. A goal proposal needs title, context in body, goal_outcome, and goal_steps; optional cron/timezone/task schedules read-only follow-up only after acceptance. use_current_source attaches the host-selected Work source, never a model-supplied path. For notes include exact source references when available. Schedule suggestions require five-field cron, IANA timezone, and a bounded task; use change_only only for tasks independent of time passing.", Parameters: json.RawMessage(`{"type":"object","required":["kind"],"properties":{"kind":{"type":"string","enum":["goal","note","skill","plan","schedule"]},"title":{"type":"string","maxLength":256},"body":{"type":"string","maxLength":24576},"goal_outcome":{"type":"string","maxLength":2048},"goal_steps":{"type":"array","minItems":1,"maxItems":8,"items":{"type":"string","maxLength":512}},"use_current_source":{"type":"boolean"},"source_refs":{"type":"array","maxItems":15,"items":{"type":"string","maxLength":512}},"skill_id":{"type":"string","maxLength":64},"description":{"type":"string","maxLength":512},"plan_change":{"type":"object"},"cron":{"type":"string","maxLength":256},"task":{"type":"string","maxLength":8192},"timezone":{"type":"string","maxLength":128},"change_only":{"type":"boolean"}},"additionalProperties":false}`)}
}

func (t SuggestChange) Execute(_ context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	var args struct {
		Kind             string          `json:"kind"`
		Title            string          `json:"title"`
		Body             string          `json:"body"`
		GoalOutcome      string          `json:"goal_outcome"`
		GoalSteps        []string        `json:"goal_steps"`
		UseCurrentSource bool            `json:"use_current_source"`
		SourceRefs       []string        `json:"source_refs"`
		SkillID          string          `json:"skill_id"`
		Description      string          `json:"description"`
		PlanChange       goal.PlanChange `json:"plan_change"`
		Cron             string          `json:"cron"`
		Task             string          `json:"task"`
		Timezone         string          `json:"timezone"`
		ChangeOnly       bool            `json:"change_only"`
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
	record, err := t.Store.Create(suggestion.Record{Kind: args.Kind, Title: args.Title, Body: args.Body, GoalSourcePath: goalSource, GoalOutcome: args.GoalOutcome, GoalSteps: args.GoalSteps, SourceRefs: args.SourceRefs, SkillID: args.SkillID, Description: args.Description, GoalID: goalID, WorkID: t.WorkID, PlanChange: args.PlanChange, Cron: args.Cron, Task: args.Task, Timezone: args.Timezone, ChangeOnly: args.ChangeOnly})
	if err != nil {
		return agent.ToolResult{}, err
	}
	content, err := success(map[string]any{"id": record.ID, "status": record.Status, "kind": record.Kind, "review": "gator suggestion show " + record.ID + "; gator suggestion accept " + record.ID})
	return agent.ToolResult{Content: content}, err
}
