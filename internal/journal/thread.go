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

	"github.com/gongahkia/gator/internal/agent"
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
	ForkedFrom    string     `json:"forked_from_state_path,omitempty"`
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
	Repository    string
	HeadStatePath string
	WorktreePath  string
	Provider      string
	Model         string
	Task          string
	TurnCount     int
	ForkedFrom    string
	UpdatedAt     time.Time
	Available     bool
}

// ThreadTurn is one retained run in a conversation lineage. StatePath is
// local-only metadata for navigation and must not be rendered as task
// content. The remaining fields are private session data intended for the
// terminal thread navigator.
type ThreadTurn struct {
	StatePath  string
	ThreadID   string
	Provider   string
	Model      string
	Mode       string
	Task       string
	Status     string
	StartedAt  time.Time
	FinishedAt time.Time
	FinalText  string
}

const maxThreadLineage = 256

// LoadThreadLineage follows the immutable parent run records from head back
// to the first turn, returning them in chronological order. A lineage must
// stay within one repository, worktree, and non-empty thread identity when
// one is recorded; this avoids presenting an accidentally or maliciously
// linked set of unrelated retained sessions as one conversation.
func LoadThreadLineage(headStatePath string) ([]ThreadTurn, error) {
	if strings.TrimSpace(headStatePath) == "" {
		return nil, errors.New("thread head state path is required")
	}
	statePath := filepath.Clean(headStatePath)
	seen := make(map[string]struct{}, maxThreadLineage)
	turns := make([]ThreadTurn, 0, 4)
	var repository, worktreePath, threadID string
	for len(turns) < maxThreadLineage {
		if _, duplicate := seen[statePath]; duplicate {
			return nil, errors.New("retained thread lineage contains a cycle")
		}
		seen[statePath] = struct{}{}

		session, err := LoadSession(statePath)
		if err != nil {
			return nil, fmt.Errorf("load retained thread turn: %w", err)
		}
		if repository == "" {
			repository = repositoryIdentity(session.Repository)
			worktreePath = repositoryIdentity(session.WorktreePath)
		} else if !sameRepository(session.Repository, repository) || repositoryIdentity(session.WorktreePath) != worktreePath {
			return nil, errors.New("retained thread lineage crosses repository or worktree")
		}
		if session.ThreadID != "" {
			if threadID == "" {
				threadID = session.ThreadID
			} else if session.ThreadID != threadID {
				return nil, errors.New("retained thread lineage crosses thread identities")
			}
		}

		result, err := loadThreadTurnResult(statePath)
		if err != nil {
			return nil, err
		}
		turns = append(turns, ThreadTurn{
			StatePath:  statePath,
			ThreadID:   session.ThreadID,
			Provider:   session.Provider,
			Model:      session.Model,
			Mode:       session.Mode,
			Task:       threadTurnTask(session),
			Status:     result.Status,
			StartedAt:  result.StartedAt,
			FinishedAt: result.FinishedAt,
			FinalText:  result.FinalText,
		})

		if strings.TrimSpace(session.ParentStatePath) == "" {
			for left, right := 0, len(turns)-1; left < right; left, right = left+1, right-1 {
				turns[left], turns[right] = turns[right], turns[left]
			}
			return turns, nil
		}
		statePath = filepath.Clean(session.ParentStatePath)
	}
	return nil, fmt.Errorf("retained thread lineage exceeds %d turns", maxThreadLineage)
}

type threadTurnResult struct {
	Status     string    `json:"status"`
	FinalText  string    `json:"final_text,omitempty"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
}

func loadThreadTurnResult(statePath string) (threadTurnResult, error) {
	resultPath := filepath.Join(statePath, "result.json")
	info, err := os.Stat(resultPath)
	if errors.Is(err, os.ErrNotExist) {
		sessionInfo, sessionErr := os.Stat(filepath.Join(statePath, "session.json"))
		if sessionErr != nil {
			return threadTurnResult{}, fmt.Errorf("stat retained thread session: %w", sessionErr)
		}
		return threadTurnResult{Status: "retained", StartedAt: sessionInfo.ModTime()}, nil
	}
	if err != nil {
		return threadTurnResult{}, fmt.Errorf("stat retained thread result: %w", err)
	}
	if !info.Mode().IsRegular() {
		return threadTurnResult{}, errors.New("retained thread result is not a regular file")
	}
	if info.Size() > 1*1024*1024 {
		return threadTurnResult{}, errors.New("retained thread result exceeds the 1 MiB limit")
	}
	contents, err := os.ReadFile(resultPath)
	if err != nil {
		return threadTurnResult{}, fmt.Errorf("read retained thread result: %w", err)
	}
	var result threadTurnResult
	if err := json.Unmarshal(contents, &result); err != nil {
		return threadTurnResult{}, fmt.Errorf("decode retained thread result: %w", err)
	}
	if strings.TrimSpace(result.Status) == "" {
		result.Status = "retained"
	}
	if result.StartedAt.IsZero() {
		result.StartedAt = info.ModTime()
	}
	if result.FinishedAt.IsZero() && result.Status != "retained" {
		result.FinishedAt = info.ModTime()
	}
	return result, nil
}

func threadTurnTask(session Session) string {
	const continuationPrefix = "Continue the original task with this developer instruction:\n"
	for index := len(session.Messages) - 1; index >= 0; index-- {
		message := session.Messages[index]
		if message.Role != agent.RoleUser || !strings.HasPrefix(message.Content, continuationPrefix) {
			continue
		}
		if task := strings.TrimSpace(strings.TrimPrefix(message.Content, continuationPrefix)); task != "" {
			return task
		}
	}
	return session.Task
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
	thread, err := loadThreadPath(path)
	if err != nil {
		return Thread{}, err
	}
	if !sameRepository(thread.Repository, repository) {
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
				Repository:    thread.Repository,
				HeadStatePath: thread.HeadStatePath,
				WorktreePath:  thread.WorktreePath,
				Provider:      thread.Provider,
				Model:         thread.Model,
				Task:          thread.Task,
				TurnCount:     thread.TurnCount,
				ForkedFrom:    thread.ForkedFrom,
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
			Repository:    repository,
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

// ListAllRecentThreads returns the newest retained threads across local
// repositories. It is intentionally opt-in: callers normally use the
// project-scoped ListRecentThreads picker instead.
func ListAllRecentThreads(stateDir string, limit int) ([]RecentThread, error) {
	if limit <= 0 {
		return nil, nil
	}
	base, err := resolveStateDir(stateDir)
	if err != nil {
		return nil, err
	}
	threadRoot := filepath.Join(base, "gator", "threads")
	repositories, err := os.ReadDir(threadRoot)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("list thread repositories: %w", err)
	}
	threads := make([]RecentThread, 0, limit)
	seen := make(map[string]bool)
	if err == nil {
		for _, repository := range repositories {
			if !repository.IsDir() {
				continue
			}
			entries, readErr := os.ReadDir(filepath.Join(threadRoot, repository.Name()))
			if readErr != nil {
				continue
			}
			for _, entry := range entries {
				if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
					continue
				}
				thread, loadErr := loadThreadPath(filepath.Join(threadRoot, repository.Name(), entry.Name()))
				if loadErr != nil {
					continue
				}
				info, statErr := os.Stat(thread.WorktreePath)
				threads = append(threads, RecentThread{
					ID:            thread.ID,
					Repository:    thread.Repository,
					HeadStatePath: thread.HeadStatePath,
					WorktreePath:  thread.WorktreePath,
					Provider:      thread.Provider,
					Model:         thread.Model,
					Task:          thread.Task,
					TurnCount:     thread.TurnCount,
					ForkedFrom:    thread.ForkedFrom,
					UpdatedAt:     thread.UpdatedAt,
					Available:     statErr == nil && info.IsDir(),
				})
				seen[thread.HeadStatePath] = true
			}
		}
	}

	legacy, err := listAllLegacyThreads(base)
	if err != nil {
		return nil, err
	}
	for _, thread := range legacy {
		if !seen[thread.HeadStatePath] {
			threads = append(threads, thread)
		}
	}
	sort.Slice(threads, func(left, right int) bool {
		return threads[left].UpdatedAt.After(threads[right].UpdatedAt)
	})
	if len(threads) > limit {
		threads = threads[:limit]
	}
	return threads, nil
}

// ListThreadForks returns branch summaries grouped by the immutable source
// turn they forked. A fork is a separate worktree by design, but exposing the
// relationship here lets a caller present a real session tree without making
// two branches share mutable source files.
func ListThreadForks(stateDir, repository string) (map[string][]RecentThread, error) {
	threads, err := ListRecentThreads(stateDir, repository, 1_000)
	if err != nil {
		return nil, err
	}
	forks := make(map[string][]RecentThread)
	for _, thread := range threads {
		if strings.TrimSpace(thread.ForkedFrom) == "" {
			continue
		}
		forks[thread.ForkedFrom] = append(forks[thread.ForkedFrom], thread)
	}
	return forks, nil
}

func loadThreadPath(path string) (Thread, error) {
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
	return thread, nil
}

func listAllLegacyThreads(base string) ([]RecentThread, error) {
	runRoot := filepath.Join(base, "gator", "runs")
	repositories, err := os.ReadDir(runRoot)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list run repositories: %w", err)
	}
	threads := make([]RecentThread, 0)
	for _, repository := range repositories {
		if !repository.IsDir() {
			continue
		}
		entries, readErr := os.ReadDir(filepath.Join(runRoot, repository.Name()))
		if readErr != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() || !validRunID(entry.Name()) {
				continue
			}
			statePath := filepath.Join(runRoot, repository.Name(), entry.Name())
			session, loadErr := LoadSession(statePath)
			if loadErr != nil {
				continue
			}
			info, statErr := os.Stat(session.WorktreePath)
			sessionInfo, sessionStatErr := os.Stat(filepath.Join(statePath, "session.json"))
			if sessionStatErr != nil {
				continue
			}
			threads = append(threads, RecentThread{
				ID:            entry.Name(),
				Repository:    session.Repository,
				HeadStatePath: statePath,
				WorktreePath:  session.WorktreePath,
				Provider:      session.Provider,
				Model:         session.Model,
				Task:          session.Task,
				TurnCount:     1,
				UpdatedAt:     sessionInfo.ModTime(),
				Available:     statErr == nil && info.IsDir(),
			})
		}
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
