package watch

import (
	"bufio"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type IgnoreMatcher struct {
	root     string
	patterns []ignorePattern
}

type ignorePattern struct {
	raw      string
	anchored bool
	dirOnly  bool
	hasSlash bool
	negated  bool
}

func LoadIgnore(root string) (*IgnoreMatcher, error) {
	matcher := &IgnoreMatcher{root: root}
	file, err := os.Open(filepath.Join(root, ".gitignore"))
	if err != nil {
		if os.IsNotExist(err) {
			return matcher, nil
		}
		return nil, err
	}
	defer func() { _ = file.Close() }()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		pattern := ignorePattern{raw: filepath.ToSlash(line)}
		if strings.HasPrefix(pattern.raw, "!") {
			pattern.negated = true
			pattern.raw = strings.TrimPrefix(pattern.raw, "!")
		}
		if strings.HasPrefix(pattern.raw, "/") {
			pattern.anchored = true
			pattern.raw = strings.TrimPrefix(pattern.raw, "/")
		}
		if strings.HasSuffix(pattern.raw, "/") {
			pattern.dirOnly = true
			pattern.raw = strings.TrimSuffix(pattern.raw, "/")
		}
		pattern.hasSlash = strings.Contains(pattern.raw, "/")
		if pattern.raw != "" {
			matcher.patterns = append(matcher.patterns, pattern)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return matcher, nil
}

func (m *IgnoreMatcher) Ignored(filePath string, isDir bool) bool {
	rel, err := filepath.Rel(m.root, filePath)
	if err != nil {
		return true
	}
	rel = filepath.ToSlash(rel)
	if rel == "." {
		return false
	}
	parts := strings.Split(rel, "/")
	for _, part := range parts {
		if part == ".git" || part == ".paw" {
			return true
		}
	}
	ignored := false
	for _, pattern := range m.patterns {
		if pattern.dirOnly && !isDir && !pathHasSegmentPrefix(rel, pattern.raw) {
			continue
		}
		if pattern.matches(rel, parts, isDir) {
			ignored = !pattern.negated
		}
	}
	return ignored
}

func (p ignorePattern) matches(rel string, parts []string, isDir bool) bool {
	if p.dirOnly && isDir && pathHasSegmentPrefix(rel, p.raw) {
		return true
	}
	if p.anchored || p.hasSlash {
		ok, _ := path.Match(p.raw, rel)
		return ok || strings.HasPrefix(rel, p.raw+"/")
	}
	for _, part := range parts {
		ok, _ := path.Match(p.raw, part)
		if ok {
			return true
		}
	}
	return false
}

func pathHasSegmentPrefix(rel, prefix string) bool {
	return rel == prefix || strings.HasPrefix(rel, prefix+"/") || strings.Contains(rel, "/"+prefix+"/")
}
