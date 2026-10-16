package main

import (
	"encoding/json"
	"strings"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
)

// A quiet watch is an explicit model decision with no durable side effect.
// If the final answer is malformed, surface it for review rather than hide it.
func classifyGoalWatchResult(finalText string, messages []agent.Message, artifacts []artifact.File) (bool, string) {
	forcedAttention := len(artifacts) > 0
	failedSuggestion := false
	for _, message := range messages {
		if message.Role != agent.RoleTool || message.ToolName != "suggest_change" {
			continue
		}
		forcedAttention = true
		var result struct {
			OK bool `json:"ok"`
		}
		if json.Unmarshal([]byte(message.Content), &result) != nil || !result.OK {
			failedSuggestion = true
		}
	}
	var decision struct {
		Attention *bool    `json:"attention"`
		Summary   string   `json:"summary"`
		Evidence  []string `json:"evidence"`
	}
	value := strings.TrimSpace(finalText)
	if strings.HasPrefix(value, "```json\n") && strings.HasSuffix(value, "\n```") {
		value = strings.TrimSuffix(strings.TrimPrefix(value, "```json\n"), "\n```")
	}
	if json.Unmarshal([]byte(value), &decision) == nil && decision.Attention != nil && strings.TrimSpace(decision.Summary) != "" {
		if failedSuggestion {
			return true, "Goal watch could not queue its proposed change; review the Work run."
		}
		summary := strings.TrimSpace(decision.Summary)
		if len([]rune(summary)) > 360 {
			summary = string([]rune(summary)[:360])
		}
		return *decision.Attention || forcedAttention, summary
	}
	return true, summarizeJobResult(finalText, artifacts)
}
