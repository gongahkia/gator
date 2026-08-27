package journal

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	childManifestVersion   = 1
	childBatchVersion      = 1
	childManifestDirectory = "children"
	childBatchDirectory    = "batches"
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
	Version             int         `json:"version"`
	ID                  string      `json:"id"`
	ParentRunID         string      `json:"parent_run_id"`
	Kind                string      `json:"kind"`
	Status              ChildStatus `json:"status"`
	Repository          string      `json:"repository"`
	ParentWorktree      string      `json:"parent_worktree,omitempty"`
	WorktreePath        string      `json:"worktree_path,omitempty"`
	BaseCommit          string      `json:"base_commit,omitempty"`
	Provider            string      `json:"provider,omitempty"`
	Model               string      `json:"model,omitempty"`
	Profile             string      `json:"profile,omitempty"`
	Role                string      `json:"role,omitempty"`
	BatchID             string      `json:"batch_id,omitempty"`
	DeclaredPaths       []string    `json:"declared_paths,omitempty"`
	ChangedPaths        []string    `json:"changed_paths,omitempty"`
	TaskSHA256          string      `json:"task_sha256"`
	VerificationSHA256  string      `json:"verification_sha256,omitempty"`
	EffectiveMode       string      `json:"effective_mode,omitempty"`
	EffectiveSandbox    string      `json:"effective_sandbox,omitempty"`
	EffectiveNetwork    string      `json:"effective_network,omitempty"`
	MaxSteps            int         `json:"max_steps,omitempty"`
	OmittedCapabilities []string    `json:"omitted_capabilities,omitempty"`
	OwnerPID            int         `json:"owner_pid,omitempty"`
	HeartbeatAt         time.Time   `json:"heartbeat_at,omitempty"`
	DeadlineAt          *time.Time  `json:"deadline_at,omitempty"`
	StartedAt           time.Time   `json:"started_at"`
	UpdatedAt           time.Time   `json:"updated_at"`
	FinishedAt          *time.Time  `json:"finished_at,omitempty"`
	StatePath           string      `json:"state_path,omitempty"`
	PatchSHA256         string      `json:"patch_sha256,omitempty"`
	PatchCommit         string      `json:"patch_commit,omitempty"`
	PatchBytes          int         `json:"patch_bytes"`
	PatchAvailable      bool        `json:"patch_available"`
	ReviewRequired      bool        `json:"review_required"`
	WorktreeRetained    bool        `json:"worktree_retained"`
	Error               string      `json:"error,omitempty"`
}

// ChildBatchStatus captures the durable scheduling state of one parallel
// writer pair. A completed batch may still contain conflicts: completion says
// both child executions reached a terminal state, never that their patches are
// safe to combine.
type ChildBatchStatus string

const (
	ChildBatchPreparing ChildBatchStatus = "preparing"
	ChildBatchRunning   ChildBatchStatus = "running"
	ChildBatchCompleted ChildBatchStatus = "completed"
	ChildBatchFailed    ChildBatchStatus = "failed"
	ChildBatchCancelled ChildBatchStatus = "cancelled"
)

// ChildConflict is durable, path-level conflict evidence for a batch. It is
// deliberately evidence rather than a merge instruction.
type ChildConflict struct {
	Kind     string   `json:"kind"`
	ChildIDs []string `json:"child_ids"`
	Paths    []string `json:"paths"`
	Detail   string   `json:"detail"`
}

// ChildBatchManifest owns the scheduling state for exactly two parallel
// writer children. The child manifests retain their individual execution
// details; this record keeps the grouping and final conflict assessment
// recoverable if Gator exits before returning it to the parent agent.
type ChildBatchManifest struct {
	Version          int              `json:"version"`
	ID               string           `json:"id"`
	ParentRunID      string           `json:"parent_run_id"`
	Status           ChildBatchStatus `json:"status"`
	ChildIDs         []string         `json:"child_ids"`
	StartedAt        time.Time        `json:"started_at"`
	UpdatedAt        time.Time        `json:"updated_at"`
	FinishedAt       *time.Time       `json:"finished_at,omitempty"`
	Conflicts        []ChildConflict  `json:"conflicts,omitempty"`
	ComparisonStatus string           `json:"comparison_status,omitempty"`
	ComparisonTree   string           `json:"comparison_tree,omitempty"`
	ComparisonDetail string           `json:"comparison_detail,omitempty"`
	Error            string           `json:"error,omitempty"`
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

// SaveChildBatchManifest atomically publishes the lifecycle and conflict
// evidence for a parallel writer pair. It is parent-owned private state and
// never contains a delegated task or patch body.
func (j *Journal) SaveChildBatchManifest(manifest ChildBatchManifest) error {
	if j == nil || j.directory == "" {
		return errors.New("journal is not initialized")
	}
	if err := validateChildBatchManifest(manifest); err != nil {
		return err
	}
	directory := filepath.Join(j.directory, childManifestDirectory, childBatchDirectory)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create child batch manifest directory: %w", err)
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
		if entry.Name() == childBatchDirectory {
			if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
				return nil, fmt.Errorf("invalid child batch manifest entry %q", entry.Name())
			}
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

// LoadChildBatchManifest reads one durable parallel-writer schedule from a
// parent run record.
func LoadChildBatchManifest(parentStatePath, batchID string) (ChildBatchManifest, error) {
	if strings.TrimSpace(parentStatePath) == "" {
		return ChildBatchManifest{}, errors.New("parent run record path is required")
	}
	if !validChildManifestID(batchID) {
		return ChildBatchManifest{}, fmt.Errorf("invalid child batch id %q", batchID)
	}
	path := filepath.Join(parentStatePath, childManifestDirectory, childBatchDirectory, batchID+".json")
	info, err := os.Lstat(path)
	if err != nil {
		return ChildBatchManifest{}, fmt.Errorf("stat child batch manifest: %w", err)
	}
	if !info.Mode().IsRegular() {
		return ChildBatchManifest{}, errors.New("child batch manifest is not a regular file")
	}
	if info.Size() > maxChildManifestBytes {
		return ChildBatchManifest{}, errors.New("child batch manifest exceeds the 64 KiB limit")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return ChildBatchManifest{}, fmt.Errorf("read child batch manifest: %w", err)
	}
	var manifest ChildBatchManifest
	if err := json.Unmarshal(contents, &manifest); err != nil {
		return ChildBatchManifest{}, fmt.Errorf("decode child batch manifest: %w", err)
	}
	if err := validateChildBatchManifest(manifest); err != nil {
		return ChildBatchManifest{}, err
	}
	if manifest.ID != batchID {
		return ChildBatchManifest{}, errors.New("child batch manifest ID does not match its filename")
	}
	return manifest, nil
}

// ListChildBatchManifests lists durable parallel-writer schedules in start
// order. It shares the same private parent run record as individual children.
func ListChildBatchManifests(parentStatePath string) ([]ChildBatchManifest, error) {
	if strings.TrimSpace(parentStatePath) == "" {
		return nil, errors.New("parent run record path is required")
	}
	directory := filepath.Join(parentStatePath, childManifestDirectory, childBatchDirectory)
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("read child batch manifest directory: %w", err)
	}
	if len(entries) > maxChildManifests {
		return nil, fmt.Errorf("child batch manifest directory contains more than %d entries", maxChildManifests)
	}
	manifests := make([]ChildBatchManifest, 0, len(entries))
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".json.tmp") {
			continue
		}
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || filepath.Ext(entry.Name()) != ".json" {
			return nil, fmt.Errorf("invalid child batch manifest entry %q", entry.Name())
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		manifest, err := LoadChildBatchManifest(parentStatePath, id)
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
	if manifest.BatchID != "" && !validChildManifestID(manifest.BatchID) {
		return errors.New("child manifest batch ID is invalid")
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
	if manifest.VerificationSHA256 != "" && !validManifestSHA256(manifest.VerificationSHA256) {
		return errors.New("child manifest verification SHA-256 is invalid")
	}
	if manifest.OwnerPID < 0 || manifest.MaxSteps < 0 {
		return errors.New("child manifest owner or step budget is invalid")
	}
	if !manifest.HeartbeatAt.IsZero() && (manifest.HeartbeatAt.Before(manifest.StartedAt) || manifest.HeartbeatAt.After(manifest.UpdatedAt)) {
		return errors.New("child manifest heartbeat is outside its lifecycle")
	}
	if manifest.DeadlineAt != nil && manifest.DeadlineAt.Before(manifest.StartedAt) {
		return errors.New("child manifest deadline precedes start")
	}
	if len(manifest.OmittedCapabilities) > 16 {
		return errors.New("child manifest has too many omitted capabilities")
	}
	for _, capability := range manifest.OmittedCapabilities {
		if capability == "" || len(capability) > 64 || strings.ContainsAny(capability, "\x00\r\n") {
			return errors.New("child manifest omitted capability is invalid")
		}
	}
	if manifest.PatchSHA256 != "" && !validManifestSHA256(manifest.PatchSHA256) {
		return errors.New("child manifest patch SHA-256 is invalid")
	}
	if manifest.PatchCommit != "" && !validGitObjectID(manifest.PatchCommit) {
		return errors.New("child manifest patch commit is invalid")
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
	if err := validateChildManifestPaths("declared paths", manifest.DeclaredPaths, 16); err != nil {
		return err
	}
	if err := validateChildManifestPaths("changed paths", manifest.ChangedPaths, 256); err != nil {
		return err
	}
	for name, value := range map[string]string{
		"worktree path":        manifest.WorktreePath,
		"parent worktree path": manifest.ParentWorktree,
		"state path":           manifest.StatePath,
		"provider":             manifest.Provider,
		"model":                manifest.Model,
		"profile":              manifest.Profile,
		"role":                 manifest.Role,
		"error":                manifest.Error,
	} {
		if len(value) > maxChildManifestText {
			return fmt.Errorf("child manifest %s exceeds the %d-byte limit", name, maxChildManifestText)
		}
	}
	if len(manifest.Repository) > maxChildManifestPath || len(manifest.ParentWorktree) > maxChildManifestPath || len(manifest.WorktreePath) > maxChildManifestPath || len(manifest.StatePath) > maxChildManifestPath {
		return errors.New("child manifest path exceeds the 4 KiB limit")
	}
	return nil
}

func validateChildBatchManifest(manifest ChildBatchManifest) error {
	if manifest.Version != childBatchVersion {
		return fmt.Errorf("unsupported child batch manifest version %d", manifest.Version)
	}
	if !validChildManifestID(manifest.ID) || !validChildManifestID(manifest.ParentRunID) {
		return errors.New("child batch manifest IDs are invalid")
	}
	switch manifest.Status {
	case ChildBatchPreparing, ChildBatchRunning, ChildBatchCompleted, ChildBatchFailed, ChildBatchCancelled:
	default:
		return fmt.Errorf("unsupported child batch manifest status %q", manifest.Status)
	}
	if len(manifest.ChildIDs) != 2 {
		return errors.New("child batch manifest requires exactly two child IDs")
	}
	seen := make(map[string]struct{}, len(manifest.ChildIDs))
	for _, id := range manifest.ChildIDs {
		if !validChildManifestID(id) {
			return errors.New("child batch manifest child ID is invalid")
		}
		if _, duplicate := seen[id]; duplicate {
			return errors.New("child batch manifest repeats a child ID")
		}
		seen[id] = struct{}{}
	}
	if manifest.StartedAt.IsZero() || manifest.UpdatedAt.IsZero() || manifest.UpdatedAt.Before(manifest.StartedAt) {
		return errors.New("child batch manifest timestamps are invalid")
	}
	if manifest.FinishedAt != nil {
		if manifest.Status == ChildBatchPreparing || manifest.Status == ChildBatchRunning {
			return errors.New("unfinished child batch manifest has a finish time")
		}
		if manifest.FinishedAt.Before(manifest.StartedAt) {
			return errors.New("child batch manifest finish precedes start")
		}
	} else if manifest.Status == ChildBatchCompleted || manifest.Status == ChildBatchFailed || manifest.Status == ChildBatchCancelled {
		return errors.New("finished child batch manifest requires a finish time")
	}
	if len(manifest.Conflicts) > 8 {
		return errors.New("child batch manifest exceeds the 8-conflict limit")
	}
	switch manifest.ComparisonStatus {
	case "", "pending", "clean", "conflict", "unavailable":
	default:
		return errors.New("child batch manifest comparison status is invalid")
	}
	if manifest.ComparisonStatus == "clean" && !validGitObjectID(manifest.ComparisonTree) {
		return errors.New("clean child batch comparison requires a result tree")
	}
	if (manifest.ComparisonStatus == "conflict" || manifest.ComparisonStatus == "unavailable") && strings.TrimSpace(manifest.ComparisonDetail) == "" {
		return errors.New("non-clean child batch comparison requires detail")
	}
	if manifest.ComparisonTree != "" && !validGitObjectID(manifest.ComparisonTree) {
		return errors.New("child batch comparison tree is invalid")
	}
	if len(manifest.ComparisonTree) > 128 || len(manifest.ComparisonDetail) > maxChildManifestText {
		return errors.New("child batch manifest comparison evidence is too large")
	}
	for _, conflict := range manifest.Conflicts {
		if conflict.Kind == "" || len(conflict.Kind) > 64 || len(conflict.Detail) == 0 || len(conflict.Detail) > maxChildManifestText || strings.ContainsAny(conflict.Detail, "\r\n") {
			return errors.New("child batch manifest conflict is invalid")
		}
		if len(conflict.ChildIDs) == 0 || len(conflict.ChildIDs) > 2 {
			return errors.New("child batch manifest conflict child IDs are invalid")
		}
		for _, id := range conflict.ChildIDs {
			if _, found := seen[id]; !found {
				return errors.New("child batch manifest conflict references an unknown child")
			}
		}
		if err := validateChildManifestPaths("conflict paths", conflict.Paths, 256); err != nil {
			return err
		}
	}
	if len(manifest.Error) > maxChildManifestText {
		return fmt.Errorf("child batch manifest error exceeds the %d-byte limit", maxChildManifestText)
	}
	return nil
}

func validateChildManifestPaths(name string, values []string, maximum int) error {
	if len(values) > maximum {
		return fmt.Errorf("child manifest %s exceed the %d-entry limit", name, maximum)
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if len(value) == 0 || len(value) > maxChildManifestPath || strings.ContainsAny(value, "\x00\r\n") {
			return fmt.Errorf("child manifest %s contain an invalid path", name)
		}
		clean := path.Clean(value)
		if clean != value || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") {
			return fmt.Errorf("child manifest %s contain an unsafe repository-relative path", name)
		}
		if _, duplicate := seen[value]; duplicate {
			return fmt.Errorf("child manifest %s contain duplicate path %q", name, value)
		}
		seen[value] = struct{}{}
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

func validGitObjectID(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
