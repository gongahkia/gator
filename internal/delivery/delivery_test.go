package delivery

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/patch"
	"github.com/gongahkia/gator/internal/sandbox"
	"github.com/gongahkia/gator/internal/workspace"
)

func TestDeliverArtifactsPersistsAppliedEffectsAcrossReload(t *testing.T) {
	bundle := artifactBundle(t, "delivery-artifacts", map[string]string{"report.md": "report\n", "summary.txt": "summary\n"})
	state, target := t.TempDir(), t.TempDir()
	store, err := Open(state)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.DeliverArtifacts(context.Background(), bundle, target, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(record.Effects) != 2 || record.Effects[0].Status != Applied || record.Effects[1].Status != Applied {
		t.Fatalf("effects = %#v", record.Effects)
	}
	for name, want := range map[string]string{"report.md": "report\n", "summary.txt": "summary\n"} {
		contents, err := os.ReadFile(filepath.Join(target, name))
		if err != nil || string(contents) != want {
			t.Fatalf("%s = %q, %v", name, contents, err)
		}
	}
	reopened, err := Open(state)
	if err != nil {
		t.Fatal(err)
	}
	targetRoot, rootErr := workspace.Open(target)
	loaded, err := reopened.Load("delivery-artifacts", record.ID)
	if err != nil || rootErr != nil || loaded.ID != record.ID || loaded.TargetPath != targetRoot.Path() {
		t.Fatalf("reloaded delivery = %#v, %v", loaded, err)
	}
}

func TestDeliverArtifactsRetainsPartialFailureAndRetriesOnlyRemainingEffects(t *testing.T) {
	bundle := artifactBundle(t, "delivery-partial", map[string]string{"a.txt": "a\n", "b.txt": "b\n", "c.txt": "c\n"})
	state, target := t.TempDir(), t.TempDir()
	store, err := Open(state)
	if err != nil {
		t.Fatal(err)
	}
	fail := true
	store.applyArtifact = func(bundle artifact.Bundle, target string, operation artifact.ApplyOperation) error {
		if operation.Path == "b.txt" && fail {
			return errors.New("simulated target write failure")
		}
		return artifact.ApplyOne(bundle, target, operation)
	}
	record, err := store.DeliverArtifacts(context.Background(), bundle, target, false)
	if err == nil {
		t.Fatal("partial delivery unexpectedly succeeded")
	}
	if statuses(record) != "a.txt=applied,b.txt=failed,c.txt=pending" {
		t.Fatalf("partial statuses = %s", statuses(record))
	}
	if _, err := os.Stat(filepath.Join(target, "a.txt")); err != nil {
		t.Fatalf("first effect was not applied: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "c.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("later pending effect was applied: %v", err)
	}

	// A fresh store proves retry comes from persisted intent, not retained model
	// state. The already-applied first artifact is not written again.
	fresh, err := Open(state)
	if err != nil {
		t.Fatal(err)
	}
	fail = false
	retried, err := fresh.Retry(context.Background(), "delivery-partial", record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if statuses(retried) != "a.txt=applied,b.txt=applied,c.txt=applied" {
		t.Fatalf("retry statuses = %s", statuses(retried))
	}
	if len(retried.Effects[0].Attempts) != 1 || len(retried.Effects[1].Attempts) != 2 || len(retried.Effects[2].Attempts) != 1 {
		t.Fatalf("retry attempts = %#v", retried.Effects)
	}
}

func TestRetryDoesNotRepeatAppliedArtifact(t *testing.T) {
	bundle := artifactBundle(t, "delivery-idempotent", map[string]string{"report.md": "sealed\n"})
	state, target := t.TempDir(), t.TempDir()
	store, err := Open(state)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.DeliverArtifacts(context.Background(), bundle, target, false)
	if err != nil {
		t.Fatal(err)
	}
	retried, err := store.Retry(context.Background(), "delivery-idempotent", record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(retried.Effects[0].Attempts) != 1 || retried.Effects[0].Status != Applied {
		t.Fatalf("already-applied retry = %#v", retried.Effects[0])
	}
	contents, err := os.ReadFile(filepath.Join(target, "report.md"))
	if err != nil || string(contents) != "sealed\n" {
		t.Fatalf("target after idempotent retry = %q, %v", contents, err)
	}
}

func TestDeliverArtifactsRecordsConflictWithoutOverwriting(t *testing.T) {
	bundle := artifactBundle(t, "delivery-conflict", map[string]string{"report.md": "sealed\n"})
	state, target := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(target, "report.md"), []byte("developer copy\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := Open(state)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.DeliverArtifacts(context.Background(), bundle, target, false)
	if err == nil || len(record.Effects) != 1 || record.Effects[0].Status != Failed || record.Effects[0].Retryable {
		t.Fatalf("conflict record = %#v, %v", record, err)
	}
	contents, readErr := os.ReadFile(filepath.Join(target, "report.md"))
	if readErr != nil || string(contents) != "developer copy\n" {
		t.Fatalf("conflict target changed = %q, %v", contents, readErr)
	}
}

func TestRetryReconcilesInterruptedLocalArtifactDelivery(t *testing.T) {
	bundle := artifactBundle(t, "delivery-reconcile", map[string]string{"report.md": "sealed\n"})
	state, target := t.TempDir(), t.TempDir()
	store, err := Open(state)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := PreviewArtifacts(bundle, target, false)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.artifactRecord(bundle, plan, false)
	if err != nil {
		t.Fatal(err)
	}
	record, err = store.storeOrLoad(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.startAttempt(&record, 0); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "report.md"), []byte("sealed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	fresh, err := Open(state)
	if err != nil {
		t.Fatal(err)
	}
	reconciled, err := fresh.Retry(context.Background(), "delivery-reconcile", record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reconciled.Effects[0].Status != Applied || reconciled.Effects[0].Attempts[0].Status != Applied {
		t.Fatalf("reconciled record = %#v", reconciled)
	}
}

func TestDeliverCandidateUsesSamePersistentLifecycle(t *testing.T) {
	bundle, source := candidateBundle(t, "delivery-code")
	target := filepath.Join(t.TempDir(), "target")
	if err := patch.InitSnapshot(context.Background(), source, target); err != nil {
		t.Fatal(err)
	}
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.DeliverCandidate(context.Background(), bundle, target, bundle.Manifest.Candidates[0].ID)
	if err != nil || len(record.Effects) != 1 || record.Effects[0].Kind != CodeCandidate || record.Effects[0].Status != Applied {
		t.Fatalf("code delivery = %#v, %v", record, err)
	}
	contents, readErr := os.ReadFile(filepath.Join(target, "a.txt"))
	if readErr != nil || string(contents) != "changed\n" {
		t.Fatalf("candidate target = %q, %v", contents, readErr)
	}
	retried, err := store.Retry(context.Background(), "delivery-code", record.ID)
	if err != nil || len(retried.Effects[0].Attempts) != 1 {
		t.Fatalf("code retry = %#v, %v", retried, err)
	}
}

func TestDeliveryRecordDoesNotCopyArtifactContents(t *testing.T) {
	secret := "private artifact payload that must remain in the bundle"
	bundle := artifactBundle(t, "delivery-private", map[string]string{"report.md": secret})
	state := t.TempDir()
	store, err := Open(state)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.DeliverArtifacts(context.Background(), bundle, t.TempDir(), false)
	if err != nil {
		t.Fatal(err)
	}
	path, err := store.WorkPath(record.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(filepath.Join(path, record.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), secret) || strings.Contains(string(payload), "objective") {
		t.Fatalf("delivery state copied private content: %s", payload)
	}
}

func statuses(record Record) string {
	parts := make([]string, 0, len(record.Effects))
	for _, effect := range record.Effects {
		parts = append(parts, effect.SourcePath+"="+string(effect.Status))
	}
	return strings.Join(parts, ",")
}

func artifactBundle(t *testing.T, runID string, files map[string]string) artifact.Bundle {
	t.Helper()
	bundlePath, sourcePath := t.TempDir(), t.TempDir()
	outputPath := filepath.Join(bundlePath, "output")
	if err := os.Mkdir(outputPath, 0o700); err != nil {
		t.Fatal(err)
	}
	paths := make([]string, 0, len(files))
	for path, contents := range files {
		paths = append(paths, path)
		full := filepath.Join(outputPath, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	sort.Strings(paths)
	source, err := workspace.Open(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	output, err := workspace.Open(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	manifest, err := artifact.Seal(output, artifact.DefaultContract(paths...), artifact.SealOptions{RunID: runID, Objective: "Generate verified files", Source: source, StartedAt: now, FinishedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	if err := artifact.WriteManifest(filepath.Join(bundlePath, "manifest.json"), manifest); err != nil {
		t.Fatal(err)
	}
	bundle, err := artifact.OpenBundle(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	return bundle
}

func candidateBundle(t *testing.T, runID string) (artifact.Bundle, string) {
	t.Helper()
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "a.txt"), []byte("base\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	payload := []byte("diff --git a/a.txt b/a.txt\n--- a/a.txt\n+++ b/a.txt\n@@ -1 +1 @@\n-base\n+changed\n")
	candidate, err := patch.Integrate(context.Background(), source, filepath.Join(t.TempDir(), "candidate"), [][]byte{payload}, []string{"code"}, nil, sandbox.Policy{Mode: sandbox.Off})
	if err != nil {
		t.Fatal(err)
	}
	bundlePath := t.TempDir()
	outputPath := filepath.Join(bundlePath, "output")
	if err := os.MkdirAll(filepath.Join(outputPath, "code"), 0o700); err != nil {
		t.Fatal(err)
	}
	candidate.PatchPath = "code/candidate.patch"
	if err := os.WriteFile(filepath.Join(outputPath, filepath.FromSlash(candidate.PatchPath)), candidate.Patch, 0o600); err != nil {
		t.Fatal(err)
	}
	sourceRoot, err := workspace.Open(source)
	if err != nil {
		t.Fatal(err)
	}
	output, err := workspace.Open(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	manifest, err := artifact.Seal(output, artifact.InspectionContract(), artifact.SealOptions{RunID: runID, Objective: "Prepare Code candidate", Source: sourceRoot, StartedAt: now, FinishedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	manifest.Candidates = []patch.Candidate{candidate}
	if err := artifact.WriteManifest(filepath.Join(bundlePath, "manifest.json"), manifest); err != nil {
		t.Fatal(err)
	}
	bundle, err := artifact.OpenBundle(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	return bundle, source
}
