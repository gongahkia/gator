package journal

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// RecentRun is a resumable run record listed for one repository. StatePath is
// local-only metadata; the terminal UI deliberately renders a summary instead
// of asking developers to copy that path.
type RecentRun struct {
	RunID        string
	StatePath    string
	WorktreePath string
	Provider     string
	Model        string
	Task         string
	UpdatedAt    time.Time
	Available    bool
}

// ListRecentRuns returns the newest resumable sessions for repository. Bad or
// incomplete records are skipped so one interrupted journal cannot hide the
// rest of a developer's retained runs.
func ListRecentRuns(stateDir, repository string, limit int) ([]RecentRun, error) {
	if strings.TrimSpace(repository) == "" {
		return nil, errors.New("recent-run repository is required")
	}
	if limit <= 0 {
		return nil, nil
	}
	base, err := resolveStateDir(stateDir)
	if err != nil {
		return nil, err
	}
	directory := filepath.Join(base, "gator", "runs", repositoryFingerprint(repository))
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list run records: %w", err)
	}
	runs := make([]RecentRun, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || !validRunID(entry.Name()) {
			continue
		}
		statePath := filepath.Join(directory, entry.Name())
		session, err := LoadSession(statePath)
		if err != nil || !sameRepository(session.Repository, repository) {
			continue
		}
		info, err := os.Stat(filepath.Join(statePath, "session.json"))
		if err != nil {
			continue
		}
		worktree, err := os.Stat(session.WorktreePath)
		runs = append(runs, RecentRun{
			RunID:        entry.Name(),
			StatePath:    statePath,
			WorktreePath: session.WorktreePath,
			Provider:     session.Provider,
			Model:        session.Model,
			Task:         session.Task,
			UpdatedAt:    info.ModTime(),
			Available:    err == nil && worktree.IsDir(),
		})
	}
	sort.Slice(runs, func(left, right int) bool {
		return runs[left].UpdatedAt.After(runs[right].UpdatedAt)
	})
	if len(runs) > limit {
		runs = runs[:limit]
	}
	return runs, nil
}

// ManagedRunRecord returns the cleaned state path if runRecord is a retained
// run directory for repository under stateDir. Membership is a path check, not
// a capped recency scan, so an old run remains managed even when it falls off
// the recent-run list.
func ManagedRunRecord(stateDir, repository, runRecord string) (string, error) {
	runRecord = strings.TrimSpace(runRecord)
	if runRecord == "" {
		return "", errors.New("retained run record is required")
	}
	if strings.TrimSpace(repository) == "" {
		return "", errors.New("recent-run repository is required")
	}
	base, err := resolveStateDir(stateDir)
	if err != nil {
		return "", err
	}
	expected := filepath.Join(base, "gator", "runs", repositoryFingerprint(repository))
	absolute, err := filepath.Abs(runRecord)
	if err != nil {
		return "", fmt.Errorf("resolve retained run record: %w", err)
	}
	record := filepath.Clean(absolute)
	if resolved, err := filepath.EvalSymlinks(expected); err == nil {
		expected = resolved
	}
	if resolved, err := filepath.EvalSymlinks(record); err == nil {
		record = resolved
	}
	relative, err := filepath.Rel(expected, record)
	if err != nil || relative == "." || !filepath.IsLocal(relative) || filepath.Base(relative) != relative || !validRunID(relative) {
		return "", errors.New("retained run is not managed for this repository")
	}
	return record, nil
}
