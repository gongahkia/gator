package lsp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/workspace"
)

func formatHover(path, server string, response json.RawMessage) (agent.ToolResult, error) {
	if string(response) == "null" {
		return boundedToolResult(struct {
			Path   string `json:"path"`
			Server string `json:"server"`
			Found  bool   `json:"found"`
		}{Path: path, Server: server, Found: false}, "hover")
	}
	var value struct {
		Contents json.RawMessage `json:"contents"`
		Range    *lspRange       `json:"range"`
	}
	if err := json.Unmarshal(response, &value); err != nil {
		return agent.ToolResult{}, fmt.Errorf("decode hover response: %w", err)
	}
	content, kind, err := hoverContent(value.Contents)
	if err != nil {
		return agent.ToolResult{}, err
	}
	result := struct {
		Path    string          `json:"path"`
		Server  string          `json:"server"`
		Found   bool            `json:"found"`
		Kind    string          `json:"kind,omitempty"`
		Content string          `json:"content,omitempty"`
		Range   *presentedRange `json:"range,omitempty"`
	}{Path: path, Server: server, Found: content != "", Kind: kind, Content: content}
	if value.Range != nil {
		presented, err := presentRange(*value.Range)
		if err != nil {
			return agent.ToolResult{}, fmt.Errorf("LSP server returned an invalid hover range: %w", err)
		}
		result.Range = &presented
	}
	return boundedToolResult(result, "hover")
}

func hoverContent(raw json.RawMessage) (string, string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", "", nil
	}
	if raw[0] == '"' {
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return "", "", err
		}
		return shorten(value, maxHoverBytes), "plaintext", nil
	}
	var markup struct {
		Kind  string `json:"kind"`
		Value string `json:"value"`
	}
	if raw[0] == '{' {
		if err := json.Unmarshal(raw, &markup); err != nil {
			return "", "", err
		}
		if markup.Value == "" {
			return "", "", nil
		}
		if markup.Kind != "" && markup.Kind != "plaintext" && markup.Kind != "markdown" {
			return "", "", errors.New("LSP server returned an unsupported hover content kind")
		}
		if markup.Kind == "" {
			markup.Kind = "plaintext"
		}
		return shorten(markup.Value, maxHoverBytes), markup.Kind, nil
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return "", "", errors.New("LSP server returned invalid hover content")
	}
	parts := make([]string, 0, len(items))
	kind := "plaintext"
	for _, item := range items {
		content, itemKind, err := hoverContent(item)
		if err != nil {
			return "", "", err
		}
		if itemKind == "markdown" {
			kind = "markdown"
		}
		if content != "" {
			parts = append(parts, content)
		}
	}
	return shorten(strings.Join(parts, "\n\n"), maxHoverBytes), kind, nil
}

func formatCompletions(path, server string, response json.RawMessage) (agent.ToolResult, error) {
	items, incomplete, err := decodeCompletions(response)
	if err != nil {
		return agent.ToolResult{}, err
	}
	presented := make([]presentedCompletion, 0, min(len(items), maxCompletions))
	truncated := false
	for _, raw := range items {
		if len(presented) == maxCompletions {
			truncated = true
			break
		}
		item, err := presentCompletion(raw)
		if err != nil {
			return agent.ToolResult{}, err
		}
		candidate := append(presented, item)
		if _, err := boundedToolResult(presentedCompletions{Path: path, Server: server, Incomplete: incomplete, Items: candidate, Truncated: truncated}, "completion"); err != nil {
			truncated = true
			break
		}
		presented = candidate
	}
	return boundedToolResult(presentedCompletions{Path: path, Server: server, Incomplete: incomplete, Items: presented, Truncated: truncated}, "completion")
}

func decodeCompletions(response json.RawMessage) ([]json.RawMessage, bool, error) {
	response = bytes.TrimSpace(response)
	if len(response) == 0 || string(response) == "null" {
		return nil, false, nil
	}
	if response[0] == '[' {
		var items []json.RawMessage
		if err := json.Unmarshal(response, &items); err != nil {
			return nil, false, fmt.Errorf("decode completion items: %w", err)
		}
		return items, false, nil
	}
	if response[0] != '{' {
		return nil, false, errors.New("LSP server returned an invalid completion response")
	}
	var list struct {
		IsIncomplete bool              `json:"isIncomplete"`
		Items        []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(response, &list); err != nil {
		return nil, false, fmt.Errorf("decode completion list: %w", err)
	}
	return list.Items, list.IsIncomplete, nil
}

func presentCompletion(raw json.RawMessage) (presentedCompletion, error) {
	var item lspCompletionItem
	if err := json.Unmarshal(raw, &item); err != nil {
		return presentedCompletion{}, fmt.Errorf("decode completion item: %w", err)
	}
	if strings.TrimSpace(item.Label) == "" || item.Kind < 0 || item.Kind > 25 {
		return presentedCompletion{}, errors.New("LSP server returned an invalid completion item")
	}
	documentation, kind, err := completionDocumentation(item.Documentation)
	if err != nil {
		return presentedCompletion{}, err
	}
	return presentedCompletion{
		Label:             shorten(item.Label, maxCompletionLabelBytes),
		Kind:              item.Kind,
		Detail:            shorten(item.Detail, maxCompletionDetailBytes),
		DocumentationKind: kind,
		Documentation:     documentation,
		InsertText:        shorten(item.InsertText, maxCompletionInsertTextBytes),
	}, nil
}

func completionDocumentation(raw json.RawMessage) (string, string, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return "", "", nil
	}
	if raw[0] == '"' {
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return "", "", fmt.Errorf("decode completion documentation: %w", err)
		}
		return shorten(value, maxCompletionDocumentationBytes), "plaintext", nil
	}
	if raw[0] != '{' {
		return "", "", errors.New("LSP server returned invalid completion documentation")
	}
	var markup struct {
		Kind  string `json:"kind"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal(raw, &markup); err != nil {
		return "", "", fmt.Errorf("decode completion documentation: %w", err)
	}
	if markup.Kind != "plaintext" && markup.Kind != "markdown" {
		return "", "", errors.New("LSP server returned an unsupported completion documentation kind")
	}
	return shorten(markup.Value, maxCompletionDocumentationBytes), markup.Kind, nil
}

func formatLocations(root workspace.RootSet, path, server string, operation lspOperation, response json.RawMessage) (agent.ToolResult, error) {
	locations, err := decodeLocations(response)
	if err != nil {
		return agent.ToolResult{}, err
	}
	presented := make([]presentedLocation, 0, min(len(locations), maxLocations))
	truncated := false
	for _, location := range locations {
		if len(presented) == maxLocations {
			truncated = true
			break
		}
		value, found, err := presentLocation(root, location)
		if err != nil {
			return agent.ToolResult{}, err
		}
		if !found {
			truncated = true
			continue
		}
		presented = append(presented, value)
	}
	return boundedToolResult(struct {
		Path      string              `json:"path"`
		Server    string              `json:"server"`
		Operation lspOperation        `json:"operation"`
		Locations []presentedLocation `json:"locations"`
		Truncated bool                `json:"truncated"`
	}{Path: path, Server: server, Operation: operation, Locations: presented, Truncated: truncated}, string(operation))
}

func decodeLocations(response json.RawMessage) ([]lspLocation, error) {
	if len(response) == 0 || string(response) == "null" {
		return nil, nil
	}
	if response[0] == '{' {
		var location lspLocation
		if err := json.Unmarshal(response, &location); err != nil {
			return nil, fmt.Errorf("decode LSP location: %w", err)
		}
		return []lspLocation{location}, nil
	}
	var locations []lspLocation
	if err := json.Unmarshal(response, &locations); err != nil {
		return nil, fmt.Errorf("decode LSP locations: %w", err)
	}
	return locations, nil
}

func presentLocation(root workspace.RootSet, location lspLocation) (presentedLocation, bool, error) {
	uri, sourceRange := location.URI, location.Range
	if location.TargetURI != "" {
		uri, sourceRange = location.TargetURI, location.TargetRange
		if location.TargetSelectionRange != nil {
			sourceRange = *location.TargetSelectionRange
		}
	}
	if uri == "" {
		return presentedLocation{}, false, errors.New("LSP server returned a location without a URI")
	}
	path, found, err := presentFileURI(root, uri)
	if err != nil || !found {
		return presentedLocation{}, found, err
	}
	presented, err := presentRange(sourceRange)
	if err != nil {
		return presentedLocation{}, false, fmt.Errorf("LSP server returned an invalid location range: %w", err)
	}
	return presentedLocation{Path: path, Range: presented}, true, nil
}

func presentFileURI(root workspace.RootSet, uri string) (string, bool, error) {
	parsed, err := url.Parse(uri)
	if err != nil || parsed.Scheme != "file" || (parsed.Host != "" && parsed.Host != "localhost") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", false, errors.New("LSP server returned an invalid location URI")
	}
	candidate := filepath.Clean(filepath.FromSlash(parsed.Path))
	resolved, err := root.ResolveReturnedFile(candidate)
	if err != nil {
		return "", false, nil
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() {
		return "", false, nil
	}
	return root.DisplayPath(resolved), true, nil
}

// presentEditableFileURI keeps LSP edit suggestions inside the isolated
// worktree even when the same server indexes ACP read-only external roots.
func presentEditableFileURI(root workspace.RootSet, uri string) (string, bool, error) {
	path, found, err := presentFileURI(root, uri)
	if err != nil || !found {
		return path, found, err
	}
	if filepath.IsAbs(path) {
		return "", false, nil
	}
	return path, true, nil
}

func presentRange(value lspRange) (presentedRange, error) {
	if value.Start.Line < 0 || value.Start.Character < 0 || value.End.Line < 0 || value.End.Character < 0 || value.End.Line < value.Start.Line || (value.End.Line == value.Start.Line && value.End.Character < value.Start.Character) {
		return presentedRange{}, errors.New("negative or reversed position")
	}
	return presentedRange{StartLine: value.Start.Line + 1, StartCharacter: value.Start.Character, EndLine: value.End.Line + 1, EndCharacter: value.End.Character}, nil
}

func formatWorkspaceSymbols(root workspace.RootSet, query, server string, response json.RawMessage) (agent.ToolResult, error) {
	if len(response) == 0 || string(response) == "null" {
		response = json.RawMessage("[]")
	}
	var rawSymbols []json.RawMessage
	if err := json.Unmarshal(response, &rawSymbols); err != nil {
		return agent.ToolResult{}, fmt.Errorf("decode workspace symbols: %w", err)
	}
	presented := make([]presentedWorkspaceSymbol, 0, min(len(rawSymbols), maxSymbols))
	truncated := false
	for _, raw := range rawSymbols {
		if len(presented) == maxSymbols {
			truncated = true
			break
		}
		var symbol struct {
			Name          string `json:"name"`
			Kind          int    `json:"kind"`
			ContainerName string `json:"containerName"`
			Location      struct {
				URI   string    `json:"uri"`
				Range *lspRange `json:"range"`
			} `json:"location"`
		}
		if err := json.Unmarshal(raw, &symbol); err != nil {
			return agent.ToolResult{}, fmt.Errorf("decode workspace symbol: %w", err)
		}
		if strings.TrimSpace(symbol.Name) == "" || symbol.Kind < 1 || symbol.Kind > 255 || symbol.Location.URI == "" {
			return agent.ToolResult{}, errors.New("LSP server returned an invalid workspace symbol")
		}
		path, found, err := presentFileURI(root, symbol.Location.URI)
		if err != nil {
			return agent.ToolResult{}, err
		}
		if !found {
			truncated = true
			continue
		}
		value := presentedWorkspaceSymbol{Path: path, Name: shorten(symbol.Name, 512), Detail: shorten(symbol.ContainerName, 1024), Kind: symbol.Kind}
		if symbol.Location.Range != nil {
			presentedRange, err := presentRange(*symbol.Location.Range)
			if err != nil {
				return agent.ToolResult{}, fmt.Errorf("LSP server returned an invalid workspace-symbol range: %w", err)
			}
			value.Range = &presentedRange
		}
		presented = append(presented, value)
	}
	return boundedToolResult(struct {
		Query     string                     `json:"query"`
		Server    string                     `json:"server"`
		Symbols   []presentedWorkspaceSymbol `json:"symbols"`
		Truncated bool                       `json:"truncated"`
	}{Query: query, Server: server, Symbols: presented, Truncated: truncated}, "workspace-symbol")
}

func formatDocumentSymbols(root workspace.RootSet, path, server string, response json.RawMessage) (agent.ToolResult, error) {
	if len(response) == 0 || string(response) == "null" {
		response = json.RawMessage("[]")
	}
	var rawSymbols []json.RawMessage
	if err := json.Unmarshal(response, &rawSymbols); err != nil {
		return agent.ToolResult{}, fmt.Errorf("decode document symbols: %w", err)
	}
	remaining := maxSymbols
	truncated := false
	presented := make([]presentedSymbol, 0, min(len(rawSymbols), maxSymbols))
	for _, raw := range rawSymbols {
		var probe struct {
			Location json.RawMessage `json:"location"`
		}
		if err := json.Unmarshal(raw, &probe); err != nil {
			return agent.ToolResult{}, fmt.Errorf("decode document symbol: %w", err)
		}
		if len(probe.Location) != 0 && string(probe.Location) != "null" {
			value, included, err := presentSymbolInformation(root, raw, &remaining, &truncated)
			if err != nil {
				return agent.ToolResult{}, err
			}
			if included {
				presented = append(presented, value)
			}
			continue
		}
		var symbol lspDocumentSymbol
		if err := json.Unmarshal(raw, &symbol); err != nil {
			return agent.ToolResult{}, fmt.Errorf("decode document symbol: %w", err)
		}
		value, included, err := presentSymbol(symbol, &remaining, &truncated)
		if err != nil {
			return agent.ToolResult{}, err
		}
		if included {
			presented = append(presented, value)
		}
	}
	return boundedToolResult(struct {
		Path      string            `json:"path"`
		Server    string            `json:"server"`
		Symbols   []presentedSymbol `json:"symbols"`
		Truncated bool              `json:"truncated"`
	}{Path: path, Server: server, Symbols: presented, Truncated: truncated}, "document-symbol")
}

func presentSymbol(symbol lspDocumentSymbol, remaining *int, truncated *bool) (presentedSymbol, bool, error) {
	if *remaining == 0 {
		*truncated = true
		return presentedSymbol{}, false, nil
	}
	if strings.TrimSpace(symbol.Name) == "" || symbol.Kind < 1 || symbol.Kind > 255 {
		return presentedSymbol{}, false, errors.New("LSP server returned an invalid document symbol")
	}
	rangeValue, err := presentRange(symbol.Range)
	if err != nil {
		return presentedSymbol{}, false, fmt.Errorf("LSP server returned an invalid document-symbol range: %w", err)
	}
	selection, err := presentRange(symbol.SelectionRange)
	if err != nil {
		return presentedSymbol{}, false, fmt.Errorf("LSP server returned an invalid document-symbol selection range: %w", err)
	}
	(*remaining)--
	value := presentedSymbol{Name: shorten(symbol.Name, 512), Detail: shorten(symbol.Detail, 1024), Kind: symbol.Kind, Range: rangeValue, SelectionRange: selection}
	for _, child := range symbol.Children {
		presentedChild, included, err := presentSymbol(child, remaining, truncated)
		if err != nil {
			return presentedSymbol{}, false, err
		}
		if included {
			value.Children = append(value.Children, presentedChild)
		}
	}
	return value, true, nil
}

func presentSymbolInformation(root workspace.RootSet, raw json.RawMessage, remaining *int, truncated *bool) (presentedSymbol, bool, error) {
	if *remaining == 0 {
		*truncated = true
		return presentedSymbol{}, false, nil
	}
	var symbol lspSymbolInformation
	if err := json.Unmarshal(raw, &symbol); err != nil {
		return presentedSymbol{}, false, fmt.Errorf("decode symbol information: %w", err)
	}
	if strings.TrimSpace(symbol.Name) == "" || symbol.Kind < 1 || symbol.Kind > 255 {
		return presentedSymbol{}, false, errors.New("LSP server returned an invalid symbol information")
	}
	location, found, err := presentLocation(root, symbol.Location)
	if err != nil {
		return presentedSymbol{}, false, err
	}
	if !found {
		*truncated = true
		return presentedSymbol{}, false, nil
	}
	(*remaining)--
	return presentedSymbol{Path: location.Path, Name: shorten(symbol.Name, 512), Detail: shorten(symbol.ContainerName, 1024), Kind: symbol.Kind, Range: location.Range, SelectionRange: location.Range}, true, nil
}

func boundedToolResult(value any, kind string) (agent.ToolResult, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return agent.ToolResult{}, err
	}
	if len(encoded) > maxToolOutput {
		return agent.ToolResult{}, fmt.Errorf("LSP %s response exceeds 64 KiB", kind)
	}
	return agent.ToolResult{Content: string(encoded)}, nil
}

func severityName(value int) string {
	switch value {
	case 1:
		return "error"
	case 2:
		return "warning"
	case 3:
		return "information"
	case 4:
		return "hint"
	default:
		return ""
	}
}

func shorten(value string, maximum int) string {
	if len(value) <= maximum {
		return value
	}
	return value[:maximum]
}

func decodeArguments(arguments json.RawMessage, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(arguments))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	return requireEOF(decoder)
}
