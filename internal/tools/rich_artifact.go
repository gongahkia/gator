package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/document"
	"github.com/gongahkia/gator/internal/presentation"
	"github.com/gongahkia/gator/internal/workbook"
	"github.com/gongahkia/gator/internal/workspace"
)

type WriteDocumentArtifact struct {
	Root       workspace.Root
	Source     workspace.Root
	Contract   artifact.Contract
	OnRenderer func(artifact.RendererEvidence)
}

func (t WriteDocumentArtifact) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{
		Name:        "work_write_document",
		Description: "Render a polished DOCX or PDF artifact from a semantic document specification. Supported blocks: heading, paragraph, callout, bullet_list, numbered_list, table, and page_break.",
		Parameters:  schema(`{"type":"object","additionalProperties":false,"required":["path","document"],"properties":{"path":{"type":"string","pattern":"\\.(docx|pdf)$"},"template_path":{"type":"string","description":"Optional source-relative DOCX template path"},"document":{"type":"object","required":["title","blocks"],"properties":{"version":{"type":"integer"},"title":{"type":"string"},"author":{"type":"string"},"subject":{"type":"string"},"theme":{"enum":["professional","minimal","report"]},"header":{"type":"string"},"footer":{"type":"string"},"blocks":{"type":"array","items":{"type":"object"}}}}}}`),
	}
}

func (t WriteDocumentArtifact) Execute(_ context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	var arguments struct {
		Path         string        `json:"path"`
		TemplatePath string        `json:"template_path"`
		Document     document.Spec `json:"document"`
	}
	if err := decodeArguments(raw, &arguments); err != nil {
		return agent.ToolResult{}, err
	}
	var contents []byte
	var templateBytes []byte
	var preview document.Preview
	var err error
	switch strings.ToLower(filepath.Ext(arguments.Path)) {
	case ".docx":
		if arguments.TemplatePath == "" {
			contents, preview, err = document.RenderDOCX(arguments.Document)
		} else {
			template, readErr := t.Source.ReadRegularFile(filepath.FromSlash(arguments.TemplatePath), 64*1024*1024)
			if readErr != nil {
				return agent.ToolResult{}, readErr
			}
			templateBytes = template
			contents, preview, err = document.RenderDOCXTemplate(arguments.Document, bytes.NewReader(template))
		}
	case ".pdf":
		if arguments.TemplatePath != "" {
			return agent.ToolResult{}, errors.New("PDF rendering uses a Gator theme and does not accept a DOCX template")
		}
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
	evidence, err := artifact.NewRendererEvidence(arguments.Path, "gator.document.v1", arguments.Document.Normalize(), templateBytes, contents)
	if err != nil {
		return agent.ToolResult{}, err
	}
	if t.OnRenderer != nil {
		t.OnRenderer(evidence)
	}
	content, err := success(struct {
		Path    string           `json:"path"`
		Bytes   int              `json:"bytes"`
		Preview document.Preview `json:"preview"`
	}{arguments.Path, len(contents), preview})
	return agent.ToolResult{Content: content}, err
}

type WriteWorkbookArtifact struct {
	Root       workspace.Root
	Source     workspace.Root
	Contract   artifact.Contract
	OnRenderer func(artifact.RendererEvidence)
}

// WritePresentationArtifact creates a new semantic deck or conservatively
// edits an existing source-relative deck. Existing deck edits deliberately
// preserve unsupported package parts such as media, animations, embedded
// objects, and VBA without exposing or executing them.
type WritePresentationArtifact struct {
	Root       workspace.Root
	Source     workspace.Root
	Contract   artifact.Contract
	OnRenderer func(artifact.RendererEvidence)
}

func (t WritePresentationArtifact) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{
		Name:        "work_write_presentation",
		Description: "Create a polished PPTX from a semantic presentation spec, or edit a source-relative PPTX with explicit bounded operations. New decks support titles, body text, speaker notes, and fade/push/wipe transitions. Existing decks preserve untouched media, embedded objects, unsupported animations, and VBA raw parts; Gator never reads, authors, edits, or executes VBA.",
		Parameters:  schema(`{"type":"object","additionalProperties":false,"required":["path"],"properties":{"path":{"type":"string","pattern":"\\.pptx$"},"template_path":{"type":"string","description":"Source-relative existing PPTX to edit; requires edits and cannot be combined with presentation"},"presentation":{"type":"object","description":"Required for a new deck; cannot be combined with template_path","properties":{"version":{"type":"integer"},"title":{"type":"string"},"theme":{"enum":["professional","minimal","report"]},"slides":{"type":"array","items":{"type":"object"}}}},"edits":{"type":"array","maxItems":256,"items":{"type":"object"}}}}`),
	}
}

func (t WritePresentationArtifact) Execute(_ context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	var arguments struct {
		Path         string              `json:"path"`
		TemplatePath string              `json:"template_path"`
		Presentation presentation.Spec   `json:"presentation"`
		Edits        []presentation.Edit `json:"edits"`
	}
	if err := decodeArguments(raw, &arguments); err != nil {
		return agent.ToolResult{}, err
	}
	if strings.ToLower(filepath.Ext(arguments.Path)) != ".pptx" {
		return agent.ToolResult{}, errors.New("presentation artifact path must end in .pptx")
	}
	var contents, templateBytes []byte
	var preview presentation.Preview
	var evidenceSpec any
	var err error
	if arguments.TemplatePath == "" {
		if len(arguments.Edits) != 0 {
			return agent.ToolResult{}, errors.New("new presentations use presentation; edits require template_path")
		}
		contents, preview, err = presentation.RenderPPTX(arguments.Presentation)
		evidenceSpec = struct {
			Kind         string            `json:"kind"`
			Presentation presentation.Spec `json:"presentation"`
		}{Kind: "new", Presentation: arguments.Presentation.Normalize()}
	} else {
		if arguments.Presentation.Title != "" || len(arguments.Presentation.Slides) != 0 || len(arguments.Edits) == 0 {
			return agent.ToolResult{}, errors.New("existing presentations require template_path and 1-256 edits, without presentation")
		}
		templateBytes, err = t.Source.ReadRegularFile(filepath.FromSlash(arguments.TemplatePath), 64*1024*1024)
		if err != nil {
			return agent.ToolResult{}, err
		}
		contents, preview, err = presentation.EditPPTX(templateBytes, arguments.Edits)
		evidenceSpec = struct {
			Kind  string              `json:"kind"`
			Edits []presentation.Edit `json:"edits"`
		}{Kind: "edit", Edits: arguments.Edits}
	}
	if err != nil {
		return agent.ToolResult{}, err
	}
	if err := artifact.WriteBinary(t.Root, t.Contract, arguments.Path, contents); err != nil {
		return agent.ToolResult{}, err
	}
	evidence, err := artifact.NewRendererEvidence(arguments.Path, "gator.presentation.v1", evidenceSpec, templateBytes, contents)
	if err != nil {
		return agent.ToolResult{}, err
	}
	if t.OnRenderer != nil {
		t.OnRenderer(evidence)
	}
	content, err := success(struct {
		Path    string               `json:"path"`
		Bytes   int                  `json:"bytes"`
		Preview presentation.Preview `json:"preview"`
	}{Path: arguments.Path, Bytes: len(contents), Preview: preview})
	return agent.ToolResult{Content: content}, err
}

func (t WriteWorkbookArtifact) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{
		Name:        "work_write_workbook",
		Description: "Render a styled XLSX artifact from a semantic workbook specification with typed cells, formulas, tables, filters, frozen rows, formats, and charts.",
		Parameters:  schema(`{"type":"object","additionalProperties":false,"required":["path","workbook"],"properties":{"path":{"type":"string","pattern":"\\.xlsx$"},"template_path":{"type":"string","description":"Optional source-relative XLSX template path"},"workbook":{"type":"object","required":["sheets"],"properties":{"version":{"type":"integer"},"title":{"type":"string"},"theme":{"enum":["professional","minimal","report"]},"sheets":{"type":"array","items":{"type":"object"}}}}}}`),
	}
}

func (t WriteWorkbookArtifact) Execute(_ context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	var arguments struct {
		Path         string        `json:"path"`
		TemplatePath string        `json:"template_path"`
		Workbook     workbook.Spec `json:"workbook"`
	}
	if err := decodeArguments(raw, &arguments); err != nil {
		return agent.ToolResult{}, err
	}
	if strings.ToLower(filepath.Ext(arguments.Path)) != ".xlsx" {
		return agent.ToolResult{}, errors.New("workbook artifact path must end in .xlsx")
	}
	var template *bytes.Reader
	var templateBytes []byte
	if arguments.TemplatePath != "" {
		data, readErr := t.Source.ReadRegularFile(filepath.FromSlash(arguments.TemplatePath), 64*1024*1024)
		if readErr != nil {
			return agent.ToolResult{}, readErr
		}
		templateBytes = data
		template = bytes.NewReader(data)
	}
	contents, preview, err := workbook.RenderXLSX(arguments.Workbook, template)
	if err != nil {
		return agent.ToolResult{}, err
	}
	if err := artifact.WriteBinary(t.Root, t.Contract, arguments.Path, contents); err != nil {
		return agent.ToolResult{}, err
	}
	evidence, err := artifact.NewRendererEvidence(arguments.Path, "gator.workbook.v1", arguments.Workbook.Normalize(), templateBytes, contents)
	if err != nil {
		return agent.ToolResult{}, err
	}
	if t.OnRenderer != nil {
		t.OnRenderer(evidence)
	}
	content, err := success(struct {
		Path    string           `json:"path"`
		Bytes   int              `json:"bytes"`
		Preview workbook.Preview `json:"preview"`
	}{arguments.Path, len(contents), preview})
	return agent.ToolResult{Content: content}, err
}
