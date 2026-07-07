package cmd

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type benchTraceEvent struct {
	Stage    string              `json:"stage"`
	Tokens   int                 `json:"tokens"`
	Envelope *benchTraceEnvelope `json:"envelope"`
}

type benchTraceEnvelope struct {
	Budget benchTraceBudget `json:"budget"`
}

type benchTraceBudget struct {
	BrainInputTokens  int `json:"brain_input_tokens"`
	BrainOutputTokens int `json:"brain_output_tokens"`
	DroneTokens       int `json:"drone_tokens"`
}

func readTrace(path string) (int, int, int, bool) {
	file, err := os.Open(path)
	if err != nil {
		return 0, 0, 0, false
	}
	defer func() { _ = file.Close() }()
	var brain, drone int
	var budget benchTraceBudget
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
		case "compress":
			drone += event.Tokens
		}
		if event.Envelope != nil {
			budget = event.Envelope.Budget
			if budget.BrainInputTokens > 0 || budget.BrainOutputTokens > 0 || budget.DroneTokens > 0 {
				hasBudget = true
			}
		}
	}
	if hasBudget {
		return budget.BrainInputTokens, budget.BrainOutputTokens, budget.DroneTokens, seen
	}
	return brain, 0, drone, seen
}

func isTracePath(path string) bool {
	clean := filepath.ToSlash(path)
	return strings.Contains(clean, "/.paw/") || strings.Contains(filepath.Base(clean), "trace")
}
