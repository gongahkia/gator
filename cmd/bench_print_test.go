package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrintBenchTableWritesStableMarkdown(t *testing.T) {
	var out bytes.Buffer
	err := printBenchTable(&out, []benchRow{{
		Config:            "raw",
		Tasks:             2,
		PassRate:          "0.50",
		BrainInputTokens:  "10",
		BrainOutputTokens: "3",
		DroneTokens:       "4",
		WallSeconds:       "1.5",
	}})
	if err != nil {
		t.Fatalf("print table: %v", err)
	}
	want := strings.Join([]string{
		"| config | tasks | pass@1 | brain_in_tok/task (median) | brain_out_tok/task (median) | drone_tok/task | wall_s/task |",
		"| --- | ---: | ---: | ---: | ---: | ---: | ---: |",
		"| raw | 2 | 0.50 | 10 | 3 | 4 | 1.5 |",
		"",
	}, "\n")
	if out.String() != want {
		t.Fatalf("table mismatch:\n%s", out.String())
	}
}

func TestUpdateResultsFileReplacesConfigRow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "RESULTS.md")
	if err := os.WriteFile(path, []byte(strings.Join([]string{
		"# Results",
		resultsStartMarker,
		"| run_id | commit | config | dataset | brain_model | drone_model | hardware | date | tasks | wall_time | tokens_brain_in | tokens_brain_out | tokens_drone | pass_rate | trace_bundle |",
		"| --- | --- | --- | --- | --- | --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | --- |",
		"| tb2-raw-tbd | TBD | [raw](results-configs/example.toml) | terminal-bench@2.0 | TBD | n/a | TBD | TBD | 0 | n/a | n/a | n/a | n/a | n/a | TBD |",
		resultsEndMarker,
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatalf("write results: %v", err)
	}
	err := updateResultsFile(path, benchRow{
		RunID:             "tb2-smoke-raw",
		Commit:            "abc123",
		Config:            "raw",
		ConfigPath:        "results-configs/tb2-smoke-raw.toml",
		Dataset:           "terminal-bench@2.0",
		BrainModel:        "openai/brain",
		DroneModel:        "n/a",
		Hardware:          "darwin/arm64",
		Date:              "2026-07-07",
		Tasks:             5,
		PassRate:          "0.80",
		BrainInputTokens:  "1200",
		BrainOutputTokens: "200",
		DroneTokens:       "0",
		WallSeconds:       "42.5",
		TraceBundle:       ".paw/bench-jobs/tb2-smoke-raw",
	})
	if err != nil {
		t.Fatalf("update results: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read results: %v", err)
	}
	if !strings.Contains(string(got), "| tb2-smoke-raw | abc123 | [raw](results-configs/tb2-smoke-raw.toml) | terminal-bench@2.0 | openai/brain | n/a | darwin/arm64 | 2026-07-07 | 5 | 42.5 | 1200 | 200 | 0 | 0.80 | .paw/bench-jobs/tb2-smoke-raw |") {
		t.Fatalf("row not updated:\n%s", got)
	}
}

func TestUpdateResultsFileInsertsRowAndRejectsMissingMarkers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "RESULTS.md")
	if err := os.WriteFile(path, []byte(strings.Join([]string{
		resultsStartMarker,
		"| run_id | commit | config | dataset | brain_model | drone_model | hardware | date | tasks | wall_time | tokens_brain_in | tokens_brain_out | tokens_drone | pass_rate | trace_bundle |",
		resultsEndMarker,
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatalf("write results: %v", err)
	}
	if err := updateResultsFile(path, benchRow{RunID: "tb2-full", Config: "full", Tasks: 1, PassRate: "1.00", BrainInputTokens: "10", BrainOutputTokens: "4", DroneTokens: "2", WallSeconds: "3.0"}); err != nil {
		t.Fatalf("insert row: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read results: %v", err)
	}
	if !strings.Contains(string(got), "| tb2-full |  | full |  |  |  |  |  | 1 | 3.0 | 10 | 4 | 2 | 1.00 |  |") {
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

func TestUpdateResultsFileDoesNotReplaceNonPlaceholderConfigRow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "RESULTS.md")
	if err := os.WriteFile(path, []byte(strings.Join([]string{
		resultsStartMarker,
		"| run_id | commit | config | dataset | brain_model | drone_model | hardware | date | tasks | wall_time | tokens_brain_in | tokens_brain_out | tokens_drone | pass_rate | trace_bundle |",
		"| --- | --- | --- | --- | --- | --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | --- |",
		"| paw-smoke-raw | smoke | [raw](results-configs/paw-smoke-raw.toml) | terminal-bench@2.0 | openai/brain | n/a | host | 2026-07-07 | 5 | 1.0 | 10 | 2 | 0 | 0.00 | .paw/bench-jobs/paw-smoke-raw |",
		resultsEndMarker,
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatalf("write results: %v", err)
	}
	if err := updateResultsFile(path, benchRow{RunID: "paw-full-raw", Config: "raw", Commit: "full", Tasks: 89}); err != nil {
		t.Fatalf("update results: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read results: %v", err)
	}
	text := string(got)
	if !strings.Contains(text, "| paw-smoke-raw | smoke | [raw](results-configs/paw-smoke-raw.toml) |") {
		t.Fatalf("smoke row replaced:\n%s", text)
	}
	if !strings.Contains(text, "| paw-full-raw | full | raw |") {
		t.Fatalf("full row not inserted:\n%s", text)
	}
}

func TestUpdateResultsFileReplacesSameRunID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "RESULTS.md")
	if err := os.WriteFile(path, []byte(strings.Join([]string{
		resultsStartMarker,
		"| run_id | commit | config | dataset | brain_model | drone_model | hardware | date | tasks | wall_time | tokens_brain_in | tokens_brain_out | tokens_drone | pass_rate | trace_bundle |",
		"| --- | --- | --- | --- | --- | --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | --- |",
		"| paw-full-raw | old | [raw](results-configs/paw-full-raw.toml) | terminal-bench@2.0 | openai/brain | n/a | host | 2026-07-07 | 1 | 1.0 | 10 | 2 | 0 | 0.00 | .paw/bench-jobs/paw-full-raw |",
		resultsEndMarker,
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatalf("write results: %v", err)
	}
	if err := updateResultsFile(path, benchRow{RunID: "paw-full-raw", Config: "raw", Commit: "new", Tasks: 89}); err != nil {
		t.Fatalf("update results: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read results: %v", err)
	}
	text := string(got)
	if strings.Contains(text, "| paw-full-raw | old |") || !strings.Contains(text, "| paw-full-raw | new |") {
		t.Fatalf("same run id not replaced:\n%s", text)
	}
}

func TestWriteBenchResultConfigCapturesReproEnv(t *testing.T) {
	t.Setenv("PAW_CALL_TIMEOUT", "120s")
	t.Setenv("PAW_BENCH_HARDWARE", "macOS arm64")
	path := filepath.Join(t.TempDir(), "RESULTS.md")
	if err := writeBenchResultConfig(path, benchRow{
		RunID:      "paw-full-raw",
		Config:     "raw",
		ConfigPath: "results-configs/paw-full-raw.toml",
		Date:       "2026-07-07",
	}, benchOptions{
		Dataset:     "terminal-bench@2.0",
		Model:       "ollama/qwen2.5-coder:1.5b",
		JobsDir:     ".paw/bench-jobs",
		JobName:     "paw-full-raw",
		NConcurrent: 1,
		NTasks:      89,
	}); err != nil {
		t.Fatalf("write config: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(filepath.Dir(path), "results-configs", "paw-full-raw.toml"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	text := string(got)
	for _, want := range []string{`PAW_CALL_TIMEOUT = "120s"`, `PAW_BENCH_HARDWARE = "macOS arm64"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q:\n%s", want, text)
		}
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
	if got := resultRowMatches("| tb2-raw-tbd | TBD | [raw](x) |", "raw"); !got {
		t.Fatalf("expected row match")
	}
	if got := resultRowRunID("| run-id | commit | raw |"); got != "run-id" {
		t.Fatalf("run id = %s", got)
	}
}
