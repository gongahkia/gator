package lsp

import (
	"context"
	"encoding/json"
)

// client is the bounded RPC surface used by approved LSP tools.
type client interface {
	Request(context.Context, string, any) (json.RawMessage, error)
	Notify(string, any) error
	Supports(lspOperation) bool
	// DocumentSyncKind is the LSP TextDocumentSyncKind advertised at
	// initialize: 0 (none), 1 (full), or 2 (incremental).
	DocumentSyncKind() int
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

type lspTextEdit struct {
	Range   lspRange `json:"range"`
	NewText string   `json:"newText"`
}

type lspWorkspaceEdit struct {
	Changes         map[string][]lspTextEdit `json:"changes"`
	DocumentChanges json.RawMessage          `json:"documentChanges"`
}

type presentedTextEdit struct {
	Path    string         `json:"path"`
	Range   presentedRange `json:"range"`
	NewText string         `json:"new_text"`
}

type presentedCodeAction struct {
	Title          string              `json:"title"`
	Kind           string              `json:"kind,omitempty"`
	Preferred      bool                `json:"preferred,omitempty"`
	DisabledReason string              `json:"disabled_reason,omitempty"`
	Edits          []presentedTextEdit `json:"edits,omitempty"`
	CommandOmitted bool                `json:"command_omitted,omitempty"`
}

type lspCompletionItem struct {
	Label         string          `json:"label"`
	Kind          int             `json:"kind,omitempty"`
	Detail        string          `json:"detail,omitempty"`
	Documentation json.RawMessage `json:"documentation,omitempty"`
	InsertText    string          `json:"insertText,omitempty"`
}

type presentedCompletion struct {
	Label             string `json:"label"`
	Kind              int    `json:"kind,omitempty"`
	Detail            string `json:"detail,omitempty"`
	DocumentationKind string `json:"documentation_kind,omitempty"`
	Documentation     string `json:"documentation,omitempty"`
	InsertText        string `json:"insert_text,omitempty"`
}

type presentedCompletions struct {
	Path       string                `json:"path"`
	Server     string                `json:"server"`
	Incomplete bool                  `json:"incomplete"`
	Items      []presentedCompletion `json:"items"`
	Truncated  bool                  `json:"truncated"`
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

type presentedWorkspaceSymbol struct {
	Path   string          `json:"path"`
	Name   string          `json:"name"`
	Detail string          `json:"detail,omitempty"`
	Kind   int             `json:"kind"`
	Range  *presentedRange `json:"range,omitempty"`
}
