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
		if name := tool.Definition().Name; strings.HasPrefix(name, "write_") || name == "artifact_status" {
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

func TestTypedArtifactToolsProduceValidStructuredFiles(t *testing.T) {
	root, err := workspace.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	jsonContract := artifact.DefaultContract("summary.json")
	executeTool(t, WriteJSONArtifact{Root: root, Contract: jsonContract}, `{"path":"summary.json","value":{"answer":42}}`)
	jsonStatus := executeTool(t, ArtifactStatus{Root: root, Contract: artifact.Contract{
		Version: artifact.ContractVersion, Artifacts: []artifact.Requirement{{Path: "summary.json", Validations: []artifact.Validation{{Kind: artifact.JSON}}}},
		MaxArtifactBytes: artifact.DefaultMaxArtifactBytes, MaxTotalBytes: artifact.DefaultMaxTotalBytes, ExternalActions: "forbid",
	}}, `{}`)
	if !strings.Contains(jsonStatus, `"passed":true`) {
		t.Fatalf("JSON status = %s", jsonStatus)
	}
	if err := os.Remove(filepath.Join(root.Path(), "summary.json")); err != nil {
		t.Fatal(err)
	}
	result := executeTool(t, WriteTableArtifact{Root: root, Contract: artifact.DefaultContract("table.csv")}, `{"path":"table.csv","headers":["name","value"],"rows":[["north","10"],["south, east","20"]]}`)
	if !strings.Contains(result, `"rows":2`) || !strings.Contains(result, `"columns":2`) {
		t.Fatalf("table result = %s", result)
	}
	contents, err := os.ReadFile(filepath.Join(root.Path(), "table.csv"))
	if err != nil || !strings.Contains(string(contents), `"south, east",20`) {
		t.Fatalf("table contents = %q, %v", contents, err)
	}
}
