package eval

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/workrun"
)

type WorkMetrics struct {
	ModelRequests        int `json:"model_requests"`
	Retries              int `json:"retries"`
	Interventions        int `json:"interventions"`
	ToolFailures         int `json:"tool_failures"`
	ArtifactChecks       int `json:"artifact_checks"`
	FailedArtifactChecks int `json:"failed_artifact_checks"`
	UnsupportedQuotes    int `json:"unsupported_quotes"`
}

func (m *WorkMetrics) observe(outcome workrun.Outcome) {
	m.ModelRequests += outcome.Manifest.Usage.ModelRequests
	for _, event := range outcome.Events {
		if event.Kind == agent.EventModelAttemptStarted && event.Attempt > 1 {
			m.Retries++
		}
		if event.Kind == agent.EventCommandApprovalRequested || event.Kind == agent.EventSteeringApplied {
			m.Interventions++
		}
		if event.ToolError != "" {
			m.ToolFailures++
		}
		if event.ToolCall != nil && event.ToolCall.Name == "check_claims" && event.Kind == agent.EventToolFinished {
			m.UnsupportedQuotes += strings.Count(event.ToolResult, `"quote_match":false`)
		}
	}
	for _, check := range outcome.Manifest.Validations {
		m.ArtifactChecks++
		if !check.Passed {
			m.FailedArtifactChecks++
		}
	}
}

func fixtureDigest(directory string) (string, error) {
	hash := sha256.New()
	total := int64(0)
	err := filepath.WalkDir(directory, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return errors.New("evaluation fixture contains symlink or special file")
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		if info.Size() > 16*1024*1024 || total > 64*1024*1024 {
			return errors.New("evaluation fixture exceeds bound")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(directory, path)
		if err != nil {
			return err
		}
		fmt.Fprintf(hash, "%s\x00%d\x00%d\x00", filepath.ToSlash(relative), info.Mode().Perm()&0111, len(data))
		hash.Write(data)
		return nil
	})
	return hex.EncodeToString(hash.Sum(nil)), err
}

func usageAdd(total *agent.Usage, next agent.Usage) {
	total.ModelRequests += next.ModelRequests
	total.UnknownRequests += next.UnknownRequests
	total.InputTokens += next.InputTokens
	total.OutputTokens += next.OutputTokens
	total.Reported = total.Reported || next.Reported
	total.Estimated = total.Estimated || next.Estimated
	// current adapters do not supply prices; unknown cost remains unknown.
	total.KnownCost = nil
}

func WorkSummary(report WorkExperiment) string {
	var usage agent.Usage
	var latency int64
	var retries, interventions, tools, checks, failed, unsupported int
	for _, trial := range report.Trials {
		usageAdd(&usage, trial.Usage)
		latency += trial.DurationMS
		retries += trial.Metrics.Retries
		interventions += trial.Metrics.Interventions
		tools += trial.Metrics.ToolFailures
		checks += trial.Metrics.ArtifactChecks
		failed += trial.Metrics.FailedArtifactChecks
		unsupported += trial.Metrics.UnsupportedQuotes
	}
	return fmt.Sprintf("%s: %d/%d scored trials; %d cases pass at least once; %d cases pass every trial.\nGrading categories: %v\nExecution outcomes: %v\nRequests: %d (%d without token usage); retries: %d; interventions: %d; tool failures: %d.\nArtifact checks: %d (%d failed); unsupported exact quotations: %d; semantic entailment is not scored by quote matching.\nReported tokens: %d input, %d output; total latency: %d ms; cost: unknown.\nScripted: %t; delegation: %t; provider/model: %s/%s; harness: %s.\n", report.ID, report.Passed, report.Total, report.CasesAtLeastOne, report.CasesAll, report.Categories, report.Outcomes, usage.ModelRequests, usage.UnknownRequests, retries, interventions, tools, checks, failed, unsupported, usage.InputTokens, usage.OutputTokens, latency, report.Scripted, report.Delegation, report.Provider, report.Model, report.Harness)
}
