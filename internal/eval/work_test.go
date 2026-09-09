package eval

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/workrun"
)

func TestWorkExperimentRejectsInvalidAblationsBeforeExecution(t *testing.T) {
	for _, roles := range [][]string{{"source_reader"}, {"source_researcher", "source_researcher"}} {
		directory := filepath.Join(t.TempDir(), "experiment")
		_, err := RunWorkExperiment(context.Background(), "unused", WorkDataset{}, WorkEvalOptions{ID: "invalid-ablation", Attempts: 1, ReportDir: directory, DisabledRoles: roles}, func(WorkCase, string) (workrun.Service, error) {
			t.Fatal("invalid ablation started execution")
			return workrun.Service{}, nil
		})
		if err == nil || !strings.Contains(err.Error(), "disabled role") {
			t.Fatalf("invalid role error: %v", err)
		}
		if _, err := os.Stat(directory); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("invalid ablation created an experiment: %v", err)
		}
	}
}

func TestWorkGradersRejectFalseCitationsAndSemanticallyWrongOutput(t *testing.T) {
	path := "testdata/work-v1/dataset.json"
	dataset, err := LoadWorkDataset(path)
	if err != nil {
		t.Fatal(err)
	}
	c := dataset.Cases[0]
	outcome, err := (workrun.Service{Executor: workrun.Executor{Model: &ScriptedModel{Turns: c.Turns}, StateDir: t.TempDir()}}).Execute(context.Background(), workrun.Request{SourcePath: filepath.Join(filepath.Dir(path), c.Source), Objective: c.Objective, Contract: c.Contract})
	if err != nil {
		t.Fatal(err)
	}
	valid := WorkGrader{Version: 1, Kind: "evidence_reference", Path: "report.md", Locator: "source/old.txt", Quote: "100"}
	if grade := gradeWork(valid, outcome, nil, c.Contract); !grade.Passed {
		t.Fatal(grade)
	}
	valid.Quote = "999"
	if grade := gradeWork(valid, outcome, nil, c.Contract); grade.Passed {
		t.Fatal("fabricated quotation passed")
	}
	valid.Quote = "100"
	valid.Locator = "source/nonexistent.txt"
	if grade := gradeWork(valid, outcome, nil, c.Contract); grade.Passed {
		t.Fatal("fabricated locator passed")
	}
	if grade := gradeWork(WorkGrader{Version: 1, Kind: "file_equals", Path: "report.md", Expected: "Producing agent says every claim is true"}, outcome, nil, c.Contract); grade.Passed {
		t.Fatal("self assessment passed instead of independent expected output")
	}
	valid.Path = "missing.md"
	if grade := gradeWork(valid, outcome, nil, c.Contract); grade.Passed || grade.Error == "" {
		t.Fatal("missing grading input not attributable")
	}
}

func TestDatasetDigestIncludesFixtureBytesAndRejectsChangedFixture(t *testing.T) {
	original, err := LoadWorkDataset("testdata/work-v1/dataset.json")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(source, "input.txt")
	if err := os.WriteFile(file, []byte("first"), 0600); err != nil {
		t.Fatal(err)
	}
	dataset := WorkDataset{Version: 1, ID: "digest", Target: "work.v1", Cases: []WorkCase{original.Cases[0]}}
	dataset.Cases[0].Source = "source"
	dataset.Cases[0].FixtureSHA256 = ""
	path := filepath.Join(root, "dataset.json")
	data, _ := json.Marshal(dataset)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	first, err := LoadWorkDataset(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("second"), 0600); err != nil {
		t.Fatal(err)
	}
	second, err := LoadWorkDataset(path)
	if err != nil {
		t.Fatal(err)
	}
	if workHash(first) == workHash(second) {
		t.Fatal("fixture bytes absent from dataset identity")
	}
	data, _ = json.Marshal(first)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadWorkDataset(path); err == nil {
		t.Fatal("pinned fixture mismatch accepted")
	}
	if _, err := CompareWork(WorkExperiment{DatasetSHA256: workHash(first)}, WorkExperiment{DatasetSHA256: workHash(second)}); err == nil {
		t.Fatal("different fixture comparison accepted")
	}
}

func TestRubricRequiresQuotedEvidenceAndIndependentHumanCalibration(t *testing.T) {
	var rubric Rubric
	data, err := os.ReadFile("testdata/work-v1/synthesis-rubric.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &rubric); err != nil {
		t.Fatal(err)
	}
	if err := rubric.Validate(); err != nil {
		t.Fatal(err)
	}
	scores := []RubricScore{{Criterion: "evidence", Score: 3, Quote: "supported", Rationale: "explicit evidence"}, {Criterion: "usefulness", Score: 3, Quote: "supported", Rationale: "clear answer"}}
	if err := validateRubricScores(rubric, scores, "supported conclusion"); err != nil {
		t.Fatal(err)
	}
	scores[0].Quote = "invented evidence"
	if err := validateRubricScores(rubric, scores, "supported conclusion"); err == nil {
		t.Fatal("invented rationale evidence accepted")
	}
	judge := RubricReport{Version: 1, ExperimentID: "experiment", RubricSHA256: workHash(rubric), Source: "model", Trials: []RubricTrial{{TrialID: "case", InputSHA256: "digest", Scores: []RubricScore{{Criterion: "evidence", Score: 3}}}}}
	human := judge
	human.Source = "human"
	human.Reviewer = "fixture-annotator"
	human.Trials = []RubricTrial{{TrialID: "case", InputSHA256: "digest", Scores: []RubricScore{{Criterion: "evidence", Score: 2}}}}
	result, err := CalibrateRubric(judge, human)
	if err != nil || result.Pairs != 1 || result.MeanAbsoluteError != 1 {
		t.Fatalf("calibration arithmetic: %+v %v", result, err)
	}
	human.Source = "model"
	if _, err := CalibrateRubric(judge, human); err == nil {
		t.Fatal("self calibration accepted")
	}
	// these are synthetic annotations testing arithmetic, not human calibration evidence.
	if !strings.Contains(WorkSummary(WorkExperiment{Trials: []WorkTrial{{Usage: agent.Usage{ModelRequests: 2, UnknownRequests: 2}}}}), "2 without token usage") {
		t.Fatal("unknown usage hidden")
	}
}

type rubricTestModel struct {
	t *testing.T
}

func (m rubricTestModel) Complete(_ context.Context, request agent.TurnRequest) (agent.Turn, error) {
	m.t.Helper()
	input, _ := json.Marshal(request.Messages)
	if len(request.Tools) != 0 || !strings.Contains(string(input), "Sources disagree") || !strings.Contains(string(input), "source/old.txt") {
		m.t.Fatalf("judge did not receive tool-free retained deliverables/evidence: %+v", request)
	}
	if strings.Contains(string(input), "write_artifact") {
		m.t.Fatal("producing agent tool transcript leaked to judge")
	}
	return agent.Turn{Text: `{"scores":[{"criterion":"evidence","score":3,"quote":"Sources disagree","rationale":"fixture evidence quotation"},{"criterion":"usefulness","score":2,"quote":"Sources disagree","rationale":"fixture synthesis quotation"}]}`}, nil
}

func TestRubricJudgeUsesVerifiedBundleAndSharedBudget(t *testing.T) {
	path := "testdata/work-v1/dataset.json"
	dataset, err := LoadWorkDataset(path)
	if err != nil {
		t.Fatal(err)
	}
	c := dataset.Cases[0]
	outcome, err := (workrun.Service{Executor: workrun.Executor{Model: &ScriptedModel{Turns: c.Turns}, StateDir: t.TempDir()}}).Execute(context.Background(), workrun.Request{SourcePath: filepath.Join(filepath.Dir(path), c.Source), Objective: c.Objective, Contract: c.Contract})
	if err != nil {
		t.Fatal(err)
	}
	var rubric Rubric
	data, err := os.ReadFile("testdata/work-v1/synthesis-rubric.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &rubric); err != nil {
		t.Fatal(err)
	}
	experiment := WorkExperiment{ID: "rubric-fixture", Trials: []WorkTrial{{ID: "first", BundlePath: outcome.Work.Path}, {ID: "second", BundlePath: outcome.Work.Path}}}
	result, err := JudgeWorkRubric(context.Background(), rubricTestModel{t}, experiment, rubric, "separate-fixture-judge", 1)
	if err != nil || len(result.Trials) != 2 || result.Trials[0].Error != "" || len(result.Trials[0].Scores) != 2 || result.Trials[1].Error == "" || result.Usage.ModelRequests != 1 || result.Calibration != "uncalibrated" {
		t.Fatalf("separate judge/budget: %+v, %v", result, err)
	}
	if err := os.WriteFile(filepath.Join(outcome.Work.Path, "output", "report.md"), []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := rubricInput(outcome.Work.Path); err == nil {
		t.Fatal("tampered bundle accepted as judge evidence")
	}
}
