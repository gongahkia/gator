package main

import (
	"context"
	"github.com/gongahkia/gator/internal/eval"
	"github.com/gongahkia/gator/internal/workrun"
	"path/filepath"
	"testing"
)

func TestWorkDepthCorpusThroughProductService(t *testing.T) {
	path := filepath.Join("..", "..", "internal", "eval", "testdata", "work-v1", "dataset.json")
	dataset, err := eval.LoadWorkDataset(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(dataset.Cases) != 24 {
		t.Fatal("corpus size changed without a version decision")
	}
	report, err := eval.RunWorkExperiment(context.Background(), path, dataset, eval.WorkEvalOptions{ID: "ci-work", Harness: "test", ReportDir: filepath.Join(t.TempDir(), "experiment"), Attempts: 1, Delegation: true}, func(c eval.WorkCase, state string) (workrun.Service, error) {
		return scriptedWorkService(c, state), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Split != eval.WorkSplitDevelopment {
		t.Fatalf("default evaluation split = %q", report.Split)
	}
	sawCode := false
	for _, trial := range report.Trials {
		if trial.Status != "passed" {
			t.Errorf("%s: %s; grades %+v", trial.CaseID, trial.Error, trial.Grades)
		}
		if trial.Transaction.History.State != "passed" || trial.Transaction.Delivery.State != "not_attempted" {
			t.Errorf("%s transaction = %#v", trial.CaseID, trial.Transaction)
		}
		if trial.CaseID == "single-code-patch" {
			sawCode = true
			if trial.Transaction.Work.State != "completed" || trial.Transaction.Verification.State != "passed" || trial.Transaction.Recovery.State != "not_run" {
				t.Errorf("Code Work transaction = %#v", trial.Transaction)
			}
		}
	}
	if !sawCode {
		t.Fatal("development Work corpus no longer exercises a Code transaction")
	}
}

func TestUserWorkCorpusThroughProductService(t *testing.T) {
	path := filepath.Join("..", "..", "internal", "eval", "testdata", "user-work-v1", "dataset.json")
	dataset, err := eval.LoadWorkDataset(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(dataset.Cases) != 8 {
		t.Fatal("user-work corpus size changed without a benchmark version decision")
	}
	report, err := eval.RunWorkExperiment(context.Background(), path, dataset, eval.WorkEvalOptions{ID: "ci-user-work", Harness: "test", ReportDir: filepath.Join(t.TempDir(), "experiment"), Attempts: 1, Split: eval.WorkSplitAll, Delegation: true}, func(c eval.WorkCase, state string) (workrun.Service, error) {
		return scriptedWorkService(c, state), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Total != 8 || report.Passed != 8 {
		for _, trial := range report.Trials {
			if trial.Status != "passed" {
				t.Errorf("%s: %s; grades %+v", trial.CaseID, trial.Error, trial.Grades)
			}
		}
		t.Fatalf("user-work corpus passed %d/%d scripted cases", report.Passed, report.Total)
	}
}

func TestUserWorkV2CorpusThroughProductService(t *testing.T) {
	path := filepath.Join("..", "..", "internal", "eval", "testdata", "user-work-v2", "dataset.json")
	dataset, err := eval.LoadWorkDataset(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(dataset.Cases) != 21 {
		t.Fatal("user-work-v2 corpus size changed without a benchmark version decision")
	}
	report, err := eval.RunWorkExperiment(context.Background(), path, dataset, eval.WorkEvalOptions{ID: "ci-user-work-v2", Harness: "test", ReportDir: filepath.Join(t.TempDir(), "experiment"), Attempts: 1, Split: eval.WorkSplitAll, Delegation: true}, func(c eval.WorkCase, state string) (workrun.Service, error) {
		return scriptedWorkService(c, state), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Total != 21 || report.Passed != 21 {
		for _, trial := range report.Trials {
			if trial.Status != "passed" {
				t.Errorf("%s: %s; grades %+v", trial.CaseID, trial.Error, trial.Grades)
			}
		}
		t.Fatalf("user-work-v2 corpus passed %d/%d scripted cases", report.Passed, report.Total)
	}
}

func TestUserWorkV3CorpusThroughProductService(t *testing.T) {
	path := filepath.Join("..", "..", "internal", "eval", "testdata", "user-work-v3", "dataset.json")
	dataset, err := eval.LoadWorkDataset(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(dataset.Cases) != 30 {
		t.Fatal("user-work-v3 corpus size changed without a benchmark version decision")
	}
	report, err := eval.RunWorkExperiment(context.Background(), path, dataset, eval.WorkEvalOptions{ID: "ci-user-work-v3", Harness: "test", ReportDir: filepath.Join(t.TempDir(), "experiment"), Attempts: 1, Split: eval.WorkSplitAll, Delegation: true}, func(c eval.WorkCase, state string) (workrun.Service, error) {
		return scriptedWorkService(c, state), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Total != 30 || report.Passed != 30 {
		for _, trial := range report.Trials {
			if trial.Status != "passed" {
				t.Errorf("%s: %s; grades %+v", trial.CaseID, trial.Error, trial.Grades)
			}
		}
		t.Fatalf("user-work-v3 corpus passed %d/%d scripted cases", report.Passed, report.Total)
	}
}

func TestUserWorkV4CorpusThroughProductService(t *testing.T) {
	path := filepath.Join("..", "..", "internal", "eval", "testdata", "user-work-v4", "dataset.json")
	dataset, err := eval.LoadWorkDataset(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(dataset.Cases) != 50 {
		t.Fatal("user-work-v4 corpus size changed without a benchmark version decision")
	}
	report, err := eval.RunWorkExperiment(context.Background(), path, dataset, eval.WorkEvalOptions{ID: "ci-user-work-v4", Harness: "test", ReportDir: filepath.Join(t.TempDir(), "experiment"), Attempts: 1, Split: eval.WorkSplitAll, Delegation: true}, func(c eval.WorkCase, state string) (workrun.Service, error) {
		return scriptedWorkService(c, state), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Total != 50 || report.Passed != 50 {
		for _, trial := range report.Trials {
			if trial.Status != "passed" {
				t.Errorf("%s: %s; grades %+v", trial.CaseID, trial.Error, trial.Grades)
			}
		}
		t.Fatalf("user-work-v4 corpus passed %d/%d scripted cases", report.Passed, report.Total)
	}
}
