package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/workspace"
)

func TestWorkFilesSeparateSourceAndOutput(t *testing.T) {
	sourcePath := t.TempDir()
	outputPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(sourcePath, "notes.txt"), []byte("source evidence\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	source, err := workspace.Open(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	output, err := workspace.Open(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	surface, err := WorkFiles(source, output, artifact.DefaultContract("report.md"), true)
	if err != nil {
		t.Fatal(err)
	}

	list := toolNamed(t, surface, "list_files")
	listed := executeTool(t, list, `{"path":"source"}`)
	if !strings.Contains(listed, `"source/notes.txt"`) {
		t.Fatalf("source listing = %s", listed)
	}
	read := toolNamed(t, surface, "read_file")
	readResult := executeTool(t, read, `{"path":"source/notes.txt"}`)
	if !strings.Contains(readResult, "source evidence") || !strings.Contains(readResult, `"path":"source/notes.txt"`) {
		t.Fatalf("source read = %s", readResult)
	}

	write := toolNamed(t, surface, "write_artifact")
	executeTool(t, write, `{"path":"report.md","content":"# Finished\\n"}`)
	status := executeTool(t, toolNamed(t, surface, "artifact_status"), `{}`)
	if !strings.Contains(status, `"passed":true`) || !strings.Contains(status, `"path":"report.md"`) {
		t.Fatalf("artifact status = %s", status)
	}
	contents, err := os.ReadFile(filepath.Join(sourcePath, "notes.txt"))
	if err != nil || string(contents) != "source evidence\n" {
		t.Fatalf("source changed: %q, %v", contents, err)
	}
}

func TestWorkFilesInspectModeHasNoMutationTools(t *testing.T) {
	source, err := workspace.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	output, err := workspace.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	surface, err := WorkFiles(source, output, artifact.DefaultContract("report.md"), false)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range surface {
		if name := tool.Definition().Name; name == "write_artifact" || name == "artifact_status" {
			t.Fatalf("inspect mode exposed %q", name)
		}
	}
}

func TestWriteArtifactRejectsEscapingPath(t *testing.T) {
	root, err := workspace.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	tool := WriteArtifact{Root: root, Contract: artifact.DefaultContract("report.md")}
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"path":"../report.md","content":"bad"}`)); err == nil {
		t.Fatal("escaping artifact path was accepted")
	}
}
