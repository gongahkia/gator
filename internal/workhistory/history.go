// Package workhistory stores a small durable index over completed and active
// Work executions. It deliberately references snapshots, manifests, and
// other existing evidence instead of copying their contents.
package workhistory

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/action"
)

const Version = 1

const (
	Running   Status = "running"
	Completed Status = "completed"
	Failed    Status = "failed"
)

// Status is the transaction-level lifecycle. It is intentionally separate
// from artifact status: a successfully sealed bundle can still belong to a
// failed Work execution if later required evidence persistence fails.
type Status string

const (
	VerificationUnavailable VerificationStatus = "unavailable"
	VerificationPassed      VerificationStatus = "passed"
	VerificationFailed      VerificationStatus = "failed"
)

// VerificationStatus is a compact projection of the existing artifact
// validation evidence. Individual validation results remain in the manifest.
type VerificationStatus string

// EvidenceReferences points to existing retained Work evidence. The record
// deliberately contains no prompt attachments, tool arguments, connector
// payloads, model messages, or artifact bytes.
type EvidenceReferences struct {
	ArtifactManifestPath string `json:"artifact_manifest_path,omitempty"`
	TracePath            string `json:"trace_path,omitempty"`
	InteractionsPath     string `json:"interactions_path,omitempty"`
	TasksPath            string `json:"tasks_path,omitempty"`
	CandidatesPath       string `json:"candidates_path,omitempty"`
	DeliveriesPath       string `json:"deliveries_path,omitempty"`
}

// Record is the durable, human-inspectable index entry for one Work execution.
// ID is the Work run ID, never a legacy Code/journal identity.
type Record struct {
	Version            int                `json:"version"`
	ID                 string             `json:"id"`
	ObjectiveSummary   string             `json:"objective_summary"`
	ObjectiveSHA256    string             `json:"objective_sha256"`
	Mode               action.Mode        `json:"mode"`
	ExternalActions    action.Disposition `json:"external_actions"`
	PolicySHA256       string             `json:"policy_sha256,omitempty"`
	ConversationID     string             `json:"conversation_id,omitempty"`
	RevisionID         string             `json:"revision_id,omitempty"`
	ParentRevisionID   string             `json:"parent_revision_id,omitempty"`
	SnapshotID         string             `json:"snapshot_id,omitempty"`
	Evidence           EvidenceReferences `json:"evidence"`
	ArtifactStatus     string             `json:"artifact_status,omitempty"`
	VerificationStatus VerificationStatus `json:"verification_status,omitempty"`
	Status             Status             `json:"status"`
	StartedAt          time.Time          `json:"started_at"`
	FinishedAt         time.Time          `json:"finished_at,omitempty"`
}

// Start is the bounded request projection retained when a Work execution
// begins. Objective is summarized and digested; its full text remains in the
// existing sealed manifest or retained conversation evidence when available.
type Start struct {
	ID               string
	Objective        string
	Mode             action.Mode
	ExternalActions  action.Disposition
	PolicySHA256     string
	ConversationID   string
	ParentRevisionID string
	StartedAt        time.Time
	Evidence         EvidenceReferences
}

// Finish supplies references produced by the existing Work executor.
type Finish struct {
	ConversationID       string
	RevisionID           string
	ParentRevisionID     string
	SnapshotID           string
	ArtifactManifestPath string
	ArtifactStatus       string
	VerificationStatus   VerificationStatus
	Succeeded            bool
	FinishedAt           time.Time
}

type Store struct{ root string }

// Open returns the private Work history store in Gator's existing state root.
func Open(stateDir string) (Store, error) {
	if strings.TrimSpace(stateDir) == "" {
		return Store{}, errors.New("work history state directory is required")
	}
	absolute, err := filepath.Abs(stateDir)
	if err != nil {
		return Store{}, err
	}
	root := filepath.Join(absolute, "gator", "work-history")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return Store{}, fmt.Errorf("create Work history store: %w", err)
	}
	if err := os.Chmod(root, 0o700); err != nil {
		return Store{}, err
	}
	return Store{root: root}, nil
}

func (s Store) Root() string { return s.root }

// Start creates an immutable Work identity in the running lifecycle state.
func (s Store) Start(start Start) (Record, error) {
	if start.StartedAt.IsZero() {
		return Record{}, errors.New("Work history start time is required")
	}
	if err := validateID(start.ID); err != nil {
		return Record{}, err
	}
	if err := start.Mode.Validate(); err != nil {
		return Record{}, err
	}
	if err := start.ExternalActions.Validate(); err != nil {
		return Record{}, err
	}
	objective := strings.TrimSpace(start.Objective)
	if objective == "" || len(objective) > 64*1024 || strings.ContainsRune(objective, 0) {
		return Record{}, errors.New("Work history objective is invalid")
	}
	if err := validateOptionalID(start.ConversationID); err != nil {
		return Record{}, err
	}
	if err := validateOptionalID(start.ParentRevisionID); err != nil {
		return Record{}, err
	}
	record := Record{
		Version:          Version,
		ID:               start.ID,
		ObjectiveSummary: summary(objective),
		ObjectiveSHA256:  digest(objective),
		Mode:             start.Mode,
		ExternalActions:  start.ExternalActions,
		PolicySHA256:     start.PolicySHA256,
		ConversationID:   start.ConversationID,
		ParentRevisionID: start.ParentRevisionID,
		Evidence:         start.Evidence,
		Status:           Running,
		StartedAt:        start.StartedAt.UTC(),
	}
	if err := validateRecord(record); err != nil {
		return Record{}, err
	}
	path := s.path(record.ID)
	if _, err := os.Lstat(path); err == nil {
		return Record{}, fmt.Errorf("Work history already exists for %q", record.ID)
	} else if !errors.Is(err, os.ErrNotExist) {
		return Record{}, err
	}
	if err := writeJSONExclusive(path, record); err != nil {
		return Record{}, err
	}
	return record, nil
}

// Finish records the terminal projection once. Existing evidence remains at
// the paths supplied by the Work executor and is never copied here.
func (s Store) Finish(id string, finish Finish) (Record, error) {
	record, err := s.Load(id)
	if err != nil {
		return Record{}, err
	}
	if record.Status != Running {
		return Record{}, errors.New("Work history is already terminal")
	}
	if finish.FinishedAt.IsZero() || finish.FinishedAt.Before(record.StartedAt) {
		return Record{}, errors.New("Work history finish time is invalid")
	}
	if err := validateOptionalID(finish.ConversationID); err != nil {
		return Record{}, err
	}
	if err := validateOptionalID(finish.RevisionID); err != nil {
		return Record{}, err
	}
	if err := validateOptionalID(finish.ParentRevisionID); err != nil {
		return Record{}, err
	}
	if finish.SnapshotID != "" && !strings.HasPrefix(finish.SnapshotID, "snap-") {
		return Record{}, errors.New("Work history snapshot identity is invalid")
	}
	record.ConversationID = firstNonEmpty(finish.ConversationID, record.ConversationID)
	record.RevisionID = finish.RevisionID
	record.ParentRevisionID = firstNonEmpty(finish.ParentRevisionID, record.ParentRevisionID)
	record.SnapshotID = finish.SnapshotID
	record.Evidence.ArtifactManifestPath = finish.ArtifactManifestPath
	record.ArtifactStatus = finish.ArtifactStatus
	record.VerificationStatus = finish.VerificationStatus
	record.FinishedAt = finish.FinishedAt.UTC()
	if finish.Succeeded {
		record.Status = Completed
	} else {
		record.Status = Failed
	}
	if err := validateRecord(record); err != nil {
		return Record{}, err
	}
	if err := writeJSON(s.path(id), record); err != nil {
		return Record{}, err
	}
	return record, nil
}

func (s Store) Load(id string) (Record, error) {
	if err := validateID(id); err != nil {
		return Record{}, err
	}
	var record Record
	if err := readJSON(s.path(id), &record); err != nil {
		return Record{}, err
	}
	if record.ID != id || validateRecord(record) != nil {
		return Record{}, errors.New("Work history record is invalid")
	}
	return record, nil
}

// List returns newest-first records with stable ID tie-breaking.
func (s Store) List(limit int) ([]Record, error) {
	return s.list("", limit, false)
}

// ListConversation returns one conversation's records in execution order.
func (s Store) ListConversation(conversationID string, limit int) ([]Record, error) {
	if err := validateID(conversationID); err != nil {
		return nil, err
	}
	return s.list(conversationID, limit, true)
}

func (s Store) list(conversationID string, limit int, oldestFirst bool) ([]Record, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, err
	}
	records := make([]Record, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		record, err := s.Load(strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			return nil, err
		}
		if conversationID == "" || record.ConversationID == conversationID {
			records = append(records, record)
		}
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].StartedAt.Equal(records[j].StartedAt) {
			if oldestFirst {
				return records[i].ID < records[j].ID
			}
			return records[i].ID > records[j].ID
		}
		if oldestFirst {
			return records[i].StartedAt.Before(records[j].StartedAt)
		}
		return records[i].StartedAt.After(records[j].StartedAt)
	})
	if limit > 0 && len(records) > limit {
		records = records[:limit]
	}
	return records, nil
}

func (s Store) path(id string) string { return filepath.Join(s.root, id+".json") }

func validateRecord(record Record) error {
	if record.Version != Version || validateID(record.ID) != nil || record.ObjectiveSummary == "" || len(record.ObjectiveSummary) > 160 || !validDigest(record.ObjectiveSHA256) {
		return errors.New("Work history identity is invalid")
	}
	if err := record.Mode.Validate(); err != nil {
		return err
	}
	if err := record.ExternalActions.Validate(); err != nil {
		return err
	}
	if record.PolicySHA256 != "" && !validDigest(record.PolicySHA256) {
		return errors.New("Work history policy digest is invalid")
	}
	for _, id := range []string{record.ConversationID, record.RevisionID, record.ParentRevisionID} {
		if err := validateOptionalID(id); err != nil {
			return err
		}
	}
	if record.SnapshotID != "" && !strings.HasPrefix(record.SnapshotID, "snap-") {
		return errors.New("Work history snapshot identity is invalid")
	}
	if record.StartedAt.IsZero() {
		return errors.New("Work history start time is invalid")
	}
	switch record.Status {
	case Running:
		if !record.FinishedAt.IsZero() {
			return errors.New("running Work history cannot be finished")
		}
	case Completed, Failed:
		if record.FinishedAt.IsZero() || record.FinishedAt.Before(record.StartedAt) {
			return errors.New("terminal Work history finish time is invalid")
		}
		if record.VerificationStatus != VerificationUnavailable && record.VerificationStatus != VerificationPassed && record.VerificationStatus != VerificationFailed {
			return errors.New("Work history verification status is invalid")
		}
	default:
		return errors.New("Work history status is invalid")
	}
	return nil
}

func validateID(id string) error {
	if id == "" || len(id) > 96 || strings.ContainsAny(id, "/\\\x00\r\n") {
		return errors.New("invalid Work history identity")
	}
	return nil
}

func validateOptionalID(id string) error {
	if id == "" {
		return nil
	}
	return validateID(id)
}

func validDigest(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func summary(objective string) string {
	if line, _, found := strings.Cut(objective, "\n"); found {
		objective = strings.TrimSpace(line)
	}
	if len(objective) > 160 {
		return strings.TrimSpace(objective[:159]) + "…"
	}
	return objective
}

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func firstNonEmpty(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func readJSON(path string, value any) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("Work history state file is not a regular file")
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("expected one Work history JSON document")
	}
	return nil
}

func writeJSONExclusive(path string, value any) error {
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(append(payload, '\n')); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}

func writeJSON(path string, value any) error {
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".work-history-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(append(payload, '\n')); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	err = directory.Sync()
	closeErr := directory.Close()
	if err != nil {
		return err
	}
	return closeErr
}
