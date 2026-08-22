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
	if !trusted.Trusted() || len(trusted.Tools(func(context.Context, string, string) error { return nil })) != 1 {
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
	client := &fakeClient{diagnostics: []Diagnostic{diagnostic("broken declaration", 3, 2, 4)}}
	approved := ""
	tool := Tool{
		root:          root,
		specification: server{Name: "fixture", Language: "go"},
		approve: func(_ context.Context, server, path string) error {
			approved = server + ":" + path
			return nil
		},
		connect: func(context.Context, workspace.Root, server) (diagnosticClient, error) { return client, nil },
	}

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"path":"pkg/example.go"}`))
	if err != nil {
		t.Fatalf("execute diagnostic tool: %v", err)
	}
	if approved != "fixture:pkg/example.go" || client.path == "" || !client.closed {
		t.Fatalf("approval=%q client=%#v", approved, client)
	}
	if !strings.Contains(result.Content, `"start_line":4`) || !strings.Contains(result.Content, `"severity":"warning"`) || !strings.Contains(result.Content, "broken declaration") {
		t.Fatalf("diagnostic result = %s", result.Content)
	}
}

func TestToolRejectsUnapprovedLookup(t *testing.T) {
	root := testWorkspace(t)
	writeFile(t, root.Path(), "pkg/example.go", "package pkg\n", 0o600)
	tool := Tool{root: root, specification: server{Name: "fixture", Language: "go"}}
	_, err := tool.Execute(context.Background(), json.RawMessage(`{"path":"pkg/example.go"}`))
	if err == nil || !strings.Contains(err.Error(), "developer approval") {
		t.Fatalf("unapproved diagnostic error = %v", err)
	}
}

func TestNativeClientUsesPullDiagnostics(t *testing.T) {
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
	diagnostics, err := client.Diagnostics(context.Background(), "/fixture/pkg/example.go")
	if err != nil {
		t.Fatalf("request diagnostics: %v", err)
	}
	if len(diagnostics) != 1 || diagnostics[0].Message != "fixture error" {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("close LSP client: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("fixture LSP: %v", err)
	}
}

type fakeClient struct {
	diagnostics []Diagnostic
	path        string
	closed      bool
}

func (c *fakeClient) Diagnostics(_ context.Context, path string) ([]Diagnostic, error) {
	c.path = path
	return c.diagnostics, nil
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
			writeFrame(output, map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(message.ID), "result": map[string]any{"capabilities": map[string]any{"diagnosticProvider": map[string]any{}}}})
		case "initialized":
			continue
		case "textDocument/diagnostic":
			writeFrame(output, map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(message.ID), "result": map[string]any{"kind": "full", "items": []Diagnostic{diagnostic("fixture error", 0, 0, 1)}}})
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
