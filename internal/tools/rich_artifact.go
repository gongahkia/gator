package tools

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/document"
	"github.com/gongahkia/gator/internal/workbook"
	"github.com/gongahkia/gator/internal/workspace"
)

type WriteDocumentArtifact struct {
	Root     workspace.Root
	Contract artifact.Contract
}

func (t WriteDocumentArtifact) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{
		Name:        "work_write_document",
		Description: "Render a polished DOCX or PDF artifact from a semantic document specification. Supported blocks: heading, paragraph, callout, bullet_list, numbered_list, table, and page_break.",
		Parameters:  schema(`{"type":"object","additionalProperties":false,"required":["path","document"],"properties":{"path":{"type":"string","pattern":"\\.(docx|pdf)$"},"document":{"type":"object","required":["title","blocks"],"properties":{"version":{"type":"integer"},"title":{"type":"string"},"author":{"type":"string"},"subject":{"type":"string"},"theme":{"enum":["professional","minimal","report"]},"header":{"type":"string"},"footer":{"type":"string"},"blocks":{"type":"array","items":{"type":"object"}}}}}}`),
	}
}

func (t WriteDocumentArtifact) Execute(_ context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	var arguments struct {
		Path     string        `json:"path"`
		Document document.Spec `json:"document"`
	}
	if err := decodeArguments(raw, &arguments); err != nil {
		return agent.ToolResult{}, err
	}
	var contents []byte
	var preview document.Preview
	var err error
	switch strings.ToLower(filepath.Ext(arguments.Path)) {
	case ".docx":
		contents, preview, err = document.RenderDOCX(arguments.Document)
	case ".pdf":
		contents, preview, err = document.RenderPDF(arguments.Document)
	default:
		return agent.ToolResult{}, errors.New("document artifact path must end in .docx or .pdf")
	}
	if err != nil {
		return agent.ToolResult{}, err
	}
	if err := artifact.WriteBinary(t.Root, t.Contract, arguments.Path, contents); err != nil {
		return agent.ToolResult{}, err
	}
	content, err := success(struct {
		Path    string           `json:"path"`
		Bytes   int              `json:"bytes"`
		Preview document.Preview `json:"preview"`
	}{arguments.Path, len(contents), preview})
	return agent.ToolResult{Content: content}, err
}

type WriteWorkbookArtifact struct {
	Root     workspace.Root
	Contract artifact.Contract
}

func (t WriteWorkbookArtifact) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{
		Name:        "work_write_workbook",
		Description: "Render a styled XLSX artifact from a semantic workbook specification with typed cells, formulas, tables, filters, frozen rows, formats, and charts.",
		Parameters:  schema(`{"type":"object","additionalProperties":false,"required":["path","workbook"],"properties":{"path":{"type":"string","pattern":"\\.xlsx$"},"workbook":{"type":"object","required":["sheets"],"properties":{"version":{"type":"integer"},"title":{"type":"string"},"theme":{"enum":["professional","minimal","report"]},"sheets":{"type":"array","items":{"type":"object"}}}}}}`),
	}
}

func (t WriteWorkbookArtifact) Execute(_ context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	var arguments struct {
		Path     string        `json:"path"`
		Workbook workbook.Spec `json:"workbook"`
	}
	if err := decodeArguments(raw, &arguments); err != nil {
		return agent.ToolResult{}, err
	}
	if strings.ToLower(filepath.Ext(arguments.Path)) != ".xlsx" {
		return agent.ToolResult{}, errors.New("workbook artifact path must end in .xlsx")
	}
	contents, preview, err := workbook.RenderXLSX(arguments.Workbook, nil)
	if err != nil {
		return agent.ToolResult{}, err
	}
	if err := artifact.WriteBinary(t.Root, t.Contract, arguments.Path, contents); err != nil {
		return agent.ToolResult{}, err
	}
	content, err := success(struct {
		Path    string           `json:"path"`
		Bytes   int              `json:"bytes"`
		Preview workbook.Preview `json:"preview"`
	}{arguments.Path, len(contents), preview})
	return agent.ToolResult{Content: content}, err
}
