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
	Version             int                   `json:"version"`
	Repository          string                `json:"repository"`
	WorktreePath        string                `json:"worktree_path"`
	BaseCommit          string                `json:"base_commit,omitempty"`
	Provider            string                `json:"provider"`
	Model               string                `json:"model"`
	BaseURL             string                `json:"base_url,omitempty"`
	Task                string                `json:"task"`
	MaxSteps            int                   `json:"max_steps"`
	Verification        [][]string            `json:"verification"`
	Scopes              []string              `json:"scopes,omitempty"`
	ThreadID            string                `json:"thread_id,omitempty"`
	Mode                string                `json:"mode,omitempty"`
	HooksHash           string                `json:"hooks_hash,omitempty"`
	Messages            []agent.Message       `json:"messages"`
	AttachmentManifest  []AttachmentReference `json:"attachment_manifest,omitempty"`
	ParentStatePath     string                `json:"parent_state_path,omitempty"`
	ForkedFromStatePath string                `json:"forked_from_state_path,omitempty"`
	AllowedCommands     [][]string            `json:"allowed_commands,omitempty"`
}

// AttachmentReference records only enough metadata to explain omitted binary
// context during a continuation. Attachment bytes are intentionally never
// persisted after a completed turn.
type AttachmentReference struct {
	Name      string `json:"name"`
	MediaType string `json:"media_type"`
	Bytes     int    `json:"bytes"`
	SHA256    string `json:"sha256"`
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
	cleanMessages, attachments := sanitizeSessionMessages(session.Messages)
	session.Messages = cleanMessages
	session.AttachmentManifest = mergeAttachmentManifest(session.AttachmentManifest, attachments)
	return writeJSON(filepath.Join(j.directory, "session.json"), session)
}

// SaveSnapshot stores the exact portable patch state of a completed turn. It
// is private local state because it can contain repository source, and it is
// separate from the metadata-only event log. A branch restores this snapshot
// into a new isolated worktree rather than sharing mutable files with another
// thread.
func (j *Journal) SaveSnapshot(snapshot []byte) error {
	if j == nil || j.directory == "" {
		return errors.New("journal is not initialized")
	}
	if len(snapshot) > 16*1024*1024 {
		return errors.New("run snapshot exceeds the 16 MiB limit")
	}
	return writePrivateBytes(filepath.Join(j.directory, "snapshot.patch"), snapshot)
}

// LoadSnapshot reads a private patch snapshot retained for a forkable turn.
func LoadSnapshot(statePath string) ([]byte, error) {
	if strings.TrimSpace(statePath) == "" {
		return nil, errors.New("run record path is required")
	}
	path := filepath.Join(statePath, "snapshot.patch")
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat run snapshot: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("run snapshot is not a regular file")
	}
	if info.Size() > 16*1024*1024 {
		return nil, errors.New("run snapshot exceeds the 16 MiB limit")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read run snapshot: %w", err)
	}
	return contents, nil
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
	hadRawAttachments := sessionHasRawAttachments(session.Messages)
	cleanMessages, attachments := sanitizeSessionMessages(session.Messages)
	session.Messages = cleanMessages
	session.AttachmentManifest = mergeAttachmentManifest(session.AttachmentManifest, attachments)
	if hadRawAttachments {
		if err := writeJSON(path, session); err != nil {
			return Session{}, fmt.Errorf("remove raw attachment bytes from run session: %w", err)
		}
	}
	return session, nil
}

func sessionHasRawAttachments(messages []agent.Message) bool {
	for _, message := range messages {
		if len(message.Images) > 0 || len(message.Attachments) > 0 {
			return true
		}
	}
	return false
}

func sanitizeSessionMessages(messages []agent.Message) ([]agent.Message, []AttachmentReference) {
	clean := make([]agent.Message, len(messages))
	attachments := make([]AttachmentReference, 0)
	for index, message := range messages {
		clean[index] = message
		clean[index].Images = nil
		clean[index].Attachments = nil
		for _, image := range message.Images {
			attachments = append(attachments, attachmentReference(image.Name, image.MediaType, image.Data))
		}
		for _, attachment := range message.Attachments {
			attachments = append(attachments, attachmentReference(attachment.Name, attachment.MediaType, attachment.Data))
		}
	}
	return clean, attachments
}

func attachmentReference(name, mediaType string, contents []byte) AttachmentReference {
	digest := sha256.Sum256(contents)
	return AttachmentReference{Name: name, MediaType: mediaType, Bytes: len(contents), SHA256: hex.EncodeToString(digest[:])}
}

func mergeAttachmentManifest(existing, additions []AttachmentReference) []AttachmentReference {
	if len(existing) == 0 && len(additions) == 0 {
		return nil
	}
	merged := make([]AttachmentReference, 0, len(existing)+len(additions))
	seen := make(map[string]struct{}, len(existing)+len(additions))
	for _, attachment := range append(append([]AttachmentReference(nil), existing...), additions...) {
		if attachment.Name == "" || attachment.MediaType == "" || attachment.Bytes < 0 || attachment.SHA256 == "" {
			continue
		}
		key := attachment.Name + "\x00" + attachment.MediaType + "\x00" + attachment.SHA256
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		merged = append(merged, attachment)
	}
	return merged
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

// ResolveStateDir returns the state root used by Gator when an explicit
// override is absent. Interactive callers pass this resolved value to the TUI
// so private drafts, recent runs, and execution journals share one location.
func ResolveStateDir(override string) (string, error) {
	return resolveStateDir(override)
}

func absoluteDirectory(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve state directory: %w", err)
	}
	return abs, nil
}

func repositoryFingerprint(repository string) string {
	digest := sha256.Sum256([]byte(repositoryIdentity(repository)))
	return hex.EncodeToString(digest[:16])
}

// repositoryIdentity makes one physical checkout map to one local-state
// namespace. macOS exposes /var as a symlink to /private/var, so preserving
// the spelling supplied by a caller would otherwise split a repository's
// threads, drafts, and run records across two identities.
func repositoryIdentity(repository string) string {
	abs, err := filepath.Abs(repository)
	if err != nil {
		return filepath.Clean(repository)
	}
	canonical, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return filepath.Clean(abs)
	}
	return filepath.Clean(canonical)
}

func sameRepository(left, right string) bool {
	return repositoryIdentity(left) == repositoryIdentity(right)
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

func writePrivateBytes(path string, contents []byte) error {
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, contents, 0o600); err != nil {
		return fmt.Errorf("write private state file: %w", err)
	}
	if err := os.Rename(temporary, path); err != nil {
		return fmt.Errorf("publish private state file: %w", err)
	}
	return nil
}
