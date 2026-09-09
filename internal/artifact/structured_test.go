package artifact

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/workspace"
)

func TestWriteJSONStagesFormattedValidatedValue(t *testing.T) {
	root, err := workspace.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	contract := DefaultContract("data.json")
	if err := WriteJSON(root, contract, "data.json", json.RawMessage(`{"answer":42,"items":[1,2]}`)); err != nil {
		t.Fatal(err)
	}
	contents, err := root.ReadRegularFile("data.json", 1024)
	if err != nil || !strings.Contains(string(contents), "  \"answer\": 42") || !strings.HasSuffix(string(contents), "\n") {
		t.Fatalf("JSON contents = %q, %v", contents, err)
	}
	if err := WriteJSON(root, contract, "data.json", json.RawMessage(`{"broken":`)); err == nil {
		t.Fatal("invalid JSON replaced the artifact")
	}
}

func TestWriteTableStagesRectangularCSV(t *testing.T) {
	root, err := workspace.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	contract := DefaultContract("results.csv")
	if err := WriteTable(root, contract, "results.csv", []string{"name", "value"}, [][]string{{"north", "10"}, {"south, east", "20"}}); err != nil {
		t.Fatal(err)
	}
	contents, err := root.ReadRegularFile("results.csv", 1024)
	if err != nil || string(contents) != "name,value\nnorth,10\n\"south, east\",20\n" {
		t.Fatalf("CSV contents = %q, %v", contents, err)
	}
	inspection, err := Inspect(root, Contract{
		Version: ContractVersion, Artifacts: []Requirement{{Path: "results.csv", Validations: []Validation{{Kind: CSV}}}},
		MaxArtifactBytes: DefaultMaxArtifactBytes, MaxTotalBytes: DefaultMaxTotalBytes, ExternalActions: "forbid",
	})
	if err != nil || !inspection.Passed {
		t.Fatalf("CSV inspection = %#v, %v", inspection, err)
	}
}

func TestWriteTableRejectsInvalidShapeBeforeMutation(t *testing.T) {
	root, err := workspace.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	contract := DefaultContract("results.csv")
	if err := WriteTable(root, contract, "results.csv", []string{"name", "name"}, [][]string{{"a", "b"}}); err == nil {
		t.Fatal("duplicate headers were accepted")
	}
	if err := WriteTable(root, contract, "results.csv", []string{"name", "value"}, [][]string{{"only one"}}); err == nil {
		t.Fatal("ragged row was accepted")
	}
}
