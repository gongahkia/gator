package cmd

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/gongahkia/paw/internal/budget"
)

type benchTraceEvent struct {
	Stage       string              `json:"stage"`
	Tokens      int                 `json:"tokens"`
	TokenSource string              `json:"token_source"`
	Envelope    *benchTraceEnvelope `json:"envelope"`
}

type benchTraceEnvelope struct {
	Budget benchTraceBudget `json:"budget"`
}

type benchTraceBudget struct {
	BrainInputTokens  int    `json:"brain_input_tokens"`
	BrainOutputTokens int    `json:"brain_output_tokens"`
	BrainTokenSource  string `json:"brain_token_source"`
	DroneTokens       int    `json:"drone_tokens"`
	DroneTokenSource  string `json:"drone_token_source"`
}

func readTrace(path string) (int, int, int, string, string, bool) {
	file, err := os.Open(path)
	if err != nil {
		return 0, 0, 0, budget.TokenSourceNone, budget.TokenSourceNone, false
	}
	defer func() { _ = file.Close() }()
	var brain, drone int
	brainSource := budget.TokenSourceNone
	droneSource := budget.TokenSourceNone
	var budgetSnapshot benchTraceBudget
	var hasBudget bool
	seen := false
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var event benchTraceEvent
		if json.Unmarshal(scanner.Bytes(), &event) != nil {
			continue
		}
		seen = true
		switch event.Stage {
		case "plan", "edit":
			brain += event.Tokens
			if event.Tokens > 0 {
				brainSource = budget.MergeTokenSource(brainSource, event.TokenSource)
			}
		case "compress":
			drone += event.Tokens
			if event.Tokens > 0 {
				droneSource = budget.MergeTokenSource(droneSource, event.TokenSource)
			}
		}
		if event.Envelope != nil {
			budgetSnapshot = event.Envelope.Budget
			if budgetSnapshot.BrainInputTokens > 0 || budgetSnapshot.BrainOutputTokens > 0 || budgetSnapshot.DroneTokens > 0 {
				hasBudget = true
			}
		}
	}
	if hasBudget {
		bs := budget.SourceForTokens(budgetSnapshot.BrainInputTokens+budgetSnapshot.BrainOutputTokens, budgetSnapshot.BrainTokenSource)
		if bs == budget.TokenSourceNone {
			bs = budget.SourceForTokens(budgetSnapshot.BrainInputTokens+budgetSnapshot.BrainOutputTokens, brainSource)
		}
		ds := budget.SourceForTokens(budgetSnapshot.DroneTokens, budgetSnapshot.DroneTokenSource)
		if ds == budget.TokenSourceNone {
			ds = budget.SourceForTokens(budgetSnapshot.DroneTokens, droneSource)
		}
		return budgetSnapshot.BrainInputTokens, budgetSnapshot.BrainOutputTokens, budgetSnapshot.DroneTokens, bs, ds, seen
	}
	return brain, 0, drone, budget.SourceForTokens(brain, brainSource), budget.SourceForTokens(drone, droneSource), seen
}

func isTracePath(path string) bool {
	clean := filepath.ToSlash(path)
	return strings.Contains(clean, "/.paw/") || strings.Contains(filepath.Base(clean), "trace")
}
