package tools

import (
	"context"
	"encoding/json"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/document"
	"github.com/gongahkia/gator/internal/workspace"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractDocumentReturnsPDFPageEvidence(t *testing.T) {
	sourcePath := t.TempDir()
	payload, _, err := document.RenderPDF(document.Spec{
		Title:  "Evidence",
		Blocks: []document.Block{{Kind: "paragraph", Text: "page-addressable fact"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourcePath, "evidence.pdf"), payload, 0600); err != nil {
		t.Fatal(err)
	}
	source, err := workspace.Open(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	result, err := (ExtractDocument{Root: source}).Execute(context.Background(), json.RawMessage(`{"path":"source/evidence.pdf"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Content, `"locator_precision":"page"`) ||
		!strings.Contains(result.Content, `"page":1`) ||
		!strings.Contains(result.Content, "page-addressable fact") ||
		!strings.Contains(result.Content, `"source_sha256"`) {
		t.Fatalf("PDF extraction result = %s", result.Content)
	}
}

func TestReconcileExactDecimalTotalsAndSourceRows(t *testing.T) {
	sourcePath := t.TempDir()
	for name, data := range map[string]string{"left.csv": "id,amount\na,0.1\na,0.2\nb,\n", "right.csv": "id,amount\na,0.3\nb,2\n"} {
		if err := os.WriteFile(filepath.Join(sourcePath, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	source, _ := workspace.Open(sourcePath)
	output, _ := workspace.Open(t.TempDir())
	tool := ReconcileTables{Source: source, Output: output, Contract: artifact.DefaultContract("report.md", "checked.xlsx")}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"left":{"path":"left.csv"},"right":{"path":"right.csv"},"key":"id","amount":"amount","workbook":"checked.xlsx","memo":"report.md"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Content, `"difference":"-2"`) {
		t.Fatal(result)
	}
	table, err := ReadTable(output, TableSource{Path: "checked.xlsx", Sheet: "Reconciliation"})
	if err != nil {
		t.Fatal(err)
	}
	if table.Rows[1][3] != "0" || !strings.Contains(table.Rows[1][4], "duplicate") || !strings.Contains(table.Rows[1][5], "row:3") {
		t.Fatalf("%+v", table.Rows)
	}
}
