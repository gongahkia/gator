package artifact

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/workspace"
)

func TestSealWritesFilesystemBackedPrivateManifest(t *testing.T) {
	t.Parallel()

	source, err := workspace.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	outputDirectory := t.TempDir()
	writeArtifact(t, outputDirectory, "report.md", "finished report\n")
	output, err := workspace.Open(outputDirectory)
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := action.NewProposal("publish-1", action.Publish, "email", "send", "reviewer@example.com", "Send report to reviewer@example.com", []byte("private payload"))
	if err != nil {
		t.Fatal(err)
	}
	started := time.Date(2026, 9, 8, 1, 2, 3, 0, time.UTC)
	contract := DefaultContract("report.md")
	contract.ExternalActions = action.Propose
	manifest, err := Seal(output, contract, SealOptions{
		RunID: "work-001", Objective: "Prepare the report", Source: source,
		Actions:   []action.Record{{Proposal: proposal, Status: action.Pending}},
		StartedAt: started, FinishedAt: started.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Status != Completed || len(manifest.Artifacts) != 1 || manifest.Artifacts[0].SHA256 == "" || manifest.ContractSHA256 == "" {
		t.Fatalf("manifest = %#v", manifest)
	}
	manifestPath := filepath.Join(t.TempDir(), "manifest.json")
	if err := WriteManifest(manifestPath, manifest); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("manifest permissions = %o", info.Mode().Perm())
	}
	contents, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(contents), "private payload") {
		t.Fatal("manifest retained an external action payload")
	}
}

func TestSealEnforcesExternalActionDisposition(t *testing.T) {
	t.Parallel()

	source, err := workspace.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	outputDirectory := t.TempDir()
	writeArtifact(t, outputDirectory, "report.md", "finished report\n")
	output, err := workspace.Open(outputDirectory)
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := action.NewProposal("publish-1", action.Publish, "email", "send", "reviewer@example.com", "Send report", []byte("payload"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	options := SealOptions{
		RunID: "work-action", Objective: "Prepare the report", Source: source,
		Actions:   []action.Record{{Proposal: proposal, Status: action.Executed}},
		StartedAt: now, FinishedAt: now,
	}
	if _, err := Seal(output, DefaultContract("report.md"), options); err == nil {
		t.Fatal("forbidden external action was sealed")
	}
	contract := DefaultContract("report.md")
	contract.ExternalActions = action.Propose
	if _, err := Seal(output, contract, options); err == nil {
		t.Fatal("executed action was sealed under draft-only contract")
	}
	contract.ExternalActions = action.Approve
	if _, err := Seal(output, contract, options); err != nil {
		t.Fatal(err)
	}
}

func TestSealRetainsFailedValidationEvidence(t *testing.T) {
	t.Parallel()

	source, err := workspace.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	output, err := workspace.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	manifest, err := Seal(output, DefaultContract("missing.md"), SealOptions{
		RunID: "work-002", Objective: "Prepare the missing report", Source: source,
		StartedAt: now, FinishedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Status != Failed || len(manifest.Validations) == 0 || manifest.Validations[0].Passed {
		t.Fatalf("failed manifest = %#v", manifest)
	}
}
