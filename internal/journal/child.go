package journal

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	childManifestVersion   = 1
	childManifestDirectory = "children"
	maxChildManifests      = 32
	maxChildManifestBytes  = 64 * 1024
	maxChildManifestText   = 8 * 1024
	maxChildManifestPath   = 4 * 1024
)

var childManifestID = regexp.MustCompile(`\A[a-zA-Z0-9][a-zA-Z0-9_-]{0,95}\z`)

// ChildStatus is the durable lifecycle state for an isolated child agent.
// A manifest is retained even when worktree creation or child startup fails,
// so interrupted orchestration remains inspectable rather than silently
// becoming an orphaned checkout.
type ChildStatus string

const (
	ChildPreparing ChildStatus = "preparing"
	ChildRunning   ChildStatus = "running"
	ChildCompleted ChildStatus = "completed"
	ChildFailed    ChildStatus = "failed"
	ChildCancelled ChildStatus = "cancelled"
)

// ChildManifest records a parent-owned, isolated child run. It deliberately
// stores task and patch digests rather than task text or patch contents: the
// child journal and retained worktree already hold the private review data.
// The parent manifest is an orchestration and recovery record, not another
// conversation transcript.
type ChildManifest struct {
	Version          int         `json:"version"`
	ID               string      `json:"id"`
	ParentRunID      string      `json:"parent_run_id"`
	Kind             string      `json:"kind"`
	Status           ChildStatus `json:"status"`
	Repository       string      `json:"repository"`
	WorktreePath     string      `json:"worktree_path,omitempty"`
	BaseCommit       string      `json:"base_commit,omitempty"`
	Role             string      `json:"role,omitempty"`
	TaskSHA256       string      `json:"task_sha256"`
	StartedAt        time.Time   `json:"started_at"`
	UpdatedAt        time.Time   `json:"updated_at"`
	FinishedAt       *time.Time  `json:"finished_at,omitempty"`
	StatePath        string      `json:"state_path,omitempty"`
	PatchSHA256      string      `json:"patch_sha256,omitempty"`
	PatchBytes       int         `json:"patch_bytes"`
	PatchAvailable   bool        `json:"patch_available"`
	ReviewRequired   bool        `json:"review_required"`
	WorktreeRetained bool        `json:"worktree_retained"`
	Error            string      `json:"error,omitempty"`
}

// SaveChildManifest atomically publishes one parent-owned child state
// transition. Child IDs are validated before they become filenames and every
// manifest remains private local state with mode 0600.
func (j *Journal) SaveChildManifest(manifest ChildManifest) error {
	if j == nil || j.directory == "" {
		return errors.New("journal is not initialized")
	}
	if err := validateChildManifest(manifest); err != nil {
		return err
	}
	directory := filepath.Join(j.directory, childManifestDirectory)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create child manifest directory: %w", err)
	}
	return writeJSON(filepath.Join(directory, manifest.ID+".json"), manifest)
}

// LoadChildManifest reads one durable child record from a parent run record.
func LoadChildManifest(parentStatePath, childID string) (ChildManifest, error) {
	if strings.TrimSpace(parentStatePath) == "" {
		return ChildManifest{}, errors.New("parent run record path is required")
	}
	if !validChildManifestID(childID) {
		return ChildManifest{}, fmt.Errorf("invalid child run id %q", childID)
	}
	path := filepath.Join(parentStatePath, childManifestDirectory, childID+".json")
	info, err := os.Lstat(path)
	if err != nil {
		return ChildManifest{}, fmt.Errorf("stat child manifest: %w", err)
	}
	if !info.Mode().IsRegular() {
		return ChildManifest{}, errors.New("child manifest is not a regular file")
	}
	if info.Size() > maxChildManifestBytes {
		return ChildManifest{}, errors.New("child manifest exceeds the 64 KiB limit")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return ChildManifest{}, fmt.Errorf("read child manifest: %w", err)
	}
	var manifest ChildManifest
	if err := json.Unmarshal(contents, &manifest); err != nil {
		return ChildManifest{}, fmt.Errorf("decode child manifest: %w", err)
	}
	if err := validateChildManifest(manifest); err != nil {
		return ChildManifest{}, err
	}
	if manifest.ID != childID {
		return ChildManifest{}, errors.New("child manifest ID does not match its filename")
	}
	return manifest, nil
}

// ListChildManifests lists the durable children owned by one parent run. It
// never follows symlinks or accepts unexpected filenames in the private child
// directory, because this record is used for recovery decisions after failure.
func ListChildManifests(parentStatePath string) ([]ChildManifest, error) {
	if strings.TrimSpace(parentStatePath) == "" {
		return nil, errors.New("parent run record path is required")
	}
	directory := filepath.Join(parentStatePath, childManifestDirectory)
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("read child manifest directory: %w", err)
	}
	if len(entries) > maxChildManifests {
		return nil, fmt.Errorf("child manifest directory contains more than %d entries", maxChildManifests)
	}
	manifests := make([]ChildManifest, 0, len(entries))
	for _, entry := range entries {
		// An interrupted atomic update can leave a private temporary file. The
		// last published JSON remains authoritative, so recovery must not fail
		// merely because the process died between write and rename.
		if strings.HasSuffix(entry.Name(), ".json.tmp") {
			continue
		}
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || filepath.Ext(entry.Name()) != ".json" {
			return nil, fmt.Errorf("invalid child manifest entry %q", entry.Name())
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		manifest, err := LoadChildManifest(parentStatePath, id)
		if err != nil {
			return nil, err
		}
		manifests = append(manifests, manifest)
	}
	sort.Slice(manifests, func(first, second int) bool {
		if manifests[first].StartedAt.Equal(manifests[second].StartedAt) {
			return manifests[first].ID < manifests[second].ID
		}
		return manifests[first].StartedAt.Before(manifests[second].StartedAt)
	})
	return manifests, nil
}

func validChildManifestID(value string) bool {
	return childManifestID.MatchString(value)
}

func validateChildManifest(manifest ChildManifest) error {
	if manifest.Version != childManifestVersion {
		return fmt.Errorf("unsupported child manifest version %d", manifest.Version)
	}
	if !validChildManifestID(manifest.ID) || !validChildManifestID(manifest.ParentRunID) {
		return errors.New("child manifest IDs are invalid")
	}
	if manifest.Kind != "writer" {
		return fmt.Errorf("unsupported child manifest kind %q", manifest.Kind)
	}
	switch manifest.Status {
	case ChildPreparing, ChildRunning, ChildCompleted, ChildFailed, ChildCancelled:
	default:
		return fmt.Errorf("unsupported child manifest status %q", manifest.Status)
	}
	if strings.TrimSpace(manifest.Repository) == "" {
		return errors.New("child manifest repository is required")
	}
	if manifest.StartedAt.IsZero() || manifest.UpdatedAt.IsZero() {
		return errors.New("child manifest timestamps are required")
	}
	if manifest.UpdatedAt.Before(manifest.StartedAt) {
		return errors.New("child manifest update precedes start")
	}
	if manifest.FinishedAt != nil {
		if manifest.Status == ChildPreparing || manifest.Status == ChildRunning {
			return errors.New("unfinished child manifest has a finish time")
		}
		if manifest.FinishedAt.Before(manifest.StartedAt) {
			return errors.New("child manifest finish precedes start")
		}
	} else if manifest.Status == ChildCompleted || manifest.Status == ChildFailed || manifest.Status == ChildCancelled {
		return errors.New("finished child manifest requires a finish time")
	}
	if !validManifestSHA256(manifest.TaskSHA256) {
		return errors.New("child manifest task SHA-256 is invalid")
	}
	if manifest.PatchSHA256 != "" && !validManifestSHA256(manifest.PatchSHA256) {
		return errors.New("child manifest patch SHA-256 is invalid")
	}
	if manifest.PatchBytes < 0 {
		return errors.New("child manifest patch bytes are invalid")
	}
	if manifest.PatchAvailable && (manifest.PatchBytes == 0 || manifest.PatchSHA256 == "") {
		return errors.New("available child patch requires bytes and SHA-256")
	}
	if manifest.PatchSHA256 != "" && manifest.PatchBytes == 0 {
		return errors.New("child manifest patch SHA-256 has no bytes")
	}
	for name, value := range map[string]string{
		"worktree path": manifest.WorktreePath,
		"state path":    manifest.StatePath,
		"role":          manifest.Role,
		"error":         manifest.Error,
	} {
		if len(value) > maxChildManifestText {
			return fmt.Errorf("child manifest %s exceeds the %d-byte limit", name, maxChildManifestText)
		}
	}
	if len(manifest.Repository) > maxChildManifestPath || len(manifest.WorktreePath) > maxChildManifestPath || len(manifest.StatePath) > maxChildManifestPath {
		return errors.New("child manifest path exceeds the 4 KiB limit")
	}
	return nil
}

func validManifestSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
