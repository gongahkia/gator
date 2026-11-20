package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func localFirstProfilePath() string {
	return filepath.Join("..", "..", "internal", "eval", "testdata", "local-first-v1", "profile.json")
}

func TestLocalFirstCommandDefaultsToOfflineDevelopmentReferences(t *testing.T) {
	secret := "P04L1_FIXTURE_KEY_NEVER_PUBLISH_7192"
	t.Setenv("OPENAI_API_KEY", secret)
	t.Setenv("ANTHROPIC_API_KEY", secret)
	dir := filepath.Join(t.TempDir(), "report")
	var out bytes.Buffer
	if err := workEvalCommand([]string{"local-first", "run", localFirstProfilePath(), "--report-dir", dir}, &out); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "experiment.json"))
	if err != nil {
		t.Fatal(err)
	}
	var report struct {
		Scripted      bool
		Split         string
		Passed, Total int
	}
	if json.Unmarshal(data, &report) != nil || !report.Scripted || report.Split != "development" || report.Passed != 25 || report.Total != 25 {
		t.Fatalf("unexpected fixture report: %+v", report)
	}
	err = filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.Contains(data, []byte(secret)) {
			t.Error("fixture credential found in a controlled artifact")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), secret) || !strings.Contains(out.String(), "Model quality NOT TESTED") {
		t.Fatal("unsafe or misleading summary")
	}
	if err := workEvalCommand([]string{"local-first", "run", localFirstProfilePath(), "--report-dir", dir}, &out); err == nil {
		t.Fatal("existing report directory overwritten")
	}
}

func TestLocalFirstCommandRejectsLiveUnknownArmsAndOverBudgetBeforeWriting(t *testing.T) {
	for _, flags := range [][]string{{"--live"}, {"--architecture", "Z"}, {"--max-model-requests", "257"}, {"--timeout-seconds", "121"}, {"--attempts", "3"}, {"--provider", "openai"}} {
		dir := filepath.Join(t.TempDir(), "must-not-exist")
		args := append([]string{"local-first", "run", localFirstProfilePath(), "--report-dir", dir}, flags...)
		if err := workEvalCommand(args, &bytes.Buffer{}); err == nil {
			t.Fatal("unsafe flags accepted", flags)
		}
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatal("rejected run created output state")
		}
	}
}

func TestLocalFirstCommandEnforcesAggregateRequestBudget(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "budget-report")
	if err := workEvalCommand([]string{"local-first", "run", localFirstProfilePath(), "--report-dir", dir, "--max-model-requests", "1"}, &bytes.Buffer{}); err == nil {
		t.Fatal("exhausted fixture budget counted as success")
	}
	data, err := os.ReadFile(filepath.Join(dir, "experiment.json"))
	if err != nil {
		t.Fatal(err)
	}
	var report struct {
		Total, Passed int
		Outcomes      map[string]int
	}
	if json.Unmarshal(data, &report) != nil || report.Passed == report.Total || report.Outcomes["budget"] == 0 {
		t.Fatal("budget failure not retained")
	}
}
