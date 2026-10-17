package main

import (
	"encoding/json"
	"strings"
	"unicode"

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
		attention := *decision.Attention || forcedAttention
		if attention {
			if evidence := boundedWatchEvidence(decision.Evidence); evidence != "" {
				summary += " [evidence: " + evidence + "]"
			} else {
				summary += " [evidence missing; review Work run]"
			}
		}
		return attention, summary
	}
	return true, summarizeJobResult(finalText, artifacts)
}

func boundedWatchEvidence(references []string) string {
	var selected []string
	seen := map[string]bool{}
	for _, reference := range references {
		clean := strings.Map(func(r rune) rune {
			if unicode.IsControl(r) {
				return ' '
			}
			return r
		}, reference)
		clean = strings.Join(strings.Fields(clean), " ")
		if clean == "" || seen[clean] {
			continue
		}
		seen[clean] = true
		if len([]rune(clean)) > 100 {
			clean = string([]rune(clean)[:100]) + "…"
		}
		selected = append(selected, clean)
		if len(selected) == 2 {
			break
		}
	}
	return strings.Join(selected, "; ")
}
