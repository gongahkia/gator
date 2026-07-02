package gather

import (
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/gongahkia/paw/internal/envelope"
)

const defaultMaxDepth = 3

func collectDirListing(cwd string, maxDepth int, maxBytes int) ([]envelope.RawUnit, error) {
	if cwd == "" {
		return nil, nil
	}
	if maxDepth <= 0 {
		maxDepth = defaultMaxDepth
	}
	if maxBytes <= 0 {
		maxBytes = defaultMaxFileBytes
	}
	var b strings.Builder
	err := filepath.WalkDir(cwd, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if path == cwd {
			return nil
		}
		rel := relPath(cwd, path)
		if d.IsDir() && skipDir(rel) {
			return filepath.SkipDir
		}
		depth := pathDepth(rel)
		if depth > maxDepth {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		prefix := "f "
		if d.IsDir() {
			prefix = "d "
		}
		line := prefix + rel + "\n"
		if b.Len()+len(line) > maxBytes {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		b.WriteString(line)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if b.Len() == 0 {
		return nil, nil
	}
	return []envelope.RawUnit{{
		Kind: "dir_listing",
		Text: b.String(),
	}}, nil
}

func skipDir(rel string) bool {
	switch filepath.Base(rel) {
	case ".git", "node_modules", ".paw":
		return true
	default:
		return false
	}
}

func pathDepth(rel string) int {
	if rel == "." || rel == "" {
		return 0
	}
	return strings.Count(filepath.Clean(rel), string(filepath.Separator)) + 1
}
