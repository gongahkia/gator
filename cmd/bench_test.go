package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBuildHarborArgsMapsConfig(t *testing.T) {
	args := buildHarborArgs(benchOptions{
		Config:          "raw",
		Dataset:         "terminal-bench@2.0",
		Model:           "openai/glm-4.6",
		JobsDir:         "jobs",
		JobName:         "paw-raw",
		AgentImportPath: "paw_harbor:PawAgent",
		NConcurrent:     4,
		NTasks:          5,
		NAttempts:       2,
	})
	got := strings.Join(args, "\x00")
	for _, want := range []string{
		"--agent-import-path\x00paw_harbor:PawAgent",
		"--agent-env\x00PAW_BENCH_CONFIG=raw",
		"--n-tasks\x005",
		"--n-attempts\x002",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("args missing %q: %v", want, args)
		}
	}
}

func TestBenchCommandDryRunPrintsQuotedHarborCommand(t *testing.T) {
	isolateEnv(t)
	out := executeRoot(t, []string{
		"bench",
		"--dry-run",
		"--config", "raw",
		"--dataset", "terminal bench@2.0",
		"--model", "openai/glm-4.6",
		"--job-name", "job with 'quote'",
		"--results-file=",
	}, "")
	for _, want := range []string{
		"harbor run",
		"--agent-env PAW_BENCH_CONFIG=raw",
		"'terminal bench@2.0'",
		"'job with '\"'\"'quote'\"'\"''",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("dry-run output missing %q:\n%s", want, out)
		}
	}
}

func TestBenchCommandRejectsUnsupportedConfig(t *testing.T) {
	isolateEnv(t)
	_, stderr, err := executeRootErr(t, []string{"bench", "--dry-run", "--config", "unknown"}, "")
	if err == nil || !strings.Contains(err.Error(), `unsupported bench config "unknown"`) {
		t.Fatalf("err = %v stderr=%s", err, stderr)
	}
}

func TestBenchCommandRunsHarborAndPrintsSummary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX shell script")
	}
	isolateEnv(t)
	root := t.TempDir()
	job := "paw-test-job"
	trial := filepath.Join(root, job, "trial-a")
	writeBenchFile(t, trial, "result.json", `{
  "id": "trial-a",
  "trial_name": "task-a__1",
  "started_at": "2026-07-04T00:00:00Z",
  "finished_at": "2026-07-04T00:00:04Z",
  "verifier_result": {"rewards": {"reward": 1}}
}`)
	harbor := filepath.Join(t.TempDir(), "harbor")
	if err := os.WriteFile(harbor, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write harbor: %v", err)
	}

	out := executeRoot(t, []string{
		"bench",
		"--config", "no-compress",
		"--harbor-bin", harbor,
		"--jobs-dir", root,
		"--job-name", job,
		"--results-file=",
	}, "")
	if !strings.Contains(out, "| no-compress | 1 | 1.00 | n/a | n/a | 4.0 |") {
		t.Fatalf("unexpected bench output:\n%s", out)
	}
}

func TestLoadBenchSummaryParsesResultsAndTrace(t *testing.T) {
	root := t.TempDir()
	trial := filepath.Join(root, "job", "trial-a")
	writeBenchFile(t, trial, "result.json", `{
  "id": "trial-a",
  "task_name": "task-a",
  "trial_name": "task-a__1",
  "started_at": "2026-07-04T00:00:00Z",
  "finished_at": "2026-07-04T00:00:12Z",
  "verifier_result": {"rewards": {"reward": 1}}
}`)
	writeBenchFile(t, filepath.Join(trial, ".paw"), "trace.ndjson", strings.Join([]string{
		`{"stage":"compress","tokens":5}`,
		`{"stage":"plan","tokens":11}`,
		`{"stage":"edit","tokens":13}`,
		"",
	}, "\n"))

	summary, err := loadBenchSummary(root)
	if err != nil {
		t.Fatalf("load summary: %v", err)
	}
	if summary.Tasks != 1 || summary.Passed != 1 {
		t.Fatalf("tasks/pass = %d/%d", summary.Tasks, summary.Passed)
	}
	if got := formatMedian(summary.BrainTokens, 0); got != "24" {
		t.Fatalf("brain median = %s", got)
	}
	if got := formatMedian(summary.DroneTokens, 0); got != "5" {
		t.Fatalf("drone median = %s", got)
	}
	if got := formatMedian(summary.WallSeconds, 1); got != "12.0" {
		t.Fatalf("wall median = %s", got)
	}

	var out bytes.Buffer
	if err := printBenchTable(&out, []benchRow{summary.row("full")}); err != nil {
		t.Fatalf("print table: %v", err)
	}
	if !strings.Contains(out.String(), "| full | 1 | 1.00 | 24 | 5 | 12.0 |") {
		t.Fatalf("unexpected table:\n%s", out.String())
	}
}

func TestLoadBenchSummaryHandlesAggregateRewardsAndFallbacks(t *testing.T) {
	root := t.TempDir()
	writeBenchFile(t, filepath.Join(root, "aggregate"), "result.json", `{
  "trial_results": [
    {
      "id": "pass",
      "agent_execution": {
        "started_at": "2026-07-04T00:00:00",
        "finished_at": "2026-07-04T00:00:10"
      },
      "verifier_result": {"rewards": {"reward": 1}}
    },
    {
      "id": "fail",
      "started_at": "2026-07-04T00:00:10Z",
      "finished_at": "2026-07-04T00:00:09Z",
      "verifier_result": {"rewards": {"reward": 0}}
    },
    {
      "id": "pass",
      "verifier_result": {"rewards": {"reward": 1}}
    }
  ]
}`)
	writeBenchFile(t, filepath.Join(root, "invalid"), "result.json", `{`)
	writeBenchFile(t, filepath.Join(root, "reward-only", "reward"), "reward.txt", "2.5\n")
	withSibling := filepath.Join(root, "with-sibling")
	writeBenchFile(t, withSibling, "result.json", `{"trial_name":"skip","verifier_result":{"rewards":{"reward":1}}}`)
	writeBenchFile(t, filepath.Join(withSibling, "reward"), "reward.txt", "1\n")
	writeBenchFile(t, filepath.Join(root, ".paw"), "trace.ndjson", strings.Join([]string{
		`not json`,
		`{"stage":"compress","tokens":2}`,
		`{"stage":"plan","tokens":7}`,
		`{"stage":"edit","tokens":8}`,
		"",
	}, "\n"))

	summary, err := loadBenchSummary(root)
	if err != nil {
		t.Fatalf("load summary: %v", err)
	}
	if summary.Tasks != 4 || summary.Passed != 3 {
		t.Fatalf("tasks/pass = %d/%d", summary.Tasks, summary.Passed)
	}
	if got := formatMedian(summary.WallSeconds, 1); got != "10.0" {
		t.Fatalf("wall median = %s", got)
	}
	if got := formatMedian(summary.BrainTokens, 0); got != "15" {
		t.Fatalf("brain median = %s", got)
	}
	if got := formatMedian(summary.DroneTokens, 0); got != "2" {
		t.Fatalf("drone median = %s", got)
	}
}

func TestLoadBenchSummaryRejectsEmptyTree(t *testing.T) {
	_, err := loadBenchSummary(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "no Harbor results found") {
		t.Fatalf("err = %v", err)
	}
}

func TestUpdateResultsFileReplacesConfigRow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "RESULTS.md")
	if err := os.WriteFile(path, []byte(strings.Join([]string{
		"# Results",
		resultsStartMarker,
		"| config | tasks | pass@1 | brain_in_tok/task (median) | drone_tok/task | wall_s/task |",
		"| --- | ---: | ---: | ---: | ---: | ---: |",
		"| raw | 0 | n/a | n/a | n/a | n/a |",
		resultsEndMarker,
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatalf("write results: %v", err)
	}
	err := updateResultsFile(path, benchRow{
		Config:      "raw",
		Tasks:       5,
		PassRate:    "0.80",
		BrainTokens: "1200",
		DroneTokens: "300",
		WallSeconds: "42.5",
	})
	if err != nil {
		t.Fatalf("update results: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read results: %v", err)
	}
	if !strings.Contains(string(got), "| raw | 5 | 0.80 | 1200 | 300 | 42.5 |") {
		t.Fatalf("row not updated:\n%s", got)
	}
}

func TestUpdateResultsFileInsertsRowAndRejectsMissingMarkers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "RESULTS.md")
	if err := os.WriteFile(path, []byte(strings.Join([]string{
		resultsStartMarker,
		"| config | tasks | pass@1 | brain_in_tok/task (median) | drone_tok/task | wall_s/task |",
		resultsEndMarker,
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatalf("write results: %v", err)
	}
	if err := updateResultsFile(path, benchRow{Config: "full", Tasks: 1, PassRate: "1.00", BrainTokens: "10", DroneTokens: "2", WallSeconds: "3.0"}); err != nil {
		t.Fatalf("insert row: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read results: %v", err)
	}
	if !strings.Contains(string(got), "| full | 1 | 1.00 | 10 | 2 | 3.0 |") {
		t.Fatalf("row not inserted:\n%s", got)
	}
	missing := filepath.Join(dir, "missing.md")
	if err := os.WriteFile(missing, []byte("# Results\n"), 0o644); err != nil {
		t.Fatalf("write missing markers: %v", err)
	}
	if err := updateResultsFile(missing, benchRow{Config: "full"}); err == nil || !strings.Contains(err.Error(), "results table markers not found") {
		t.Fatalf("missing markers err = %v", err)
	}
}

func TestBenchHelpers(t *testing.T) {
	if got := ratio(0, 0); got != "n/a" {
		t.Fatalf("zero ratio = %s", got)
	}
	if got := formatMedian(nil, 0); got != "n/a" {
		t.Fatalf("empty median = %s", got)
	}
	if got := formatMedian([]float64{4, 2}, 1); got != "3.0" {
		t.Fatalf("even median = %s", got)
	}
	if got := numberValue(json.Number("2.5")); got != 2.5 {
		t.Fatalf("json number = %v", got)
	}
	if got := numberValue("bad"); got != 0 {
		t.Fatalf("bad number = %v", got)
	}
	if got := shellQuote(""); got != "''" {
		t.Fatalf("empty quote = %s", got)
	}
	if got := shellCommand("paw bin", []string{"arg", "two words"}); got != "'paw bin' arg 'two words'" {
		t.Fatalf("shell command = %s", got)
	}
}

func writeBenchFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}
