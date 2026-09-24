package workrun

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
	"github.com/gongahkia/gator/internal/snapshot"
	"github.com/gongahkia/gator/internal/workhistory"
	"github.com/gongahkia/gator/internal/worksession"
)

func TestServiceRetainsCompletedWorkHistoryWithoutSensitivePayloads(t *testing.T) {
	state, source := t.TempDir(), t.TempDir()
	model := &scriptedModel{turns: []agent.Turn{
		{ToolCalls: []agent.ToolCall{{ID: "write", Name: "write_artifact", Arguments: json.RawMessage(`{"path":"report.md","content":"ready"}`)}}},
		{Text: "Prepared the report."},
	}}
	outcome, err := (Service{Executor: Executor{Model: model, StateDir: state}}).Execute(context.Background(), Request{
		RunID: "work-history-completed", SourcePath: source, Objective: "Prepare a report\nraw-objective-secret", Contract: artifact.DefaultContract("report.md"), MaxSteps: 3,
		Attachments: []agent.Attachment{{Name: "private.txt", MediaType: "text/plain", Data: []byte("raw-attachment-secret")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	store, err := workhistory.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load("work-history-completed")
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != workhistory.Completed || record.ArtifactStatus != string(artifact.Completed) || record.VerificationStatus != workhistory.VerificationPassed {
		t.Fatalf("history status = %#v", record)
	}
	if record.ConversationID != outcome.ConversationID || record.RevisionID != outcome.RevisionID || record.SnapshotID != outcome.SnapshotID || record.Evidence.ArtifactManifestPath != outcome.Work.ManifestPath {
		t.Fatalf("history references = %#v; outcome = %#v", record, outcome)
	}
	if _, err := snapshot.Open(state, record.SnapshotID); err != nil {
		t.Fatalf("history snapshot does not resolve: %v", err)
	}
	if bundle, err := artifact.OpenBundle(filepath.Dir(record.Evidence.ArtifactManifestPath)); err != nil || bundle.Manifest.RunID != record.ID {
		t.Fatalf("history bundle does not resolve: %#v, %v", bundle.Manifest, err)
	}
	if _, err := os.Stat(record.Evidence.TracePath); err != nil {
		t.Fatalf("history trace does not resolve: %v", err)
	}
	sessions, err := worksession.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	if revision, err := sessions.LoadRevision(record.ConversationID, record.RevisionID); err != nil || revision.SnapshotID != record.SnapshotID {
		t.Fatalf("history revision does not resolve: %#v, %v", revision, err)
	}
	payload, err := os.ReadFile(filepath.Join(store.Root(), record.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"raw-objective-secret", "raw-attachment-secret", "private.txt"} {
		if strings.Contains(string(payload), secret) {
			t.Fatalf("transaction copied sensitive/raw value %q: %s", secret, payload)
		}
	}
}

func TestServiceRetainsFailedWorkHistory(t *testing.T) {
	state, source := t.TempDir(), t.TempDir()
	model := &scriptedModel{turns: []agent.Turn{{Text: "Done without creating the report."}}}
	outcome, err := (Service{Executor: Executor{Model: model, StateDir: state}}).Execute(context.Background(), Request{
		RunID: "work-history-failed", SourcePath: source, Objective: "Write a report", Contract: artifact.DefaultContract("report.md"), MaxSteps: 1,
	})
	if err == nil || !errors.Is(err, context.DeadlineExceeded) && !strings.Contains(err.Error(), "step limit") {
		t.Fatalf("execution error = %v", err)
	}
	store, openErr := workhistory.Open(state)
	if openErr != nil {
		t.Fatal(openErr)
	}
	record, loadErr := store.Load("work-history-failed")
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if record.Status != workhistory.Failed || record.ArtifactStatus != string(artifact.Failed) || record.VerificationStatus != workhistory.VerificationFailed {
		t.Fatalf("failed history = %#v", record)
	}
	if record.RevisionID == "" || record.Evidence.ArtifactManifestPath == "" || record.SnapshotID == "" || outcome.Manifest.Status != artifact.Failed {
		t.Fatalf("failed history/outcome = %#v / %#v", record, outcome)
	}
	if _, statErr := os.Stat(record.Evidence.ArtifactManifestPath); statErr != nil {
		t.Fatalf("failed manifest is not retained: %v", statErr)
	}
}

func TestServicePublishesRunningWorkHistoryBeforeCompletion(t *testing.T) {
	state := t.TempDir()
	model := &controlledModel{entered: make(chan struct{}), release: make(chan struct{})}
	operation := (Service{Executor: Executor{Model: model, StateDir: state}}).Start(context.Background(), Request{
		RunID: "work-history-running", SourcePath: t.TempDir(), Objective: "Inspect", Mode: action.Inspect, Contract: artifact.InspectionContract(), MaxSteps: 1,
	})
	<-model.entered
	record, err := mustWorkHistory(t, state).Load("work-history-running")
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != workhistory.Running || !record.FinishedAt.IsZero() {
		t.Fatalf("running history = %#v", record)
	}
	close(model.release)
	for range operation.Events {
	}
	completion := <-operation.Done
	if completion.Err != nil {
		t.Fatal(completion.Err)
	}
}

func TestHistoryRecordsAuthorityProjection(t *testing.T) {
	state := t.TempDir()
	outcome, err := (Service{Executor: Executor{Model: &scriptedModel{turns: []agent.Turn{{Text: "Inspected."}}}, StateDir: state}}).Execute(context.Background(), Request{
		RunID: "work-history-authority", SourcePath: t.TempDir(), Objective: "Inspect", Mode: action.Inspect, Contract: artifact.InspectionContract(), MaxSteps: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	record, err := mustWorkHistory(t, state).Load(outcome.Work.ID)
	if err != nil {
		t.Fatal(err)
	}
	if record.Mode != action.Inspect || record.ExternalActions != action.Forbid || record.PolicySHA256 == "" {
		t.Fatalf("authority projection = %#v", record)
	}
}

func mustWorkHistory(t *testing.T, state string) workhistory.Store {
	t.Helper()
	store, err := workhistory.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	return store
}
