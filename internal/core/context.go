package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type FileReference struct {
	Path      string
	StartLine int
	EndLine   int
}

type PreparedContext struct {
	Manifest ContextManifest
	Prompt   string
}

var redactionRules = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(authorization\s*:\s*bearer\s+)[^\s,;]+`),
	regexp.MustCompile(`(?i)(bearer\s+)[a-z0-9._~+/-]+`),
	regexp.MustCompile(`(?i)(api[_ -]?key|token|secret|password|credential)\s*[:=]\s*[^\s,;]+`),
	regexp.MustCompile(`\b(?:gh[pousr]_[A-Za-z0-9_-]+|github_pat_[A-Za-z0-9_-]+|sk-[A-Za-z0-9_-]+)\b`),
}

func redactText(value string) (string, int) {
	matches := 0
	for _, rule := range redactionRules {
		value = rule.ReplaceAllStringFunc(value, func(match string) string {
			matches++
			if groups := rule.FindStringSubmatch(match); len(groups) > 1 && groups[1] != "" {
				return groups[1] + "[REDACTED]"
			}
			return "[REDACTED]"
		})
	}
	return value, matches
}

func BuildContext(ctx context.Context, workspace Workspace, objective string, refs []FileReference, includeDiff bool, policy Policy) (PreparedContext, error) {
	if strings.TrimSpace(objective) == "" {
		return PreparedContext{}, fmt.Errorf("objective must be non-empty")
	}
	if len(refs) > policy.Context.MaxFiles {
		return PreparedContext{}, fmt.Errorf("context file count exceeds policy limit")
	}
	manifest := ContextManifest{Entries: []ContextEntry{}, PreparedAt: time.Now().UTC()}
	sections := []string{"# Gator task", "", "## Objective", objective}
	remaining := policy.Context.MaxBytes
	for _, ref := range refs {
		if err := ctx.Err(); err != nil {
			return PreparedContext{}, err
		}
		entry, content, err := readReference(workspace.Root, ref, remaining)
		if err != nil {
			return PreparedContext{}, err
		}
		if entry.Bytes > remaining {
			return PreparedContext{}, fmt.Errorf("context exceeds policy byte limit")
		}
		remaining -= entry.Bytes
		manifest.TotalBytes += entry.Bytes
		manifest.Redactions += entry.Redactions
		manifest.Entries = append(manifest.Entries, entry)
		sections = append(sections, "", "## Context file: "+entry.Path, "", "```", content, "```")
	}
	if includeDiff {
		diff, digest, err := WorkspaceDiff(ctx, workspace)
		if err != nil {
			return PreparedContext{}, err
		}
		redacted, matches := redactText(diff)
		if len(redacted) > remaining {
			return PreparedContext{}, fmt.Errorf("Git diff exceeds policy context byte limit")
		}
		remaining -= len(redacted)
		manifest.TotalBytes += len(redacted)
		manifest.Redactions += matches
		manifest.Entries = append(manifest.Entries, ContextEntry{Kind: "git_diff", Bytes: len(redacted), SHA256: digest, Redactions: matches})
		if redacted != "" {
			sections = append(sections, "", "## Current Git diff", "", "```diff", redacted, "```")
		}
	}
	manifest.Description = fmt.Sprintf("%d context entries, %d bytes", len(manifest.Entries), manifest.TotalBytes)
	return PreparedContext{Manifest: manifest, Prompt: strings.Join(sections, "\n")}, nil
}

func readReference(root string, ref FileReference, maximum int) (ContextEntry, string, error) {
	if ref.Path == "" {
		return ContextEntry{}, "", fmt.Errorf("context path is required")
	}
	path := ref.Path
	if filepath.IsAbs(path) {
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return ContextEntry{}, "", err
		}
		path = relative
	}
	path = filepath.Clean(path)
	if path == "." || strings.HasPrefix(path, ".."+string(filepath.Separator)) || path == ".." {
		return ContextEntry{}, "", fmt.Errorf("context path must remain inside workspace")
	}
	data, err := os.ReadFile(filepath.Join(root, path))
	if err != nil {
		return ContextEntry{}, "", fmt.Errorf("read context file %s: %w", path, err)
	}
	if strings.IndexByte(string(data), 0) >= 0 {
		return ContextEntry{}, "", fmt.Errorf("context file %s is binary", path)
	}
	lines := strings.Split(string(data), "\n")
	first, last := ref.StartLine, ref.EndLine
	if first == 0 && last == 0 {
		first, last = 1, len(lines)
	}
	if first < 1 || last < first || last > len(lines) {
		return ContextEntry{}, "", fmt.Errorf("context range is outside %s", path)
	}
	content, matches := redactText(strings.Join(lines[first-1:last], "\n"))
	if len(content) > maximum {
		return ContextEntry{}, "", fmt.Errorf("context file %s exceeds remaining byte limit", path)
	}
	digest := sha256.Sum256([]byte(content))
	return ContextEntry{
		Kind: "file", Path: filepath.ToSlash(path), StartLine: first, EndLine: last, Bytes: len(content),
		SHA256: hex.EncodeToString(digest[:]), Redactions: matches,
	}, content, nil
}

func ParseFileReference(value string) (FileReference, error) {
	parts := strings.Split(value, ":")
	if len(parts) != 1 && len(parts) != 3 {
		return FileReference{}, fmt.Errorf("file context must be PATH or PATH:START:END")
	}
	ref := FileReference{Path: parts[0]}
	if len(parts) == 3 {
		if _, err := fmt.Sscanf(parts[1]+":"+parts[2], "%d:%d", &ref.StartLine, &ref.EndLine); err != nil {
			return FileReference{}, fmt.Errorf("parse context range: %w", err)
		}
	}
	return ref, nil
}
