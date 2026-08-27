package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/workspace"
)

func TestLoadPinsExecutableAndTrustsOnlyMatchingBundle(t *testing.T) {
	directory := t.TempDir()
	writeFile(t, directory, ".gator/diagnostic-server", "#!/bin/sh\n", 0o755)
	writeFile(t, directory, ".gator/lsp.json", `{"version":1,"servers":[{"name":"fixture","command":[".gator/diagnostic-server"],"language":"go"}]}`, 0o600)

	digest, err := BundleHash(directory)
	if err != nil {
		t.Fatalf("hash LSP bundle: %v", err)
	}
	untrusted, err := Load(directory, "")
	if err != nil {
		t.Fatalf("load untrusted bundle: %v", err)
	}
	if !untrusted.Configured() || untrusted.Trusted() || len(untrusted.Tools(nil)) != 0 {
		t.Fatalf("untrusted LSP bundle = %#v", untrusted)
	}
	trusted, err := Load(directory, digest)
	if err != nil {
		t.Fatalf("load trusted bundle: %v", err)
	}
	if !trusted.Trusted() || len(trusted.Tools(func(context.Context, string, string, string) error { return nil })) != len(lspOperations) {
		t.Fatalf("trusted LSP bundle = %#v", trusted)
	}

	writeFile(t, directory, ".gator/diagnostic-server", "#!/bin/sh\necho changed\n", 0o755)
	changed, err := BundleHash(directory)
	if err != nil {
		t.Fatalf("hash changed LSP bundle: %v", err)
	}
	if changed == digest {
		t.Fatal("LSP bundle hash did not change after executable changed")
	}
}

func TestToolApprovesAndFormatsDiagnostics(t *testing.T) {
	root := testWorkspace(t)
	writeFile(t, root.Path(), "pkg/example.go", "package pkg\n", 0o600)
	connection := &fakeClient{responses: map[string]json.RawMessage{
		"textDocument/diagnostic": json.RawMessage(`{"kind":"full","items":[{"range":{"start":{"line":3,"character":2},"end":{"line":3,"character":4}},"severity":2,"message":"broken declaration"}]}`),
	}}
	approved := ""
	tool := Tool{
		root:          root,
		specification: server{Name: "fixture", Language: "go"},
		operation:     diagnosticsOperation,
		approve: func(_ context.Context, server, operation, path string) error {
			approved = server + ":" + operation + ":" + path
			return nil
		},
		connect: func(context.Context, workspace.Root, server) (client, error) { return connection, nil },
	}

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"path":"pkg/example.go"}`))
	if err != nil {
		t.Fatalf("execute diagnostic tool: %v", err)
	}
	if approved != "fixture:diagnostics:pkg/example.go" || connection.method != "textDocument/diagnostic" || !connection.closed {
		t.Fatalf("approval=%q client=%#v", approved, connection)
	}
	if !strings.Contains(result.Content, `"start_line":4`) || !strings.Contains(result.Content, `"severity":"warning"`) || !strings.Contains(result.Content, "broken declaration") {
		t.Fatalf("diagnostic result = %s", result.Content)
	}
}

func TestToolRejectsUnapprovedLookup(t *testing.T) {
	root := testWorkspace(t)
	writeFile(t, root.Path(), "pkg/example.go", "package pkg\n", 0o600)
	tool := Tool{root: root, specification: server{Name: "fixture", Language: "go"}, operation: diagnosticsOperation}
	_, err := tool.Execute(context.Background(), json.RawMessage(`{"path":"pkg/example.go"}`))
	if err == nil || !strings.Contains(err.Error(), "developer approval") {
		t.Fatalf("unapproved diagnostic error = %v", err)
	}
}

func TestToolRejectsUnsupportedServerOperationBeforeRequest(t *testing.T) {
	root := testWorkspace(t)
	writeFile(t, root.Path(), "pkg/example.go", "package pkg\n", 0o600)
	connection := &fakeClient{supported: map[lspOperation]bool{hoverOperation: false}}
	tool := Tool{
		root: root, specification: server{Name: "fixture", Language: "go"}, operation: hoverOperation,
		approve: func(context.Context, string, string, string) error { return nil },
		connect: func(context.Context, workspace.Root, server) (client, error) { return connection, nil },
	}
	_, err := tool.Execute(context.Background(), json.RawMessage(`{"path":"pkg/example.go","line":1,"character":0}`))
	if err == nil || !strings.Contains(err.Error(), "does not support hover") || connection.method != "" || !connection.closed {
		t.Fatalf("unsupported hover error=%v client=%#v", err, connection)
	}
}

func TestToolsFormatReadOnlyNavigationAndKeepLocationsInWorkspace(t *testing.T) {
	root := testWorkspace(t)
	writeFile(t, root.Path(), "pkg/example.go", "package pkg\n", 0o600)
	writeFile(t, root.Path(), "pkg/target.go", "package pkg\n", 0o600)
	localURI := fileURI(filepath.Join(root.Path(), "pkg", "target.go"))
	externalURI := fileURI("/outside/gator-secret.go")
	location := func(uri string) map[string]any {
		return map[string]any{"uri": uri, "range": map[string]any{"start": map[string]int{"line": 2, "character": 1}, "end": map[string]int{"line": 2, "character": 7}}}
	}
	encoded := func(value any) json.RawMessage {
		result, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	responses := map[string]json.RawMessage{
		"textDocument/hover": encoded(map[string]any{
			"contents": map[string]string{"kind": "markdown", "value": "**Target**"},
			"range":    location(localURI)["range"],
		}),
		"textDocument/completion": encoded(map[string]any{
			"isIncomplete": true,
			"items": []any{map[string]any{
				"label": "Target", "kind": 5, "detail": "fixture symbol", "insertText": "Target",
				"documentation":       map[string]string{"kind": "markdown", "value": "**Target** completion"},
				"command":             map[string]string{"title": "untrusted", "command": "dangerous.command"},
				"additionalTextEdits": []any{map[string]any{"newText": "gator-secret"}},
			}},
		}),
		"textDocument/codeAction": encoded([]any{
			map[string]any{
				"title": "Fix Target", "kind": "quickfix", "isPreferred": true,
				"edit": map[string]any{"changes": map[string]any{localURI: []any{map[string]any{"range": location(localURI)["range"], "newText": "Fixed"}}}},
			},
			map[string]any{
				"title": "External edit", "edit": map[string]any{"changes": map[string]any{externalURI: []any{map[string]any{"range": location(externalURI)["range"], "newText": "outside secret"}}}},
			},
			map[string]any{"title": "Run server command", "command": map[string]string{"title": "Danger", "command": "dangerous.command"}},
		}),
		"textDocument/formatting": encoded([]any{map[string]any{
			"range": location(localURI)["range"], "newText": "Formatted",
		}}),
		"textDocument/rename": encoded(map[string]any{"changes": map[string]any{
			localURI:    []any{map[string]any{"range": location(localURI)["range"], "newText": "Renamed"}},
			externalURI: []any{map[string]any{"range": location(externalURI)["range"], "newText": "outside secret"}},
		}}),
		"textDocument/definition": encoded([]any{location(localURI), location(externalURI)}),
		"textDocument/references": encoded([]any{location(localURI)}),
		"textDocument/documentSymbol": encoded([]any{map[string]any{
			"name": "Target", "kind": 5, "range": location(localURI)["range"], "selectionRange": location(localURI)["range"],
			"children": []any{map[string]any{"name": "Method", "kind": 6, "range": location(localURI)["range"], "selectionRange": location(localURI)["range"]}},
		}}),
		"workspace/symbol": encoded([]any{map[string]any{
			"name": "Target", "kind": 5, "containerName": "pkg", "location": location(localURI),
		}, map[string]any{
			"name": "External", "kind": 5, "location": location(externalURI),
		}}),
	}
	cases := []struct {
		operation lspOperation
		arguments string
		method    string
		contains  []string
	}{
		{operation: hoverOperation, arguments: `{"path":"pkg/example.go","line":3,"character":1}`, method: "textDocument/hover", contains: []string{`"found":true`, `"kind":"markdown"`, "Target"}},
		{operation: completionOperation, arguments: `{"path":"pkg/example.go","line":3,"character":1}`, method: "textDocument/completion", contains: []string{`"incomplete":true`, `"documentation_kind":"markdown"`, `"insert_text":"Target"`}},
		{operation: codeActionsOperation, arguments: `{"path":"pkg/example.go","line":3,"character":1,"end_line":3,"end_character":5}`, method: "textDocument/codeAction", contains: []string{"Fix Target", `"new_text":"Fixed"`, `"command_omitted":true`, `"truncated":true`}},
		{operation: formatOperation, arguments: `{"path":"pkg/example.go"}`, method: "textDocument/formatting", contains: []string{`"operation":"format"`, `"new_text":"Formatted"`}},
		{operation: renameOperation, arguments: `{"path":"pkg/example.go","line":3,"character":1,"new_name":"Renamed"}`, method: "textDocument/rename", contains: []string{`"operation":"rename"`, `"truncated":true`}},
		{operation: definitionOperation, arguments: `{"path":"pkg/example.go","line":3,"character":1}`, method: "textDocument/definition", contains: []string{"pkg/target.go", `"truncated":true`}},
		{operation: referencesOperation, arguments: `{"path":"pkg/example.go","line":3,"character":1,"include_declaration":true}`, method: "textDocument/references", contains: []string{"pkg/target.go"}},
		{operation: documentSymbolsOperation, arguments: `{"path":"pkg/example.go"}`, method: "textDocument/documentSymbol", contains: []string{"Target", "Method"}},
		{operation: workspaceSymbolsOperation, arguments: `{"query":"Target"}`, method: "workspace/symbol", contains: []string{`"query":"Target"`, "pkg/target.go", `"truncated":true`}},
	}
	for _, test := range cases {
		t.Run(string(test.operation), func(t *testing.T) {
			connection := &fakeClient{responses: responses}
			approved := ""
			tool := Tool{
				root: root, specification: server{Name: "fixture", Language: "go"}, operation: test.operation,
				approve: func(_ context.Context, server, operation, path string) error {
					approved = server + ":" + operation + ":" + path
					return nil
				},
				connect: func(context.Context, workspace.Root, server) (client, error) { return connection, nil },
			}
			result, err := tool.Execute(context.Background(), json.RawMessage(test.arguments))
			if err != nil {
				t.Fatalf("execute %s: %v", test.operation, err)
			}
			target := "pkg/example.go"
			if test.operation == workspaceSymbolsOperation {
				target = "Target"
			}
			if approved != "fixture:"+string(test.operation)+":"+target || connection.method != test.method || !connection.closed {
				t.Fatalf("approval=%q client=%#v", approved, connection)
			}
			for _, expected := range test.contains {
				if !strings.Contains(result.Content, expected) {
					t.Fatalf("%s result missing %q: %s", test.operation, expected, result.Content)
				}
			}
			if strings.Contains(result.Content, "gator-secret") || strings.Contains(result.Content, "outside secret") || strings.Contains(result.Content, "dangerous.command") {
				t.Fatalf("%s leaked external LSP location: %s", test.operation, result.Content)
			}
			if test.operation == referencesOperation {
				encoded, err := json.Marshal(connection.parameters)
				if err != nil || !strings.Contains(string(encoded), `"includeDeclaration":true`) {
					t.Fatalf("references parameters = %s, %v", encoded, err)
				}
			}
			if test.operation == workspaceSymbolsOperation {
				encoded, err := json.Marshal(connection.parameters)
				if err != nil || !strings.Contains(string(encoded), `"query":"Target"`) {
					t.Fatalf("workspace-symbol parameters = %s, %v", encoded, err)
				}
			}
			if test.operation == completionOperation {
				encoded, err := json.Marshal(connection.parameters)
				if err != nil || !strings.Contains(string(encoded), `"line":2`) || !strings.Contains(string(encoded), `"character":1`) {
					t.Fatalf("completion parameters = %s, %v", encoded, err)
				}
				if strings.Contains(result.Content, "dangerous.command") || strings.Contains(result.Content, "gator-secret") {
					t.Fatalf("completion leaked an edit or command: %s", result.Content)
				}
			}
			if test.operation == codeActionsOperation {
				encoded, err := json.Marshal(connection.parameters)
				if err != nil || !strings.Contains(string(encoded), `"start":{"character":1,"line":2}`) || !strings.Contains(string(encoded), `"end":{"character":5,"line":2}`) {
					t.Fatalf("code-action parameters = %s, %v", encoded, err)
				}
			}
			if test.operation == renameOperation {
				encoded, err := json.Marshal(connection.parameters)
				if err != nil || !strings.Contains(string(encoded), `"line":2`) || !strings.Contains(string(encoded), `"character":1`) || !strings.Contains(string(encoded), `"newName":"Renamed"`) {
					t.Fatalf("rename parameters = %s, %v", encoded, err)
				}
			}
		})
	}
}

func TestFormatCompletionsBoundsItemsAndRejectsInvalidData(t *testing.T) {
	items := make([]map[string]any, maxCompletions+1)
	for index := range items {
		items[index] = map[string]any{"label": "Item" + strconv.Itoa(index)}
	}
	response, err := json.Marshal(map[string]any{"isIncomplete": true, "items": items})
	if err != nil {
		t.Fatal(err)
	}
	result, err := formatCompletions("pkg/example.go", "fixture", response)
	if err != nil {
		t.Fatalf("format completions: %v", err)
	}
	var payload struct {
		Incomplete bool                  `json:"incomplete"`
		Items      []presentedCompletion `json:"items"`
		Truncated  bool                  `json:"truncated"`
	}
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil || !payload.Incomplete || !payload.Truncated || len(payload.Items) != maxCompletions || payload.Items[0].Label != "Item0" {
		t.Fatalf("completion payload = %#v, %v", payload, err)
	}
	result, err = formatCompletions("pkg/example.go", "fixture", json.RawMessage(`[{"label":"array item"}]`))
	if err != nil || !strings.Contains(result.Content, "array item") || strings.Contains(result.Content, `"incomplete":true`) {
		t.Fatalf("completion array result = %s, %v", result.Content, err)
	}
	if _, err := formatCompletions("pkg/example.go", "fixture", json.RawMessage(`[{"label":"","documentation":{"kind":"html","value":"bad"}}]`)); err == nil || !strings.Contains(err.Error(), "invalid completion item") {
		t.Fatalf("invalid completion item error = %v", err)
	}
	if _, err := formatCompletions("pkg/example.go", "fixture", json.RawMessage(`[{"label":"Target","documentation":{"kind":"html","value":"bad"}}]`)); err == nil || !strings.Contains(err.Error(), "unsupported completion documentation kind") {
		t.Fatalf("invalid completion documentation error = %v", err)
	}
	escaped := make([]map[string]any, maxCompletions)
	for index := range escaped {
		escaped[index] = map[string]any{"label": "Item" + strconv.Itoa(index), "documentation": strings.Repeat("\x00", maxCompletionDocumentationBytes)}
	}
	response, err = json.Marshal(escaped)
	if err != nil {
		t.Fatal(err)
	}
	result, err = formatCompletions("pkg/example.go", "fixture", response)
	if err != nil {
		t.Fatalf("format escaped completions: %v", err)
	}
	if len(result.Content) > maxToolOutput || !strings.Contains(result.Content, `"truncated":true`) {
		t.Fatalf("escaped completion result is not bounded: %d bytes, %s", len(result.Content), result.Content)
	}
}

func TestWorkspaceSymbolsAcceptURIOnlyLocationAndRejectInvalidQuery(t *testing.T) {
	root := testWorkspace(t)
	writeFile(t, root.Path(), "pkg/target.go", "package pkg\n", 0o600)
	uri := fileURI(filepath.Join(root.Path(), "pkg", "target.go"))
	result, err := formatWorkspaceSymbols(workspace.NewRootSet(root, nil), "Target", "fixture", json.RawMessage(`[{"name":"Target","kind":5,"location":{"uri":`+strconv.Quote(uri)+`}}]`))
	if err != nil {
		t.Fatalf("format URI-only workspace symbol: %v", err)
	}
	var payload struct {
		Symbols []struct {
			Name  string          `json:"name"`
			Path  string          `json:"path"`
			Range *presentedRange `json:"range"`
		} `json:"symbols"`
	}
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil || len(payload.Symbols) != 1 || payload.Symbols[0].Name != "Target" || payload.Symbols[0].Path != "pkg/target.go" || payload.Symbols[0].Range != nil {
		t.Fatalf("URI-only workspace symbol payload = %#v, %v", payload, err)
	}
	tool := Tool{root: root, specification: server{Name: "fixture", Language: "go"}, operation: workspaceSymbolsOperation}
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"query":"line\nbreak"}`)); err == nil || !strings.Contains(err.Error(), "workspace-symbol query") {
		t.Fatalf("invalid workspace-symbol query error = %v", err)
	}
}

func TestCodeActionsRejectUnsafeEditsAndSelectionRanges(t *testing.T) {
	root := testWorkspace(t)
	writeFile(t, root.Path(), "pkg/example.go", "package pkg\n", 0o600)
	uri := fileURI(filepath.Join(root.Path(), "pkg", "example.go"))
	result, err := formatCodeActions(workspace.NewRootSet(root, nil), "pkg/example.go", "fixture", json.RawMessage(`[{"title":"create file","edit":{"documentChanges":[{"kind":"create","uri":"file:///tmp/outside"}]}},{"title":"local edit","edit":{"changes":{"`+uri+`":[{"range":{"start":{"line":0,"character":0},"end":{"line":0,"character":0}},"newText":"fixed"}]}}}]`))
	if err != nil || !strings.Contains(result.Content, "local edit") || strings.Contains(result.Content, "create file") || !strings.Contains(result.Content, `"truncated":true`) {
		t.Fatalf("resource-operation code actions = %s, %v", result.Content, err)
	}
	result, err = formatCodeActions(workspace.NewRootSet(root, nil), "pkg/example.go", "fixture", json.RawMessage(`[{"title":"large edit","edit":{"changes":{"`+uri+`":[{"range":{"start":{"line":0,"character":0},"end":{"line":0,"character":0}},"newText":"`+strings.Repeat("x", maxCodeActionNewTextBytes+1)+`"}]}}}]`))
	if err == nil || !strings.Contains(err.Error(), "exceeds 16 KiB") {
		t.Fatalf("oversized code action error = %v", err)
	}
	if result.Content != "" {
		t.Fatalf("unexpected result for oversized code action: %s", result.Content)
	}
	tool := Tool{root: root, specification: server{Name: "fixture", Language: "go"}, operation: codeActionsOperation}
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"path":"pkg/example.go","line":2,"character":1,"end_line":1,"end_character":1}`)); err == nil || !strings.Contains(err.Error(), "selection end") {
		t.Fatalf("invalid code-action selection error = %v", err)
	}
}

func TestFormattingAndRenameOmitIncompleteOrOversizedEdits(t *testing.T) {
	root := testWorkspace(t)
	writeFile(t, root.Path(), "pkg/example.go", "package pkg\n", 0o600)
	uri := fileURI(filepath.Join(root.Path(), "pkg", "example.go"))
	formatting, err := formatFormatting("pkg/example.go", "fixture", json.RawMessage(`[{"range":{"start":{"line":0,"character":0},"end":{"line":0,"character":0}},"newText":"fixed"}]`))
	if err != nil || !strings.Contains(formatting.Content, `"new_text":"fixed"`) || strings.Contains(formatting.Content, `"truncated":true`) {
		t.Fatalf("formatting result = %s, %v", formatting.Content, err)
	}
	many := make([]lspTextEdit, maxCodeActionEdits+1)
	for index := range many {
		many[index].Range.Start = Position{Line: index, Character: 0}
		many[index].Range.End = Position{Line: index, Character: 0}
		many[index].NewText = "x"
	}
	rawMany, err := json.Marshal(many)
	if err != nil {
		t.Fatal(err)
	}
	formatting, err = formatFormatting("pkg/example.go", "fixture", rawMany)
	if err != nil || !strings.Contains(formatting.Content, `"truncated":true`) || strings.Contains(formatting.Content, `"new_text"`) {
		t.Fatalf("oversized formatting result = %s, %v", formatting.Content, err)
	}
	rename, err := formatRename(workspace.NewRootSet(root, nil), "pkg/example.go", "fixture", json.RawMessage(`{"documentChanges":[{"kind":"rename","oldUri":`+strconv.Quote(uri)+`,"newUri":"file:///tmp/outside"}]}`))
	if err != nil || !strings.Contains(rename.Content, `"truncated":true`) || strings.Contains(rename.Content, `"new_text"`) {
		t.Fatalf("unsafe rename result = %s, %v", rename.Content, err)
	}
	tool := Tool{root: root, specification: server{Name: "fixture", Language: "go"}, operation: renameOperation}
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"path":"pkg/example.go","line":1,"character":0,"new_name":"line\nbreak"}`)); err == nil || !strings.Contains(err.Error(), "rename new_name") {
		t.Fatalf("invalid rename error = %v", err)
	}
}

func TestManagerReusesOneClientForMultipleApprovedLookups(t *testing.T) {
	root := testWorkspace(t)
	writeFile(t, root.Path(), "pkg/example.go", "package pkg\n", 0o600)
	connection := &fakeClient{responses: map[string]json.RawMessage{
		"textDocument/hover":          json.RawMessage(`{"contents":"fixture hover"}`),
		"textDocument/documentSymbol": json.RawMessage(`[]`),
	}}
	starts := 0
	manager := &Manager{
		root: root, trusted: true, servers: []server{{Name: "fixture", Language: "go"}}, clients: make(map[string]client),
		connect: func(context.Context, workspace.Root, server) (client, error) {
			starts++
			return connection, nil
		},
	}
	var approvals []string
	tools := manager.Tools(func(_ context.Context, server, operation, target string) error {
		approvals = append(approvals, server+":"+operation+":"+target)
		return nil
	})
	lookup := func(operation lspOperation) Tool {
		for _, tool := range tools {
			candidate := tool.(Tool)
			if candidate.operation == operation {
				return candidate
			}
		}
		t.Fatalf("missing manager tool %s", operation)
		return Tool{}
	}
	if _, err := lookup(hoverOperation).Execute(context.Background(), json.RawMessage(`{"path":"pkg/example.go","line":1,"character":0}`)); err != nil {
		t.Fatalf("manager hover: %v", err)
	}
	if _, err := lookup(documentSymbolsOperation).Execute(context.Background(), json.RawMessage(`{"path":"pkg/example.go"}`)); err != nil {
		t.Fatalf("manager document symbols: %v", err)
	}
	if starts != 1 || connection.closed || !reflect.DeepEqual(approvals, []string{"fixture:hover:pkg/example.go", "fixture:document_symbols:pkg/example.go"}) {
		t.Fatalf("manager starts=%d closed=%t approvals=%#v", starts, connection.closed, approvals)
	}
	if !reflect.DeepEqual(connection.notifications, []string{"textDocument/didOpen"}) {
		t.Fatalf("document notifications = %#v", connection.notifications)
	}
	if err := manager.Close(); err != nil || !connection.closed {
		t.Fatalf("close manager: %v, client=%#v", err, connection)
	}
	if !reflect.DeepEqual(connection.notifications, []string{"textDocument/didOpen", "textDocument/didClose"}) {
		t.Fatalf("close notifications = %#v", connection.notifications)
	}
}

func TestManagerSynchronizesChangesAndReopensAfterDiscard(t *testing.T) {
	root := testWorkspace(t)
	path := filepath.Join(root.Path(), "pkg/example.go")
	writeFile(t, root.Path(), "pkg/example.go", "package pkg\n", 0o600)
	first := &fakeClient{responses: map[string]json.RawMessage{"textDocument/hover": json.RawMessage(`{"contents":"first"}`)}}
	second := &fakeClient{responses: map[string]json.RawMessage{"textDocument/hover": json.RawMessage(`{"contents":"second"}`)}}
	connections := []client{first, second}
	manager := &Manager{
		root: root, trusted: true, servers: []server{{Name: "fixture", Language: "go"}}, clients: make(map[string]client),
		connect: func(context.Context, workspace.Root, server) (client, error) {
			connection := connections[0]
			connections = connections[1:]
			return connection, nil
		},
	}
	tool := manager.Tools(func(context.Context, string, string, string) error { return nil })[1].(Tool)
	arguments := json.RawMessage(`{"path":"pkg/example.go","line":1,"character":0}`)
	if _, err := tool.Execute(context.Background(), arguments); err != nil {
		t.Fatalf("first hover: %v", err)
	}
	if err := os.WriteFile(path, []byte("package pkg\n\nvar Changed = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := tool.Execute(context.Background(), arguments); err != nil {
		t.Fatalf("changed hover: %v", err)
	}
	manager.Discard("fixture")
	if _, err := tool.Execute(context.Background(), arguments); err != nil {
		t.Fatalf("reconnected hover: %v", err)
	}
	if !reflect.DeepEqual(first.notifications, []string{"textDocument/didOpen", "textDocument/didChange", "textDocument/didClose"}) {
		t.Fatalf("first client notifications = %#v", first.notifications)
	}
	if !reflect.DeepEqual(second.notifications, []string{"textDocument/didOpen"}) {
		t.Fatalf("second client notifications = %#v", second.notifications)
	}
	_ = manager.Close()
}

func TestLSPExternalRootIsInspectableButNotEditable(t *testing.T) {
	root := testWorkspace(t)
	externalPath := t.TempDir()
	writeFile(t, externalPath, "external.go", "package external\n", 0o600)
	external, err := workspace.Open(externalPath)
	if err != nil {
		t.Fatal(err)
	}
	externalFile := filepath.Join(external.Path(), "external.go")
	connection := &fakeClient{responses: map[string]json.RawMessage{
		"textDocument/hover": json.RawMessage(`{"contents":"external hover"}`),
	}}
	manager := &Manager{
		root: root, roots: workspace.NewRootSet(root, []workspace.Root{external}), trusted: true,
		servers: []server{{Name: "fixture", Language: "go"}}, clients: make(map[string]client),
		connect: func(context.Context, workspace.Root, server) (client, error) { return connection, nil },
	}
	tools := manager.Tools(func(context.Context, string, string, string) error { return nil })
	var hover, format Tool
	for _, candidate := range tools {
		tool := candidate.(Tool)
		switch tool.operation {
		case hoverOperation:
			hover = tool
		case formatOperation:
			format = tool
		}
	}
	result, err := hover.Execute(context.Background(), json.RawMessage(`{"path":`+strconv.Quote(externalFile)+`,"line":1,"character":0}`))
	if err != nil || !strings.Contains(result.Content, externalFile) {
		t.Fatalf("external hover result=%q err=%v", result.Content, err)
	}
	if _, err := format.Execute(context.Background(), json.RawMessage(`{"path":`+strconv.Quote(externalFile)+`}`)); err == nil || !strings.Contains(err.Error(), "read-only external") {
		t.Fatalf("external format error = %v", err)
	}
}

func TestDocumentSyncKind(t *testing.T) {
	for raw, want := range map[string]int{
		`0`:            0,
		`1`:            1,
		`2`:            2,
		`3`:            0,
		`false`:        0,
		`{"change":1}`: 1,
		`{"change":2}`: 2,
		`{"change":0}`: 0,
	} {
		if got := documentSyncKind(json.RawMessage(raw)); got != want {
			t.Errorf("documentSyncKind(%s) = %d, want %d", raw, got, want)
		}
	}
}

func TestNativeClientUsesReadOnlyLookups(t *testing.T) {
	inputReader, inputWriter := io.Pipe()
	outputReader, outputWriter := io.Pipe()
	client := &nativeClient{
		input:   inputWriter,
		output:  bufio.NewReader(outputReader),
		command: waitFunc(func() error { return nil }),
		cleanup: func() {},
	}
	done := make(chan error, 1)
	go serveFixtureLSP(inputReader, outputWriter, done)

	if err := client.initialize(context.Background(), "/fixture"); err != nil {
		t.Fatalf("initialize LSP client: %v", err)
	}
	for _, operation := range lspOperations {
		if !client.Supports(operation) {
			t.Fatalf("fixture LSP does not advertise %s: %#v", operation, client.support)
		}
	}
	if client.Supports(lspOperation("unsupported")) {
		t.Fatalf("fixture LSP capability set = %#v", client.support)
	}
	for _, lookup := range []struct {
		method string
		want   string
	}{
		{method: "textDocument/diagnostic", want: "fixture error"},
		{method: "textDocument/hover", want: "fixture hover"},
		{method: "textDocument/completion", want: "fixture completion"},
		{method: "textDocument/codeAction", want: "[]"},
		{method: "textDocument/formatting", want: "[]"},
		{method: "textDocument/rename", want: "{}"},
		{method: "textDocument/definition", want: "[]"},
		{method: "textDocument/references", want: "[]"},
		{method: "textDocument/documentSymbol", want: "[]"},
		{method: "workspace/symbol", want: "[]"},
	} {
		response, err := client.Request(context.Background(), lookup.method, map[string]any{"textDocument": map[string]string{"uri": fileURI("/fixture/pkg/example.go")}})
		if err != nil {
			t.Fatalf("request %s: %v", lookup.method, err)
		}
		if !strings.Contains(string(response), lookup.want) {
			t.Fatalf("%s response = %#v", lookup.method, response)
		}
	}
	if err := client.Close(); err != nil {
		t.Fatalf("close LSP client: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("fixture LSP: %v", err)
	}
}

type fakeClient struct {
	responses     map[string]json.RawMessage
	supported     map[lspOperation]bool
	method        string
	parameters    any
	notifications []string
	closed        bool
}

func (c *fakeClient) Request(_ context.Context, method string, parameters any) (json.RawMessage, error) {
	c.method = method
	c.parameters = parameters
	return c.responses[method], nil
}

func (c *fakeClient) Supports(operation lspOperation) bool {
	return c.supported == nil || c.supported[operation]
}

func (c *fakeClient) Notify(method string, _ any) error {
	c.notifications = append(c.notifications, method)
	return nil
}

func (c *fakeClient) DocumentSyncKind() int { return 1 }

func (c *fakeClient) Close() error {
	c.closed = true
	return nil
}

type waitFunc func() error

func (f waitFunc) Wait() error { return f() }

func serveFixtureLSP(input io.Reader, output io.Writer, done chan<- error) {
	reader := bufio.NewReader(input)
	for {
		frame, err := readFrame(reader)
		if err != nil {
			done <- err
			return
		}
		var message struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if err := json.Unmarshal(frame, &message); err != nil {
			done <- err
			return
		}
		switch message.Method {
		case "initialize":
			writeFrame(output, map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(message.ID), "result": map[string]any{"capabilities": map[string]any{
				"textDocumentSync":           1,
				"diagnosticProvider":         map[string]any{},
				"hoverProvider":              true,
				"completionProvider":         map[string]any{},
				"codeActionProvider":         true,
				"documentFormattingProvider": true,
				"renameProvider":             true,
				"definitionProvider":         true,
				"referencesProvider":         true,
				"documentSymbolProvider":     true,
				"workspaceSymbolProvider":    true,
			}}})
		case "initialized":
			continue
		case "textDocument/didOpen", "textDocument/didChange", "textDocument/didClose":
			continue
		case "textDocument/diagnostic":
			writeFrame(output, map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(message.ID), "result": map[string]any{"kind": "full", "items": []Diagnostic{diagnostic("fixture error", 0, 0, 1)}}})
		case "textDocument/hover":
			writeFrame(output, map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(message.ID), "result": map[string]any{"contents": "fixture hover"}})
		case "textDocument/completion":
			writeFrame(output, map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(message.ID), "result": map[string]any{"isIncomplete": false, "items": []any{map[string]string{"label": "fixture completion"}}}})
		case "textDocument/codeAction", "textDocument/formatting", "textDocument/definition", "textDocument/references", "textDocument/documentSymbol", "workspace/symbol":
			writeFrame(output, map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(message.ID), "result": []any{}})
		case "textDocument/rename":
			writeFrame(output, map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(message.ID), "result": map[string]any{}})
		case "shutdown":
			writeFrame(output, map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(message.ID), "result": nil})
		case "exit":
			done <- nil
			return
		default:
			done <- io.ErrUnexpectedEOF
			return
		}
	}
}

func writeFrame(writer io.Writer, value any) {
	payload, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	if _, err := writer.Write([]byte("Content-Length: " + strconv.Itoa(len(payload)) + "\r\n\r\n")); err != nil {
		panic(err)
	}
	if _, err := writer.Write(payload); err != nil {
		panic(err)
	}
}

func diagnostic(message string, line, start, end int) Diagnostic {
	var value Diagnostic
	value.Range.Start = Position{Line: line, Character: start}
	value.Range.End = Position{Line: line, Character: end}
	value.Severity = 2
	value.Message = message
	return value
}

func testWorkspace(t *testing.T) workspace.Root {
	t.Helper()
	root, err := workspace.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	return root
}

func writeFile(t *testing.T, root, relative, contents string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(root, relative)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), mode); err != nil {
		t.Fatal(err)
	}
}
