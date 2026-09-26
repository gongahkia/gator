package eval

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestLearningExperimentMeasuresScopedFutureWorkAndKeepsHeldOutCasesSeparate(t *testing.T) {
	path := filepath.Join("testdata", "learning-v1", "dataset.json")
	dataset, err := LoadLearningDataset(path)
	if err != nil {
		t.Fatal(err)
	}
	run := func(id, split string) LearningExperiment {
		t.Helper()
		report, runErr := RunLearningExperiment(context.Background(), path, dataset, LearningEvalOptions{ID: id, Harness: "test", ReportDir: filepath.Join(t.TempDir(), "experiment"), Split: split})
		if runErr != nil {
			t.Fatal(runErr)
		}
		if report.Passed != report.Total {
			t.Fatalf("%s: %s", id, LearningSummary(report))
		}
		return report
	}
	development := run("learning-development", "")
	if development.Split != WorkSplitDevelopment || development.Total == 0 {
		t.Fatalf("development split = %#v", development)
	}
	for _, trial := range development.Trials {
		if trial.Split != WorkSplitDevelopment {
			t.Fatalf("held-out trial leaked into development: %#v", trial)
		}
		if trial.WorkID == "" || trial.ArtifactPath != "decision.txt" || trial.Output == "" {
			t.Fatalf("probe did not produce a normal Work artifact: %#v", trial)
		}
	}
	if trial := learningTrial(development, "repo-package-manager", "same-repository"); !trial.Ablated || len(trial.ActiveLearningIDs) != 1 || trial.LearningCount != 1 || !strings.Contains(trial.Output, "package_manager=pnpm") || !strings.Contains(trial.ControlOutput, "package_manager=npm") {
		t.Fatalf("repository counterfactual trial = %#v", trial)
	}
	if trial := learningTrial(development, "repo-package-manager", "unrelated-repository"); strings.Contains(trial.Output, "package_manager=pnpm") || !strings.Contains(trial.Output, "package_manager=npm") {
		t.Fatalf("repository scope leaked: %#v", trial)
	}
	if trial := learningTrial(development, "output-format-scope", "same-task-category"); !strings.Contains(trial.Output, "report_format=markdown") || !strings.Contains(trial.ControlOutput, "report_format=pdf") {
		t.Fatalf("output-format counterfactual trial = %#v", trial)
	}
	if trial := learningTrial(development, "output-format-scope", "other-report-project"); strings.Contains(trial.Output, "report_format=markdown") || !strings.Contains(trial.Output, "report_format=pdf") {
		t.Fatalf("output-format scope leaked: %#v", trial)
	}
	if trial := learningTrial(development, "targeted-verification", "relevant-api-change"); len(trial.ActiveLearningIDs) != 1 || !strings.Contains(trial.Output, "verification_plan=api-package-tests") || !strings.Contains(trial.ControlOutput, "verification_plan=standard") {
		t.Fatalf("failure-prevention counterfactual trial = %#v", trial)
	}
	if trial := learningTrial(development, "targeted-verification", "unrelated-web-change"); strings.Contains(trial.Output, "verification_plan=api-package-tests") || !strings.Contains(trial.Output, "verification_plan=standard") {
		t.Fatalf("failure-prevention scope leaked: %#v", trial)
	}
	if trial := learningTrial(development, "delivery-uncertainty", "no-unsafe-retry-rule"); trial.LearningCount != 0 || len(trial.ActiveLearningIDs) != 0 || !strings.Contains(trial.Output, "delivery_retry=inspect-before-retry") || strings.Contains(trial.Output, "delivery_retry=automatic") {
		t.Fatalf("unknown delivery created active guidance: %#v", trial)
	}
	if trial := learningTrial(development, "current-instruction-wins", "csv-overrides-persistent-default"); !strings.Contains(trial.Output, "report_format=csv") || strings.Contains(trial.Output, "report_format=markdown") {
		t.Fatalf("current instruction did not override persistent learning: %#v", trial)
	}
	heldOut := run("learning-held-out", WorkSplitHeldOut)
	if heldOut.Split != WorkSplitHeldOut || heldOut.Total == 0 {
		t.Fatalf("held-out split = %#v", heldOut)
	}
	for _, trial := range heldOut.Trials {
		if trial.Split != WorkSplitHeldOut {
			t.Fatalf("development trial leaked into held-out: %#v", trial)
		}
		if trial.WorkID == "" || trial.ArtifactPath != "decision.txt" || trial.Output == "" {
			t.Fatalf("held-out probe did not produce a normal Work artifact: %#v", trial)
		}
	}
	if trial := learningTrial(heldOut, "user-reversal", "before-reversal"); !strings.Contains(trial.Output, "package_manager=yarn") || !strings.Contains(trial.ControlOutput, "package_manager=npm") {
		t.Fatalf("active learning did not affect future Work: %#v", trial)
	}
	if trial := learningTrial(heldOut, "user-reversal", "after-reversal"); len(trial.ActiveLearningIDs) != 0 || trial.LearningCount != 0 || trial.RetainedLearningCount != 1 || len(trial.ProvenanceWorkIDs) != 1 || trial.ProvenanceWorkIDs[0] != "work-legacy-preference" || strings.Contains(trial.Output, "package_manager=yarn") || !strings.Contains(trial.Output, "package_manager=npm") {
		t.Fatalf("reversed learning reached future Work: %#v", trial)
	}
	if trial := learningTrial(heldOut, "explicit-overrides-inference", "explicit-wins"); !strings.Contains(trial.Output, "package_manager=pnpm") || strings.Contains(trial.Output, "package_manager=npm") {
		t.Fatalf("explicit user rule did not beat inference in Work behavior: %#v", trial)
	}
	if trial := learningTrial(heldOut, "scoped-rule-wins", "specificity-wins"); !strings.Contains(trial.Output, "package_manager=pnpm") || strings.Contains(trial.Output, "package_manager=npm") {
		t.Fatalf("project-specific rule did not beat global rule: %#v", trial)
	}
	if trial := learningTrial(heldOut, "scoped-rule-wins", "global-fallback"); !strings.Contains(trial.Output, "package_manager=npm") || strings.Contains(trial.Output, "package_manager=pnpm") {
		t.Fatalf("global fallback was not limited to its scope: %#v", trial)
	}
	if trial := learningTrial(heldOut, "candidate-only", "candidate-does-not-leak"); trial.LearningCount != 0 || strings.Contains(trial.Output, "package_manager=pnpm") || !strings.Contains(trial.Output, "package_manager=npm") {
		t.Fatalf("inactive candidate affected future Work: %#v", trial)
	}
}

func TestLearningExperimentReportsDiagnosticInsteadOfAggregateScore(t *testing.T) {
	dataset := LearningDataset{
		Version: LearningDatasetVersion, ID: "diagnostic", Target: "learning.v1",
		Cases: []LearningCase{{
			ID: "scope-leak", Family: "diagnostics", Split: WorkSplitDevelopment, Project: "repos/one", ExpectedLearnings: 1,
			Learnings: []LearningSeed{{ID: "learning-one", Type: "preference", Key: "tool", Content: "Use pnpm.", Scope: "project", Origin: "user", Status: "active", WorkID: "work-one"}},
			Phases:    []LearningPhase{{ID: "future-work", Probes: []LearningProbe{{ID: "forced-leak", Project: "repos/one", Objective: "Prepare the repository package command.", Forbid: []string{"package_manager=pnpm"}}}}},
		}},
	}
	report, err := RunLearningExperiment(context.Background(), "unused", dataset, LearningEvalOptions{ID: "diagnostic-run", Harness: "test", ReportDir: filepath.Join(t.TempDir(), "experiment")})
	if err != nil {
		t.Fatal(err)
	}
	if report.Passed != 0 || report.Total != 1 || report.Categories["scope_leak"] != 1 || len(report.Trials[0].Diagnostics) != 1 || report.Trials[0].Diagnostics[0].Code != "scope_leak" {
		t.Fatalf("diagnostic report = %#v", report)
	}
	if !strings.Contains(LearningSummary(report), "scope_leak") {
		t.Fatalf("summary hid diagnostic: %s", LearningSummary(report))
	}
}

func learningTrial(report LearningExperiment, caseID, probe string) LearningTrial {
	for _, trial := range report.Trials {
		if trial.CaseID == caseID && trial.Probe == probe {
			return trial
		}
	}
	return LearningTrial{}
}
