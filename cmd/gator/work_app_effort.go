package main

import (
	"context"
	"errors"

	"github.com/gongahkia/gator/internal/state"
	"github.com/gongahkia/gator/internal/workrun"
	"github.com/gongahkia/gator/internal/worksession"
)

const workEffortEffect = "Applies to future foreground messages in this conversation. Existing running, queued and reviewed tasks keep their captured budgets. Background commitments keep their own limits. Step limits are not a price estimate; shared provider request/token limits still apply."

func (c gatorAppController) inspectWorkEffort(conversation string) (any, error) {
	if conversation == "" && c.request != nil {
		conversation = c.request.ConversationID
		if c.request.OriginConversationID != "" {
			conversation = c.request.OriginConversationID
		}
	}
	if conversation == "" {
		return map[string]any{"available": false, "effect": workEffortEffect}, nil
	}
	s, err := worksession.Open(c.stateDir)
	if err != nil {
		return nil, err
	}
	preferences, err := s.LoadPreferences(conversation)
	if err != nil {
		return nil, err
	}
	options, err := loadWorkTUIConversationOptions(s, conversation)
	if err != nil {
		return nil, err
	}
	result := map[string]any{"available": true, "conversation_id": conversation, "saved": preferences.WorkEffort != nil, "work_steps": options.MaxSteps, "code_steps": options.Code.MaxSteps, "choices": []string{"low", "standard", "high"}, "effect": workEffortEffect}
	if preferences.WorkEffort != nil {
		result["effort"] = *preferences.WorkEffort
	}
	if c.request != nil {
		result["current_task"] = map[string]any{"conversation_id": c.request.ConversationID, "work_steps": c.request.MaxSteps, "code_steps": c.request.Code.MaxSteps, "kind": "captured_request_limits", "note": "Runtime role policies can further narrow these ceilings. The run's execution accounting retains the applied limits."}
	}
	return result, nil
}

func (c gatorAppController) prepareWorkEffort(change workrun.AppChange) (workrun.AppMutation, error) {
	var value struct {
		Effort string `json:"effort"`
	}
	if err := workrun.DecodeAppArguments(change.Value, &value); err != nil {
		return workrun.AppMutation{}, err
	}
	budget, err := worksession.WorkEffortBudget(value.Effort)
	if err != nil {
		return workrun.AppMutation{}, err
	}
	if change.ConversationID == "" {
		return workrun.AppMutation{}, errors.New("a saved conversation is required for work effort")
	}
	s, err := worksession.Open(c.stateDir)
	if err != nil {
		return workrun.AppMutation{}, err
	}
	prior, err := s.LoadPreferences(change.ConversationID)
	if err != nil {
		return workrun.AppMutation{}, err
	}
	before, err := c.inspectWorkEffort(change.ConversationID)
	if err != nil {
		return workrun.AppMutation{}, err
	}
	return workrun.AppMutation{AfterRun: true, Summary: "Work effort saved for future messages in this conversation: " + value.Effort + ".", Preview: map[string]any{"setting": "work_effort", "before": before, "after": budget, "effect": workEffortEffect}, Apply: func(ctx context.Context) (any, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		release, err := state.Guard(c.stateDir, false)
		if err != nil {
			return nil, err
		}
		defer release()
		next := prior
		next.WorkEffort = &value.Effort
		if err := s.SavePreferences(change.ConversationID, next, &prior); err != nil {
			return nil, err
		}
		return c.inspectWorkEffort(change.ConversationID)
	}}, nil
}
