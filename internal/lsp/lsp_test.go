package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
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
			if strings.Contains(result.Content, "gator-secret") {
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
		})
	}
}

func TestWorkspaceSymbolsAcceptURIOnlyLocationAndRejectInvalidQuery(t *testing.T) {
	root := testWorkspace(t)
	writeFile(t, root.Path(), "pkg/target.go", "package pkg\n", 0o600)
	uri := fileURI(filepath.Join(root.Path(), "pkg", "target.go"))
	result, err := formatWorkspaceSymbols(root, "Target", "fixture", json.RawMessage(`[{"name":"Target","kind":5,"location":{"uri":`+strconv.Quote(uri)+`}}]`))
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
	responses  map[string]json.RawMessage
	supported  map[lspOperation]bool
	method     string
	parameters any
	closed     bool
}

func (c *fakeClient) Request(_ context.Context, method string, parameters any) (json.RawMessage, error) {
	c.method = method
	c.parameters = parameters
	return c.responses[method], nil
}

func (c *fakeClient) Supports(operation lspOperation) bool {
	return c.supported == nil || c.supported[operation]
}

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
				"diagnosticProvider":      map[string]any{},
				"hoverProvider":           true,
				"definitionProvider":      true,
				"referencesProvider":      true,
				"documentSymbolProvider":  true,
				"workspaceSymbolProvider": true,
			}}})
		case "initialized":
			continue
		case "textDocument/diagnostic":
			writeFrame(output, map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(message.ID), "result": map[string]any{"kind": "full", "items": []Diagnostic{diagnostic("fixture error", 0, 0, 1)}}})
		case "textDocument/hover":
			writeFrame(output, map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(message.ID), "result": map[string]any{"contents": "fixture hover"}})
		case "textDocument/definition", "textDocument/references", "textDocument/documentSymbol", "workspace/symbol":
			writeFrame(output, map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(message.ID), "result": []any{}})
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
