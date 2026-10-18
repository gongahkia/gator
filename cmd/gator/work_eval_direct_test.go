package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/eval"
)

type directBaselineModel struct{ request agent.TurnRequest }

func (model *directBaselineModel) Complete(_ context.Context, request agent.TurnRequest) (agent.Turn, error) {
	model.request = request
	return agent.Turn{Text: "Subject: Customer update\n\nDraft based on notes."}, nil
}

func TestDirectBaselineUsesSameSourcesWithoutTools(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "sources", "email")
	if err := os.MkdirAll(source, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "notes.md"), []byte("Confirmed update at 16:00 UTC."), 0o600); err != nil {
		t.Fatal(err)
	}
	dataset := eval.WorkDataset{ID: "product", Cases: []eval.WorkCase{{ID: "email", Family: "email", Source: "sources/email", Objective: "Draft an email."}}}
	backend := &directBaselineModel{}
	dir := filepath.Join(root, "report")
	report, err := runDirectBaseline(context.Background(), filepath.Join(root, "dataset.json"), dataset, []string{"email"}, backend, dir, "gemini", "test-model")
	if err != nil || len(report.Trials) != 1 || report.Trials[0].Status != "completed" || report.Trials[0].SourceSHA256 == "" {
		t.Fatalf("report = %#v; %v", report, err)
	}
	if len(backend.request.Tools) != 0 || !strings.Contains(backend.request.Messages[0].Content, "Confirmed update at 16:00 UTC") || !strings.Contains(backend.request.Messages[0].Content, "Draft an email") {
		t.Fatalf("direct request = %#v", backend.request)
	}
	if _, err := os.Stat(filepath.Join(dir, "email.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "report.json")); err != nil {
		t.Fatal(err)
	}
}
