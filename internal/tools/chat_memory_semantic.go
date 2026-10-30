package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/memory"
)

func (t rememberChat) semanticRemember(ctx context.Context, input memory.ChatInput, scope string, unrecognizedRequest bool, suitabilityReview string) (agent.ToolResult, error) {
	// Learning already enabled authorizes ordinary passive statements even if
	// a model needlessly sets explicit=true. Off-mode and actual unfamiliar
	// restoration requests still require a fresh host confirmation.
	if suitabilityReview != "" || unrecognizedRequest && !t.Automatic {
		reason := suitabilityReview
		if unrecognizedRequest {
			reason += " Confirm this is an explicit request to remember these words."
		}
		if err := t.review(ctx, map[string]any{"setting": "memory", "action": "remember", "saved_text": input.Quote, "scope": scope, "reason": strings.TrimSpace(reason), "effect": "Save these exact words for future chats if their semantic identity can be reconciled."}); err != nil {
			return agent.ToolResult{}, err
		}
		if unrecognizedRequest {
			input.Explicit = true
		}
	}
	plan, err := t.Store.AnalyzeChat(ctx, t.Semantics, input, t.UserMessage)
	if err != nil {
		return agent.ToolResult{}, fmt.Errorf("memory was not saved because semantic comparison could not complete: %w", err)
	}
	if plan.Relation == memory.Withdrawn && !input.Explicit && unrecognizedRequest {
		if err := t.review(ctx, map[string]any{"setting": "memory", "action": "restore", "saved_text": input.Quote, "scope": scope, "reason": "The request's remember intent needs confirmation for a previously withdrawn topic.", "effect": "Explicitly restore this topic using the current words. Old historical sources remain suppressed."}); err != nil {
			return agent.ToolResult{}, err
		}
		input.Explicit = true
	}
	reviewed := false
	if len(plan.MatchIDs) > 1 && (plan.Relation == memory.Equivalent || plan.Relation == memory.Contradictory) {
		var matches []memory.SemanticCandidate
		for _, candidate := range plan.Snapshot.Candidates {
			for _, id := range plan.MatchIDs {
				if candidate.ID == id {
					matches = append(matches, candidate)
				}
			}
		}
		if err := t.review(ctx, map[string]any{"setting": "memory", "action": "reconcile", "saved_text": input.Quote, "scope": scope, "existing_memories": matches, "reason": "Multiple saved records may describe this concept. Review all exact inputs before consolidation.", "effect": "Consolidate these inspected memories and replace their representation with the current exact user statement."}); err != nil {
			return agent.ToolResult{}, err
		}
		reviewed = true
	}
	if plan.Relation == memory.Uncertain {
		return agent.ToolResult{}, errors.New("memory meaning remains uncertain; inspect and reconcile the saved records before adding another authoritative fact")
	}
	record, err := t.Store.ApplySemanticChat(ctx, input, plan, reviewed)
	if err != nil {
		return agent.ToolResult{}, err
	}
	content, err := success(map[string]any{"saved": true, "memory": record, "scope": scope, "semantic_relation": plan.Relation})
	return agent.ToolResult{Content: content}, err
}
