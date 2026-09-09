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
	for _, trial := range report.Trials {
		if trial.Status != "passed" {
			t.Errorf("%s: %s; grades %+v", trial.CaseID, trial.Error, trial.Grades)
		}
	}
}
