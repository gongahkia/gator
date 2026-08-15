// Package journal stores privacy-bounded local evidence for agent runs.
package journal

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/agent"
)

// Journal records durable, local run metadata outside the developer's Git
// checkout. It intentionally excludes tool arguments, tool outputs, prompts,
// and source text; the isolated worktree retains the reviewable patch.
type Journal struct {
	directory string
	events    *os.File
	startedAt time.Time
}

// Record identifies the stable artifacts Gator stores for a run.
type Record struct {
	RunID        string
	StatePath    string
	WorktreePath string
	StartedAt    time.Time
}

// Session contains the private, local context needed to continue a stopped
// run. Unlike events.jsonl, it contains model conversation content and is
// intentionally written with mode 0600.
type Session struct {
	Version         int             `json:"version"`
	Repository      string          `json:"repository"`
	WorktreePath    string          `json:"worktree_path"`
	Provider        string          `json:"provider"`
	Model           string          `json:"model"`
	BaseURL         string          `json:"base_url,omitempty"`
	Task            string          `json:"task"`
	MaxSteps        int             `json:"max_steps"`
	Verification    [][]string      `json:"verification"`
	Messages        []agent.Message `json:"messages"`
	ParentStatePath string          `json:"parent_state_path,omitempty"`
}

// Open begins recording a run. StateDir overrides the platform default when
// nonempty, which is useful for tests and portable local setups.
func Open(repository, runID, worktreePath, stateDir string, now time.Time) (*Journal, Record, error) {
	if strings.TrimSpace(repository) == "" {
		return nil, Record{}, errors.New("journal repository is required")
	}
	if !validRunID(runID) {
		return nil, Record{}, fmt.Errorf("invalid journal run id %q", runID)
	}
	base, err := resolveStateDir(stateDir)
	if err != nil {
		return nil, Record{}, err
	}
	repositoryID := repositoryFingerprint(repository)
	directory := filepath.Join(base, "gator", "runs", repositoryID, runID)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, Record{}, fmt.Errorf("create run journal directory: %w", err)
	}
	events, err := os.OpenFile(filepath.Join(directory, "events.jsonl"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, Record{}, fmt.Errorf("create run event journal: %w", err)
	}
	record := Record{RunID: runID, StatePath: directory, WorktreePath: worktreePath, StartedAt: now}
	if err := writeJSON(filepath.Join(directory, "run.json"), struct {
		RunID        string    `json:"run_id"`
		WorktreePath string    `json:"worktree_path"`
		StartedAt    time.Time `json:"started_at"`
	}{RunID: runID, WorktreePath: worktreePath, StartedAt: now}); err != nil {
		events.Close()
		return nil, Record{}, err
	}
	return &Journal{directory: directory, events: events, startedAt: now}, record, nil
}

// Append stores event metadata without raw model or tool content.
func (j *Journal) Append(event agent.Event) error {
	if j == nil || j.events == nil {
		return errors.New("journal is closed")
	}
	record := eventRecord{
		Kind:      event.Kind,
		At:        event.At,
		Step:      event.Step,
		ToolError: event.ToolError,
	}
	if event.ToolCall != nil {
		record.ToolName = event.ToolCall.Name
		record.ToolCallID = event.ToolCall.ID
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encode run event: %w", err)
	}
	if _, err := j.events.Write(append(payload, '\n')); err != nil {
		return fmt.Errorf("append run event: %w", err)
	}
	return nil
}

// Finish writes the terminal state and closes the append-only event stream.
func (j *Journal) Finish(status, finalText string, finishedAt time.Time) error {
	if j == nil || j.events == nil {
		return errors.New("journal is closed")
	}
	if err := j.events.Close(); err != nil {
		return fmt.Errorf("close run event journal: %w", err)
	}
	j.events = nil
	return writeJSON(filepath.Join(j.directory, "result.json"), struct {
		Status     string    `json:"status"`
		FinalText  string    `json:"final_text,omitempty"`
		StartedAt  time.Time `json:"started_at"`
		FinishedAt time.Time `json:"finished_at"`
	}{Status: status, FinalText: finalText, StartedAt: j.startedAt, FinishedAt: finishedAt})
}

// SaveSession atomically publishes continuation state for this run.
func (j *Journal) SaveSession(session Session) error {
	if j == nil || j.directory == "" {
		return errors.New("journal is not initialized")
	}
	if session.Version != 2 {
		return fmt.Errorf("unsupported session version %d", session.Version)
	}
	if strings.TrimSpace(session.Repository) == "" || strings.TrimSpace(session.WorktreePath) == "" || strings.TrimSpace(session.Provider) == "" || strings.TrimSpace(session.Task) == "" {
		return errors.New("session repository, worktree path, provider, and task are required")
	}
	return writeJSON(filepath.Join(j.directory, "session.json"), session)
}

// LoadSession reads a session from a run record path printed by Gator.
func LoadSession(statePath string) (Session, error) {
	if strings.TrimSpace(statePath) == "" {
		return Session{}, errors.New("run record path is required")
	}
	path := filepath.Join(statePath, "session.json")
	info, err := os.Stat(path)
	if err != nil {
		return Session{}, fmt.Errorf("stat run session: %w", err)
	}
	if !info.Mode().IsRegular() {
		return Session{}, errors.New("run session is not a regular file")
	}
	if info.Size() > 16*1024*1024 {
		return Session{}, errors.New("run session exceeds the 16 MiB limit")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return Session{}, fmt.Errorf("read run session: %w", err)
	}
	var session Session
	if err := json.Unmarshal(contents, &session); err != nil {
		return Session{}, fmt.Errorf("decode run session: %w", err)
	}
	if session.Version != 2 {
		return Session{}, fmt.Errorf("unsupported session version %d", session.Version)
	}
	if strings.TrimSpace(session.Repository) == "" || strings.TrimSpace(session.WorktreePath) == "" || strings.TrimSpace(session.Provider) == "" || strings.TrimSpace(session.Task) == "" {
		return Session{}, errors.New("run session is incomplete")
	}
	return session, nil
}

// Close releases a journal after a caller-side failure. It does not create a
// result record because the caller has no trustworthy terminal outcome.
func (j *Journal) Close() error {
	if j == nil || j.events == nil {
		return nil
	}
	err := j.events.Close()
	j.events = nil
	return err
}

type eventRecord struct {
	Kind       agent.EventKind `json:"kind"`
	At         time.Time       `json:"at"`
	Step       int             `json:"step"`
	ToolName   string          `json:"tool_name,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	ToolError  string          `json:"tool_error,omitempty"`
}

func resolveStateDir(override string) (string, error) {
	if strings.TrimSpace(override) != "" {
		return absoluteDirectory(override)
	}
	if configured := os.Getenv("GATOR_STATE_DIR"); strings.TrimSpace(configured) != "" {
		return absoluteDirectory(configured)
	}
	if configured := os.Getenv("XDG_STATE_HOME"); strings.TrimSpace(configured) != "" {
		return absoluteDirectory(configured)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user state directory: %w", err)
	}
	return filepath.Join(home, ".local", "state"), nil
}

func absoluteDirectory(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve state directory: %w", err)
	}
	return abs, nil
}

func repositoryFingerprint(repository string) string {
	digest := sha256.Sum256([]byte(repository))
	return hex.EncodeToString(digest[:16])
}

func validRunID(runID string) bool {
	return runID != "" && filepath.Base(runID) == runID && !strings.ContainsAny(runID, "\\/")
}

func writeJSON(path string, value any) error {
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode run journal file: %w", err)
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, append(payload, '\n'), 0o600); err != nil {
		return fmt.Errorf("write run journal file: %w", err)
	}
	if err := os.Rename(temporary, path); err != nil {
		return fmt.Errorf("publish run journal file: %w", err)
	}
	return nil
}
