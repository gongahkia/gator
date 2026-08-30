package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEvalCommandRequiresFixtureDirectory(t *testing.T) {
	var output bytes.Buffer
	err := evalCommand(nil, &output)
	if err == nil || !strings.Contains(err.Error(), "usage: gator eval") {
		t.Fatalf("eval error = %v", err)
	}
}

func TestParseEvalArgumentsAcceptsFixtureBeforeFlags(t *testing.T) {
	directory, options, err := parseEvalArguments("eval", []string{"fixture", "--run-id", "headless-001", "--report=report.json", "--require-resolved"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(directory) || options.runID != "headless-001" || options.reportPath != "report.json" || !options.requireResolved {
		t.Fatalf("parsed eval options = directory=%q options=%#v", directory, options)
	}
}

func TestEvalCommandWritesReportBeforeRequireResolvedFailure(t *testing.T) {
	directory := t.TempDir()
	repository := filepath.Join(directory, "repository")
	if err := os.MkdirAll(repository, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, "go.mod"), []byte("module example.com/eval\n\ngo 1.25.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	spec := `{"version":1,"id":"headless","task":"Do nothing","max_steps":1,"verify":[["go","test","./..."]],"sandbox":"off","repository":"repository"}`
	if err := os.WriteFile(filepath.Join(directory, "eval.json"), []byte(spec), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "script.json"), []byte(`[{"text":"done"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	reportPath := filepath.Join(t.TempDir(), "report.json")
	var output bytes.Buffer
	err := evalCommand([]string{directory, "--run-id", "headless-001", "--report", reportPath, "--require-resolved"}, &output)
	if err == nil || !strings.Contains(err.Error(), "report was written") {
		t.Fatalf("eval error = %v", err)
	}
	contents, readErr := os.ReadFile(reportPath)
	if readErr != nil {
		t.Fatalf("read report after unresolved run: %v", readErr)
	}
	if !strings.Contains(string(contents), `"status": "unresolved"`) || !strings.Contains(output.String(), "status=unresolved") {
		t.Fatalf("output=%q report=%s", output.String(), contents)
	}
}
