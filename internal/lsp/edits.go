package lsp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/workspace"
)

func (t Tool) format(path string, response json.RawMessage) (agent.ToolResult, error) {
	roots := t.rootSet()
	switch t.operation {
	case diagnosticsOperation:
		var result struct {
			Kind  string       `json:"kind"`
			Items []Diagnostic `json:"items"`
		}
		if err := json.Unmarshal(response, &result); err != nil {
			return agent.ToolResult{}, fmt.Errorf("decode diagnostic response: %w", err)
		}
		if result.Kind != "full" && result.Kind != "unchanged" {
			return agent.ToolResult{}, errors.New("LSP server returned an unsupported diagnostic report")
		}
		return formatDiagnostics(path, t.specification.Name, result.Items)
	case hoverOperation:
		return formatHover(path, t.specification.Name, response)
	case completionOperation:
		return formatCompletions(path, t.specification.Name, response)
	case codeActionsOperation:
		return formatCodeActions(roots, path, t.specification.Name, response)
	case formatOperation:
		return formatFormatting(path, t.specification.Name, response)
	case renameOperation:
		return formatRename(roots, path, t.specification.Name, response)
	case definitionOperation, referencesOperation:
		return formatLocations(roots, path, t.specification.Name, t.operation, response)
	case documentSymbolsOperation:
		return formatDocumentSymbols(roots, path, t.specification.Name, response)
	case workspaceSymbolsOperation:
		return formatWorkspaceSymbols(roots, path, t.specification.Name, response)
	default:
		return agent.ToolResult{}, fmt.Errorf("unsupported LSP operation %q", t.operation)
	}
}

func formatDiagnostics(path, server string, diagnostics []Diagnostic) (agent.ToolResult, error) {
	presented := make([]presentedDiagnostic, 0, min(len(diagnostics), maxDiagnostics))
	truncated := len(diagnostics) > maxDiagnostics
	for _, item := range diagnostics {
		if len(presented) == maxDiagnostics {
			truncated = true
			break
		}
		if item.Range.Start.Line < 0 || item.Range.Start.Character < 0 || item.Range.End.Line < 0 || item.Range.End.Character < 0 {
			return agent.ToolResult{}, errors.New("LSP server returned a negative diagnostic position")
		}
		code := ""
		if len(item.Code) != 0 && string(item.Code) != "null" {
			if !json.Valid(item.Code) || len(item.Code) > 256 {
				return agent.ToolResult{}, errors.New("LSP server returned an invalid diagnostic code")
			}
			code = string(item.Code)
		}
		value := presentedDiagnostic{
			StartLine:      item.Range.Start.Line + 1,
			StartCharacter: item.Range.Start.Character,
			EndLine:        item.Range.End.Line + 1,
			EndCharacter:   item.Range.End.Character,
			Severity:       severityName(item.Severity),
			Code:           code,
			Source:         shorten(item.Source, 256),
			Message:        shorten(item.Message, 4096),
		}
		presented = append(presented, value)
	}
	result := struct {
		Path        string                `json:"path"`
		Server      string                `json:"server"`
		Diagnostics []presentedDiagnostic `json:"diagnostics"`
		Truncated   bool                  `json:"truncated"`
	}{Path: path, Server: server, Diagnostics: presented, Truncated: truncated}
	return boundedToolResult(result, "diagnostic")
}

func formatCodeActions(root workspace.RootSet, path, server string, response json.RawMessage) (agent.ToolResult, error) {
	response = bytes.TrimSpace(response)
	if len(response) == 0 || string(response) == "null" {
		response = json.RawMessage("[]")
	}
	var rawActions []json.RawMessage
	if err := json.Unmarshal(response, &rawActions); err != nil {
		return agent.ToolResult{}, fmt.Errorf("decode code actions: %w", err)
	}
	actions := make([]presentedCodeAction, 0, min(len(rawActions), maxCodeActions))
	truncated := len(rawActions) > maxCodeActions
	remainingEdits := maxCodeActionEdits
	for _, raw := range rawActions {
		if len(actions) == maxCodeActions {
			truncated = true
			break
		}
		action, include, omitted, err := presentCodeAction(root, raw, &remainingEdits)
		if err != nil {
			return agent.ToolResult{}, err
		}
		if omitted {
			truncated = true
		}
		if !include {
			continue
		}
		candidate := append(actions, action)
		if _, err := boundedToolResult(struct {
			Path      string                `json:"path"`
			Server    string                `json:"server"`
			Actions   []presentedCodeAction `json:"actions"`
			Truncated bool                  `json:"truncated"`
		}{Path: path, Server: server, Actions: candidate, Truncated: true}, "code-action"); err != nil {
			truncated = true
			break
		}
		actions = candidate
	}
	return boundedToolResult(struct {
		Path      string                `json:"path"`
		Server    string                `json:"server"`
		Actions   []presentedCodeAction `json:"actions"`
		Truncated bool                  `json:"truncated"`
	}{Path: path, Server: server, Actions: actions, Truncated: truncated}, "code-action")
}

func presentCodeAction(root workspace.RootSet, raw json.RawMessage, remainingEdits *int) (presentedCodeAction, bool, bool, error) {
	var value struct {
		Title       string `json:"title"`
		Kind        string `json:"kind"`
		IsPreferred bool   `json:"isPreferred"`
		Disabled    *struct {
			Reason string `json:"reason"`
		} `json:"disabled"`
		Edit    json.RawMessage `json:"edit"`
		Command json.RawMessage `json:"command"`
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return presentedCodeAction{}, false, false, fmt.Errorf("decode code action: %w", err)
	}
	if strings.TrimSpace(value.Title) == "" {
		return presentedCodeAction{}, false, false, errors.New("LSP server returned a code action without a title")
	}
	action := presentedCodeAction{
		Title:          shorten(value.Title, maxCodeActionTitleBytes),
		Kind:           shorten(value.Kind, maxCodeActionKindBytes),
		Preferred:      value.IsPreferred,
		CommandOmitted: len(bytes.TrimSpace(value.Command)) != 0 && string(bytes.TrimSpace(value.Command)) != "null",
	}
	if value.Disabled != nil {
		action.DisabledReason = shorten(value.Disabled.Reason, maxCodeActionDisabledBytes)
	}
	if len(bytes.TrimSpace(value.Edit)) == 0 || string(bytes.TrimSpace(value.Edit)) == "null" {
		return action, true, false, nil
	}
	edits, complete, err := presentWorkspaceEdit(root, value.Edit)
	if err != nil {
		return presentedCodeAction{}, false, false, err
	}
	if !complete || len(edits) > *remainingEdits {
		return presentedCodeAction{}, false, true, nil
	}
	*remainingEdits -= len(edits)
	action.Edits = edits
	return action, true, false, nil
}

func presentWorkspaceEdit(root workspace.RootSet, raw json.RawMessage) ([]presentedTextEdit, bool, error) {
	var edit lspWorkspaceEdit
	if err := json.Unmarshal(raw, &edit); err != nil {
		return nil, false, fmt.Errorf("decode code-action workspace edit: %w", err)
	}
	if len(edit.Changes) > 0 && len(bytes.TrimSpace(edit.DocumentChanges)) > 0 && string(bytes.TrimSpace(edit.DocumentChanges)) != "null" {
		return nil, false, nil
	}
	if len(edit.Changes) > 0 {
		uris := make([]string, 0, len(edit.Changes))
		for uri := range edit.Changes {
			uris = append(uris, uri)
		}
		sort.Strings(uris)
		result := make([]presentedTextEdit, 0)
		for _, uri := range uris {
			path, found, err := presentEditableFileURI(root, uri)
			if err != nil {
				return nil, false, err
			}
			if !found {
				return nil, false, nil
			}
			edits, err := presentTextEdits(path, edit.Changes[uri])
			if err != nil {
				return nil, false, err
			}
			result = append(result, edits...)
		}
		return result, true, nil
	}
	if len(bytes.TrimSpace(edit.DocumentChanges)) == 0 || string(bytes.TrimSpace(edit.DocumentChanges)) == "null" {
		return nil, true, nil
	}
	var changes []json.RawMessage
	if err := json.Unmarshal(edit.DocumentChanges, &changes); err != nil {
		return nil, false, fmt.Errorf("decode code-action document changes: %w", err)
	}
	result := make([]presentedTextEdit, 0)
	for _, rawChange := range changes {
		var change struct {
			TextDocument struct {
				URI string `json:"uri"`
			} `json:"textDocument"`
			Edits []lspTextEdit `json:"edits"`
		}
		if err := json.Unmarshal(rawChange, &change); err != nil {
			return nil, false, fmt.Errorf("decode code-action document change: %w", err)
		}
		if change.TextDocument.URI == "" {
			return nil, false, nil
		}
		path, found, err := presentEditableFileURI(root, change.TextDocument.URI)
		if err != nil {
			return nil, false, err
		}
		if !found {
			return nil, false, nil
		}
		edits, err := presentTextEdits(path, change.Edits)
		if err != nil {
			return nil, false, err
		}
		result = append(result, edits...)
	}
	return result, true, nil
}

// formatFormatting presents one document-format result as a patch suggestion.
// Formatting edits are a single atomic proposal: if their count exceeds the
// shared edit bound, none are returned for the model to partially reproduce.
func formatFormatting(path, server string, response json.RawMessage) (agent.ToolResult, error) {
	response = bytes.TrimSpace(response)
	if len(response) == 0 || string(response) == "null" {
		response = json.RawMessage("[]")
	}
	var rawEdits []lspTextEdit
	if err := json.Unmarshal(response, &rawEdits); err != nil {
		return agent.ToolResult{}, fmt.Errorf("decode formatting edits: %w", err)
	}
	truncated := len(rawEdits) > maxCodeActionEdits
	edits := make([]presentedTextEdit, 0)
	if !truncated {
		var err error
		edits, err = presentTextEdits(path, rawEdits)
		if err != nil {
			return agent.ToolResult{}, err
		}
	}
	return boundedToolResult(struct {
		Path      string              `json:"path"`
		Server    string              `json:"server"`
		Operation lspOperation        `json:"operation"`
		Edits     []presentedTextEdit `json:"edits"`
		Truncated bool                `json:"truncated"`
	}{Path: path, Server: server, Operation: formatOperation, Edits: edits, Truncated: truncated}, "format")
}

// formatRename accepts only a complete, workspace-confined WorkspaceEdit. A
// rename can affect several files, so exposing a partial proposal would make
// the suggested change misleading; any unsafe or oversized edit is omitted.
func formatRename(root workspace.RootSet, path, server string, response json.RawMessage) (agent.ToolResult, error) {
	response = bytes.TrimSpace(response)
	if len(response) == 0 || string(response) == "null" {
		response = json.RawMessage("{}")
	}
	edits, complete, err := presentWorkspaceEdit(root, response)
	if err != nil {
		return agent.ToolResult{}, err
	}
	truncated := !complete || len(edits) > maxCodeActionEdits
	if truncated {
		edits = nil
	}
	return boundedToolResult(struct {
		Path      string              `json:"path"`
		Server    string              `json:"server"`
		Operation lspOperation        `json:"operation"`
		Edits     []presentedTextEdit `json:"edits"`
		Truncated bool                `json:"truncated"`
	}{Path: path, Server: server, Operation: renameOperation, Edits: edits, Truncated: truncated}, "rename")
}

func presentTextEdits(path string, edits []lspTextEdit) ([]presentedTextEdit, error) {
	result := make([]presentedTextEdit, 0, len(edits))
	for _, edit := range edits {
		if len(edit.NewText) > maxCodeActionNewTextBytes {
			return nil, errors.New("LSP server returned a code-action edit that exceeds 16 KiB")
		}
		rangeValue, err := presentRange(edit.Range)
		if err != nil {
			return nil, fmt.Errorf("LSP server returned an invalid code-action edit range: %w", err)
		}
		result = append(result, presentedTextEdit{Path: path, Range: rangeValue, NewText: edit.NewText})
	}
	return result, nil
}
