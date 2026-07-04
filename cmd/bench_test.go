package cmd

import (
	"bytes"
	"os"
	"path/filepath"
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
	printBenchTable(&out, []benchRow{summary.row("full")})
	if !strings.Contains(out.String(), "| full | 1 | 1.00 | 24 | 5 | 12.0 |") {
		t.Fatalf("unexpected table:\n%s", out.String())
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

func writeBenchFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}
