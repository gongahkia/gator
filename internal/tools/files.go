package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/workspace"
)

const (
	defaultMaxReadBytes = 64 * 1024
	defaultMaxReadLines = 400
	defaultMaxResults   = 200
	maxSearchFileBytes  = 1024 * 1024
)

// ReadFile reads a bounded line range from a regular text file in the run
// workspace.
type ReadFile struct {
	Root            workspace.Root
	AdditionalRoots []workspace.Root
	NamedRoots      []workspace.NamedRoot
	MaxBytes        int
	MaxLines        int
}

func (t ReadFile) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{
		Name:        "read_file",
		Description: "Read a bounded line range from a UTF-8 file in the workspace. Named mounts such as source/... and output/... are shown when available.",
		Parameters:  schema(`{"type":"object","additionalProperties":false,"required":["path"],"properties":{"path":{"type":"string","description":"Workspace-relative path, named-mount path, or explicitly approved external-root absolute path"},"start_line":{"type":"integer","minimum":1},"end_line":{"type":"integer","minimum":1}}}`),
	}
}

func (t ReadFile) Execute(_ context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	var arguments struct {
		Path      string `json:"path"`
		StartLine int    `json:"start_line"`
		EndLine   int    `json:"end_line"`
	}
	if err := decodeArguments(raw, &arguments); err != nil {
		return agent.ToolResult{}, err
	}
	roots, err := workspace.NewNamedRootSet(t.Root, t.AdditionalRoots, t.NamedRoots)
	if err != nil {
		return agent.ToolResult{}, err
	}
	path, err := roots.ResolveFile(arguments.Path)
	if err != nil {
		return agent.ToolResult{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return agent.ToolResult{}, fmt.Errorf("stat %q: %w", arguments.Path, err)
	}
	if !info.Mode().IsRegular() {
		return agent.ToolResult{}, fmt.Errorf("path %q is not a regular file", arguments.Path)
	}
	maxBytes := positiveOr(t.MaxBytes, defaultMaxReadBytes)
	if info.Size() > int64(maxBytes*16) {
		return agent.ToolResult{}, fmt.Errorf("file %q is too large to read safely", arguments.Path)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return agent.ToolResult{}, fmt.Errorf("read %q: %w", arguments.Path, err)
	}
	if !utf8.Valid(contents) || bytesContainNUL(contents) {
		return agent.ToolResult{}, fmt.Errorf("file %q is not UTF-8 text", arguments.Path)
	}

	lines := strings.Split(string(contents), "\n")
	start := arguments.StartLine
	if start == 0 {
		start = 1
	}
	if start < 1 || start > len(lines) {
		return agent.ToolResult{}, fmt.Errorf("start line %d is outside %q", start, arguments.Path)
	}
	maxLines := positiveOr(t.MaxLines, defaultMaxReadLines)
	end := arguments.EndLine
	if end == 0 {
		end = min(len(lines), start+maxLines-1)
	}
	if end < start {
		return agent.ToolResult{}, errors.New("end line must be at or after start line")
	}
	end = min(end, len(lines))
	selected := strings.Join(lines[start-1:end], "\n")
	truncated := end < len(lines)
	if len(selected) > maxBytes {
		selected = selected[:maxBytes]
		for !utf8.ValidString(selected) {
			selected = selected[:len(selected)-1]
		}
		truncated = true
	}
	result, err := success(struct {
		Path      string `json:"path"`
		StartLine int    `json:"start_line"`
		EndLine   int    `json:"end_line"`
		Content   string `json:"content"`
		Truncated bool   `json:"truncated"`
	}{
		Path:      roots.DisplayPath(path),
		StartLine: start,
		EndLine:   end,
		Content:   selected,
		Truncated: truncated,
	})
	if err != nil {
		return agent.ToolResult{}, err
	}
	return agent.ToolResult{Content: result}, nil
}

// ListFiles lists regular files under one workspace-relative directory.
type ListFiles struct {
	Root            workspace.Root
	AdditionalRoots []workspace.Root
	NamedRoots      []workspace.NamedRoot
	MaxResults      int
}

func (t ListFiles) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{
		Name:        "list_files",
		Description: "List files below a workspace directory, including a named mount such as source or output. Use this before guessing paths.",
		Parameters:  schema(`{"type":"object","additionalProperties":false,"properties":{"path":{"type":"string","description":"Workspace-relative directory, named mount, or explicitly approved external-root absolute directory"},"max_results":{"type":"integer","minimum":1,"maximum":500}}}`),
	}
}

func (t ListFiles) Execute(_ context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	var arguments struct {
		Path       string `json:"path"`
		MaxResults int    `json:"max_results"`
	}
	if err := decodeArguments(raw, &arguments); err != nil {
		return agent.ToolResult{}, err
	}
	roots, err := workspace.NewNamedRootSet(t.Root, t.AdditionalRoots, t.NamedRoots)
	if err != nil {
		return agent.ToolResult{}, err
	}
	directory, err := resolveDirectory(roots, arguments.Path)
	if err != nil {
		return agent.ToolResult{}, err
	}
	maxResults := boundedResultLimit(arguments.MaxResults, t.MaxResults)
	files, truncated, err := walkFiles(roots, directory, maxResults, func(_ string, _ fs.DirEntry) bool { return true })
	if err != nil {
		return agent.ToolResult{}, err
	}
	result, err := success(struct {
		Files     []string `json:"files"`
		Truncated bool     `json:"truncated"`
	}{Files: files, Truncated: truncated})
	if err != nil {
		return agent.ToolResult{}, err
	}
	return agent.ToolResult{Content: result}, nil
}

// SearchFiles finds literal query matches in bounded text files.
type SearchFiles struct {
	Root            workspace.Root
	AdditionalRoots []workspace.Root
	NamedRoots      []workspace.NamedRoot
	MaxResults      int
}

func (t SearchFiles) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{
		Name:        "search_files",
		Description: "Search literal text in workspace files and return matching lines with paths and line numbers. Named mounts such as source and output are supported.",
		Parameters:  schema(`{"type":"object","additionalProperties":false,"required":["query"],"properties":{"query":{"type":"string","minLength":1},"path":{"type":"string","description":"Workspace-relative directory, named mount, or explicitly approved external-root absolute directory"},"max_results":{"type":"integer","minimum":1,"maximum":500}}}`),
	}
}

func (t SearchFiles) Execute(_ context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	var arguments struct {
		Query      string `json:"query"`
		Path       string `json:"path"`
		MaxResults int    `json:"max_results"`
	}
	if err := decodeArguments(raw, &arguments); err != nil {
		return agent.ToolResult{}, err
	}
	if strings.TrimSpace(arguments.Query) == "" {
		return agent.ToolResult{}, errors.New("search query is required")
	}
	roots, err := workspace.NewNamedRootSet(t.Root, t.AdditionalRoots, t.NamedRoots)
	if err != nil {
		return agent.ToolResult{}, err
	}
	directory, err := resolveDirectory(roots, arguments.Path)
	if err != nil {
		return agent.ToolResult{}, err
	}
	maxResults := boundedResultLimit(arguments.MaxResults, t.MaxResults)
	type match struct {
		Path string `json:"path"`
		Line int    `json:"line"`
		Text string `json:"text"`
	}
	matches := make([]match, 0)
	truncated := false
	err = filepath.WalkDir(directory, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != directory && ignoredDirectory(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return nil
		}
		info, err := entry.Info()
		if err != nil || info.Size() > maxSearchFileBytes {
			return nil
		}
		contents, err := os.ReadFile(path)
		if err != nil || !utf8.Valid(contents) || bytesContainNUL(contents) {
			return nil
		}
		for index, line := range strings.Split(string(contents), "\n") {
			if !strings.Contains(line, arguments.Query) {
				continue
			}
			matches = append(matches, match{Path: roots.DisplayPath(path), Line: index + 1, Text: line})
			if len(matches) >= maxResults {
				truncated = true
				return fs.SkipAll
			}
		}
		return nil
	})
	if err != nil {
		return agent.ToolResult{}, fmt.Errorf("search workspace files: %w", err)
	}
	result, err := success(struct {
		Matches   []match `json:"matches"`
		Truncated bool    `json:"truncated"`
	}{Matches: matches, Truncated: truncated})
	if err != nil {
		return agent.ToolResult{}, err
	}
	return agent.ToolResult{Content: result}, nil
}

func schema(value string) json.RawMessage {
	return json.RawMessage(value)
}

func resolveDirectory(roots workspace.RootSet, path string) (string, error) {
	directory, err := roots.ResolveDirectory(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(directory)
	if err != nil {
		return "", fmt.Errorf("stat directory %q: %w", path, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("path %q is not a directory", path)
	}
	return directory, nil
}

func walkFiles(roots workspace.RootSet, directory string, maxResults int, keep func(string, fs.DirEntry) bool) ([]string, bool, error) {
	var files []string
	truncated := false
	err := filepath.WalkDir(directory, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != directory && ignoredDirectory(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return nil
		}
		if len(files) >= maxResults {
			truncated = true
			return fs.SkipAll
		}
		if keep(path, entry) {
			files = append(files, roots.DisplayPath(path))
		}
		return nil
	})
	if err != nil {
		return nil, false, fmt.Errorf("walk workspace files: %w", err)
	}
	sort.Strings(files)
	return files, truncated, nil
}

func ignoredDirectory(name string) bool {
	return name == ".git" || name == ".gator"
}

func boundedResultLimit(requested, configured int) int {
	maximum := positiveOr(configured, defaultMaxResults)
	if requested == 0 {
		return maximum
	}
	return min(requested, maximum)
}

func positiveOr(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func bytesContainNUL(value []byte) bool {
	for _, character := range value {
		if character == 0 {
			return true
		}
	}
	return false
}
