package cmd

import (
	"path/filepath"
	"strings"
	"testing"
)

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

func TestReadTraceCountsKnownStages(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".paw")
	writeBenchFile(t, dir, "trace.ndjson", strings.Join([]string{
		`{"stage":"compress","tokens":3}`,
		`{"stage":"plan","tokens":5}`,
		`not json`,
		`{"stage":"edit","tokens":7}`,
		`{"stage":"other","tokens":11}`,
		"",
	}, "\n"))
	path := filepath.Join(dir, "trace.ndjson")
	if !isTracePath(path) {
		t.Fatalf("expected trace path")
	}
	brain, drone, ok := readTrace(path)
	if !ok || brain != 12 || drone != 3 {
		t.Fatalf("trace = brain:%d drone:%d ok:%v", brain, drone, ok)
	}
}
