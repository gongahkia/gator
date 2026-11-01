package sandbox

import (
	"path/filepath"
	"strings"
)

// runtimeEnvironment removes PATH entries the sandbox cannot read. Some native
// spawn implementations stop at EPERM rather than continuing to /bin/sh. This
// changes no filesystem grant and never adds the ambient user's home to it.
func runtimeEnvironment(environment, readRoots []string) []string {
	result := append([]string(nil), environment...)
	for i, entry := range result {
		value, ok := strings.CutPrefix(entry, "PATH=")
		if !ok {
			continue
		}
		paths := []string{}
		seen := map[string]bool{}
		for _, part := range filepath.SplitList(value) {
			if !filepath.IsAbs(part) {
				continue
			}
			resolved, err := filepath.EvalSymlinks(part)
			if err != nil {
				continue
			}
			for _, root := range readRoots {
				relative, err := filepath.Rel(root, resolved)
				if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
					if !seen[resolved] {
						paths = append(paths, resolved)
						seen[resolved] = true
					}
					break
				}
			}
		}
		result[i] = "PATH=" + strings.Join(paths, string(filepath.ListSeparator))
	}
	return result
}
