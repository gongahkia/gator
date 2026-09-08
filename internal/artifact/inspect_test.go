package artifact

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gongahkia/gator/internal/workspace"
)

func TestInspectBuildsEvidenceForCompleteArtifacts(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	writeArtifact(t, directory, "report.md", "# Review\n\nSources\n")
	writeArtifact(t, directory, "tables/summary.csv", "name,value\nalpha,1\n")
	writeArtifact(t, directory, "notes.txt", "retained extra output\n")
	root, err := workspace.Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	contract := DefaultContract("report.md", "tables/summary.csv")
	contract.Artifacts[0].MediaTypes = []string{"text/markdown"}
	contract.Artifacts[0].Validations = append(contract.Artifacts[0].Validations,
		Validation{Kind: UTF8}, Validation{Kind: Contains, Value: "Sources"})
	contract.Artifacts[1].MediaTypes = []string{"text/csv"}
	contract.Artifacts[1].Validations = append(contract.Artifacts[1].Validations, Validation{Kind: CSV})

	inspection, err := Inspect(root, contract)
	if err != nil {
		t.Fatal(err)
	}
	if !inspection.Passed || len(inspection.Files) != 3 {
		t.Fatalf("inspection = %#v", inspection)
	}
	if inspection.Files[0].Path != "notes.txt" || inspection.Files[1].Path != "report.md" || inspection.Files[2].Path != "tables/summary.csv" {
		t.Fatalf("artifact files are not stable and sorted: %#v", inspection.Files)
	}
	for _, result := range inspection.Validations {
		if !result.Passed || result.Diagnostic != "" {
			t.Fatalf("unexpected validation result: %#v", result)
		}
	}
}

func TestInspectReportsMissingAndMalformedArtifacts(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	writeArtifact(t, directory, "data.json", "{bad json")
	root, err := workspace.Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	contract := DefaultContract("data.json", "missing.csv")
	contract.Artifacts[0].Validations = append(contract.Artifacts[0].Validations, Validation{Kind: JSON})
	contract.Artifacts[1].Validations = append(contract.Artifacts[1].Validations, Validation{Kind: CSV})

	inspection, err := Inspect(root, contract)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.Passed {
		t.Fatal("malformed output passed inspection")
	}
	failed := 0
	for _, result := range inspection.Validations {
		if !result.Passed {
			failed++
		}
	}
	if failed != 4 {
		t.Fatalf("failed validation count = %d; results = %#v", failed, inspection.Validations)
	}
}

func TestInspectRejectsSymlinkAndByteLimitEscapes(t *testing.T) {
	t.Parallel()

	t.Run("symlink", func(t *testing.T) {
		directory := t.TempDir()
		if err := os.Symlink(filepath.Join(t.TempDir(), "outside"), filepath.Join(directory, "report.md")); err != nil {
			t.Fatal(err)
		}
		root, err := workspace.Open(directory)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := Inspect(root, DefaultContract("report.md")); err == nil {
			t.Fatal("symlink artifact was accepted")
		}
	})

	t.Run("limit", func(t *testing.T) {
		directory := t.TempDir()
		writeArtifact(t, directory, "report.md", "too large")
		root, err := workspace.Open(directory)
		if err != nil {
			t.Fatal(err)
		}
		contract := DefaultContract("report.md")
		contract.MaxArtifactBytes = 4
		contract.MaxTotalBytes = 4
		if _, err := Inspect(root, contract); err == nil {
			t.Fatal("oversized artifact was accepted")
		}
	})
}

func writeArtifact(t *testing.T, root, path, contents string) {
	t.Helper()
	target := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}
