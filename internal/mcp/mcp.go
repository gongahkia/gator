// Package mcp exposes explicitly trusted Model Context Protocol servers as
// bounded Gator tools. It supports the protocol's stdio and Streamable HTTP
// transports; OAuth is intentionally not synthesized from project config.
package mcp

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
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/sandbox"
	"github.com/gongahkia/gator/internal/workspace"
)

const (
	manifestPath    = ".gator/mcp.json"
	manifestVersion = 1
	maxManifest     = 64 * 1024
	maxServers      = 32
	maxToolOutput   = 64 * 1024
)

var namePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,47}$`)

// Trust is a user-owned record pinning one project MCP configuration bundle.
type Trust struct {
	Repository string `json:"repository"`
	Hash       string `json:"hash"`
}

type manifest struct {
	Version int      `json:"version"`
	Servers []server `json:"servers"`
}

type server struct {
	Name      string   `json:"name"`
	Transport string   `json:"transport"`
	Command   []string `json:"command,omitempty"`
	URL       string   `json:"url,omitempty"`
	Network   string   `json:"network,omitempty"`
}

type client interface {
	Call(context.Context, string, any) (json.RawMessage, error)
	Close() error
}

type toolDescription struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

type loadedServer struct {
	name   string
	client client
	tools  []toolDescription
}

// Set owns connected trusted MCP clients. Always call Close when a run ends.
type Set struct {
	configuredHash string
	trusted        bool
	servers        []loadedServer
}

func (s Set) Configured() bool { return s.configuredHash != "" }
func (s Set) Trusted() bool    { return s.trusted }
func (s Set) Hash() string     { return s.configuredHash }

// Load starts and initializes each trusted server. A valid but untrusted
// project manifest produces an inert set, rather than starting arbitrary code.
func Load(ctx context.Context, worktreePath, trustedHash string) (Set, error) {
	return load(ctx, worktreePath, trustedHash, auth.Store{})
}

// LoadWithCredentials is Load with Gator's private credential store available
// to explicitly trusted Streamable HTTP servers. Credentials are selected only
// by an exact canonical MCP resource identifier.
func LoadWithCredentials(ctx context.Context, worktreePath, trustedHash string, credentials auth.Store) (Set, error) {
	return load(ctx, worktreePath, trustedHash, credentials)
}

func load(ctx context.Context, worktreePath, trustedHash string, credentials auth.Store) (Set, error) {
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
	set := Set{configuredHash: digest}
	if trustedHash == "" || trustedHash != digest {
		return set, nil
	}
	set.trusted = true
	for _, specification := range document.Servers {
		connected, err := connect(ctx, root, specification, credentials)
		if err != nil {
			_ = set.Close()
			return Set{}, fmt.Errorf("connect MCP server %q: %w", specification.Name, err)
		}
		tools, err := listTools(ctx, connected)
		if err != nil {
			_ = connected.Close()
			_ = set.Close()
			return Set{}, fmt.Errorf("list MCP tools from %q: %w", specification.Name, err)
		}
		set.servers = append(set.servers, loadedServer{name: specification.Name, client: connected, tools: tools})
	}
	return set, nil
}

// Tools returns model-callable MCP tools. Each call requires the provided
// explicit approval callback; no MCP operation is auto-approved by default.
func (s Set) Tools(approve func(context.Context, string, string) error) []agent.Tool {
	var result []agent.Tool
	for _, server := range s.servers {
		for _, specification := range server.tools {
			result = append(result, Tool{server: server, specification: specification, approve: approve})
		}
	}
	return result
}

func (s Set) Close() error {
	var first error
	for _, server := range s.servers {
		if err := server.client.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// BundleHash validates and hashes a manifest plus each local stdio executable.
// Streamable HTTP endpoints are pinned by their reviewed manifest entry; remote
// executable identity requires a transport-level trust system not in this repo.
func BundleHash(repository string) (string, error) {
	root, err := workspace.Open(repository)
	if err != nil {
		return "", err
	}
	_, digest, err := loadManifest(root)
	return digest, err
}

func CanonicalRepository(repository string) (string, error) {
	root, err := workspace.Open(repository)
	if err != nil {
		return "", err
	}
	return root.Path(), nil
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
		return manifest{}, "", errors.New("MCP manifest has an unsupported version or too many servers")
	}
	digest := sha256.New()
	_, _ = digest.Write([]byte(manifestPath + "\x00"))
	_, _ = digest.Write(contents)
	seen := make(map[string]struct{}, len(document.Servers))
	local := make(map[string][]byte)
	for index, specification := range document.Servers {
		if err := validateServer(specification); err != nil {
			return manifest{}, "", fmt.Errorf("MCP server %d: %w", index+1, err)
		}
		if _, duplicate := seen[specification.Name]; duplicate {
			return manifest{}, "", fmt.Errorf("MCP server name %q is repeated", specification.Name)
		}
		seen[specification.Name] = struct{}{}
		if specification.Transport == "stdio" {
			path := filepath.ToSlash(specification.Command[0])
			if _, found := local[path]; !found {
				contents, err := executableContents(root, path)
				if err != nil {
					return manifest{}, "", fmt.Errorf("MCP server %q: %w", specification.Name, err)
				}
				local[path] = contents
			}
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
	switch server.Transport {
	case "stdio":
		if len(server.Command) == 0 || len(server.Command) > 32 || filepath.IsAbs(server.Command[0]) || !safeRelative(server.Command[0]) || server.URL != "" {
			return errors.New("stdio server requires a repository-relative command and no URL")
		}
		for _, value := range server.Command {
			if len(value) > 4096 || strings.ContainsRune(value, '\x00') {
				return errors.New("stdio command argument is invalid")
			}
		}
	case "streamable_http":
		if len(server.Command) != 0 || strings.TrimSpace(server.URL) == "" {
			return errors.New("Streamable HTTP server requires a URL and no command")
		}
		endpoint, err := url.Parse(server.URL)
		if err != nil || endpoint.Host == "" || (endpoint.Scheme != "https" && endpoint.Scheme != "http") || endpoint.User != nil {
			return errors.New("Streamable HTTP server requires an absolute http(s) URL")
		}
		if endpoint.Scheme == "http" && endpoint.Hostname() != "127.0.0.1" && endpoint.Hostname() != "localhost" && endpoint.Hostname() != "::1" {
			return errors.New("non-local Streamable HTTP servers require HTTPS")
		}
	default:
		return fmt.Errorf("unsupported MCP transport %q", server.Transport)
	}
	if server.Network != "" && server.Network != "allow" && server.Network != "deny" {
		return fmt.Errorf("unsupported MCP network mode %q", server.Network)
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
		return nil, errors.New("MCP stdio executable must be an executable regular file")
	}
	return root.ReadRegularFile(filepath.FromSlash(relative), maxManifest)
}

func connect(ctx context.Context, root workspace.Root, specification server, credentials auth.Store) (client, error) {
	if specification.Transport == "streamable_http" {
		token, configured, err := accessToken(ctx, credentials, specification.URL, time.Now())
		if err != nil {
			return nil, fmt.Errorf("load OAuth credential for MCP server %q: %w", specification.Name, err)
		}
		return newAuthorizedHTTPClient(specification.URL, token, specification.Name, configured), nil
	}
	policy := sandbox.DefaultPolicy()
	if specification.Network == "allow" {
		policy.Network = sandbox.AllowNetwork
	}
	argv := append([]string{filepath.Join(root.Path(), filepath.FromSlash(specification.Command[0]))}, specification.Command[1:]...)
	prepared, err := sandbox.Prepare(ctx, sandbox.Request{Dir: root.Path(), Argv: argv, Policy: policy})
	if err != nil {
		return nil, err
	}
	stdin, err := prepared.Command.StdinPipe()
	if err != nil {
		prepared.Cleanup()
		return nil, err
	}
	stdout, err := prepared.Command.StdoutPipe()
	if err != nil {
		prepared.Cleanup()
		return nil, err
	}
	if err := prepared.Command.Start(); err != nil {
		prepared.Cleanup()
		return nil, err
	}
	return &stdioClient{command: prepared.Command, input: stdin, output: bufio.NewScanner(stdout), cleanup: prepared.Cleanup}, nil
}

func listTools(ctx context.Context, client client) ([]toolDescription, error) {
	if _, err := client.Call(ctx, "initialize", map[string]any{"protocolVersion": "2025-03-26", "capabilities": map[string]any{}, "clientInfo": map[string]string{"name": "gator", "version": "dev"}}); err != nil {
		return nil, err
	}
	if notifier, ok := client.(interface {
		Notify(context.Context, string, any) error
	}); ok {
		if err := notifier.Notify(ctx, "notifications/initialized", map[string]any{}); err != nil {
			return nil, err
		}
	}
	raw, err := client.Call(ctx, "tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var result struct {
		Tools []toolDescription `json:"tools"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	for _, tool := range result.Tools {
		if !namePattern.MatchString(tool.Name) || len(tool.InputSchema) == 0 || !json.Valid(tool.InputSchema) {
			return nil, errors.New("MCP server returned an invalid tool definition")
		}
	}
	return result.Tools, nil
}

// Tool adapts one MCP remote tool to the agent tool contract.
type Tool struct {
	server        loadedServer
	specification toolDescription
	approve       func(context.Context, string, string) error
}

func (t Tool) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{Name: "mcp_" + strings.ReplaceAll(t.server.name, "-", "_") + "_" + strings.ReplaceAll(t.specification.Name, "-", "_"), Description: "MCP " + t.server.name + ": " + t.specification.Description, Parameters: append(json.RawMessage(nil), t.specification.InputSchema...)}
}
func (t Tool) Execute(ctx context.Context, arguments json.RawMessage) (agent.ToolResult, error) {
	if !json.Valid(arguments) {
		return agent.ToolResult{}, errors.New("MCP tool arguments must be valid JSON")
	}
	if t.approve == nil {
		return agent.ToolResult{}, errors.New("MCP tool requires developer approval")
	}
	if err := t.approve(ctx, t.server.name, t.specification.Name); err != nil {
		return agent.ToolResult{}, err
	}
	raw, err := t.server.client.Call(ctx, "tools/call", map[string]any{"name": t.specification.Name, "arguments": json.RawMessage(arguments)})
	if err != nil {
		return agent.ToolResult{}, err
	}
	if len(raw) > maxToolOutput {
		return agent.ToolResult{}, errors.New("MCP tool response exceeds 64 KiB")
	}
	return agent.ToolResult{Content: string(raw)}, nil
}

type stdioClient struct {
	command *exec.Cmd
	input   io.WriteCloser
	output  *bufio.Scanner
	cleanup func()
	mu      sync.Mutex
	next    int64
}

func (c *stdioClient) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.next++
	return rpcExchange(ctx, method, params, c.next, func(value []byte) error { _, err := c.input.Write(append(value, '\n')); return err }, func() ([]byte, error) {
		if !c.output.Scan() {
			if err := c.output.Err(); err != nil {
				return nil, err
			}
			return nil, io.EOF
		}
		return c.output.Bytes(), nil
	})
}
func (c *stdioClient) Notify(_ context.Context, method string, params any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	value, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
	if err != nil {
		return err
	}
	_, err = c.input.Write(append(value, '\n'))
	return err
}
func (c *stdioClient) Close() error {
	_ = c.input.Close()
	err := c.command.Wait()
	c.cleanup()
	return err
}

type httpClient struct {
	endpoint string
	client   *http.Client
	access   string
	server   string
	oauth    bool
	mu       sync.Mutex
	next     int64
	session  string
}

func newHTTPClient(endpoint string) *httpClient {
	return &httpClient{endpoint: endpoint, client: &http.Client{Timeout: 2 * time.Minute}}
}

func newAuthorizedHTTPClient(endpoint, access, server string, oauth bool) *httpClient {
	client := newHTTPClient(endpoint)
	client.access = access
	client.server = server
	client.oauth = oauth
	return client
}
func (c *httpClient) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.next++
	id := c.next
	value, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(value))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("MCP-Protocol-Version", "2025-03-26")
	if c.access != "" {
		request.Header.Set("Authorization", "Bearer "+c.access)
	}
	if c.session != "" {
		request.Header.Set("Mcp-Session-Id", c.session)
	}
	response, err := c.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode/100 != 2 {
		if response.StatusCode == http.StatusUnauthorized && c.server != "" {
			if c.oauth {
				return nil, fmt.Errorf("MCP HTTP server returned 401 Unauthorized; its stored OAuth credential is invalid or expired, run 'gator mcp login %s'", c.server)
			}
			return nil, fmt.Errorf("MCP HTTP server returned 401 Unauthorized; authenticate it with 'gator mcp login %s'", c.server)
		}
		return nil, fmt.Errorf("MCP HTTP server returned %s", response.Status)
	}
	if session := response.Header.Get("Mcp-Session-Id"); session != "" {
		c.session = session
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxToolOutput+1024))
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(response.Header.Get("Content-Type"), "text/event-stream") {
		body = firstSSEData(body)
	}
	return parseRPCResponse(body, id)
}
func (c *httpClient) Close() error { return nil }

func rpcExchange(ctx context.Context, method string, params any, id int64, send func([]byte) error, receive func() ([]byte, error)) (json.RawMessage, error) {
	value, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	if err != nil {
		return nil, err
	}
	if err := send(value); err != nil {
		return nil, err
	}
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		response, err := receive()
		if err != nil {
			return nil, err
		}
		raw, err := parseRPCResponse(response, id)
		if errors.Is(err, errOtherResponse) {
			continue
		}
		return raw, err
	}
}

var errOtherResponse = errors.New("other RPC response")

func parseRPCResponse(value []byte, id int64) (json.RawMessage, error) {
	var response struct {
		ID     any             `json:"id"`
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(value, &response); err != nil {
		return nil, err
	}
	number, ok := response.ID.(float64)
	if !ok || int64(number) != id {
		return nil, errOtherResponse
	}
	if response.Error != nil {
		return nil, fmt.Errorf("MCP RPC error %d: %s", response.Error.Code, response.Error.Message)
	}
	if len(response.Result) == 0 {
		return nil, errors.New("MCP RPC response has no result")
	}
	return response.Result, nil
}
func firstSSEData(value []byte) []byte {
	for _, line := range bytes.Split(value, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if bytes.HasPrefix(line, []byte("data:")) {
			return bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
		}
	}
	return value
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
