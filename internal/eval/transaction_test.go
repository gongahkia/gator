package eval

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/delivery"
	"github.com/gongahkia/gator/internal/workhistory"
	"github.com/gongahkia/gator/internal/workrun"
)

func TestInspectWorkTransactionSeparatesVerificationFromDelivery(t *testing.T) {
	state, outcome := completedTransactionWork(t, "transaction-undelivered")
	fidelity, err := InspectWorkTransaction(state, outcome.Manifest.RunID, action.Draft)
	if err != nil {
		t.Fatal(err)
	}
	if fidelity.Work.State != workCompleted || fidelity.History.State != transactionPassed || fidelity.Verification.State != transactionPassed || fidelity.Delivery.State != deliveryNotAttempted || fidelity.Recovery.State != recoveryNotRun {
		t.Fatalf("undelivered transaction = %#v", fidelity)
	}

	store, err := delivery.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.DeliverArtifacts(context.Background(), mustBundle(t, outcome), t.TempDir(), false); err != nil {
		t.Fatal(err)
	}
	fidelity, err = InspectWorkTransaction(state, outcome.Manifest.RunID, action.Draft)
	if err != nil {
		t.Fatal(err)
	}
	if fidelity.Delivery.State != deliveryFully || fidelity.Recovery.State != recoveryNotRequired {
		t.Fatalf("fully delivered transaction = %#v", fidelity)
	}
}

func TestTransactionDeliveryClassifierRecognizesPartialRetryAndConflict(t *testing.T) {
	partial := []delivery.Record{{Effects: []delivery.Effect{
		{ID: "effect-a", Kind: delivery.ArtifactFile, Status: delivery.Applied},
		{ID: "effect-b", Kind: delivery.ArtifactFile, Status: delivery.Failed, Retryable: true},
		{ID: "effect-c", Kind: delivery.ArtifactFile, Status: delivery.Pending, Retryable: true},
	}}}
	if got := deliveryDimension(partial); got.State != deliveryPartial {
		t.Fatalf("partial delivery = %#v", got)
	}
	if got := recoveryDimension(partial, TransactionDimension{State: externalNone}); got.State != recoveryRetryEligible {
		t.Fatalf("partial recovery = %#v", got)
	}
	conflict := []delivery.Record{{Effects: []delivery.Effect{{ID: "effect-conflict", Kind: delivery.ArtifactFile, Status: delivery.Failed, Retryable: false}}}}
	if got := deliveryDimension(conflict); got.State != deliveryFailed {
		t.Fatalf("conflict delivery = %#v", got)
	}
	if got := recoveryDimension(conflict, TransactionDimension{State: externalNone}); got.State != recoveryBlockedConflict {
		t.Fatalf("conflict recovery = %#v", got)
	}
}

func TestRetryLeavesTheOriginalWorkTransactionAndModelUntouched(t *testing.T) {
	state, outcome := completedTransactionWork(t, "transaction-retry")
	store, err := delivery.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.DeliverArtifacts(context.Background(), mustBundle(t, outcome), t.TempDir(), false)
	if err != nil {
		t.Fatal(err)
	}
	history, err := workhistory.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	before, err := history.List(0)
	if err != nil {
		t.Fatal(err)
	}
	retried, err := store.Retry(context.Background(), outcome.Manifest.RunID, record.ID)
	if err != nil {
		t.Fatal(err)
	}
	after, err := history.List(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 1 || len(after) != 1 || after[0].ID != outcome.Manifest.RunID || len(retried.Effects) != 1 || len(retried.Effects[0].Attempts) != 1 {
		t.Fatalf("retry fabricated Work or reapplied an effect: before=%#v after=%#v record=%#v", before, after, retried)
	}
}

func TestUnknownExternalActionIsNotClassifiedAsReplaySafe(t *testing.T) {
	proposal, err := action.NewProposal("external-one", action.Publish, "release", "publish", "https://example.test/release", "publish release", []byte("hidden payload"))
	if err != nil {
		t.Fatal(err)
	}
	external := externalActionDimension([]action.Record{{Proposal: proposal, Status: action.Unknown, Error: "connection closed after send"}})
	if external.State != externalUnknown {
		t.Fatalf("external state = %#v", external)
	}
	if recovery := recoveryDimension(nil, external); recovery.State != recoveryBlockedUnknown {
		t.Fatalf("unknown external recovery = %#v", recovery)
	}
}

func TestFailedWorkIsNotMisreportedAsDeliveryFailure(t *testing.T) {
	state, source := t.TempDir(), t.TempDir()
	outcome, err := (workrun.Service{Executor: workrun.Executor{Model: &ScriptedModel{Turns: []agent.Turn{{Text: "No deliverable was produced."}}}, StateDir: state}}).Execute(context.Background(), workrun.Request{
		RunID: "transaction-work-failed", SourcePath: source, Objective: "Write report", Contract: artifact.DefaultContract("report.md"), Mode: action.Draft, MaxSteps: 1,
	})
	if err == nil {
		t.Fatal("failed Work unexpectedly succeeded")
	}
	fidelity, inspectErr := InspectWorkTransaction(state, outcome.Manifest.RunID, action.Draft)
	if inspectErr != nil {
		t.Fatal(inspectErr)
	}
	if fidelity.Work.State != workFailed || fidelity.History.State != transactionPassed || fidelity.Verification.State != transactionFailed || fidelity.Delivery.State != deliveryNotAttempted {
		t.Fatalf("failed Work transaction = %#v", fidelity)
	}
}

func TestTransactionInspectorDetectsMissingAndMismatchedHistory(t *testing.T) {
	missing, err := InspectWorkTransaction(t.TempDir(), "transaction-missing", action.Draft)
	if err != nil || missing.History.State != transactionFailed || !stringsContains(missing.History.Error, "missing") {
		t.Fatalf("missing history = %#v, %v", missing, err)
	}
	state, outcome := completedTransactionWork(t, "transaction-mode")
	mismatched, err := InspectWorkTransaction(state, outcome.Manifest.RunID, action.Inspect)
	if err != nil || mismatched.History.State != transactionFailed || !stringsContains(mismatched.History.Error, "mode") {
		t.Fatalf("mismatched history = %#v, %v", mismatched, err)
	}
}

func completedTransactionWork(t *testing.T, runID string) (string, workrun.Outcome) {
	t.Helper()
	state, source := t.TempDir(), t.TempDir()
	arguments, err := json.Marshal(map[string]string{"path": "report.md", "content": "verified report\n"})
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := (workrun.Service{Executor: workrun.Executor{Model: &ScriptedModel{Turns: []agent.Turn{
		{ToolCalls: []agent.ToolCall{{ID: "write", Name: "write_artifact", Arguments: arguments}}},
		{Text: "Prepared the report."},
	}}, StateDir: state}}).Execute(context.Background(), workrun.Request{
		RunID: runID, SourcePath: source, Objective: "Write a report", Contract: artifact.DefaultContract("report.md"), Mode: action.Draft, MaxSteps: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	return state, outcome
}

func mustBundle(t *testing.T, outcome workrun.Outcome) artifact.Bundle {
	t.Helper()
	bundle, err := artifact.OpenBundle(outcome.Work.Path)
	if err != nil {
		t.Fatal(err)
	}
	return bundle
}

func stringsContains(value, substring string) bool { return strings.Contains(value, substring) }
