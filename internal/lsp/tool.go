package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/workspace"
)

type lspOperation string

const (
	diagnosticsOperation      lspOperation = "diagnostics"
	hoverOperation            lspOperation = "hover"
	completionOperation       lspOperation = "completion"
	codeActionsOperation      lspOperation = "code_actions"
	formatOperation           lspOperation = "format"
	renameOperation           lspOperation = "rename"
	definitionOperation       lspOperation = "definition"
	referencesOperation       lspOperation = "references"
	documentSymbolsOperation  lspOperation = "document_symbols"
	workspaceSymbolsOperation lspOperation = "workspace_symbols"
)

var lspOperations = []lspOperation{
	diagnosticsOperation,
	hoverOperation,
	completionOperation,
	codeActionsOperation,
	formatOperation,
	renameOperation,
	definitionOperation,
	referencesOperation,
	documentSymbolsOperation,
	workspaceSymbolsOperation,
}

// Tool adapts one trusted, read-only LSP operation to the agent tool contract.
type Tool struct {
	// root and connect retain the narrow pre-multi-root test seam. Production
	// construction supplies roots and connectRoots.
	root          workspace.Root
	roots         workspace.RootSet
	specification server
	operation     lspOperation
	approve       func(context.Context, string, string, string) error
	connect       func(context.Context, workspace.Root, server) (client, error)
	connectRoots  func(context.Context, workspace.RootSet, server) (client, error)
	manager       *Manager
}

func (t Tool) rootSet() workspace.RootSet {
	if t.roots.Primary().Path() != "" {
		return t.roots
	}
	return workspace.NewRootSet(t.root, nil)
}

func (t Tool) Definition() agent.ToolDefinition {
	prefix := "lsp_" + strings.ReplaceAll(t.specification.Name, "-", "_") + "_"
	definition := agent.ToolDefinition{
		Name:        prefix + string(t.operation),
		Description: t.description(),
	}
	switch t.operation {
	case diagnosticsOperation, documentSymbolsOperation:
		definition.Parameters = json.RawMessage(`{"type":"object","additionalProperties":false,"required":["path"],"properties":{"path":{"type":"string","description":"Primary-worktree-relative source-file path or approved external-root absolute source-file path"}}}`)
	case workspaceSymbolsOperation:
		definition.Parameters = json.RawMessage(`{"type":"object","additionalProperties":false,"required":["query"],"properties":{"query":{"type":"string","minLength":1,"maxLength":512,"description":"Symbol-name query within this workspace"}}}`)
	case referencesOperation:
		definition.Parameters = json.RawMessage(`{"type":"object","additionalProperties":false,"required":["path","line","character"],"properties":{"path":{"type":"string","description":"Primary-worktree-relative source-file path or approved external-root absolute source-file path"},"line":{"type":"integer","minimum":1,"description":"One-based source line"},"character":{"type":"integer","minimum":0,"description":"Zero-based UTF-16 character offset"},"include_declaration":{"type":"boolean","description":"Include the symbol declaration in results; defaults to false"}}}`)
	case codeActionsOperation:
		definition.Parameters = json.RawMessage(`{"type":"object","additionalProperties":false,"required":["path","line","character"],"properties":{"path":{"type":"string","description":"Workspace-relative source-file path"},"line":{"type":"integer","minimum":1,"description":"One-based selection start line"},"character":{"type":"integer","minimum":0,"description":"Zero-based UTF-16 selection start offset"},"end_line":{"type":"integer","minimum":1,"description":"Optional one-based selection end line; defaults to line"},"end_character":{"type":"integer","minimum":0,"description":"Optional zero-based UTF-16 selection end offset; defaults to character"}}}`)
	case formatOperation:
		definition.Parameters = json.RawMessage(`{"type":"object","additionalProperties":false,"required":["path"],"properties":{"path":{"type":"string","description":"Primary-worktree-relative source-file path; external roots are read-only"}}}`)
	case renameOperation:
		definition.Parameters = json.RawMessage(`{"type":"object","additionalProperties":false,"required":["path","line","character","new_name"],"properties":{"path":{"type":"string","description":"Workspace-relative source-file path"},"line":{"type":"integer","minimum":1,"description":"One-based source line"},"character":{"type":"integer","minimum":0,"description":"Zero-based UTF-16 character offset"},"new_name":{"type":"string","minLength":1,"maxLength":256,"description":"Requested symbol name"}}}`)
	case hoverOperation, completionOperation, definitionOperation:
		definition.Parameters = json.RawMessage(`{"type":"object","additionalProperties":false,"required":["path","line","character"],"properties":{"path":{"type":"string","description":"Primary-worktree-relative source-file path or approved external-root absolute source-file path"},"line":{"type":"integer","minimum":1,"description":"One-based source line"},"character":{"type":"integer","minimum":0,"description":"Zero-based UTF-16 character offset"}}}`)
	default:
		definition.Parameters = json.RawMessage(`{"type":"object","additionalProperties":false,"required":["path","line","character"],"properties":{"path":{"type":"string","description":"Workspace-relative source-file path"},"line":{"type":"integer","minimum":1,"description":"One-based source line"},"character":{"type":"integer","minimum":0,"description":"Zero-based UTF-16 character offset"}}}`)
	}
	return definition
}

func (t Tool) description() string {
	server := t.specification.Name + " (" + t.specification.Language + ")"
	switch t.operation {
	case diagnosticsOperation:
		return "Read-only LSP pull diagnostics using " + server + ". The path may be primary-worktree-relative or an approved external-root absolute source file."
	case hoverOperation:
		return "Read-only LSP hover information using " + server + ". line is one-based; character is a zero-based UTF-16 offset."
	case completionOperation:
		return "Read-only LSP completion lookup using " + server + ". line is one-based; character is a zero-based UTF-16 offset. It returns bounded suggestions and does not apply edits."
	case codeActionsOperation:
		return "Read-only LSP code-action lookup using " + server + ". It returns bounded, workspace-confined suggested edits for the selected range. Gator never executes server-provided commands or applies the edits automatically."
	case formatOperation:
		return "Read-only LSP document-format lookup using " + server + ". It returns bounded suggested edits for this workspace file; Gator does not apply them automatically."
	case renameOperation:
		return "Read-only LSP rename lookup using " + server + ". It returns bounded, workspace-confined suggested edits; Gator does not apply them automatically."
	case definitionOperation:
		return "Read-only LSP go-to-definition lookup using " + server + ". line is one-based; character is a zero-based UTF-16 offset. Only workspace locations are returned."
	case referencesOperation:
		return "Read-only LSP find-references lookup using " + server + ". line is one-based; character is a zero-based UTF-16 offset. Only workspace locations are returned."
	case documentSymbolsOperation:
		return "Read-only LSP document-symbol listing using " + server + ". The path may be primary-worktree-relative or an approved external-root absolute source file."
	case workspaceSymbolsOperation:
		return "Read-only LSP workspace-symbol search using " + server + ". The query is limited to symbols in the active workspace; only workspace locations are returned."
	default:
		return "Read-only LSP inspection using " + server + "."
	}
}

type toolParameters struct {
	Path               string `json:"path"`
	Line               int    `json:"line"`
	Character          int    `json:"character"`
	EndLine            int    `json:"end_line"`
	EndCharacter       int    `json:"end_character"`
	IncludeDeclaration bool   `json:"include_declaration"`
	Query              string `json:"query"`
	NewName            string `json:"new_name"`
}

func (t Tool) Execute(ctx context.Context, arguments json.RawMessage) (agent.ToolResult, error) {
	var params toolParameters
	if err := decodeArguments(arguments, &params); err != nil {
		return agent.ToolResult{}, err
	}
	resolved := ""
	target := params.Path
	if t.operation == workspaceSymbolsOperation {
		params.Query = strings.TrimSpace(params.Query)
		if params.Query == "" || len(params.Query) > maxSymbolQuery || strings.ContainsAny(params.Query, "\r\n\x00") {
			return agent.ToolResult{}, fmt.Errorf("LSP workspace-symbol query must contain 1-%d printable bytes", maxSymbolQuery)
		}
		target = params.Query
	} else {
		var err error
		roots := t.rootSet()
		resolved, err = roots.ResolveFile(params.Path)
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
		if !roots.IsPrimary(resolved) && (t.operation == codeActionsOperation || t.operation == formatOperation || t.operation == renameOperation) {
			return agent.ToolResult{}, fmt.Errorf("LSP %s is unavailable for read-only external workspace files", t.operation)
		}
		target = roots.DisplayPath(resolved)
	}
	if t.requiresPosition() && (params.Line < 1 || params.Character < 0) {
		return agent.ToolResult{}, errors.New("LSP position requires a one-based line and a zero-based UTF-16 character offset")
	}
	if t.operation == codeActionsOperation {
		if _, err := codeActionRange(params); err != nil {
			return agent.ToolResult{}, err
		}
	}
	if t.operation == renameOperation {
		params.NewName = strings.TrimSpace(params.NewName)
		if params.NewName == "" || len(params.NewName) > maxRenameNameBytes || strings.ContainsAny(params.NewName, "\r\n\x00") {
			return agent.ToolResult{}, fmt.Errorf("LSP rename new_name must contain 1-%d printable bytes", maxRenameNameBytes)
		}
	}
	if t.approve == nil {
		return agent.ToolResult{}, fmt.Errorf("LSP %s requires developer approval", t.operation)
	}
	if err := t.approve(ctx, t.specification.Name, string(t.operation), target); err != nil {
		return agent.ToolResult{}, err
	}
	method, request := t.request(resolved, params)
	var response json.RawMessage
	var err error
	if t.manager != nil {
		response, err = t.manager.request(ctx, t.specification, t.operation, resolved, method, request)
	} else {
		connection, release, openErr := t.open(ctx)
		if openErr != nil {
			return agent.ToolResult{}, fmt.Errorf("start LSP server %q: %w", t.specification.Name, openErr)
		}
		defer release()
		if !connection.Supports(t.operation) {
			return agent.ToolResult{}, fmt.Errorf("LSP server %q does not support %s", t.specification.Name, t.operation)
		}
		if resolved != "" && connection.DocumentSyncKind() != 0 {
			contents, readErr := os.ReadFile(resolved)
			if readErr != nil {
				return agent.ToolResult{}, fmt.Errorf("read LSP synchronized document: %w", readErr)
			}
			if len(contents) > maxDocumentSyncBytes {
				return agent.ToolResult{}, fmt.Errorf("LSP synchronized document exceeds the %d KiB limit", maxDocumentSyncBytes/1024)
			}
			document := map[string]any{"uri": fileURI(resolved), "languageId": t.specification.Language, "version": 1, "text": string(contents)}
			if notifyErr := connection.Notify("textDocument/didOpen", map[string]any{"textDocument": document}); notifyErr != nil {
				return agent.ToolResult{}, fmt.Errorf("open synchronized LSP document: %w", notifyErr)
			}
			defer connection.Notify("textDocument/didClose", map[string]any{"textDocument": map[string]any{"uri": fileURI(resolved)}})
		}
		response, err = connection.Request(ctx, method, request)
	}
	if err != nil {
		return agent.ToolResult{}, fmt.Errorf("request %s from LSP server %q: %w", t.operation, t.specification.Name, err)
	}
	return t.format(target, response)
}

func (t Tool) open(ctx context.Context) (client, func(), error) {
	if t.manager != nil {
		connection, err := t.manager.connection(ctx, t.specification)
		return connection, func() {}, err
	}
	if t.connect == nil {
		return nil, nil, errors.New("LSP connection is not configured")
	}
	if t.connectRoots != nil {
		connection, err := t.connectRoots(ctx, t.rootSet(), t.specification)
		if err != nil {
			return nil, nil, err
		}
		return connection, func() { _ = connection.Close() }, nil
	}
	connection, err := t.connect(ctx, t.rootSet().Primary(), t.specification)
	if err != nil {
		return nil, nil, err
	}
	return connection, func() { _ = connection.Close() }, nil
}

func (t Tool) requiresPosition() bool {
	return t.operation == hoverOperation || t.operation == completionOperation || t.operation == codeActionsOperation || t.operation == renameOperation || t.operation == definitionOperation || t.operation == referencesOperation
}

func (t Tool) request(path string, params toolParameters) (string, any) {
	document := map[string]string{"uri": fileURI(path)}
	position := map[string]int{"line": params.Line - 1, "character": params.Character}
	switch t.operation {
	case diagnosticsOperation:
		return "textDocument/diagnostic", map[string]any{"textDocument": document}
	case hoverOperation:
		return "textDocument/hover", map[string]any{"textDocument": document, "position": position}
	case completionOperation:
		return "textDocument/completion", map[string]any{"textDocument": document, "position": position}
	case codeActionsOperation:
		selection, _ := codeActionRange(params)
		return "textDocument/codeAction", map[string]any{"textDocument": document, "range": selection, "context": map[string]any{"diagnostics": []any{}}}
	case formatOperation:
		return "textDocument/formatting", map[string]any{"textDocument": document, "options": map[string]any{"tabSize": 4, "insertSpaces": true, "trimTrailingWhitespace": false, "insertFinalNewline": false, "trimFinalNewlines": false}}
	case renameOperation:
		return "textDocument/rename", map[string]any{"textDocument": document, "position": position, "newName": params.NewName}
	case definitionOperation:
		return "textDocument/definition", map[string]any{"textDocument": document, "position": position}
	case referencesOperation:
		return "textDocument/references", map[string]any{"textDocument": document, "position": position, "context": map[string]bool{"includeDeclaration": params.IncludeDeclaration}}
	case documentSymbolsOperation:
		return "textDocument/documentSymbol", map[string]any{"textDocument": document}
	case workspaceSymbolsOperation:
		return "workspace/symbol", map[string]string{"query": params.Query}
	default:
		return "", nil
	}
}

func codeActionRange(params toolParameters) (map[string]map[string]int, error) {
	endLine, endCharacter := params.EndLine, params.EndCharacter
	if endLine == 0 {
		endLine, endCharacter = params.Line, params.Character
	}
	if endLine < params.Line || (endLine == params.Line && endCharacter < params.Character) || endCharacter < 0 {
		return nil, errors.New("LSP code-action selection end must not precede its start")
	}
	return map[string]map[string]int{
		"start": {"line": params.Line - 1, "character": params.Character},
		"end":   {"line": endLine - 1, "character": endCharacter},
	}, nil
}
