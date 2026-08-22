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

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/sandbox"
	"github.com/gongahkia/gator/internal/workspace"
)

const (
	manifestPath    = ".gator/lsp.json"
	manifestVersion = 1
	maxManifest     = 64 * 1024
	maxServers      = 32
	maxFrame        = 256 * 1024
	maxDiagnostics  = 128
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

// Tools returns one diagnostic tool for every trusted server. The approval
// callback is invoked before starting a server because an LSP executable is a
// process-capability boundary even though diagnostics are read-only output.
func (s Set) Tools(approve func(context.Context, string, string) error) []agent.Tool {
	if !s.trusted {
		return nil
	}
	result := make([]agent.Tool, 0, len(s.servers))
	for _, specification := range s.servers {
		result = append(result, Tool{root: s.root, specification: specification, approve: approve, connect: connect})
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
	local := make(map[string][]byte)
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
			executable, err := executableContents(root, path)
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
		_, _ = digest.Write(local[path])
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

func executableContents(root workspace.Root, relative string) ([]byte, error) {
	resolved, err := root.ResolveFile(filepath.FromSlash(relative))
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(resolved)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
		return nil, errors.New("LSP executable must be an executable regular file")
	}
	return root.ReadRegularFile(filepath.FromSlash(relative), maxManifest)
}

// Tool adapts one local LSP server to the agent tool contract.
type Tool struct {
	root          workspace.Root
	specification server
	approve       func(context.Context, string, string) error
	connect       func(context.Context, workspace.Root, server) (diagnosticClient, error)
}

func (t Tool) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{
		Name:        "lsp_" + strings.ReplaceAll(t.specification.Name, "-", "_") + "_diagnostics",
		Description: "Diagnostic-only LSP inspection using " + t.specification.Name + " (" + t.specification.Language + "). The path must be a workspace-relative source file.",
		Parameters:  json.RawMessage(`{"type":"object","additionalProperties":false,"required":["path"],"properties":{"path":{"type":"string","description":"Workspace-relative source-file path"}}}`),
	}
}

func (t Tool) Execute(ctx context.Context, arguments json.RawMessage) (agent.ToolResult, error) {
	var params struct {
		Path string `json:"path"`
	}
	if err := decodeArguments(arguments, &params); err != nil {
		return agent.ToolResult{}, err
	}
	resolved, err := t.root.ResolveFile(params.Path)
	if err != nil {
		return agent.ToolResult{}, err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return agent.ToolResult{}, fmt.Errorf("stat LSP diagnostic path: %w", err)
	}
	if !info.Mode().IsRegular() {
		return agent.ToolResult{}, errors.New("LSP diagnostics require a regular source file")
	}
	if t.approve == nil {
		return agent.ToolResult{}, errors.New("LSP diagnostics require developer approval")
	}
	if err := t.approve(ctx, t.specification.Name, params.Path); err != nil {
		return agent.ToolResult{}, err
	}
	if t.connect == nil {
		return agent.ToolResult{}, errors.New("LSP diagnostic connection is not configured")
	}
	client, err := t.connect(ctx, t.root, t.specification)
	if err != nil {
		return agent.ToolResult{}, fmt.Errorf("start LSP server %q: %w", t.specification.Name, err)
	}
	defer client.Close()
	diagnostics, err := client.Diagnostics(ctx, resolved)
	if err != nil {
		return agent.ToolResult{}, fmt.Errorf("request diagnostics from LSP server %q: %w", t.specification.Name, err)
	}
	return formatDiagnostics(params.Path, t.specification.Name, diagnostics)
}

type diagnosticClient interface {
	Diagnostics(context.Context, string) ([]Diagnostic, error)
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
	encoded, err := json.Marshal(result)
	if err != nil {
		return agent.ToolResult{}, err
	}
	if len(encoded) > maxToolOutput {
		return agent.ToolResult{}, errors.New("LSP diagnostic response exceeds 64 KiB")
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
	cleanup func()
	next    int64
}

func connect(ctx context.Context, root workspace.Root, specification server) (diagnosticClient, error) {
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
	client := &nativeClient{input: input, output: bufio.NewReaderSize(output, maxFrame), command: prepared.Command, cleanup: prepared.Cleanup}
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
			"workspace":    map[string]any{"configuration": true, "workspaceFolders": true},
			"textDocument": map[string]any{"diagnostic": map[string]any{"dynamicRegistration": false}},
		},
	})
	if err != nil {
		return err
	}
	var response struct {
		Capabilities struct {
			DiagnosticProvider json.RawMessage `json:"diagnosticProvider"`
		} `json:"capabilities"`
	}
	if err := json.Unmarshal(result, &response); err != nil {
		return fmt.Errorf("decode initialize response: %w", err)
	}
	if len(response.Capabilities.DiagnosticProvider) == 0 || string(response.Capabilities.DiagnosticProvider) == "false" || string(response.Capabilities.DiagnosticProvider) == "null" {
		return errors.New("LSP server does not support pull diagnostics")
	}
	return c.notify("initialized", map[string]any{})
}

func (c *nativeClient) Diagnostics(ctx context.Context, path string) ([]Diagnostic, error) {
	result, err := c.call(ctx, "textDocument/diagnostic", map[string]any{"textDocument": map[string]string{"uri": fileURI(path)}})
	if err != nil {
		return nil, err
	}
	var response struct {
		Kind  string       `json:"kind"`
		Items []Diagnostic `json:"items"`
	}
	if err := json.Unmarshal(result, &response); err != nil {
		return nil, fmt.Errorf("decode diagnostic response: %w", err)
	}
	if response.Kind != "full" && response.Kind != "unchanged" {
		return nil, errors.New("LSP server returned an unsupported diagnostic report")
	}
	return response.Items, nil
}

func (c *nativeClient) Close() error {
	_, _ = c.call(context.Background(), "shutdown", nil)
	_ = c.notify("exit", nil)
	_ = c.input.Close()
	err := c.command.Wait()
	c.cleanup()
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
