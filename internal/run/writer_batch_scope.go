package run

import (
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/gongahkia/gator/internal/instructions"
)

type writerBatchAssignment struct {
	Task  string   `json:"task"`
	Role  string   `json:"role"`
	Paths []string `json:"paths"`
}

type writerConflict struct {
	Kind    string   `json:"kind"`
	Writers []string `json:"writers"`
	Paths   []string `json:"paths"`
	Detail  string   `json:"detail"`
}

// preparedWriterAssignment is validated before any child worktree is created.
type preparedWriterAssignment struct {
	task  string
	role  instructions.Role
	paths []string
}

func (t writerBatchTool) prepare(item writerBatchAssignment, index int) (preparedWriterAssignment, error) {
	task := strings.TrimSpace(item.Task)
	if task == "" {
		return preparedWriterAssignment{}, fmt.Errorf("delegate_writers task %d is required", index)
	}
	if len(task) > maxWriterTaskBytes {
		return preparedWriterAssignment{}, fmt.Errorf("delegate_writers task %d exceeds %d bytes", index, maxWriterTaskBytes)
	}
	role, err := resolveRole(t.writer.roles, item.Role)
	if err != nil {
		return preparedWriterAssignment{}, err
	}
	paths, err := normalizeWriterPaths(item.Paths)
	if err != nil {
		return preparedWriterAssignment{}, fmt.Errorf("delegate_writers task %d paths: %w", index, err)
	}
	return preparedWriterAssignment{task: task, role: role, paths: paths}, nil
}

func normalizeWriterPaths(values []string) ([]string, error) {
	if len(values) == 0 || len(values) > 16 {
		return nil, errors.New("require between 1 and 16 repository-relative paths")
	}
	paths := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || len(value) > 1024 || strings.ContainsAny(value, "\x00\r\n\\") {
			return nil, fmt.Errorf("invalid path %q", value)
		}
		clean := path.Clean(value)
		if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") {
			return nil, fmt.Errorf("unsafe repository-relative path %q", value)
		}
		if _, duplicate := seen[clean]; duplicate {
			return nil, fmt.Errorf("path %q is listed more than once", clean)
		}
		seen[clean] = struct{}{}
		paths = append(paths, clean)
	}
	sort.Strings(paths)
	return paths, nil
}

func overlappingDeclaredWriterPaths(first, second []string) []string {
	overlaps := make([]string, 0)
	for _, left := range first {
		for _, right := range second {
			if writerPathsOverlap(left, right) {
				overlaps = append(overlaps, left+" ↔ "+right)
			}
		}
	}
	return overlaps
}

func writerPathsOverlap(first, second string) bool {
	return first == second || strings.HasPrefix(first, second+"/") || strings.HasPrefix(second, first+"/")
}

func inspectWriterConflicts(reports []delegatedWriterReport) []writerConflict {
	conflicts := make([]writerConflict, 0)
	for _, report := range reports {
		outside := changedPathsOutsideScope(report.ChangedPaths, report.DeclaredPaths)
		if len(outside) > 0 {
			conflicts = append(conflicts, writerConflict{
				Kind:    "out_of_scope_change",
				Writers: []string{report.RunID},
				Paths:   outside,
				Detail:  "writer changed paths outside its declared scope",
			})
		}
	}
	for first := 0; first < len(reports); first++ {
		for second := first + 1; second < len(reports); second++ {
			overlaps := changedPathOverlaps(reports[first].ChangedPaths, reports[second].ChangedPaths)
			if len(overlaps) == 0 {
				continue
			}
			conflicts = append(conflicts, writerConflict{
				Kind:    "changed_path_overlap",
				Writers: []string{reports[first].RunID, reports[second].RunID},
				Paths:   overlaps,
				Detail:  "writer deltas touch the same or nested paths",
			})
		}
	}
	sort.Slice(conflicts, func(first, second int) bool {
		if conflicts[first].Kind == conflicts[second].Kind {
			return strings.Join(conflicts[first].Writers, "\x00") < strings.Join(conflicts[second].Writers, "\x00")
		}
		return conflicts[first].Kind < conflicts[second].Kind
	})
	return conflicts
}

func changedPathsOutsideScope(changed, scopes []string) []string {
	outside := make([]string, 0)
	for _, changedPath := range changed {
		inScope := false
		for _, scope := range scopes {
			if changedPath == scope || strings.HasPrefix(changedPath, scope+"/") {
				inScope = true
				break
			}
		}
		if !inScope {
			outside = append(outside, changedPath)
		}
	}
	return outside
}

func changedPathOverlaps(first, second []string) []string {
	overlaps := make(map[string]struct{})
	for _, left := range first {
		for _, right := range second {
			if writerPathsOverlap(left, right) {
				overlaps[left] = struct{}{}
				overlaps[right] = struct{}{}
			}
		}
	}
	result := make([]string, 0, len(overlaps))
	for overlap := range overlaps {
		result = append(result, overlap)
	}
	sort.Strings(result)
	return result
}
