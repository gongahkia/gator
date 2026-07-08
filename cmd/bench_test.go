package cmd

import (
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
		"--artifact\x00/workspace/.paw/trace.ndjson",
		"--allow-agent-host\x00host.docker.internal",
		"--yes",
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
		"--artifact /workspace/.paw/trace.ndjson",
		"--allow-agent-host host.docker.internal",
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
	if !strings.Contains(out, "| no-compress | 1 | 1.00 | n/a | n/a | n/a | 4.0 |") {
		t.Fatalf("unexpected bench output:\n%s", out)
	}
}

func TestBenchCommandRejectsPartialResultsBeforeWriting(t *testing.T) {
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
  "verifier_result": {"rewards": {"reward": 1}}
}`)
	harbor := filepath.Join(t.TempDir(), "harbor")
	if err := os.WriteFile(harbor, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write harbor: %v", err)
	}
	results := filepath.Join(t.TempDir(), "RESULTS.md")
	original := strings.Join([]string{
		resultsStartMarker,
		"| run_id | commit | config | dataset | brain_model | drone_model | hardware | date | tasks | wall_time | tokens_brain_in | tokens_brain_out | tokens_drone | pass_rate | trace_bundle |",
		resultsEndMarker,
		"",
	}, "\n")
	if err := os.WriteFile(results, []byte(original), 0o644); err != nil {
		t.Fatalf("write results: %v", err)
	}

	stdout, stderr, err := executeRootErr(t, []string{
		"bench",
		"--config", "raw",
		"--harbor-bin", harbor,
		"--jobs-dir", root,
		"--job-name", job,
		"--n-tasks", "2",
		"--results-file", results,
	}, "")
	if err == nil || !strings.Contains(err.Error(), "incomplete Harbor results") {
		t.Fatalf("err = %v stdout=%s stderr=%s", err, stdout, stderr)
	}
	got, err := os.ReadFile(results)
	if err != nil {
		t.Fatalf("read results: %v", err)
	}
	if string(got) != original {
		t.Fatalf("results file changed:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(results), "results-configs", job+".toml")); !os.IsNotExist(err) {
		t.Fatalf("partial run wrote config, err=%v", err)
	}
}

func TestValidateBenchSummaryExpectedTrials(t *testing.T) {
	complete := benchSummary{Tasks: 6}
	opts := benchOptions{JobName: "job", NTasks: 3, NAttempts: 2}
	if err := validateBenchSummary(complete, opts); err != nil {
		t.Fatalf("complete summary rejected: %v", err)
	}
	partial := benchSummary{Tasks: 5}
	if err := validateBenchSummary(partial, opts); err == nil || !strings.Contains(err.Error(), "expected 6") {
		t.Fatalf("partial summary err = %v", err)
	}
	if err := validateBenchSummary(partial, benchOptions{JobName: "unknown", NTasks: 0}); err != nil {
		t.Fatalf("zero-task expected should not enforce count: %v", err)
	}
	unfinished := benchSummary{Tasks: 6, JobFinishedKnown: true, JobFinished: false}
	if err := validateBenchSummary(unfinished, opts); err == nil || !strings.Contains(err.Error(), "did not finish") {
		t.Fatalf("unfinished job err = %v", err)
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
