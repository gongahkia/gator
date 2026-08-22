// Package lsp exposes explicitly trusted local Language Server Protocol
// diagnostic servers as bounded Gator tools.
package lsp

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/sandbox"
	"github.com/gongahkia/gator/internal/workspace"
)

const (
	manifestPath    = ".gator/lsp.json"
	manifestVersion = 1
	maxManifest     = 64 * 1024
	maxExecutable   = 128 * 1024 * 1024
	maxServers      = 32
	maxFrame        = 256 * 1024
	maxDiagnostics  = 128
	maxLocations    = 128
	maxSymbols      = 128
	maxHoverBytes   = 32 * 1024
	maxToolOutput   = 64 * 1024
)

var namePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,47}$`)

// Trust is a user-owned record pinning one project LSP configuration bundle.
type Trust struct {
	Repository string `json:"repository"`
	Hash       string `json:"hash"`
}

type manifest struct {
	Version int      `json:"version"`
	Servers []server `json:"servers"`
}

type server struct {
	Name     string   `json:"name"`
	Command  []string `json:"command"`
	Language string   `json:"language"`
	Network  string   `json:"network,omitempty"`
}

// Set is the validated project LSP configuration. It does not start a server
// until a model requests diagnostics and the developer approves that request.
type Set struct {
	configuredHash string
	trusted        bool
	root           workspace.Root
	servers        []server
}

func (s Set) Configured() bool { return s.configuredHash != "" }
func (s Set) Trusted() bool    { return s.trusted }
func (s Set) Hash() string     { return s.configuredHash }

// Load validates an LSP manifest. A valid but untrusted manifest is inert.
func Load(worktreePath, trustedHash string) (Set, error) {
	root, err := workspace.Open(worktreePath)
	if err != nil {
		return Set{}, err
	}
	document, digest, err := loadManifest(root)
	if err != nil {
		return Set{}, err
	}
	if digest == "" {
		return Set{}, nil
	}
	set := Set{configuredHash: digest, root: root}
	if trustedHash == "" || trustedHash != digest {
		return set, nil
	}
	set.trusted = true
	set.servers = append([]server(nil), document.Servers...)
	return set, nil
}

// BundleHash validates and hashes the manifest and every configured server
// executable. Changing either disables the configuration until it is reviewed
// and trusted again.
func BundleHash(repository string) (string, error) {
	root, err := workspace.Open(repository)
	if err != nil {
		return "", err
	}
	_, digest, err := loadManifest(root)
	return digest, err
}

// CanonicalRepository returns a canonical repository path for trust records.
func CanonicalRepository(repository string) (string, error) {
	root, err := workspace.Open(repository)
	if err != nil {
		return "", err
	}
	return root.Path(), nil
}

// Tools returns read-only inspection tools for every trusted server. The
// approval callback is invoked before starting a server because an LSP
// executable is a process-capability boundary even though its results are
// read-only output.
func (s Set) Tools(approve func(context.Context, string, string, string) error) []agent.Tool {
	if !s.trusted {
		return nil
	}
	result := make([]agent.Tool, 0, len(s.servers)*len(lspOperations))
	for _, specification := range s.servers {
		for _, operation := range lspOperations {
			result = append(result, Tool{root: s.root, specification: specification, operation: operation, approve: approve, connect: connect})
		}
	}
	return result
}

func loadManifest(root workspace.Root) (manifest, string, error) {
	contents, err := root.ReadRegularFile(manifestPath, maxManifest)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return manifest{}, "", nil
		}
		return manifest{}, "", fmt.Errorf("read %s: %w", manifestPath, err)
	}
	var document manifest
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return manifest{}, "", fmt.Errorf("decode %s: %w", manifestPath, err)
	}
	if err := requireEOF(decoder); err != nil {
		return manifest{}, "", fmt.Errorf("decode %s: %w", manifestPath, err)
	}
	if document.Version != manifestVersion || len(document.Servers) > maxServers {
		return manifest{}, "", errors.New("LSP manifest has an unsupported version or too many servers")
	}
	digest := sha256.New()
	_, _ = digest.Write([]byte(manifestPath + "\x00"))
	_, _ = digest.Write(contents)
	seen := make(map[string]struct{}, len(document.Servers))
	local := make(map[string]string)
	for index, specification := range document.Servers {
		if err := validateServer(specification); err != nil {
			return manifest{}, "", fmt.Errorf("LSP server %d: %w", index+1, err)
		}
		if _, duplicate := seen[specification.Name]; duplicate {
			return manifest{}, "", fmt.Errorf("LSP server name %q is repeated", specification.Name)
		}
		seen[specification.Name] = struct{}{}
		path := filepath.ToSlash(specification.Command[0])
		if _, found := local[path]; !found {
			executable, err := executableHash(root, path)
			if err != nil {
				return manifest{}, "", fmt.Errorf("LSP server %q: %w", specification.Name, err)
			}
			local[path] = executable
		}
	}
	paths := make([]string, 0, len(local))
	for path := range local {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		_, _ = digest.Write([]byte("\x00" + path + "\x00"))
		_, _ = digest.Write([]byte(local[path]))
	}
	return document, hex.EncodeToString(digest.Sum(nil)), nil
}

func validateServer(server server) error {
	if !namePattern.MatchString(server.Name) {
		return fmt.Errorf("invalid server name %q", server.Name)
	}
	if !namePattern.MatchString(server.Language) {
		return fmt.Errorf("invalid LSP language %q", server.Language)
	}
	if len(server.Command) == 0 || len(server.Command) > 32 || filepath.IsAbs(server.Command[0]) || !safeRelative(server.Command[0]) {
		return errors.New("LSP server requires a repository-relative command")
	}
	for _, value := range server.Command {
		if len(value) > 4096 || strings.ContainsRune(value, '\x00') {
			return errors.New("LSP command argument is invalid")
		}
	}
	if server.Network != "" && server.Network != "allow" && server.Network != "deny" {
		return fmt.Errorf("unsupported LSP network mode %q", server.Network)
	}
	return nil
}

func safeRelative(value string) bool {
	clean := filepath.Clean(value)
	return clean != "." && clean != ".." && !strings.HasPrefix(clean, ".."+string(filepath.Separator))
}

func executableHash(root workspace.Root, relative string) (string, error) {
	resolved, err := root.ResolveFile(filepath.FromSlash(relative))
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(resolved)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
		return "", errors.New("LSP executable must be an executable regular file")
	}
	return root.SHA256RegularFile(filepath.FromSlash(relative), maxExecutable)
}

type lspOperation string

const (
	diagnosticsOperation     lspOperation = "diagnostics"
	hoverOperation           lspOperation = "hover"
	definitionOperation      lspOperation = "definition"
	referencesOperation      lspOperation = "references"
	documentSymbolsOperation lspOperation = "document_symbols"
)

var lspOperations = []lspOperation{
	diagnosticsOperation,
	hoverOperation,
	definitionOperation,
	referencesOperation,
	documentSymbolsOperation,
}

// Tool adapts one trusted, read-only LSP operation to the agent tool contract.
type Tool struct {
	root          workspace.Root
	specification server
	operation     lspOperation
	approve       func(context.Context, string, string, string) error
	connect       func(context.Context, workspace.Root, server) (client, error)
}

func (t Tool) Definition() agent.ToolDefinition {
	prefix := "lsp_" + strings.ReplaceAll(t.specification.Name, "-", "_") + "_"
	definition := agent.ToolDefinition{
		Name:        prefix + string(t.operation),
		Description: t.description(),
	}
	switch t.operation {
	case diagnosticsOperation, documentSymbolsOperation:
		definition.Parameters = json.RawMessage(`{"type":"object","additionalProperties":false,"required":["path"],"properties":{"path":{"type":"string","description":"Workspace-relative source-file path"}}}`)
	case referencesOperation:
		definition.Parameters = json.RawMessage(`{"type":"object","additionalProperties":false,"required":["path","line","character"],"properties":{"path":{"type":"string","description":"Workspace-relative source-file path"},"line":{"type":"integer","minimum":1,"description":"One-based source line"},"character":{"type":"integer","minimum":0,"description":"Zero-based UTF-16 character offset"},"include_declaration":{"type":"boolean","description":"Include the symbol declaration in results; defaults to false"}}}`)
	default:
		definition.Parameters = json.RawMessage(`{"type":"object","additionalProperties":false,"required":["path","line","character"],"properties":{"path":{"type":"string","description":"Workspace-relative source-file path"},"line":{"type":"integer","minimum":1,"description":"One-based source line"},"character":{"type":"integer","minimum":0,"description":"Zero-based UTF-16 character offset"}}}`)
	}
	return definition
}

func (t Tool) description() string {
	server := t.specification.Name + " (" + t.specification.Language + ")"
	switch t.operation {
	case diagnosticsOperation:
		return "Read-only LSP pull diagnostics using " + server + ". The path must be a workspace-relative source file."
	case hoverOperation:
		return "Read-only LSP hover information using " + server + ". line is one-based; character is a zero-based UTF-16 offset."
	case definitionOperation:
		return "Read-only LSP go-to-definition lookup using " + server + ". line is one-based; character is a zero-based UTF-16 offset. Only workspace locations are returned."
	case referencesOperation:
		return "Read-only LSP find-references lookup using " + server + ". line is one-based; character is a zero-based UTF-16 offset. Only workspace locations are returned."
	case documentSymbolsOperation:
		return "Read-only LSP document-symbol listing using " + server + ". The path must be a workspace-relative source file."
	default:
		return "Read-only LSP inspection using " + server + "."
	}
}

type toolParameters struct {
	Path               string `json:"path"`
	Line               int    `json:"line"`
	Character          int    `json:"character"`
	IncludeDeclaration bool   `json:"include_declaration"`
}

func (t Tool) Execute(ctx context.Context, arguments json.RawMessage) (agent.ToolResult, error) {
	var params toolParameters
	if err := decodeArguments(arguments, &params); err != nil {
		return agent.ToolResult{}, err
	}
	resolved, err := t.root.ResolveFile(params.Path)
	if err != nil {
		return agent.ToolResult{}, err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return agent.ToolResult{}, fmt.Errorf("stat LSP %s path: %w", t.operation, err)
	}
	if !info.Mode().IsRegular() {
		return agent.ToolResult{}, fmt.Errorf("LSP %s requires a regular source file", t.operation)
	}
	if t.requiresPosition() && (params.Line < 1 || params.Character < 0) {
		return agent.ToolResult{}, errors.New("LSP position requires a one-based line and a zero-based UTF-16 character offset")
	}
	if t.approve == nil {
		return agent.ToolResult{}, fmt.Errorf("LSP %s requires developer approval", t.operation)
	}
	if err := t.approve(ctx, t.specification.Name, string(t.operation), params.Path); err != nil {
		return agent.ToolResult{}, err
	}
	if t.connect == nil {
		return agent.ToolResult{}, errors.New("LSP connection is not configured")
	}
	connection, err := t.connect(ctx, t.root, t.specification)
	if err != nil {
		return agent.ToolResult{}, fmt.Errorf("start LSP server %q: %w", t.specification.Name, err)
	}
	defer connection.Close()
	if !connection.Supports(t.operation) {
		return agent.ToolResult{}, fmt.Errorf("LSP server %q does not support %s", t.specification.Name, t.operation)
	}
	method, request := t.request(resolved, params)
	response, err := connection.Request(ctx, method, request)
	if err != nil {
		return agent.ToolResult{}, fmt.Errorf("request %s from LSP server %q: %w", t.operation, t.specification.Name, err)
	}
	return t.format(params.Path, response)
}

func (t Tool) requiresPosition() bool {
	return t.operation == hoverOperation || t.operation == definitionOperation || t.operation == referencesOperation
}

func (t Tool) request(path string, params toolParameters) (string, any) {
	document := map[string]string{"uri": fileURI(path)}
	position := map[string]int{"line": params.Line - 1, "character": params.Character}
	switch t.operation {
	case diagnosticsOperation:
		return "textDocument/diagnostic", map[string]any{"textDocument": document}
	case hoverOperation:
		return "textDocument/hover", map[string]any{"textDocument": document, "position": position}
	case definitionOperation:
		return "textDocument/definition", map[string]any{"textDocument": document, "position": position}
	case referencesOperation:
		return "textDocument/references", map[string]any{"textDocument": document, "position": position, "context": map[string]bool{"includeDeclaration": params.IncludeDeclaration}}
	case documentSymbolsOperation:
		return "textDocument/documentSymbol", map[string]any{"textDocument": document}
	default:
		return "", nil
	}
}

func (t Tool) format(path string, response json.RawMessage) (agent.ToolResult, error) {
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
	case definitionOperation, referencesOperation:
		return formatLocations(t.root, path, t.specification.Name, t.operation, response)
	case documentSymbolsOperation:
		return formatDocumentSymbols(t.root, path, t.specification.Name, response)
	default:
		return agent.ToolResult{}, fmt.Errorf("unsupported LSP operation %q", t.operation)
	}
}

type client interface {
	Request(context.Context, string, any) (json.RawMessage, error)
	Supports(lspOperation) bool
	Close() error
}

// Diagnostic is the LSP diagnostic subset Gator accepts from a local server.
// LSP ranges are zero-based UTF-16 positions; Tool output deliberately
// converts lines to one-based values for a terminal user.
type Diagnostic struct {
	Range struct {
		Start Position `json:"start"`
		End   Position `json:"end"`
	} `json:"range"`
	Severity int             `json:"severity,omitempty"`
	Code     json.RawMessage `json:"code,omitempty"`
	Source   string          `json:"source,omitempty"`
	Message  string          `json:"message"`
}

type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type presentedDiagnostic struct {
	StartLine      int    `json:"start_line"`
	StartCharacter int    `json:"start_character"`
	EndLine        int    `json:"end_line"`
	EndCharacter   int    `json:"end_character"`
	Severity       string `json:"severity,omitempty"`
	Code           string `json:"code,omitempty"`
	Source         string `json:"source,omitempty"`
	Message        string `json:"message"`
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

type lspRange struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

type lspLocation struct {
	URI                  string    `json:"uri"`
	Range                lspRange  `json:"range"`
	TargetURI            string    `json:"targetUri"`
	TargetRange          lspRange  `json:"targetRange"`
	TargetSelectionRange *lspRange `json:"targetSelectionRange"`
}

type presentedRange struct {
	StartLine      int `json:"start_line"`
	StartCharacter int `json:"start_character"`
	EndLine        int `json:"end_line"`
	EndCharacter   int `json:"end_character"`
}

type presentedLocation struct {
	Path  string         `json:"path"`
	Range presentedRange `json:"range"`
}

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

func formatLocations(root workspace.Root, path, server string, operation lspOperation, response json.RawMessage) (agent.ToolResult, error) {
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

func presentLocation(root workspace.Root, location lspLocation) (presentedLocation, bool, error) {
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
	parsed, err := url.Parse(uri)
	if err != nil || parsed.Scheme != "file" || (parsed.Host != "" && parsed.Host != "localhost") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return presentedLocation{}, false, errors.New("LSP server returned an invalid location URI")
	}
	candidate := filepath.Clean(filepath.FromSlash(parsed.Path))
	relative, err := filepath.Rel(root.Path(), candidate)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return presentedLocation{}, false, nil
	}
	resolved, err := root.ResolveFile(relative)
	if err != nil {
		return presentedLocation{}, false, nil
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() {
		return presentedLocation{}, false, nil
	}
	returnedPath, err := filepath.Rel(root.Path(), resolved)
	if err != nil {
		return presentedLocation{}, false, nil
	}
	presented, err := presentRange(sourceRange)
	if err != nil {
		return presentedLocation{}, false, fmt.Errorf("LSP server returned an invalid location range: %w", err)
	}
	return presentedLocation{Path: filepath.ToSlash(returnedPath), Range: presented}, true, nil
}

func presentRange(value lspRange) (presentedRange, error) {
	if value.Start.Line < 0 || value.Start.Character < 0 || value.End.Line < 0 || value.End.Character < 0 || value.End.Line < value.Start.Line || (value.End.Line == value.Start.Line && value.End.Character < value.Start.Character) {
		return presentedRange{}, errors.New("negative or reversed position")
	}
	return presentedRange{StartLine: value.Start.Line + 1, StartCharacter: value.Start.Character, EndLine: value.End.Line + 1, EndCharacter: value.End.Character}, nil
}

type lspDocumentSymbol struct {
	Name           string              `json:"name"`
	Detail         string              `json:"detail"`
	Kind           int                 `json:"kind"`
	Range          lspRange            `json:"range"`
	SelectionRange lspRange            `json:"selectionRange"`
	Children       []lspDocumentSymbol `json:"children"`
}

type lspSymbolInformation struct {
	Name          string      `json:"name"`
	Kind          int         `json:"kind"`
	Location      lspLocation `json:"location"`
	ContainerName string      `json:"containerName"`
}

type presentedSymbol struct {
	Path           string            `json:"path,omitempty"`
	Name           string            `json:"name"`
	Detail         string            `json:"detail,omitempty"`
	Kind           int               `json:"kind"`
	Range          presentedRange    `json:"range"`
	SelectionRange presentedRange    `json:"selection_range"`
	Children       []presentedSymbol `json:"children,omitempty"`
}

func formatDocumentSymbols(root workspace.Root, path, server string, response json.RawMessage) (agent.ToolResult, error) {
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

func presentSymbolInformation(root workspace.Root, raw json.RawMessage, remaining *int, truncated *bool) (presentedSymbol, bool, error) {
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

type nativeClient struct {
	input   io.WriteCloser
	output  *bufio.Reader
	command interface{ Wait() error }
	kill    func() error
	cleanup func()
	next    int64
	support map[lspOperation]bool
}

func connect(ctx context.Context, root workspace.Root, specification server) (client, error) {
	policy := sandbox.DefaultPolicy()
	if specification.Network == "allow" {
		policy.Network = sandbox.AllowNetwork
	}
	argv := append([]string{filepath.Join(root.Path(), filepath.FromSlash(specification.Command[0]))}, specification.Command[1:]...)
	prepared, err := sandbox.Prepare(ctx, sandbox.Request{Dir: root.Path(), Argv: argv, Policy: policy})
	if err != nil {
		return nil, err
	}
	input, err := prepared.Command.StdinPipe()
	if err != nil {
		prepared.Cleanup()
		return nil, err
	}
	output, err := prepared.Command.StdoutPipe()
	if err != nil {
		prepared.Cleanup()
		return nil, err
	}
	if err := prepared.Command.Start(); err != nil {
		prepared.Cleanup()
		return nil, err
	}
	client := &nativeClient{input: input, output: bufio.NewReaderSize(output, maxFrame), command: prepared.Command, kill: prepared.Command.Process.Kill, cleanup: prepared.Cleanup}
	if err := client.initialize(ctx, root.Path()); err != nil {
		_ = client.Close()
		return nil, err
	}
	return client, nil
}

func (c *nativeClient) initialize(ctx context.Context, root string) error {
	uri := fileURI(root)
	result, err := c.call(ctx, "initialize", map[string]any{
		"processId":        os.Getpid(),
		"clientInfo":       map[string]string{"name": "gator", "version": "dev"},
		"rootUri":          uri,
		"workspaceFolders": []map[string]string{{"uri": uri, "name": filepath.Base(root)}},
		"capabilities": map[string]any{
			"workspace": map[string]any{"configuration": true, "workspaceFolders": true},
			"textDocument": map[string]any{
				"diagnostic":     map[string]any{"dynamicRegistration": false},
				"hover":          map[string]any{"dynamicRegistration": false, "contentFormat": []string{"plaintext", "markdown"}},
				"definition":     map[string]any{"dynamicRegistration": false},
				"references":     map[string]any{"dynamicRegistration": false},
				"documentSymbol": map[string]any{"hierarchicalDocumentSymbolSupport": true},
			},
		},
	})
	if err != nil {
		return err
	}
	var response struct {
		Capabilities struct {
			DiagnosticProvider     json.RawMessage `json:"diagnosticProvider"`
			HoverProvider          json.RawMessage `json:"hoverProvider"`
			DefinitionProvider     json.RawMessage `json:"definitionProvider"`
			ReferencesProvider     json.RawMessage `json:"referencesProvider"`
			DocumentSymbolProvider json.RawMessage `json:"documentSymbolProvider"`
		} `json:"capabilities"`
	}
	if err := json.Unmarshal(result, &response); err != nil {
		return fmt.Errorf("decode initialize response: %w", err)
	}
	c.support = map[lspOperation]bool{
		diagnosticsOperation:     capabilityEnabled(response.Capabilities.DiagnosticProvider),
		hoverOperation:           capabilityEnabled(response.Capabilities.HoverProvider),
		definitionOperation:      capabilityEnabled(response.Capabilities.DefinitionProvider),
		referencesOperation:      capabilityEnabled(response.Capabilities.ReferencesProvider),
		documentSymbolsOperation: capabilityEnabled(response.Capabilities.DocumentSymbolProvider),
	}
	return c.notify("initialized", map[string]any{})
}

func capabilityEnabled(value json.RawMessage) bool {
	return len(value) != 0 && string(value) != "false" && string(value) != "null"
}

func (c *nativeClient) Supports(operation lspOperation) bool {
	return c.support[operation]
}

func (c *nativeClient) Request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	return c.call(ctx, method, params)
}

func (c *nativeClient) Close() error {
	shutdown := make(chan error, 1)
	go func() {
		_, err := c.call(context.Background(), "shutdown", nil)
		shutdown <- err
	}()
	var shutdownErr error
	select {
	case shutdownErr = <-shutdown:
		if shutdownErr == nil {
			_ = c.notify("exit", nil)
		}
	case <-time.After(2 * time.Second):
		shutdownErr = errors.New("LSP server did not acknowledge shutdown within 2 seconds")
		if c.kill != nil {
			_ = c.kill()
		}
	}
	_ = c.input.Close()
	err := c.command.Wait()
	c.cleanup()
	if shutdownErr != nil {
		return shutdownErr
	}
	return err
}

func (c *nativeClient) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	c.next++
	id := c.next
	if err := c.write(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
		return nil, err
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		frame, err := readFrame(c.output)
		if err != nil {
			return nil, err
		}
		var message struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(frame, &message); err != nil {
			return nil, fmt.Errorf("decode LSP message: %w", err)
		}
		if message.Method != "" {
			if len(message.ID) != 0 && string(message.ID) != "null" {
				if err := c.respond(message.ID, message.Method); err != nil {
					return nil, err
				}
			}
			continue
		}
		var responseID int64
		if err := json.Unmarshal(message.ID, &responseID); err != nil || responseID != id {
			continue
		}
		if message.Error != nil {
			return nil, fmt.Errorf("LSP RPC error %d: %s", message.Error.Code, message.Error.Message)
		}
		if len(message.Result) == 0 {
			return nil, errors.New("LSP RPC response has no result")
		}
		return message.Result, nil
	}
}

func (c *nativeClient) respond(id json.RawMessage, method string) error {
	result := any(nil)
	if method == "workspace/configuration" {
		result = []any{}
	}
	return c.write(map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(id), "result": result})
}

func (c *nativeClient) notify(method string, params any) error {
	return c.write(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

func (c *nativeClient) write(value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(payload) > maxFrame {
		return errors.New("LSP message exceeds 256 KiB")
	}
	_, err = fmt.Fprintf(c.input, "Content-Length: %d\r\n\r\n%s", len(payload), payload)
	return err
}

func readFrame(reader *bufio.Reader) ([]byte, error) {
	length := -1
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		if len(line) > 8192 {
			return nil, errors.New("LSP header exceeds 8 KiB")
		}
		line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		if line == "" {
			break
		}
		name, value, found := strings.Cut(line, ":")
		if !found {
			return nil, errors.New("invalid LSP header")
		}
		if strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			parsed, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil || parsed < 1 || parsed > maxFrame {
				return nil, errors.New("invalid LSP Content-Length")
			}
			length = parsed
		}
	}
	if length < 0 {
		return nil, errors.New("LSP message has no Content-Length")
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func fileURI(path string) string {
	return (&url.URL{Scheme: "file", Path: path}).String()
}

func requireEOF(decoder *json.Decoder) error {
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("unexpected second JSON value")
		}
		return err
	}
	return nil
}
