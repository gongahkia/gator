package eval

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
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

func TestWorkExperimentDefaultsToDevelopmentAndRequiresExplicitHeldOutSelection(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "source"), 0o700); err != nil {
		t.Fatal(err)
	}
	dataset := WorkDataset{Version: 1, ID: "splits", Target: "work.v1", Cases: []WorkCase{
		{ID: "development-case", Split: WorkSplitDevelopment, Source: "source", Objective: "Write report", Contract: artifact.DefaultContract("report.md"), Mode: action.Draft, MaxSteps: 2, Graders: []WorkGrader{{Version: 1, Kind: "file_contains", Path: "report.md", Expected: "ready"}}},
		{ID: "held-out-case", Split: WorkSplitHeldOut, Source: "source", Objective: "Write report", Contract: artifact.DefaultContract("report.md"), Mode: action.Draft, MaxSteps: 2, Graders: []WorkGrader{{Version: 1, Kind: "file_contains", Path: "report.md", Expected: "ready"}}},
	}}
	factory := func(_ WorkCase, state string) (workrun.Service, error) {
		return workrun.Service{Executor: workrun.Executor{
			Model: &ScriptedModel{Turns: []agent.Turn{
				{ToolCalls: []agent.ToolCall{{ID: "write", Name: "write_artifact", Arguments: json.RawMessage(`{"path":"report.md","content":"ready"}`)}}},
				{Text: "Done."},
			}},
			StateDir: state,
		}}, nil
	}
	run := func(id, split string) WorkExperiment {
		t.Helper()
		report, err := RunWorkExperiment(context.Background(), filepath.Join(root, "dataset.json"), dataset, WorkEvalOptions{ID: id, Attempts: 1, ReportDir: filepath.Join(root, id), Split: split}, factory)
		if err != nil {
			t.Fatal(err)
		}
		return report
	}
	development := run("development", "")
	if development.Split != WorkSplitDevelopment || development.Total != 1 || development.Trials[0].Split != WorkSplitDevelopment {
		t.Fatalf("default split = %#v", development)
	}
	retained, err := os.ReadFile(filepath.Join(root, "development", "dataset.json"))
	if err != nil || strings.Contains(string(retained), "held-out-case") {
		t.Fatalf("development report retained held-out material: %s; %v", retained, err)
	}
	heldOut := run("held-out", WorkSplitHeldOut)
	if heldOut.Split != WorkSplitHeldOut || heldOut.Total != 1 || heldOut.Trials[0].Split != WorkSplitHeldOut {
		t.Fatalf("held-out split = %#v", heldOut)
	}
	all := run("all", WorkSplitAll)
	if all.Split != WorkSplitAll || all.Total != 2 {
		t.Fatalf("all split = %#v", all)
	}
	if _, _, err := selectWorkCases(dataset, "unexpected"); err == nil {
		t.Fatal("unknown split accepted")
	}
}

func TestWorkExperimentDoesNotContinueFailedParent(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "source"), 0o700); err != nil {
		t.Fatal(err)
	}
	caseWithFollowup := WorkCase{
		ID: "failed-parent", Family: "continuation", Split: WorkSplitDevelopment, Source: "source",
		Objective: "Create a report", Contract: artifact.DefaultContract("report.md"), Mode: action.Draft, MaxSteps: 2,
		Followups: []WorkFollowup{{Objective: "Revise the report", Parent: 0, Turns: []agent.Turn{{Text: "This should never run."}}}},
		Graders:   []WorkGrader{{Version: 1, Kind: "status", Expected: "failed"}},
	}
	dataset := WorkDataset{Version: 1, ID: "failed-parent", Target: "work.v1", Cases: []WorkCase{caseWithFollowup}}
	report, err := RunWorkExperiment(context.Background(), filepath.Join(root, "dataset.json"), dataset, WorkEvalOptions{ID: "failed-parent", Attempts: 1, ReportDir: filepath.Join(root, "report")}, func(_ WorkCase, state string) (workrun.Service, error) {
		return workrun.Service{Executor: workrun.Executor{Model: &ScriptedModel{}, StateDir: state}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Total != 1 || len(report.Trials) != 1 || strings.Contains(report.Trials[0].RevisionID, "-rev1") {
		t.Fatalf("follow-up ran after failed parent: %+v", report.Trials)
	}
}

func TestPersonalContinuityCorpusExercisesChangedSourceAndGoalState(t *testing.T) {
	path := "testdata/personal-continuity-v1/dataset.json"
	dataset, err := LoadWorkDataset(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(dataset.Cases) < 20 {
		t.Fatalf("continuity corpus has %d cases", len(dataset.Cases))
	}
	selected := map[string]bool{"venue-change": true, "brief-preference-correction": true, "goal-priority-change": true}
	var cases []WorkCase
	for _, item := range dataset.Cases {
		if selected[item.ID] {
			cases = append(cases, item)
		}
	}
	dataset.Cases = cases
	factory := func(item WorkCase, state string) (workrun.Service, error) {
		return workrun.Service{Executor: workrun.Executor{Model: &ScriptedModel{Turns: item.Turns}, StateDir: state}}, nil
	}
	root := t.TempDir()
	options := WorkEvalOptions{ID: "personal-agent", Provider: "scripted", Model: "same", Attempts: 1, ReportDir: filepath.Join(root, "agent"), Split: WorkSplitAll}
	agentReport, err := RunWorkExperiment(context.Background(), path, dataset, options, factory)
	if err != nil || agentReport.Passed != 3 {
		t.Fatalf("agent report = %#v, %v", agentReport, err)
	}
	options.ID, options.ReportDir, options.BaselineNoGoal = "personal-baseline", filepath.Join(root, "baseline"), true
	baselineReport, err := RunWorkExperiment(context.Background(), path, dataset, options, factory)
	if err != nil || baselineReport.Total != 3 || !baselineReport.BaselineNoGoal {
		t.Fatalf("baseline report = %#v, %v", baselineReport, err)
	}
	if _, err := CompareWork(baselineReport, agentReport); err != nil {
		t.Fatalf("same-model comparison failed: %v", err)
	}
	if baselineReport.Trials[0].PolicySHA256 == agentReport.Trials[0].PolicySHA256 || baselineReport.Trials[0].PolicySHA256 == "" {
		t.Fatal("baseline retained the same goal-aware policy")
	}
	if original, err := os.ReadFile(filepath.Join(filepath.Dir(path), "sources", "venue-change", "status.md")); err != nil || !strings.Contains(string(original), "Venue A is available") {
		t.Fatalf("fixture was changed by trial: %q, %v", original, err)
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

func TestUserWorkGradersRejectWrongFieldsAndMissingClaims(t *testing.T) {
	path := "testdata/user-work-v1/dataset.json"
	dataset, err := LoadWorkDataset(path)
	if err != nil {
		t.Fatal(err)
	}
	var c WorkCase
	for _, candidate := range dataset.Cases {
		if candidate.ID == "meeting-actions" {
			c = candidate
			break
		}
	}
	if c.ID == "" {
		t.Fatal("meeting actions case is missing")
	}
	outcome, err := (workrun.Service{Executor: workrun.Executor{Model: &ScriptedModel{Turns: c.Turns}, StateDir: t.TempDir()}}).Execute(context.Background(), workrun.Request{SourcePath: filepath.Join(filepath.Dir(path), c.Source), Objective: c.Objective, Contract: c.Contract, Mode: c.Mode})
	if err != nil {
		t.Fatal(err)
	}
	for _, grader := range []WorkGrader{
		{Kind: "json_pointer_equals", Path: "actions.json", Locator: "/actions/Mina/due", Expected: `"2026-10-22"`},
		{Kind: "json_pointer_contains", Path: "actions.json", Locator: "/actions/Mina/task", Expected: "rollback"},
		{Kind: "json_pointer_absent", Path: "actions.json", Locator: "/actions/Mina"},
		{Kind: "file_excludes", Path: "actions.json", Expected: "Mina"},
		{Kind: "file_regex", Path: "actions.json", Expected: `(?i)not a real action`},
		{Kind: "file_not_regex", Path: "actions.json", Expected: `(?i)Mina`},
	} {
		if grade := gradeWork(grader, outcome, nil, c.Contract); grade.Passed {
			t.Errorf("wrong field or claim passed: %+v", grader)
		}
	}
}

func TestPPTXSlideGradersRejectWrongSlideAndText(t *testing.T) {
	path := "testdata/user-work-v2/dataset.json"
	dataset, err := LoadWorkDataset(path)
	if err != nil {
		t.Fatal(err)
	}
	var c WorkCase
	for _, candidate := range dataset.Cases {
		if candidate.ID == "vendor-review-deck" {
			c = candidate
			break
		}
	}
	if c.ID == "" {
		t.Fatal("vendor deck case is missing")
	}
	outcome, err := (workrun.Service{Executor: workrun.Executor{Model: &ScriptedModel{Turns: c.Turns}, StateDir: t.TempDir()}}).Execute(context.Background(), workrun.Request{SourcePath: filepath.Join(filepath.Dir(path), c.Source), Objective: c.Objective, Contract: c.Contract, Mode: c.Mode})
	if err != nil {
		t.Fatal(err)
	}
	if evidence, err := rubricInput(outcome.Work.Path); err != nil || !strings.Contains(evidence, "visible slide 2") || !strings.Contains(evidence, "Cobalt") {
		t.Fatalf("rubric did not receive visible deck text: %v, %q", err, evidence)
	}
	for _, grader := range []WorkGrader{
		{Kind: "pptx_slide_count", Path: "deck.pptx", Expected: "4"},
		{Kind: "pptx_slide_contains", Path: "deck.pptx", Locator: "1", Expected: "Cobalt"},
		{Kind: "pptx_slide_contains", Path: "deck.pptx", Locator: "2", Expected: "contract signed"},
		{Kind: "pptx_slide_excludes", Path: "deck.pptx", Locator: "2", Expected: "Fir"},
		{Kind: "pptx_text_contains", Path: "deck.pptx", Expected: "contract signed"},
		{Kind: "pptx_text_excludes", Path: "deck.pptx", Expected: "Cobalt"},
	} {
		if grade := gradeWork(grader, outcome, nil, c.Contract); grade.Passed {
			t.Errorf("wrong slide claim passed: %+v", grader)
		}
	}
}

func TestUserWorkV4DocumentAndWorkbookGradersRejectWrongContent(t *testing.T) {
	path := "testdata/user-work-v4/dataset.json"
	dataset, err := LoadWorkDataset(path)
	if err != nil {
		t.Fatal(err)
	}
	checks := map[string]WorkGrader{
		"qbr-pdf-to-memo":         {Kind: "document_text_contains", Path: "memo.docx", Expected: "100% adoption"},
		"client-proposal-pdf":     {Kind: "document_text_contains", Path: "proposal.pdf", Expected: "USD 9,999"},
		"scenario-model-workbook": {Kind: "cell_formula_equals", Path: "model.xlsx", Sheet: "Scenarios", Cell: "C2", Expected: "A1+B1"},
		"funnel-chart-workbook":   {Kind: "xlsx_chart_count", Path: "funnel.xlsx", Expected: "2"},
	}
	for _, c := range dataset.Cases {
		wrong, selected := checks[c.ID]
		if !selected {
			continue
		}
		t.Run(c.ID, func(t *testing.T) {
			outcome, err := (workrun.Service{Executor: workrun.Executor{Model: &ScriptedModel{Turns: c.Turns}, StateDir: t.TempDir()}}).Execute(context.Background(), workrun.Request{SourcePath: filepath.Join(filepath.Dir(path), c.Source), Objective: c.Objective, Contract: c.Contract, Mode: c.Mode})
			if err != nil {
				t.Fatal(err)
			}
			if grade := gradeWork(wrong, outcome, nil, c.Contract); grade.Passed || grade.Error != "" {
				t.Fatalf("wrong output passed or could not be read: %+v", grade)
			}
			if c.ID == "qbr-pdf-to-memo" || c.ID == "client-proposal-pdf" {
				input, err := rubricInput(outcome.Work.Path)
				if err != nil || !strings.Contains(input, "Artifact ") {
					t.Fatalf("document text absent from rubric input: %v, %q", err, input)
				}
			}
		})
	}
}

func TestUserWorkV2ReviewRubricValid(t *testing.T) {
	data, err := os.ReadFile("testdata/user-work-v2/review-rubric.json")
	if err != nil {
		t.Fatal(err)
	}
	var rubric Rubric
	if err := json.Unmarshal(data, &rubric); err != nil {
		t.Fatal(err)
	}
	if err := rubric.Validate(); err != nil {
		t.Fatal(err)
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
