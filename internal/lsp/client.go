package lsp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/gongahkia/gator/internal/sandbox"
	"github.com/gongahkia/gator/internal/workspace"
)

type nativeClient struct {
	input        io.WriteCloser
	output       *bufio.Reader
	command      interface{ Wait() error }
	kill         func() error
	cleanup      func()
	next         int64
	support      map[lspOperation]bool
	documentSync int
}

func connect(ctx context.Context, roots workspace.RootSet, specification server) (client, error) {
	root := roots.Primary()
	policy := sandbox.DefaultPolicy()
	if specification.Network == "allow" {
		policy.Network = sandbox.AllowNetwork
	}
	for _, additional := range roots.Additional() {
		policy.ReadOnlyRoots = append(policy.ReadOnlyRoots, additional.Path())
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
	if err := client.initialize(ctx, root.Path(), roots.Additional()...); err != nil {
		_ = client.Close()
		return nil, err
	}
	return client, nil
}

func (c *nativeClient) initialize(ctx context.Context, root string, additional ...workspace.Root) error {
	uri := fileURI(root)
	folders := []map[string]string{{"uri": uri, "name": filepath.Base(root)}}
	for _, extra := range additional {
		folders = append(folders, map[string]string{"uri": fileURI(extra.Path()), "name": filepath.Base(extra.Path())})
	}
	result, err := c.call(ctx, "initialize", map[string]any{
		"processId":        os.Getpid(),
		"clientInfo":       map[string]string{"name": "gator", "version": "dev"},
		"rootUri":          uri,
		"workspaceFolders": folders,
		"capabilities": map[string]any{
			"workspace": map[string]any{
				"configuration":    true,
				"workspaceFolders": true,
				"symbol":           map[string]any{"dynamicRegistration": false},
			},
			"textDocument": map[string]any{
				"diagnostic":     map[string]any{"dynamicRegistration": false},
				"hover":          map[string]any{"dynamicRegistration": false, "contentFormat": []string{"plaintext", "markdown"}},
				"completion":     map[string]any{"dynamicRegistration": false, "completionItem": map[string]any{"documentationFormat": []string{"plaintext", "markdown"}}},
				"codeAction":     map[string]any{"dynamicRegistration": false, "isPreferredSupport": true, "disabledSupport": true, "dataSupport": false},
				"formatting":     map[string]any{"dynamicRegistration": false},
				"rename":         map[string]any{"dynamicRegistration": false, "prepareSupport": false},
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
			DiagnosticProvider         json.RawMessage `json:"diagnosticProvider"`
			HoverProvider              json.RawMessage `json:"hoverProvider"`
			CompletionProvider         json.RawMessage `json:"completionProvider"`
			CodeActionProvider         json.RawMessage `json:"codeActionProvider"`
			DocumentFormattingProvider json.RawMessage `json:"documentFormattingProvider"`
			RenameProvider             json.RawMessage `json:"renameProvider"`
			DefinitionProvider         json.RawMessage `json:"definitionProvider"`
			ReferencesProvider         json.RawMessage `json:"referencesProvider"`
			DocumentSymbolProvider     json.RawMessage `json:"documentSymbolProvider"`
			WorkspaceSymbolProvider    json.RawMessage `json:"workspaceSymbolProvider"`
			TextDocumentSync           json.RawMessage `json:"textDocumentSync"`
		} `json:"capabilities"`
	}
	if err := json.Unmarshal(result, &response); err != nil {
		return fmt.Errorf("decode initialize response: %w", err)
	}
	c.support = map[lspOperation]bool{
		diagnosticsOperation:      capabilityEnabled(response.Capabilities.DiagnosticProvider),
		hoverOperation:            capabilityEnabled(response.Capabilities.HoverProvider),
		completionOperation:       capabilityEnabled(response.Capabilities.CompletionProvider),
		codeActionsOperation:      capabilityEnabled(response.Capabilities.CodeActionProvider),
		formatOperation:           capabilityEnabled(response.Capabilities.DocumentFormattingProvider),
		renameOperation:           capabilityEnabled(response.Capabilities.RenameProvider),
		definitionOperation:       capabilityEnabled(response.Capabilities.DefinitionProvider),
		referencesOperation:       capabilityEnabled(response.Capabilities.ReferencesProvider),
		documentSymbolsOperation:  capabilityEnabled(response.Capabilities.DocumentSymbolProvider),
		workspaceSymbolsOperation: capabilityEnabled(response.Capabilities.WorkspaceSymbolProvider),
	}
	c.documentSync = documentSyncKind(response.Capabilities.TextDocumentSync)
	return c.notify("initialized", map[string]any{})
}

func capabilityEnabled(value json.RawMessage) bool {
	return len(value) != 0 && string(value) != "false" && string(value) != "null"
}

// documentSyncKind accepts the two LSP ServerCapabilities forms: a numeric
// TextDocumentSyncKind or TextDocumentSyncOptions with its change kind.
func documentSyncKind(raw json.RawMessage) int {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return 0
	}
	var kind int
	if json.Unmarshal(raw, &kind) == nil {
		if kind == 1 || kind == 2 {
			return kind
		}
		return 0
	}
	var options struct {
		Change int `json:"change"`
	}
	if json.Unmarshal(raw, &options) == nil && (options.Change == 1 || options.Change == 2) {
		return options.Change
	}
	return 0
}

func (c *nativeClient) Supports(operation lspOperation) bool {
	return c.support[operation]
}

func (c *nativeClient) DocumentSyncKind() int { return c.documentSync }

func (c *nativeClient) Request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	return c.call(ctx, method, params)
}

func (c *nativeClient) Notify(method string, params any) error {
	return c.notify(method, params)
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
