// Package lsp exposes explicitly trusted local Language Server Protocol
// code-intelligence servers as bounded Gator tools.
package lsp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/workspace"
)

const (
	manifestPath                    = ".gator/lsp.json"
	manifestVersion                 = 1
	maxManifest                     = 64 * 1024
	maxExecutable                   = 128 * 1024 * 1024
	maxServers                      = 32
	maxFrame                        = 256 * 1024
	maxDiagnostics                  = 128
	maxLocations                    = 128
	maxSymbols                      = 128
	maxCompletions                  = 128
	maxCodeActions                  = 64
	maxCodeActionEdits              = 128
	maxHoverBytes                   = 32 * 1024
	maxCompletionLabelBytes         = 512
	maxCompletionDetailBytes        = 512
	maxCompletionDocumentationBytes = 2 * 1024
	maxCompletionInsertTextBytes    = 512
	maxCodeActionTitleBytes         = 512
	maxCodeActionKindBytes          = 256
	maxCodeActionDisabledBytes      = 1024
	maxCodeActionNewTextBytes       = 16 * 1024
	maxDocumentSyncBytes            = 128 * 1024
	maxRenameNameBytes              = 256
	maxSymbolQuery                  = 512
	maxToolOutput                   = 64 * 1024
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
// until a model requests a supported lookup and the developer approves it.
type Set struct {
	configuredHash string
	trusted        bool
	root           workspace.Root
	additional     []workspace.Root
	servers        []server
}

func (s Set) Configured() bool { return s.configuredHash != "" }
func (s Set) Trusted() bool    { return s.trusted }
func (s Set) Hash() string     { return s.configuredHash }

// WithAdditionalRoots returns a copy that may inspect the supplied
// read-only directories. They do not change manifest trust or grant patch or
// command authority.
func (s Set) WithAdditionalRoots(roots []workspace.Root) Set {
	s.additional = workspace.NewRootSet(s.root, roots).Additional()
	return s
}

func (s Set) rootSet() workspace.RootSet {
	return workspace.NewRootSet(s.root, s.additional)
}

func (s Set) rootKey() string {
	return strings.Join(s.rootSet().Paths(), "\x00")
}

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
		return Set{root: root}, nil
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
			result = append(result, Tool{root: s.root, roots: s.rootSet(), specification: specification, operation: operation, approve: approve, connectRoots: connect})
		}
	}
	return result
}

// Manager lazily owns at most one sandboxed client for each trusted server.
// Tool calls remain individually approved. A Registry may retain the manager
// for compatible later native-session runs, never across worktrees.
type Manager struct {
	root         workspace.Root
	roots        workspace.RootSet
	trusted      bool
	servers      []server
	connect      func(context.Context, workspace.Root, server) (client, error)
	connectRoots func(context.Context, workspace.RootSet, server) (client, error)

	mu        sync.Mutex
	requestMu sync.Mutex
	closed    bool
	clients   map[string]client
	documents map[string]documentState
}

type documentState struct {
	server  string
	path    string
	uri     string
	version int
	digest  [sha256.Size]byte
}

// NewManager constructs a per-run client manager. Call Close when the run
// exits so every lazily started language server is terminated.
func (s Set) NewManager() *Manager {
	return &Manager{
		root:         s.root,
		roots:        s.rootSet(),
		trusted:      s.trusted,
		servers:      append([]server(nil), s.servers...),
		connectRoots: connect,
		clients:      make(map[string]client),
		documents:    make(map[string]documentState),
	}
}

// ServerInspect is a process-start snapshot for one configured server. It
// never contacts the language server.
type ServerInspect struct {
	Name     string
	Language string
	Started  bool
}

// Inspect copies configured servers and whether a client has already been
// started. It must not call connect or Tools.
func (m *Manager) Inspect() []ServerInspect {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]ServerInspect, 0, len(m.servers))
	for _, specification := range m.servers {
		result = append(result, ServerInspect{
			Name:     specification.Name,
			Language: specification.Language,
			Started:  m.clients[specification.Name] != nil,
		})
	}
	return result
}

func (m *Manager) rootSet() workspace.RootSet {
	if m.roots.Primary().Path() != "" {
		return m.roots
	}
	return workspace.NewRootSet(m.root, nil)
}

// Tools returns this manager's read-only tool surface. An untrusted or empty
// set has no model-visible tools.
func (m *Manager) Tools(approve func(context.Context, string, string, string) error) []agent.Tool {
	if m == nil || !m.trusted {
		return nil
	}
	result := make([]agent.Tool, 0, len(m.servers)*len(lspOperations))
	for _, specification := range m.servers {
		for _, operation := range lspOperations {
			result = append(result, Tool{roots: m.rootSet(), specification: specification, operation: operation, approve: approve, manager: m})
		}
	}
	return result
}

func (m *Manager) connection(ctx context.Context, specification server) (client, error) {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil, errors.New("LSP manager is closed")
	}
	if existing := m.clients[specification.Name]; existing != nil {
		m.mu.Unlock()
		return existing, nil
	}
	connect := m.connect
	connectRoots := m.connectRoots
	m.mu.Unlock()
	if connect == nil && connectRoots == nil {
		return nil, errors.New("LSP connection is not configured")
	}
	var opened client
	var err error
	if connectRoots != nil {
		opened, err = connectRoots(ctx, m.rootSet(), specification)
	} else {
		opened, err = connect(ctx, m.rootSet().Primary(), specification)
	}
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		_ = opened.Close()
		return nil, errors.New("LSP manager is closed")
	}
	if existing := m.clients[specification.Name]; existing != nil {
		m.mu.Unlock()
		_ = opened.Close()
		return existing, nil
	}
	m.clients[specification.Name] = opened
	m.mu.Unlock()
	return opened, nil
}

// request serializes access to one language-server connection. JSON-RPC IDs
// and framed responses share one transport, so concurrent tool calls cannot
// safely interleave them.
func (m *Manager) request(ctx context.Context, specification server, operation lspOperation, path, method string, parameters any) (json.RawMessage, error) {
	m.requestMu.Lock()
	defer m.requestMu.Unlock()
	connection, err := m.connection(ctx, specification)
	if err != nil {
		return nil, err
	}
	if !connection.Supports(operation) {
		return nil, fmt.Errorf("LSP server %q does not support %s", specification.Name, operation)
	}
	if path != "" {
		if err := m.syncDocument(connection, specification, path); err != nil {
			m.discard(specification.Name)
			return nil, err
		}
	}
	response, err := connection.Request(ctx, method, parameters)
	if err != nil {
		m.discard(specification.Name)
	}
	return response, err
}

func (m *Manager) syncDocument(connection client, specification server, path string) error {
	if connection.DocumentSyncKind() == 0 {
		return nil
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read LSP synchronized document: %w", err)
	}
	if len(contents) > maxDocumentSyncBytes {
		return fmt.Errorf("LSP synchronized document exceeds the %d KiB limit", maxDocumentSyncBytes/1024)
	}
	digest := sha256.Sum256(contents)
	key := specification.Name + "\x00" + path
	if m.documents == nil {
		m.documents = make(map[string]documentState)
	}
	state, opened := m.documents[key]
	if opened && state.digest == digest {
		return nil
	}
	if !opened {
		state = documentState{server: specification.Name, path: path, uri: fileURI(path), version: 1, digest: digest}
		if err := connection.Notify("textDocument/didOpen", map[string]any{"textDocument": map[string]any{
			"uri": state.uri, "languageId": specification.Language, "version": state.version, "text": string(contents),
		}}); err != nil {
			return fmt.Errorf("open synchronized LSP document: %w", err)
		}
	} else {
		state.version++
		state.digest = digest
		if err := connection.Notify("textDocument/didChange", map[string]any{
			"textDocument":   map[string]any{"uri": state.uri, "version": state.version},
			"contentChanges": []map[string]any{{"text": string(contents)}},
		}); err != nil {
			return fmt.Errorf("change synchronized LSP document: %w", err)
		}
	}
	m.documents[key] = state
	return nil
}

// Discard drops an unhealthy client after an I/O failure so a later approved
// lookup can create a fresh sandboxed process instead of reusing bad state.
func (m *Manager) Discard(name string) {
	if m == nil {
		return
	}
	m.requestMu.Lock()
	defer m.requestMu.Unlock()
	m.discard(name)
}

func (m *Manager) discard(name string) {
	m.mu.Lock()
	connection := m.clients[name]
	delete(m.clients, name)
	m.mu.Unlock()
	if connection != nil {
		m.closeDocuments(name, connection)
		_ = connection.Close()
	}
}

func (m *Manager) closeDocuments(server string, connection client) {
	if connection == nil {
		return
	}
	for key, state := range m.documents {
		if state.server != server {
			continue
		}
		if connection.DocumentSyncKind() != 0 {
			_ = connection.Notify("textDocument/didClose", map[string]any{"textDocument": map[string]any{"uri": state.uri}})
		}
		delete(m.documents, key)
	}
}

// Close stops every client started for this run. The first shutdown failure is
// returned after all clients have been given a chance to exit.
func (m *Manager) Close() error {
	if m == nil {
		return nil
	}
	m.requestMu.Lock()
	defer m.requestMu.Unlock()
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	type namedClient struct {
		name       string
		connection client
	}
	connections := make([]namedClient, 0, len(m.clients))
	for name, connection := range m.clients {
		connections = append(connections, namedClient{name: name, connection: connection})
	}
	m.clients = make(map[string]client)
	m.mu.Unlock()
	var first error
	for _, item := range connections {
		m.closeDocuments(item.name, item.connection)
		if err := item.connection.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
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
