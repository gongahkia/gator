package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkLearningEvalUsesDevelopmentByDefaultAndWritesInspectableReport(t *testing.T) {
	path := filepath.Join("..", "..", "internal", "eval", "testdata", "learning-v1", "dataset.json")
	var output bytes.Buffer
	if err := workEvalCommand([]string{"learning", "validate", path}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "learning-effectiveness-v1: 9 valid learning cases") {
		t.Fatalf("validation output = %q", output.String())
	}
	reportDir := filepath.Join(t.TempDir(), "experiment")
	output.Reset()
	if err := workEvalCommand([]string{"learning", "run", path, "--report-dir", reportDir, "--id", "cli-learning"}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "split=development") || !strings.Contains(output.String(), "learning probes pass") {
		t.Fatalf("run output = %q", output.String())
	}
	output.Reset()
	if err := workEvalCommand([]string{"learning", "show", filepath.Join(reportDir, "experiment.json")}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "cli-learning:") || !strings.Contains(output.String(), "Split: development") {
		t.Fatalf("show output = %q", output.String())
	}
}
