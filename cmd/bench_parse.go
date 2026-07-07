package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type benchSummary struct {
	Tasks       int
	Passed      int
	BrainTokens []float64
	DroneTokens []float64
	WallSeconds []float64
}

func loadBenchSummary(root string) (benchSummary, error) {
	var summary benchSummary
	seenTrials := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		switch {
		case entry.Name() == "result.json":
			return readResultJSON(path, &summary, seenTrials)
		case entry.Name() == "reward.txt" && !hasSiblingTrialResult(path):
			reward, ok := readRewardText(path)
			if ok {
				summary.Tasks++
				if reward >= 1 {
					summary.Passed++
				}
			}
		case strings.HasSuffix(entry.Name(), ".ndjson") && isTracePath(path):
			brain, drone, ok := readTrace(path)
			if ok {
				summary.BrainTokens = append(summary.BrainTokens, float64(brain))
				summary.DroneTokens = append(summary.DroneTokens, float64(drone))
			}
		}
		return nil
	})
	if err != nil {
		return benchSummary{}, err
	}
	if summary.Tasks == 0 && len(summary.BrainTokens) == 0 && len(summary.DroneTokens) == 0 {
		return benchSummary{}, fmt.Errorf("no Harbor results found under %s", root)
	}
	return summary, nil
}

func readResultJSON(path string, summary *benchSummary, seen map[string]bool) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil
	}
	if trials, ok := doc["trial_results"].([]any); ok {
		for _, trial := range trials {
			if obj, ok := trial.(map[string]any); ok {
				addTrialResult(obj, summary, seen)
			}
		}
		return nil
	}
	if _, ok := doc["trial_name"]; ok {
		addTrialResult(doc, summary, seen)
	}
	return nil
}

func addTrialResult(trial map[string]any, summary *benchSummary, seen map[string]bool) {
	id := stringField(trial, "id")
	if id == "" {
		id = stringField(trial, "task_name") + "/" + stringField(trial, "trial_name")
	}
	if id == "/" || seen[id] {
		return
	}
	seen[id] = true
	passed, ok := trialPassed(trial)
	if ok {
		summary.Tasks++
		if passed {
			summary.Passed++
		}
	}
	if seconds, ok := trialWallSeconds(trial); ok {
		summary.WallSeconds = append(summary.WallSeconds, seconds)
	}
}

func trialPassed(trial map[string]any) (bool, bool) {
	verifier, ok := trial["verifier_result"].(map[string]any)
	if !ok {
		return false, false
	}
	rewards, ok := verifier["rewards"].(map[string]any)
	if !ok || len(rewards) == 0 {
		return false, false
	}
	for _, value := range rewards {
		if numberValue(value) >= 1 {
			return true, true
		}
	}
	return false, true
}

func trialWallSeconds(trial map[string]any) (float64, bool) {
	if seconds, ok := durationFields(trial); ok {
		return seconds, true
	}
	if phase, ok := trial["agent_execution"].(map[string]any); ok {
		return durationFields(phase)
	}
	return 0, false
}

func durationFields(obj map[string]any) (float64, bool) {
	started, ok := parseJSONTime(stringField(obj, "started_at"))
	if !ok {
		return 0, false
	}
	finished, ok := parseJSONTime(stringField(obj, "finished_at"))
	if !ok || finished.Before(started) {
		return 0, false
	}
	return finished.Sub(started).Seconds(), true
}

func parseJSONTime(raw string) (time.Time, bool) {
	if raw == "" {
		return time.Time{}, false
	}
	layouts := []string{
		time.RFC3339Nano,
		"2006-01-02T15:04:05.999999",
		"2006-01-02T15:04:05",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func readRewardText(path string) (float64, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	reward, err := strconv.ParseFloat(strings.TrimSpace(string(data)), 64)
	return reward, err == nil
}

func hasSiblingTrialResult(rewardPath string) bool {
	trialDir := filepath.Dir(filepath.Dir(rewardPath))
	_, err := os.Stat(filepath.Join(trialDir, "result.json"))
	return err == nil
}

func stringField(obj map[string]any, key string) string {
	value, _ := obj[key].(string)
	return value
}

func numberValue(value any) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case json.Number:
		n, _ := v.Float64()
		return n
	default:
		return 0
	}
}
