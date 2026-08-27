package journal

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Draft is the private composer state that can be restored after Gator exits.
// It intentionally contains no provider credentials, tool outputs, or source
// content. Repository context references remain part of the task text only.
type Draft struct {
	Version         int       `json:"version"`
	Repository      string    `json:"repository"`
	Task            string    `json:"task"`
	Verification    string    `json:"verification"`
	Provider        string    `json:"provider"`
	Model           string    `json:"model"`
	MaxSteps        int       `json:"max_steps,omitempty"`
	BaseRef         string    `json:"base_ref,omitempty"`
	BaseURL         string    `json:"base_url,omitempty"`
	Sandbox         string    `json:"sandbox,omitempty"`
	Network         string    `json:"network,omitempty"`
	Setup           string    `json:"setup,omitempty"`
	CopyIgnored     bool      `json:"copy_ignored,omitempty"`
	Scopes          string    `json:"scopes,omitempty"`
	Profile         string    `json:"profile,omitempty"`
	Scouts          string    `json:"scouts,omitempty"`
	AllowedCommands string    `json:"allowed_commands,omitempty"`
	CommandPrefixes string    `json:"command_prefixes,omitempty"`
	UpdatedAt       time.Time `json:"updated_at"`
}

const draftVersion = 1

// SaveDraft atomically writes the unfinished composer state for one
// repository. The state lives outside the checkout and is private to the
// current user.
func SaveDraft(stateDir string, draft Draft) error {
	if strings.TrimSpace(draft.Repository) == "" {
		return errors.New("draft repository is required")
	}
	base, err := resolveStateDir(stateDir)
	if err != nil {
		return err
	}
	directory := filepath.Join(base, "gator", "drafts")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create draft directory: %w", err)
	}
	draft.Version = draftVersion
	if draft.UpdatedAt.IsZero() {
		draft.UpdatedAt = time.Now()
	}
	return writeJSON(filepath.Join(directory, repositoryFingerprint(draft.Repository)+".json"), draft)
}

// LoadDraft returns the saved composer state for repository. A missing draft
// is not an error, which keeps first launch and explicit draft clearing simple.
func LoadDraft(stateDir, repository string) (Draft, bool, error) {
	if strings.TrimSpace(repository) == "" {
		return Draft{}, false, errors.New("draft repository is required")
	}
	base, err := resolveStateDir(stateDir)
	if err != nil {
		return Draft{}, false, err
	}
	path := filepath.Join(base, "gator", "drafts", repositoryFingerprint(repository)+".json")
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return Draft{}, false, nil
	}
	if err != nil {
		return Draft{}, false, fmt.Errorf("stat draft: %w", err)
	}
	if !info.Mode().IsRegular() {
		return Draft{}, false, errors.New("draft is not a regular file")
	}
	if info.Size() > 128*1024 {
		return Draft{}, false, errors.New("draft exceeds the 128 KiB limit")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return Draft{}, false, fmt.Errorf("read draft: %w", err)
	}
	var draft Draft
	if err := json.Unmarshal(contents, &draft); err != nil {
		return Draft{}, false, fmt.Errorf("decode draft: %w", err)
	}
	if draft.Version != draftVersion {
		return Draft{}, false, fmt.Errorf("unsupported draft version %d", draft.Version)
	}
	if draft.Repository != repository {
		return Draft{}, false, errors.New("draft is incomplete or belongs to another repository")
	}
	return draft, true, nil
}

// DeleteDraft removes the private draft after a run has a durable record. A
// missing draft is already the desired state.
func DeleteDraft(stateDir, repository string) error {
	if strings.TrimSpace(repository) == "" {
		return errors.New("draft repository is required")
	}
	base, err := resolveStateDir(stateDir)
	if err != nil {
		return err
	}
	path := filepath.Join(base, "gator", "drafts", repositoryFingerprint(repository)+".json")
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete draft: %w", err)
	}
	return nil
}
