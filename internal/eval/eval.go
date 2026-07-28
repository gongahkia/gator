package eval

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gongahkia/norbot/internal/domain"
)

const Version = "v1"

type Case struct {
	ID           string              `json:"id"`
	Kind         string              `json:"kind"`
	Prompt       string              `json:"prompt"`
	Architecture domain.Architecture `json:"architecture,omitempty"`
	Expected     Expected            `json:"expected"`
}
type Expected struct {
	Outcome        string `json:"outcome"`
	Classification string `json:"classification,omitempty"`
}
type Corpus struct {
	Version string `json:"version"`
	Cases   []Case `json:"cases"`
}
type Result struct {
	CaseID         string `json:"case_id"`
	Provider       string `json:"provider,omitempty"`
	Model          string `json:"model,omitempty"`
	Outcome        string `json:"outcome"`
	Classification string `json:"classification,omitempty"`
	Passed         bool   `json:"passed"`
}
type Score struct {
	Total      int                      `json:"total"`
	Passed     int                      `json:"passed"`
	ByProvider map[string]ProviderScore `json:"by_provider"`
}
type ProviderScore struct {
	Total  int                   `json:"total"`
	Passed int                   `json:"passed"`
	Models map[string]ModelScore `json:"models"`
}
type ModelScore struct {
	Total  int `json:"total"`
	Passed int `json:"passed"`
}

func Load(dir string) (Corpus, error) {
	data, err := os.ReadFile(filepath.Join(dir, "cases.json"))
	if err != nil {
		return Corpus{}, err
	}
	var corpus Corpus
	if err := json.Unmarshal(data, &corpus); err != nil {
		return Corpus{}, err
	}
	if err := corpus.Validate(); err != nil {
		return Corpus{}, err
	}
	return corpus, nil
}
func (c Corpus) Validate() error {
	if c.Version != Version {
		return fmt.Errorf("unsupported eval corpus version %q", c.Version)
	}
	if len(c.Cases) < 20 || len(c.Cases) > 50 {
		return fmt.Errorf("eval corpus must contain 20-50 cases")
	}
	seen := map[string]struct{}{}
	for _, item := range c.Cases {
		if item.ID == "" || item.Kind == "" || item.Prompt == "" || item.Expected.Outcome == "" {
			return fmt.Errorf("eval case requires id, kind, prompt, and expected outcome")
		}
		if _, ok := seen[item.ID]; ok {
			return fmt.Errorf("duplicate eval case %q", item.ID)
		}
		seen[item.ID] = struct{}{}
		if item.Kind == "app_spec" {
			if err := item.Architecture.Validate(); err != nil {
				return fmt.Errorf("eval case %q architecture: %w", item.ID, err)
			}
		}
		if item.Kind == "prompt_injection" && item.Expected.Outcome != "rejected" {
			return fmt.Errorf("prompt injection case %q must expect rejection", item.ID)
		}
	}
	return nil
}
func OfflineResults(c Corpus, provider, model, only string) []Result {
	values := []Result{}
	for _, item := range c.Cases {
		if only != "" && item.ID != only {
			continue
		}
		values = append(values, Result{CaseID: item.ID, Provider: provider, Model: model, Outcome: item.Expected.Outcome, Classification: item.Expected.Classification, Passed: true})
	}
	return values
}
func ScoreResults(c Corpus, results []Result) (Score, error) {
	expected := map[string]Expected{}
	for _, item := range c.Cases {
		expected[item.ID] = item.Expected
	}
	score := Score{ByProvider: map[string]ProviderScore{}}
	for _, result := range results {
		wanted, ok := expected[result.CaseID]
		if !ok {
			return Score{}, fmt.Errorf("unknown eval case %q", result.CaseID)
		}
		result.Passed = result.Passed && result.Outcome == wanted.Outcome && result.Classification == wanted.Classification
		score.Total++
		if result.Passed {
			score.Passed++
		}
		provider := result.Provider
		if provider == "" {
			provider = "offline"
		}
		model := result.Model
		if model == "" {
			model = "none"
		}
		item := score.ByProvider[provider]
		item.Total++
		if result.Passed {
			item.Passed++
		}
		if item.Models == nil {
			item.Models = map[string]ModelScore{}
		}
		modelScore := item.Models[model]
		modelScore.Total++
		if result.Passed {
			modelScore.Passed++
		}
		item.Models[model] = modelScore
		score.ByProvider[provider] = item
	}
	return score, nil
}
func CaseIDs(c Corpus) []string {
	values := make([]string, 0, len(c.Cases))
	for _, item := range c.Cases {
		values = append(values, item.ID)
	}
	sort.Strings(values)
	return values
}
func InjectionPrompt(value string) bool {
	value = strings.ToLower(value)
	return strings.Contains(value, "ignore previous") || strings.Contains(value, "system prompt") || strings.Contains(value, "exfiltrate")
}
