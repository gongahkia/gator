package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/workspace"
)

func TestPresentationToolSchemaExposesVisibleSlideBody(t *testing.T) {
	definition := (WritePresentationArtifact{}).Definition()
	var spec struct {
		Properties map[string]struct {
			Properties map[string]struct {
				Items struct {
					Required   []string                   `json:"required"`
					Properties map[string]json.RawMessage `json:"properties"`
				} `json:"items"`
			} `json:"properties"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(definition.Parameters, &spec); err != nil {
		t.Fatalf("presentation tool has invalid JSON schema: %v", err)
	}
	slides := spec.Properties["presentation"].Properties["slides"].Items
	if _, ok := slides.Properties["body"]; !ok {
		t.Fatal("model cannot see the visible slide body field")
	}
	for _, name := range slides.Required {
		if name == "body" {
			return
		}
	}
	t.Fatal("visible slide body is not required")
}

func TestWriteWorkbookWithoutTemplate(t *testing.T) {
	output, err := workspace.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	tool := WriteWorkbookArtifact{Root: output, Contract: artifact.DefaultContract("book.xlsx")}
	_, err = tool.Execute(context.Background(), json.RawMessage(`{"path":"book.xlsx","workbook":{"title":"Smoke","sheets":[{"name":"Data","columns":[{"header":"Value"}],"rows":[[{"value":7}]]}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := output.ReadRegularFile("book.xlsx", 16*1024*1024); err != nil {
		t.Fatal(err)
	}
}
