// Package review builds and mutates a bounded, human-facing Git review state.
//
// It is deliberately separate from the agent tool loop. Every mutation is
// limited to a retained Gator worktree, and callers must explicitly select a
// file or a unified-diff hunk before it can be staged or unstaged.
package review

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/gongahkia/gator/internal/workspace"
)

const maxDiffBytes = 2 * 1024 * 1024

// Scope identifies the Git index state a review operation addresses. All is a
// presentation-only scope: hunk mutations deliberately require Staged or
// Unstaged so a developer cannot accidentally apply a combined patch twice.
type Scope string

const (
	All      Scope = "all"
	Staged   Scope = "staged"
	Unstaged Scope = "unstaged"
)

// Snapshot is one reloadable review projection for a retained worktree. It
// contains source code and must stay local; callers must never treat it as a
// telemetry or remote-control payload.
type Snapshot struct {
	BaseCommit string    `json:"base_commit,omitempty"`
	All        ChangeSet `json:"all"`
	Staged     ChangeSet `json:"staged"`
	Unstaged   ChangeSet `json:"unstaged"`
	Truncated  bool      `json:"truncated"`
}

// ChangeSet groups files and aggregate line counts for one index scope.
type ChangeSet struct {
	Files []File `json:"files"`
	Stats Stats  `json:"stats"`
}

// Stats is deliberately computed from parsed change lines rather than Git's
// human-formatted --stat output, so the terminal and browser agree exactly.
type Stats struct {
	Files     int `json:"files"`
	Additions int `json:"additions"`
	Deletions int `json:"deletions"`
}

// File is a repository-relative changed path. Patch is retained so a selected
// hunk can be applied without accepting a patch from a UI client.
type File struct {
	ID      string `json:"id"`
	Path    string `json:"path"`
	OldPath string `json:"old_path,omitempty"`
	Status  string `json:"status"`
	Binary  bool   `json:"binary,omitempty"`
	Patch   string `json:"-"`
	Hunks   []Hunk `json:"hunks"`
	Stats   Stats  `json:"stats"`
}

// Hunk is a selectable ordinary unified-diff hunk. Binary changes and file
// mode-only changes intentionally have no selectable hunks.
type Hunk struct {
	ID      string `json:"id"`
	Header  string `json:"header"`
	Patch   string `json:"-"`
	Lines   []Line `json:"lines"`
	Stats   Stats  `json:"stats"`
	OldFrom int    `json:"old_from"`
	NewFrom int    `json:"new_from"`
}

// Line preserves a single unified-diff row and its old/new line numbers when
// applicable. Kind is one of context, addition, deletion, or meta.
type Line struct {
	Kind    string `json:"kind"`
	Text    string `json:"text"`
	OldLine int    `json:"old_line,omitempty"`
	NewLine int    `json:"new_line,omitempty"`
}

// Load reads staged, unstaged, and combined changes from a retained worktree.
// BaseCommit is the run's recorded base, not necessarily the worktree's HEAD,
// so the review remains correct if a retained agent created commits.
func Load(ctx context.Context, worktreePath, baseCommit string) (Snapshot, error) {
	root, err := workspace.Open(worktreePath)
	if err != nil {
		return Snapshot{}, fmt.Errorf("open review worktree: %w", err)
	}
	base := strings.TrimSpace(baseCommit)
	if base == "" {
		base = "HEAD"
	}
	all, allTruncated, err := diff(ctx, root.Path(), "diff", "--no-ext-diff", "--binary", "--find-renames", base)
	if err != nil {
		return Snapshot{}, err
	}
	staged, stagedTruncated, err := diff(ctx, root.Path(), "diff", "--cached", "--no-ext-diff", "--binary", "--find-renames")
	if err != nil {
		return Snapshot{}, err
	}
	unstaged, unstagedTruncated, err := diff(ctx, root.Path(), "diff", "--no-ext-diff", "--binary", "--find-renames")
	if err != nil {
		return Snapshot{}, err
	}

	// Git diff excludes untracked files. Add them to the combined and working
	// sets so the review surface, staging controls, and eventual export agree.
	untracked, untrackedTruncated, err := untrackedPatch(ctx, root)
	if err != nil {
		return Snapshot{}, err
	}
	all += untracked
	unstaged += untracked

	result := Snapshot{
		BaseCommit: baseCommit,
		All:        parseChangeSet(all),
		Staged:     parseChangeSet(staged),
		Unstaged:   parseChangeSet(unstaged),
		Truncated:  allTruncated || stagedTruncated || unstagedTruncated || untrackedTruncated,
	}
	return result, nil
}

// ChangeSet returns the requested portion of a review snapshot. All is useful
// for comprehension; only Staged and Unstaged can be passed to hunk mutation.
func (s Snapshot) ChangeSet(scope Scope) ChangeSet {
	switch scope {
	case Staged:
		return s.Staged
	case Unstaged:
		return s.Unstaged
	default:
		return s.All
	}
}

// StageFile stages exactly one repository-relative path in the retained
// worktree. It never accepts an absolute path or operates in another checkout.
func StageFile(ctx context.Context, worktreePath, path string) error {
	return updateFile(ctx, worktreePath, path, true)
}

// UnstageFile removes exactly one path from the index while retaining its
// working-tree content for further review.
func UnstageFile(ctx context.Context, worktreePath, path string) error {
	return updateFile(ctx, worktreePath, path, false)
}

func updateFile(ctx context.Context, worktreePath, path string, stage bool) error {
	root, relative, err := reviewPath(worktreePath, path)
	if err != nil {
		return err
	}
	arguments := []string{"add", "--", relative}
	if !stage {
		arguments = []string{"reset", "--", relative}
	}
	if _, _, err := git(ctx, root.Path(), nil, arguments...); err != nil {
		verb := "stage"
		if !stage {
			verb = "unstage"
		}
		return fmt.Errorf("%s %q: %w", verb, relative, err)
	}
	return nil
}

// StageHunk stages one hunk found in the current unstaged diff. The source
// patch is reloaded locally; callers supply only its stable opaque ID.
func StageHunk(ctx context.Context, worktreePath, hunkID string) error {
	return updateHunk(ctx, worktreePath, hunkID, Unstaged, false)
}

// UnstageHunk reverses one hunk found in the current staged diff. It does not
// reset other hunks in the same file.
func UnstageHunk(ctx context.Context, worktreePath, hunkID string) error {
	return updateHunk(ctx, worktreePath, hunkID, Staged, true)
}

func updateHunk(ctx context.Context, worktreePath, hunkID string, scope Scope, reverse bool) error {
	if !validID(hunkID) {
		return errors.New("review hunk id is invalid")
	}
	root, err := workspace.Open(worktreePath)
	if err != nil {
		return fmt.Errorf("open review worktree: %w", err)
	}
	arguments := []string{"diff", "--no-ext-diff", "--binary", "--find-renames"}
	if scope == Staged {
		arguments = append(arguments, "--cached")
	}
	patch, truncated, err := diff(ctx, root.Path(), arguments...)
	if err != nil {
		return err
	}
	if truncated {
		return errors.New("review diff is truncated; refresh a smaller change before staging a hunk")
	}
	selected, found := findHunk(parseChangeSet(patch), hunkID)
	if !found {
		return errors.New("review hunk is no longer present; refresh the review")
	}
	if selected.Patch == "" {
		return errors.New("binary or mode-only changes must be staged or unstaged by file")
	}
	applyArguments := []string{"apply", "--cached", "--recount", "--unidiff-zero", "--whitespace=nowarn"}
	if reverse {
		applyArguments = append(applyArguments, "--reverse")
	}
	if _, _, err := git(ctx, root.Path(), []byte(selected.Patch), applyArguments...); err != nil {
		verb := "stage"
		if reverse {
			verb = "unstage"
		}
		return fmt.Errorf("%s selected hunk: %w", verb, err)
	}
	return nil
}

func findHunk(set ChangeSet, id string) (Hunk, bool) {
	for _, file := range set.Files {
		for _, hunk := range file.Hunks {
			if hunk.ID == id {
				return hunk, true
			}
		}
	}
	return Hunk{}, false
}

func reviewPath(worktreePath, path string) (workspace.Root, string, error) {
	root, err := workspace.Open(worktreePath)
	if err != nil {
		return workspace.Root{}, "", fmt.Errorf("open review worktree: %w", err)
	}
	if strings.ContainsRune(path, 0) {
		return workspace.Root{}, "", errors.New("review path contains NUL")
	}
	clean := filepath.ToSlash(filepath.Clean(path))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") || clean == ".gator" || strings.HasPrefix(clean, ".gator/") {
		return workspace.Root{}, "", fmt.Errorf("review path %q is outside the retained worktree", path)
	}
	return root, clean, nil
}

func diff(ctx context.Context, directory string, arguments ...string) (string, bool, error) {
	output, truncated, err := git(ctx, directory, nil, arguments...)
	if err != nil {
		return "", truncated, fmt.Errorf("git %s: %w", strings.Join(arguments, " "), err)
	}
	return string(output), truncated, nil
}

func untrackedPatch(ctx context.Context, root workspace.Root) (string, bool, error) {
	output, _, err := git(ctx, root.Path(), nil, "status", "--porcelain=v1", "--untracked-files=all", "-z")
	if err != nil {
		return "", false, fmt.Errorf("list untracked review files: %w", err)
	}
	var patch strings.Builder
	truncated := false
	paths := make([]string, 0)
	for _, value := range bytes.Split(output, []byte{0}) {
		entry := string(value)
		if !strings.HasPrefix(entry, "?? ") {
			continue
		}
		path := strings.TrimPrefix(entry, "?? ")
		if clean, err := safePatchPath(path); err == nil && clean != "" {
			paths = append(paths, clean)
		}
	}
	sort.Strings(paths)
	for _, path := range paths {
		resolved, err := root.ResolveFile(path)
		if err != nil {
			return "", truncated, fmt.Errorf("resolve untracked review file %q: %w", path, err)
		}
		info, err := os.Stat(resolved)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		fragment, wasTruncated, exitCode, commandErr := gitExit(ctx, root.Path(), nil, "diff", "--no-index", "--binary", "--", "/dev/null", path)
		if commandErr != nil && exitCode != 1 {
			return "", truncated, fmt.Errorf("diff untracked review file %q: %w", path, commandErr)
		}
		if patch.Len()+len(fragment) > maxDiffBytes {
			remaining := max(0, maxDiffBytes-patch.Len())
			patch.Write(fragment[:remaining])
			return patch.String(), true, nil
		}
		patch.Write(fragment)
		truncated = truncated || wasTruncated
	}
	return patch.String(), truncated, nil
}

func git(ctx context.Context, directory string, input []byte, arguments ...string) ([]byte, bool, error) {
	output, truncated, exitCode, err := gitExit(ctx, directory, input, arguments...)
	if err == nil {
		return output, truncated, nil
	}
	if len(output) > 0 {
		return output, truncated, fmt.Errorf("exit %d: %w: %s", exitCode, err, strings.TrimSpace(string(output)))
	}
	return output, truncated, fmt.Errorf("exit %d: %w", exitCode, err)
}

func gitExit(ctx context.Context, directory string, input []byte, arguments ...string) ([]byte, bool, int, error) {
	command := exec.CommandContext(ctx, "git", arguments...)
	command.Dir = directory
	if input != nil {
		command.Stdin = bytes.NewReader(input)
	}
	output := &limitedBuffer{limit: maxDiffBytes}
	command.Stdout = output
	command.Stderr = output
	err := command.Run()
	if err == nil {
		return output.Bytes(), output.truncated, 0, nil
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return output.Bytes(), output.truncated, exitError.ExitCode(), err
	}
	return output.Bytes(), output.truncated, -1, err
}

type limitedBuffer struct {
	bytes.Buffer
	limit     int
	truncated bool
}

func (b *limitedBuffer) Write(value []byte) (int, error) {
	if b.limit <= b.Len() {
		b.truncated = true
		return len(value), nil
	}
	remaining := b.limit - b.Len()
	if len(value) > remaining {
		_, _ = b.Buffer.Write(value[:remaining])
		b.truncated = true
		return len(value), nil
	}
	return b.Buffer.Write(value)
}

var hunkHeader = regexp.MustCompile(`^@@ -([0-9]+)(?:,[0-9]+)? \+([0-9]+)(?:,[0-9]+)? @@`)

func parseChangeSet(patch string) ChangeSet {
	files := splitFiles(patch)
	set := ChangeSet{Files: make([]File, 0, len(files))}
	for _, raw := range files {
		file := parseFile(raw)
		if file.Path == "" {
			continue
		}
		set.Files = append(set.Files, file)
		set.Stats.Files++
		set.Stats.Additions += file.Stats.Additions
		set.Stats.Deletions += file.Stats.Deletions
	}
	return set
}

func splitFiles(patch string) []string {
	if patch == "" {
		return nil
	}
	lines := strings.SplitAfter(patch, "\n")
	files := make([]string, 0)
	var current strings.Builder
	for _, line := range lines {
		if strings.HasPrefix(line, "diff --git ") && current.Len() > 0 {
			files = append(files, current.String())
			current.Reset()
		}
		if strings.HasPrefix(line, "diff --git ") || current.Len() > 0 {
			current.WriteString(line)
		}
	}
	if current.Len() > 0 {
		files = append(files, current.String())
	}
	return files
}

func parseFile(patch string) File {
	lines := strings.Split(strings.TrimSuffix(patch, "\n"), "\n")
	file := File{Patch: patch}
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "rename from "):
			file.OldPath = strings.TrimPrefix(line, "rename from ")
			file.Status = "renamed"
		case strings.HasPrefix(line, "rename to "):
			file.Path = strings.TrimPrefix(line, "rename to ")
			file.Status = "renamed"
		case strings.HasPrefix(line, "new file mode "):
			file.Status = "added"
		case strings.HasPrefix(line, "deleted file mode "):
			file.Status = "deleted"
		case strings.HasPrefix(line, "Binary files ") || strings.HasPrefix(line, "GIT binary patch"):
			file.Binary = true
		}
		if strings.HasPrefix(line, "--- ") {
			if path := markerPath(strings.TrimPrefix(line, "--- ")); path != "" && path != "/dev/null" {
				file.OldPath = path
			}
		}
		if strings.HasPrefix(line, "+++ ") {
			if path := markerPath(strings.TrimPrefix(line, "+++ ")); path != "" && path != "/dev/null" {
				file.Path = path
			}
		}
	}
	if file.Path == "" {
		file.Path = file.OldPath
	}
	if file.OldPath == "" && file.Status != "added" {
		file.OldPath = file.Path
	}
	if file.Status == "" {
		switch {
		case file.OldPath == "":
			file.Status = "added"
		case file.Path == "":
			file.Status = "deleted"
		default:
			file.Status = "modified"
		}
	}
	file.Path, _ = safePatchPath(file.Path)
	file.OldPath, _ = safePatchPath(file.OldPath)
	file.ID = stableID(file.Path + "\x00" + file.OldPath + "\x00" + file.Status)
	file.Hunks = parseHunks(file)
	for _, hunk := range file.Hunks {
		file.Stats.Additions += hunk.Stats.Additions
		file.Stats.Deletions += hunk.Stats.Deletions
	}
	return file
}

func markerPath(value string) string {
	value = strings.SplitN(value, "\t", 2)[0]
	value = strings.TrimSpace(value)
	if value == "/dev/null" {
		return value
	}
	value = strings.TrimPrefix(value, "a/")
	value = strings.TrimPrefix(value, "b/")
	if strings.HasPrefix(value, "\"") {
		if unquoted, err := strconv.Unquote(value); err == nil {
			value = unquoted
		}
	}
	return value
}

func parseHunks(file File) []Hunk {
	lines := strings.Split(strings.TrimSuffix(file.Patch, "\n"), "\n")
	first := -1
	for index, line := range lines {
		if strings.HasPrefix(line, "@@ ") {
			first = index
			break
		}
	}
	if first < 0 {
		return nil
	}
	header := strings.Join(lines[:first], "\n") + "\n"
	var hunks []Hunk
	for index := first; index < len(lines); {
		if !strings.HasPrefix(lines[index], "@@ ") {
			index++
			continue
		}
		end := index + 1
		for end < len(lines) && !strings.HasPrefix(lines[end], "@@ ") {
			end++
		}
		raw := append([]string(nil), lines[index:end]...)
		patch := header + strings.Join(raw, "\n") + "\n"
		hunk := Hunk{Header: raw[0], Patch: patch}
		if matches := hunkHeader.FindStringSubmatch(hunk.Header); len(matches) == 3 {
			hunk.OldFrom, _ = strconv.Atoi(matches[1])
			hunk.NewFrom, _ = strconv.Atoi(matches[2])
		}
		hunk.Lines, hunk.Stats = parseLines(raw[1:], hunk.OldFrom, hunk.NewFrom)
		hunk.ID = stableID(file.ID + "\x00" + hunk.Header + "\x00" + strings.Join(raw[1:], "\n"))
		hunks = append(hunks, hunk)
		index = end
	}
	return hunks
}

func parseLines(raw []string, oldLine, newLine int) ([]Line, Stats) {
	lines := make([]Line, 0, len(raw))
	stats := Stats{}
	for _, value := range raw {
		line := Line{Kind: "meta", Text: value}
		switch {
		case strings.HasPrefix(value, "+") && !strings.HasPrefix(value, "+++"):
			line.Kind, line.NewLine = "addition", newLine
			newLine++
			stats.Additions++
		case strings.HasPrefix(value, "-") && !strings.HasPrefix(value, "---"):
			line.Kind, line.OldLine = "deletion", oldLine
			oldLine++
			stats.Deletions++
		case strings.HasPrefix(value, " "):
			line.Kind, line.OldLine, line.NewLine = "context", oldLine, newLine
			oldLine++
			newLine++
		}
		lines = append(lines, line)
	}
	return lines, stats
}

func safePatchPath(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	if strings.ContainsRune(path, 0) {
		return "", errors.New("review path contains NUL")
	}
	clean := filepath.ToSlash(filepath.Clean(path))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") || clean == ".gator" || strings.HasPrefix(clean, ".gator/") {
		return "", fmt.Errorf("unsafe review path %q", path)
	}
	return clean, nil
}

func stableID(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:12])
}

func validID(value string) bool {
	if len(value) != 24 {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'f') || (character >= '0' && character <= '9') {
			continue
		}
		return false
	}
	return true
}
