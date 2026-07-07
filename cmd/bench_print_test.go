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
		Config:      "raw",
		Tasks:       2,
		PassRate:    "0.50",
		BrainTokens: "10",
		DroneTokens: "4",
		WallSeconds: "1.5",
	}})
	if err != nil {
		t.Fatalf("print table: %v", err)
	}
	want := strings.Join([]string{
		"| config | tasks | pass@1 | brain_in_tok/task (median) | drone_tok/task | wall_s/task |",
		"| --- | ---: | ---: | ---: | ---: | ---: |",
		"| raw | 2 | 0.50 | 10 | 4 | 1.5 |",
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
