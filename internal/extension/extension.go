// Package extension loads Gator extensions from an explicit user install or a
// trusted repository. Extensions use manifests and a JSON sidecar protocol so
// they are independent of Gator's implementation language.
package extension

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/workspace"
)

const (
	manifestName         = "gator-extension.json"
	manifestVersion      = 1
	maxManifestBytes     = 64 * 1024
	maxExtensionBytes    = 8 * 1024 * 1024
	maxInstructionBytes  = 128 * 1024
	maxToolOutputBytes   = 64 * 1024
	defaultToolTimeout   = time.Minute
	maximumToolTimeout   = 2 * time.Minute
	maximumExtensionName = 64
)

var extensionIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)
var toolNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,47}$`)

// Manifest declares every extension resource and executable tool. Resource
// paths are exact, relative paths; globs and implicit startup hooks are not
// supported so an extension's effect is inspectable.
type Manifest struct {
	Version     int            `json:"version"`
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Skills      []string       `json:"skills,omitempty"`
	Prompts     []string       `json:"prompts,omitempty"`
	Tools       []ToolManifest `json:"tools,omitempty"`
}

// ToolManifest defines one model-callable JSONL sidecar. The command is argv,
// never a shell string.
type ToolManifest struct {
	Name           string          `json:"name"`
	Description    string          `json:"description"`
	Parameters     json.RawMessage `json:"parameters"`
	Command        []string        `json:"command"`
	TimeoutSeconds int             `json:"timeout_seconds,omitempty"`
}

// Installed is a validated manifest tied to its non-symlinked root.
type Installed struct {
	Manifest Manifest
	Root     string
	Project  bool
}

// Set is the extension surface prepared for one native run.
type Set struct {
	extensions []Installed
}

// Empty reports whether the set has no installed or trusted project extension.
func (s Set) Empty() bool { return len(s.extensions) == 0 }

// Instructions reads only declared skill and prompt files. It labels each
// source so developer-installed guidance is distinguishable from repository
// instructions.
func (s Set) Instructions() (string, error) {
	var sections []string
	total := 0
	for _, installed := range s.extensions {
		resources := append(append([]string(nil), installed.Manifest.Skills...), installed.Manifest.Prompts...)
		for _, resource := range resources {
			contents, err := readResource(installed.Root, resource)
			if err != nil {
				return "", fmt.Errorf("load extension %q resource %q: %w", installed.Manifest.ID, resource, err)
			}
			total += len(contents)
			if total > maxInstructionBytes {
				return "", fmt.Errorf("enabled extension instructions exceed the %d-byte limit", maxInstructionBytes)
			}
			sections = append(sections, "Extension "+installed.Manifest.ID+" guidance (developer installed or explicitly trusted):\n"+strings.TrimSpace(string(contents)))
		}
	}
	return strings.Join(sections, "\n\n"), nil
}

// Tools exposes extension sidecars only in Execute mode. The sidecar receives
// an explicit JSON request over stdin and one bounded JSON response on stdout.
func (s Set) Tools(root workspace.Root) []agent.Tool {
	var result []agent.Tool
	for _, installed := range s.extensions {
		for _, specification := range installed.Manifest.Tools {
			result = append(result, SidecarTool{installed: installed, specification: specification, root: root})
		}
	}
	return result
}

// SidecarTool is one manifest-declared extension tool.
type SidecarTool struct {
	installed     Installed
	specification ToolManifest
	root          workspace.Root
}

func (t SidecarTool) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{
		Name:        externalToolName(t.installed.Manifest.ID, t.specification.Name),
		Description: "Extension " + t.installed.Manifest.ID + ": " + t.specification.Description,
		Parameters:  append(json.RawMessage(nil), t.specification.Parameters...),
	}
}

func (t SidecarTool) Execute(ctx context.Context, arguments json.RawMessage) (agent.ToolResult, error) {
	if !json.Valid(arguments) {
		return agent.ToolResult{}, errors.New("extension tool arguments must be valid JSON")
	}
	commandPath, err := commandPath(t.installed.Root, t.specification.Command[0])
	if err != nil {
		return agent.ToolResult{}, err
	}
	timeout := defaultToolTimeout
	if t.specification.TimeoutSeconds > 0 {
		timeout = time.Duration(t.specification.TimeoutSeconds) * time.Second
	}
	if timeout > maximumToolTimeout {
		timeout = maximumToolTimeout
	}
	request, err := json.Marshal(struct {
		Version    int             `json:"version"`
		Method     string          `json:"method"`
		Extension  string          `json:"extension"`
		Tool       string          `json:"tool"`
		Arguments  json.RawMessage `json:"arguments"`
		Repository string          `json:"repository"`
		Worktree   string          `json:"worktree"`
	}{
		Version: 1, Method: "tool", Extension: t.installed.Manifest.ID, Tool: t.specification.Name,
		Arguments: arguments, Repository: t.root.Repository(), Worktree: t.root.Path(),
	})
	if err != nil {
		return agent.ToolResult{}, fmt.Errorf("encode extension request: %w", err)
	}
	runContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	output := &boundedBuffer{limit: maxToolOutputBytes}
	command := exec.CommandContext(runContext, commandPath, t.specification.Command[1:]...)
	command.Dir = t.root.Path()
	command.Stdin = bytes.NewReader(request)
	command.Stdout = output
	command.Stderr = output
	if err := command.Run(); err != nil {
		if errors.Is(runContext.Err(), context.DeadlineExceeded) {
			return agent.ToolResult{}, fmt.Errorf("extension %q tool %q timed out", t.installed.Manifest.ID, t.specification.Name)
		}
		message := strings.TrimSpace(output.String())
		if output.truncated {
			message += " (output truncated)"
		}
		if message == "" {
			return agent.ToolResult{}, fmt.Errorf("extension %q tool %q failed: %w", t.installed.Manifest.ID, t.specification.Name, err)
		}
		return agent.ToolResult{}, fmt.Errorf("extension %q tool %q failed: %s", t.installed.Manifest.ID, t.specification.Name, message)
	}
	if output.truncated {
		return agent.ToolResult{}, fmt.Errorf("extension %q tool %q response exceeds %d bytes", t.installed.Manifest.ID, t.specification.Name, maxToolOutputBytes)
	}
	var response struct {
		Content string `json:"content"`
	}
	decoder := json.NewDecoder(strings.NewReader(output.String()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&response); err != nil {
		return agent.ToolResult{}, fmt.Errorf("decode extension %q tool response: %w", t.installed.Manifest.ID, err)
	}
	if err := requireEOF(decoder); err != nil {
		return agent.ToolResult{}, fmt.Errorf("decode extension %q tool response: %w", t.installed.Manifest.ID, err)
	}
	if len(response.Content) > maxToolOutputBytes {
		return agent.ToolResult{}, fmt.Errorf("extension %q tool %q content exceeds %d bytes", t.installed.Manifest.ID, t.specification.Name, maxToolOutputBytes)
	}
	return agent.ToolResult{Content: response.Content}, nil
}

type boundedBuffer struct {
	buffer    bytes.Buffer
	limit     int
	truncated bool
}

func (b *boundedBuffer) Write(value []byte) (int, error) {
	remaining := b.limit - b.buffer.Len()
	if remaining <= 0 {
		b.truncated = true
		return len(value), nil
	}
	if len(value) > remaining {
		_, _ = b.buffer.Write(value[:remaining])
		b.truncated = true
		return len(value), nil
	}
	_, _ = b.buffer.Write(value)
	return len(value), nil
}

func (b *boundedBuffer) String() string { return b.buffer.String() }

// Resolver determines the enabled global extensions and repository extensions
// trusted by the developer in config.json.
type Resolver struct {
	store   Store
	enabled map[string]bool
	trusted map[string]bool
}

// DefaultResolver creates a resolver from one user settings document.
func DefaultResolver(settings config.Settings) (Resolver, error) {
	store, err := DefaultStore()
	if err != nil {
		return Resolver{}, err
	}
	return NewResolver(store, settings), nil
}

// NewResolver is useful for tests and managed installations.
func NewResolver(store Store, settings config.Settings) Resolver {
	enabled := make(map[string]bool, len(settings.Extensions))
	for _, extension := range settings.Extensions {
		enabled[extension.ID] = extension.Enabled
	}
	trusted := make(map[string]bool, len(settings.TrustedRepositories))
	for _, repository := range settings.TrustedRepositories {
		trusted[repository] = true
	}
	return Resolver{store: store, enabled: enabled, trusted: trusted}
}

// Load resolves extensions for repository. Installed extensions are enabled
// only when config says so; repository extensions require explicit trust.
func (r Resolver) Load(repository string) (Set, error) {
	var loaded []Installed
	global, err := r.store.List()
	if err != nil {
		return Set{}, err
	}
	for _, installed := range global {
		if r.enabled[installed.Manifest.ID] {
			loaded = append(loaded, installed)
		}
	}
	canonical, err := canonicalRepository(repository)
	if err != nil {
		return Set{}, err
	}
	if !r.trusted[canonical] {
		return Set{extensions: loaded}, nil
	}
	project, err := loadProjectExtensions(canonical)
	if err != nil {
		return Set{}, err
	}
	ids := make(map[string]struct{}, len(loaded))
	for _, installed := range loaded {
		ids[installed.Manifest.ID] = struct{}{}
	}
	for _, installed := range project {
		if _, exists := ids[installed.Manifest.ID]; exists {
			return Set{}, fmt.Errorf("project extension %q conflicts with an enabled global extension", installed.Manifest.ID)
		}
		ids[installed.Manifest.ID] = struct{}{}
		loaded = append(loaded, installed)
	}
	return Set{extensions: loaded}, nil
}

func externalToolName(extensionID, toolName string) string {
	return "extension_" + strings.ReplaceAll(extensionID, "-", "_") + "_" + toolName
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

func commandPath(root, value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", errors.New("extension tool command is required")
	}
	if filepath.IsAbs(value) {
		return "", errors.New("extension tool command must be relative to its extension")
	}
	path, err := safePath(root, value)
	if err != nil {
		return "", fmt.Errorf("resolve extension tool command: %w", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("stat extension tool command: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
		return "", errors.New("extension tool command must be an executable regular file")
	}
	return path, nil
}

func readResource(root, resource string) ([]byte, error) {
	path, err := safePath(root, resource)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("resource must be a regular file")
	}
	if info.Size() > maxInstructionBytes {
		return nil, fmt.Errorf("resource exceeds %d bytes", maxInstructionBytes)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if !strings.HasSuffix(resource, ".md") && !strings.HasSuffix(resource, ".txt") {
		return nil, errors.New("resource must be a .md or .txt file")
	}
	return contents, nil
}

func safePath(root, value string) (string, error) {
	if strings.TrimSpace(value) == "" || filepath.IsAbs(value) {
		return "", errors.New("path must be a non-empty relative path")
	}
	clean := filepath.Clean(value)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("path escapes the extension root")
	}
	return filepath.Join(root, clean), nil
}

func canonicalRepository(repository string) (string, error) {
	abs, err := filepath.Abs(repository)
	if err != nil {
		return "", fmt.Errorf("resolve repository: %w", err)
	}
	canonical, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("canonicalize repository: %w", err)
	}
	return canonical, nil
}

func loadProjectExtensions(repository string) ([]Installed, error) {
	directory := filepath.Join(repository, ".gator", "extensions")
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list project extensions: %w", err)
	}
	var installed []Installed
	for _, entry := range entries {
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("project extension %q must be a real directory", entry.Name())
		}
		candidate, err := load(filepath.Join(directory, entry.Name()), true)
		if err != nil {
			return nil, err
		}
		installed = append(installed, candidate)
	}
	sort.Slice(installed, func(first, second int) bool { return installed[first].Manifest.ID < installed[second].Manifest.ID })
	return installed, nil
}
