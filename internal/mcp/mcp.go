// Package mcp exposes explicitly trusted Model Context Protocol servers as
// bounded Gator tools. It supports the protocol's stdio and Streamable HTTP
// transports; OAuth is intentionally not synthesized from project config.
package mcp

import (
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
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/auth"
	"github.com/gongahkia/gator/internal/sandbox"
	"github.com/gongahkia/gator/internal/workspace"
)

const (
	manifestPath    = ".gator/mcp.json"
	manifestVersion = 1
	protocolVersion = "2025-11-25"
	modernVersion   = "2026-07-28"
	maxManifest     = 64 * 1024
	maxExecutable   = 128 * 1024 * 1024
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

// InvokeRemoteTool performs one bounded call against an explicitly configured
// Streamable HTTP MCP endpoint. Connector policy and approvals remain owned by
// the caller; this function only handles the protocol transport.
func InvokeRemoteTool(ctx context.Context, endpoint, token, serverName, toolName string, arguments json.RawMessage) (json.RawMessage, error) {
	if !namePattern.MatchString(serverName) || !toolNamePattern.MatchString(toolName) || !objectJSON(arguments) {
		return nil, errors.New("remote MCP call identity or arguments are invalid")
	}
	client := newAuthorizedHTTPClient(endpoint, token)
	defer client.Close()
	tools, err := listTools(ctx, client)
	if err != nil {
		return nil, err
	}
	found := false
	for _, tool := range tools {
		found = found || tool.Name == toolName
	}
	if !found {
		return nil, fmt.Errorf("MCP server %q does not expose mapped tool %q", serverName, toolName)
	}
	result, err := client.Call(ctx, "tools/call", map[string]any{"name": toolName, "arguments": arguments})
	if err != nil {
		return nil, err
	}
	if len(result) > maxToolOutput {
		return nil, errors.New("MCP tool response exceeds 64 KiB")
	}
	var envelope struct {
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(result, &envelope); err != nil {
		return nil, errors.New("MCP tool returned an invalid result")
	}
	if envelope.IsError {
		return nil, fmt.Errorf("MCP server %q reported that mapped tool %q failed", serverName, toolName)
	}
	return result, nil
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
	aliases := map[string]bool{}
	totalTools := 0
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
		totalTools += len(tools)
		for _, tool := range tools {
			alias := toolAlias(specification.Name, tool.Name)
			if aliases[alias] {
				_ = connected.Close()
				_ = set.Close()
				return Set{}, errors.New("MCP tool aliases collide across configured servers")
			}
			aliases[alias] = true
		}
		if totalTools > maxTools {
			_ = connected.Close()
			_ = set.Close()
			return Set{}, errors.New("MCP configuration exceeds 256 total tools")
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
	local := make(map[string]string)
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
				contents, err := executableHash(root, path)
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
		_, _ = digest.Write([]byte(local[path]))
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

func executableHash(root workspace.Root, relative string) (string, error) {
	resolved, err := root.ResolveFile(filepath.FromSlash(relative))
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(resolved)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
		return "", errors.New("MCP stdio executable must be an executable regular file")
	}
	return root.SHA256RegularFile(filepath.FromSlash(relative), maxExecutable)
}

func connect(ctx context.Context, root workspace.Root, specification server, credentials auth.Store) (client, error) {
	if specification.Transport == "streamable_http" {
		policy, err := sandbox.EffectivePolicy(ctx, sandbox.Policy{Network: sandbox.AllowNetwork})
		if err != nil {
			return nil, err
		}
		if policy.Network != sandbox.AllowNetwork {
			return nil, errors.New("Work policy denies remote MCP network access")
		}
		token, _, err := accessToken(ctx, credentials, specification.URL, time.Now())
		if err != nil {
			return nil, fmt.Errorf("load OAuth credential for MCP server %q: %w", specification.Name, err)
		}
		return newAuthorizedHTTPClient(specification.URL, token), nil
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
	return startStdio(ctx, prepared.Command, prepared.Cleanup)
}

// Tool adapts one MCP remote tool to the agent tool contract.
type Tool struct {
	server        loadedServer
	specification toolDescription
	approve       func(context.Context, string, string) error
}

func (t Tool) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{Name: toolAlias(t.server.name, t.specification.Name), Description: "MCP " + t.server.name + ": " + t.specification.Description, Parameters: append(json.RawMessage(nil), t.specification.InputSchema...)}
}
func (t Tool) Execute(ctx context.Context, arguments json.RawMessage) (agent.ToolResult, error) {
	if !objectJSON(arguments) {
		return agent.ToolResult{}, errors.New("MCP tool arguments must be a JSON object")
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
	if err := validateToolResult(raw); err != nil {
		return agent.ToolResult{}, err
	}
	return agent.ToolResult{Content: string(raw)}, nil
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
