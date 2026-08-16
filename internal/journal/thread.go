package journal

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const threadVersion = 1

// Thread is the durable identity for a conversation that shares one retained
// worktree. Individual run records remain immutable; HeadStatePath advances
// after each completed planning or execution turn.
type Thread struct {
	Version       int        `json:"version"`
	ID            string     `json:"id"`
	Repository    string     `json:"repository"`
	WorktreePath  string     `json:"worktree_path"`
	Provider      string     `json:"provider"`
	Model         string     `json:"model"`
	BaseURL       string     `json:"base_url,omitempty"`
	Task          string     `json:"task"`
	MaxSteps      int        `json:"max_steps"`
	Verification  [][]string `json:"verification"`
	HeadStatePath string     `json:"head_state_path"`
	TurnCount     int        `json:"turn_count"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// RecentThread is a presentation-safe summary for thread pickers. StatePath
// remains local-only metadata and must not be displayed as task content.
type RecentThread struct {
	ID            string
	HeadStatePath string
	WorktreePath  string
	Provider      string
	Model         string
	Task          string
	TurnCount     int
	UpdatedAt     time.Time
	Available     bool
}

// SaveThread atomically creates or advances a private thread record.
func SaveThread(stateDir string, thread Thread) error {
	if err := validateThread(thread); err != nil {
		return err
	}
	base, err := resolveStateDir(stateDir)
	if err != nil {
		return err
	}
	directory := filepath.Join(base, "gator", "threads", repositoryFingerprint(thread.Repository))
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create thread directory: %w", err)
	}
	return writeJSON(filepath.Join(directory, thread.ID+".json"), thread)
}

// LoadThread reads a private thread record for one repository.
func LoadThread(stateDir, repository, id string) (Thread, error) {
	if strings.TrimSpace(repository) == "" {
		return Thread{}, errors.New("thread repository is required")
	}
	if !validRunID(id) {
		return Thread{}, fmt.Errorf("invalid thread id %q", id)
	}
	base, err := resolveStateDir(stateDir)
	if err != nil {
		return Thread{}, err
	}
	path := filepath.Join(base, "gator", "threads", repositoryFingerprint(repository), id+".json")
	info, err := os.Stat(path)
	if err != nil {
		return Thread{}, fmt.Errorf("stat thread: %w", err)
	}
	if !info.Mode().IsRegular() {
		return Thread{}, errors.New("thread record is not a regular file")
	}
	if info.Size() > 1*1024*1024 {
		return Thread{}, errors.New("thread record exceeds the 1 MiB limit")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return Thread{}, fmt.Errorf("read thread: %w", err)
	}
	var thread Thread
	if err := json.Unmarshal(contents, &thread); err != nil {
		return Thread{}, fmt.Errorf("decode thread: %w", err)
	}
	if err := validateThread(thread); err != nil {
		return Thread{}, err
	}
	if filepath.Clean(thread.Repository) != filepath.Clean(repository) {
		return Thread{}, errors.New("thread repository does not match requested repository")
	}
	return thread, nil
}

// ListRecentThreads returns saved conversation threads and falls back to
// legacy run records so upgrading does not hide retained worktrees.
func ListRecentThreads(stateDir, repository string, limit int) ([]RecentThread, error) {
	if strings.TrimSpace(repository) == "" {
		return nil, errors.New("thread repository is required")
	}
	if limit <= 0 {
		return nil, nil
	}
	base, err := resolveStateDir(stateDir)
	if err != nil {
		return nil, err
	}
	directory := filepath.Join(base, "gator", "threads", repositoryFingerprint(repository))
	entries, err := os.ReadDir(directory)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("list threads: %w", err)
	}
	threads := make([]RecentThread, 0, limit)
	seen := make(map[string]bool)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
				continue
			}
			id := strings.TrimSuffix(entry.Name(), ".json")
			thread, loadErr := LoadThread(stateDir, repository, id)
			if loadErr != nil {
				continue
			}
			info, statErr := os.Stat(thread.WorktreePath)
			threads = append(threads, RecentThread{
				ID:            thread.ID,
				HeadStatePath: thread.HeadStatePath,
				WorktreePath:  thread.WorktreePath,
				Provider:      thread.Provider,
				Model:         thread.Model,
				Task:          thread.Task,
				TurnCount:     thread.TurnCount,
				UpdatedAt:     thread.UpdatedAt,
				Available:     statErr == nil && info.IsDir(),
			})
			seen[thread.HeadStatePath] = true
		}
	}
	legacy, err := ListRecentRuns(stateDir, repository, limit)
	if err != nil {
		return nil, err
	}
	for _, run := range legacy {
		if seen[run.StatePath] {
			continue
		}
		threads = append(threads, RecentThread{
			ID:            filepath.Base(run.StatePath),
			HeadStatePath: run.StatePath,
			WorktreePath:  run.WorktreePath,
			Provider:      run.Provider,
			Model:         run.Model,
			Task:          run.Task,
			TurnCount:     1,
			UpdatedAt:     run.UpdatedAt,
			Available:     run.Available,
		})
	}
	sort.Slice(threads, func(left, right int) bool {
		return threads[left].UpdatedAt.After(threads[right].UpdatedAt)
	})
	if len(threads) > limit {
		threads = threads[:limit]
	}
	return threads, nil
}

func validateThread(thread Thread) error {
	if thread.Version != threadVersion {
		return fmt.Errorf("unsupported thread version %d", thread.Version)
	}
	if !validRunID(thread.ID) {
		return fmt.Errorf("invalid thread id %q", thread.ID)
	}
	if strings.TrimSpace(thread.Repository) == "" || strings.TrimSpace(thread.WorktreePath) == "" || strings.TrimSpace(thread.Provider) == "" || strings.TrimSpace(thread.Task) == "" || strings.TrimSpace(thread.HeadStatePath) == "" {
		return errors.New("thread repository, worktree path, provider, task, and head state path are required")
	}
	if thread.TurnCount < 1 {
		return errors.New("thread turn count must be positive")
	}
	return nil
}
