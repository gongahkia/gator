// Package delivery records explicit local delivery of a verified Work result.
// It deliberately separates a sealed result from applying that result to a
// user-selected target. Records contain hashes and references, never artifact
// bytes, prompts, tool payloads, or credentials.
package delivery

import (
	"context"
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
	"unicode/utf8"

	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/learning"
	"github.com/gongahkia/gator/internal/patch"
	"github.com/gongahkia/gator/internal/workspace"
)

const Version = 1

// Kind identifies a concrete local delivery primitive. New kinds must have a
// real executor; this is intentionally not a generic action registry.
type Kind string

const (
	ArtifactFile  Kind = "artifact_file"
	CodeCandidate Kind = "code_candidate"
)

// Status is an effect's known delivery state. Unknown is deliberately not
// retryable without reconciliation because a prior attempt may have applied.
type Status string

const (
	Pending    Status = "pending"
	Applied    Status = "applied"
	Failed     Status = "failed"
	Unknown    Status = "unknown"
	Superseded Status = "superseded"
)

// Attempt records a bounded observable delivery attempt. A Pending attempt
// with no FinishedAt represents an interrupted process and is reconciled
// conservatively before any later retry.
type Attempt struct {
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at,omitempty"`
	Status     Status    `json:"status"`
	Error      string    `json:"error,omitempty"`
}

// ArtifactPrecondition captures the exact preflight state checked again by
// artifact.ApplyOperation before an atomic local write.
type ArtifactPrecondition struct {
	Disposition    artifact.ApplyDisposition `json:"disposition"`
	ExistingSHA256 string                    `json:"existing_sha256,omitempty"`
}

// Effect is one intended local change. SourcePath is output-relative for an
// artifact or candidate patch; SourceSHA256 binds it to sealed evidence.
type Effect struct {
	ID           string                `json:"id"`
	Kind         Kind                  `json:"kind"`
	SourcePath   string                `json:"source_path"`
	SourceSHA256 string                `json:"source_sha256"`
	CandidateID  string                `json:"candidate_id,omitempty"`
	Precondition *ArtifactPrecondition `json:"artifact_precondition,omitempty"`
	Status       Status                `json:"status"`
	Retryable    bool                  `json:"retryable"`
	Attempts     []Attempt             `json:"attempts,omitempty"`
}

// Record is a human-inspectable delivery ledger for one Work result and one
// explicit target. The Work transaction remains the audit root; this record
// only references its already-sealed bundle.
type Record struct {
	Version    int       `json:"version"`
	ID         string    `json:"id"`
	WorkID     string    `json:"work_id"`
	BundlePath string    `json:"bundle_path"`
	TargetPath string    `json:"target_path"`
	Effects    []Effect  `json:"effects"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Store persists local delivery records under Gator's existing state root.
// Test-only executor fields let deterministic tests prove partial failure
// persistence without adding an executor abstraction to the product.
type Store struct {
	root           string
	now            func() time.Time
	applyArtifact  func(artifact.Bundle, string, artifact.ApplyOperation) error
	applyCandidate func(context.Context, string, patch.Candidate, []byte) error
}

// Open creates the private delivery root. Each Work ID has its own directory
// so the Work-history record can reference it directly.
func Open(stateDir string) (Store, error) {
	if strings.TrimSpace(stateDir) == "" {
		return Store{}, errors.New("delivery state directory is required")
	}
	absolute, err := filepath.Abs(stateDir)
	if err != nil {
		return Store{}, err
	}
	root := filepath.Join(absolute, "gator", "delivery")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return Store{}, fmt.Errorf("create delivery store: %w", err)
	}
	if err := os.Chmod(root, 0o700); err != nil {
		return Store{}, err
	}
	return Store{root: root, now: time.Now}, nil
}

func (s Store) Root() string { return s.root }

// WorkPath returns the location containing delivery records for one Work
// transaction. It is safe to retain as a history evidence reference.
func (s Store) WorkPath(workID string) (string, error) {
	if err := validateID(workID); err != nil {
		return "", err
	}
	return filepath.Join(s.root, workID), nil
}

// PreviewArtifacts retains existing side-effect-free apply planning.
func PreviewArtifacts(bundle artifact.Bundle, target string, replace bool) (artifact.ApplyPlan, error) {
	return artifact.PlanApply(bundle, target, replace)
}

// PreviewCandidate preserves the existing side-effect-free compatibility
// check for a selected verified Code candidate.
func PreviewCandidate(ctx context.Context, bundle artifact.Bundle, target, candidateID string) (patch.Candidate, error) {
	if err := artifact.VerifyBundle(bundle); err != nil {
		return patch.Candidate{}, err
	}
	candidate, found := findCandidate(bundle, candidateID)
	if !found {
		return patch.Candidate{}, errors.New("select a verified Code candidate retained by this bundle")
	}
	payload, err := bundle.Output.ReadRegularFile(filepath.FromSlash(candidate.PatchPath), 16*1024*1024)
	if err != nil {
		return patch.Candidate{}, err
	}
	if err := patch.ApplyCandidate(ctx, target, candidate, payload, true); err != nil {
		return patch.Candidate{}, err
	}
	return candidate, nil
}

// DeliverArtifacts persists the intended files before attempting writes. A
// failed effect leaves later effects pending, so a retry does not regenerate
// Work or repeat an already-applied file.
func (s Store) DeliverArtifacts(ctx context.Context, bundle artifact.Bundle, target string, replace bool) (Record, error) {
	plan, err := artifact.PlanApply(bundle, target, replace)
	if err != nil {
		return Record{}, err
	}
	record, err := s.artifactRecord(bundle, plan, replace)
	if err != nil {
		return Record{}, err
	}
	record, err = s.storeOrLoad(record)
	if err != nil {
		return Record{}, err
	}
	return s.deliver(ctx, record)
}

// DeliverCandidate persists one verified Code candidate as an intended local
// effect. Candidate baseline checks remain owned by patch.ApplyCandidate.
func (s Store) DeliverCandidate(ctx context.Context, bundle artifact.Bundle, target, candidateID string) (Record, error) {
	if err := artifact.VerifyBundle(bundle); err != nil {
		return Record{}, err
	}
	candidate, found := findCandidate(bundle, candidateID)
	if !found {
		return Record{}, errors.New("select a verified Code candidate retained by this bundle")
	}
	absoluteTarget, err := absoluteTarget(target)
	if err != nil {
		return Record{}, err
	}
	now := s.clock()().UTC()
	effect := Effect{
		Kind: CodeCandidate, SourcePath: candidate.PatchPath, SourceSHA256: candidate.SHA256,
		CandidateID: candidate.ID, Status: Pending, Retryable: true,
	}
	effect.ID = effectID(bundle.Manifest.RunID, absoluteTarget, effect)
	record := Record{
		Version: Version, WorkID: bundle.Manifest.RunID, BundlePath: bundle.Path, TargetPath: absoluteTarget,
		Effects: []Effect{effect}, CreatedAt: now, UpdatedAt: now,
	}
	record.ID = recordID(record)
	if err := validateRecord(record); err != nil {
		return Record{}, err
	}
	record, err = s.storeOrLoad(record)
	if err != nil {
		return Record{}, err
	}
	return s.deliver(ctx, record)
}

// Retry attempts only known failed or pending retryable effects in one
// existing record. It never invokes a model or alters the Work transaction.
func (s Store) Retry(ctx context.Context, workID, deliveryID string) (Record, error) {
	record, err := s.selectRecord(workID, deliveryID)
	if err != nil {
		return Record{}, err
	}
	return s.deliver(ctx, record)
}

// PreviewRetry returns the selected persisted record without performing I/O
// beyond loading local state. The caller can show effect states before an
// explicit retry confirmation.
func (s Store) PreviewRetry(workID, deliveryID string) (Record, error) {
	return s.selectRecord(workID, deliveryID)
}

// ListWork returns newest-first durable delivery records for Work review.
func (s Store) ListWork(workID string) ([]Record, error) {
	directory, err := s.WorkPath(workID)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	records := make([]Record, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		record, err := s.Load(workID, strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	sort.Slice(records, func(left, right int) bool {
		if records[left].UpdatedAt.Equal(records[right].UpdatedAt) {
			return records[left].ID > records[right].ID
		}
		return records[left].UpdatedAt.After(records[right].UpdatedAt)
	})
	return records, nil
}

func (s Store) Load(workID, deliveryID string) (Record, error) {
	if err := validateID(workID); err != nil {
		return Record{}, err
	}
	if err := validateID(deliveryID); err != nil {
		return Record{}, err
	}
	path := filepath.Join(s.root, workID, deliveryID+".json")
	payload, err := readJSON(path)
	if err != nil {
		return Record{}, err
	}
	var record Record
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&record); err != nil {
		return Record{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Record{}, errors.New("expected one delivery JSON document")
	}
	if record.WorkID != workID || record.ID != deliveryID || validateRecord(record) != nil {
		return Record{}, errors.New("delivery record is invalid")
	}
	return record, nil
}

func (s Store) artifactRecord(bundle artifact.Bundle, plan artifact.ApplyPlan, replace bool) (Record, error) {
	now := s.clock()().UTC()
	effects := make([]Effect, 0, len(plan.Operations))
	for index, operation := range plan.Operations {
		file := bundle.Manifest.Artifacts[index]
		effect := Effect{
			Kind: ArtifactFile, SourcePath: file.Path, SourceSHA256: file.SHA256,
			Precondition: &ArtifactPrecondition{Disposition: operation.Disposition, ExistingSHA256: operation.ExistingSHA256},
			Status:       Pending, Retryable: operation.Disposition == artifact.ApplyCreate || operation.Disposition == artifact.ApplyReplace,
		}
		switch operation.Disposition {
		case artifact.ApplyUnchanged:
			effect.Status = Applied
			effect.Attempts = []Attempt{{StartedAt: now, FinishedAt: now, Status: Applied}}
		case artifact.ApplyConflict:
			effect.Status, effect.Retryable = Failed, false
			effect.Attempts = []Attempt{{StartedAt: now, FinishedAt: now, Status: Failed, Error: "target exists with different contents; create a new reviewed apply plan to replace it"}}
		}
		effect.ID = effectID(bundle.Manifest.RunID, plan.Target, effect)
		effects = append(effects, effect)
	}
	record := Record{Version: Version, WorkID: bundle.Manifest.RunID, BundlePath: bundle.Path, TargetPath: plan.Target, Effects: effects, CreatedAt: now, UpdatedAt: now}
	record.ID = recordIDWithReplace(record, replace)
	if err := validateRecord(record); err != nil {
		return Record{}, err
	}
	return record, nil
}

func (s Store) storeOrLoad(record Record) (Record, error) {
	directory, err := s.WorkPath(record.WorkID)
	if err != nil {
		return Record{}, err
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return Record{}, err
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return Record{}, err
	}
	path := filepath.Join(directory, record.ID+".json")
	if _, err := os.Lstat(path); err == nil {
		return s.Load(record.WorkID, record.ID)
	} else if !errors.Is(err, os.ErrNotExist) {
		return Record{}, err
	}
	if err := writeJSON(path, record); err != nil {
		return Record{}, err
	}
	return record, nil
}

func (s Store) selectRecord(workID, deliveryID string) (Record, error) {
	if deliveryID != "" {
		return s.Load(workID, deliveryID)
	}
	records, err := s.ListWork(workID)
	if err != nil {
		return Record{}, err
	}
	if len(records) == 0 {
		return Record{}, fmt.Errorf("Work %q has no recorded delivery", workID)
	}
	if len(records) > 1 {
		return Record{}, fmt.Errorf("Work %q has %d delivery records; choose one with --delivery", workID, len(records))
	}
	return records[0], nil
}

func (s Store) deliver(ctx context.Context, record Record) (result Record, resultErr error) {
	result = record
	defer func() { s.observeDelivery(result) }()
	bundle, err := artifact.OpenBundle(record.BundlePath)
	if err != nil {
		return record, fmt.Errorf("open retained Work result: %w", err)
	}
	if err := artifact.VerifyBundle(bundle); err != nil {
		return record, err
	}
	if bundle.Manifest.RunID != record.WorkID {
		return record, errors.New("delivery record does not match retained Work result")
	}
	if err := s.reconcileInterrupted(record, bundle); err != nil {
		return record, err
	}
	record, err = s.Load(record.WorkID, record.ID)
	if err != nil {
		return Record{}, err
	}
	for index := range record.Effects {
		effect := record.Effects[index]
		if effect.Status == Applied || effect.Status == Unknown || effect.Status == Superseded || !effect.Retryable {
			continue
		}
		if effect.Status != Pending && effect.Status != Failed {
			continue
		}
		if err := s.startAttempt(&record, index); err != nil {
			return record, err
		}
		err := s.apply(ctx, bundle, record.TargetPath, effect)
		status := Applied
		if err != nil {
			status = Failed
			if ctx.Err() != nil {
				status = Unknown
			}
		}
		if err := s.finishAttempt(&record, index, status, err); err != nil {
			return record, err
		}
		if status != Applied {
			return record, incomplete(record)
		}
	}
	if err := s.save(record); err != nil {
		return record, err
	}
	return record, incomplete(record)
}

// observeDelivery records only failed or uncertain delivery outcomes. Applied
// effects and retry attempts remain in the delivery ledger; no operational
// outcome can create a candidate rule in this tranche.
func (s Store) observeDelivery(record Record) {
	if record.WorkID == "" || record.ID == "" {
		return
	}
	failed, unknown := false, false
	for _, effect := range record.Effects {
		failed = failed || effect.Status == Failed
		unknown = unknown || effect.Status == Unknown
	}
	if !failed && !unknown {
		return
	}
	stateDir := filepath.Dir(filepath.Dir(s.root))
	store, err := learning.Open(stateDir)
	if err != nil {
		return
	}
	reference := filepath.ToSlash(filepath.Join("gator", "delivery", record.WorkID, record.ID+".json"))
	if failed {
		_, _ = store.UpsertObservation(learning.ObservationInput{
			ID: "observation-delivery-failed-" + record.ID, Signal: learning.DeliveryFailed,
			WorkID: record.WorkID, Summary: "A local delivery effect failed; inspect the retained delivery record before retrying.", EvidenceRefs: []string{reference},
		})
	}
	if unknown {
		_, _ = store.UpsertObservation(learning.ObservationInput{
			ID: "observation-delivery-unknown-" + record.ID, Signal: learning.DeliveryUnknown,
			WorkID: record.WorkID, Summary: "A delivery effect has an unknown outcome and must not be retried automatically.", EvidenceRefs: []string{reference},
		})
	}
}

func (s Store) reconcileInterrupted(record Record, bundle artifact.Bundle) error {
	changed := false
	for index := range record.Effects {
		effect := &record.Effects[index]
		if len(effect.Attempts) == 0 || effect.Attempts[len(effect.Attempts)-1].Status != Pending {
			continue
		}
		attempt := &effect.Attempts[len(effect.Attempts)-1]
		attempt.FinishedAt = s.clock()().UTC()
		if effect.Kind == ArtifactFile && targetMatchesArtifact(record.TargetPath, effect.SourcePath, effect.SourceSHA256) {
			effect.Status, attempt.Status = Applied, Applied
		} else {
			effect.Status, attempt.Status = Unknown, Unknown
			attempt.Error = "previous delivery attempt was interrupted; outcome could not be established safely"
		}
		changed = true
	}
	if !changed {
		return nil
	}
	record.UpdatedAt = s.clock()().UTC()
	return s.save(record)
}

func (s Store) startAttempt(record *Record, index int) error {
	now := s.clock()().UTC()
	record.Effects[index].Attempts = append(record.Effects[index].Attempts, Attempt{StartedAt: now, Status: Pending})
	record.UpdatedAt = now
	return s.save(*record)
}

func (s Store) finishAttempt(record *Record, index int, status Status, err error) error {
	if status != Applied && status != Failed && status != Unknown {
		return errors.New("delivery attempt has invalid terminal status")
	}
	effect := &record.Effects[index]
	if len(effect.Attempts) == 0 || effect.Attempts[len(effect.Attempts)-1].Status != Pending {
		return errors.New("delivery attempt is not pending")
	}
	now := s.clock()().UTC()
	attempt := &effect.Attempts[len(effect.Attempts)-1]
	attempt.FinishedAt, attempt.Status = now, status
	attempt.Error = boundedError(err)
	effect.Status = status
	record.UpdatedAt = now
	return s.save(*record)
}

func (s Store) apply(ctx context.Context, bundle artifact.Bundle, target string, effect Effect) error {
	switch effect.Kind {
	case ArtifactFile:
		if effect.Precondition == nil {
			return errors.New("artifact delivery is missing a precondition")
		}
		operation := artifact.ApplyOperation{Path: effect.SourcePath, Disposition: effect.Precondition.Disposition, ExistingSHA256: effect.Precondition.ExistingSHA256}
		if s.applyArtifact != nil {
			return s.applyArtifact(bundle, target, operation)
		}
		return artifact.ApplyOne(bundle, target, operation)
	case CodeCandidate:
		candidate, found := findCandidate(bundle, effect.CandidateID)
		if !found || candidate.PatchPath != effect.SourcePath || candidate.SHA256 != effect.SourceSHA256 {
			return errors.New("verified Code candidate no longer matches delivery record")
		}
		payload, err := bundle.Output.ReadRegularFile(filepath.FromSlash(candidate.PatchPath), 16*1024*1024)
		if err != nil {
			return err
		}
		if s.applyCandidate != nil {
			return s.applyCandidate(ctx, target, candidate, payload)
		}
		return patch.ApplyCandidate(ctx, target, candidate, payload, false)
	default:
		return fmt.Errorf("unsupported delivery effect kind %q", effect.Kind)
	}
}

func (s Store) save(record Record) error {
	if err := validateRecord(record); err != nil {
		return err
	}
	directory, err := s.WorkPath(record.WorkID)
	if err != nil {
		return err
	}
	return writeJSON(filepath.Join(directory, record.ID+".json"), record)
}

func (s Store) clock() func() time.Time {
	if s.now != nil {
		return s.now
	}
	return time.Now
}

func findCandidate(bundle artifact.Bundle, id string) (patch.Candidate, bool) {
	for _, candidate := range bundle.Manifest.Candidates {
		if candidate.ID == id && candidate.Status == "verified" {
			return candidate, true
		}
	}
	return patch.Candidate{}, false
}

func targetMatchesArtifact(target, relative, digest string) bool {
	root, err := workspace.Open(target)
	if err != nil {
		return false
	}
	value, err := root.SHA256RegularFile(filepath.FromSlash(relative), 100*1024*1024)
	return err == nil && value == digest
}

func absoluteTarget(target string) (string, error) {
	root, err := workspace.Open(target)
	if err != nil {
		return "", err
	}
	return root.Path(), nil
}

func incomplete(record Record) error {
	for _, effect := range record.Effects {
		if effect.Status != Applied && effect.Status != Superseded {
			return fmt.Errorf("delivery %s remains incomplete: %s is %s", record.ID, effect.SourcePath, effect.Status)
		}
	}
	return nil
}

func recordID(record Record) string { return recordIDWithReplace(record, false) }

func recordIDWithReplace(record Record, replace bool) string {
	hash := sha256.New()
	hash.Write([]byte(record.WorkID + "\x00" + record.TargetPath + "\x00"))
	if replace {
		hash.Write([]byte("replace\x00"))
	}
	for _, effect := range record.Effects {
		hash.Write([]byte(string(effect.Kind) + "\x00" + effect.SourcePath + "\x00" + effect.SourceSHA256 + "\x00" + effect.CandidateID + "\x00"))
		if effect.Precondition != nil {
			hash.Write([]byte(string(effect.Precondition.Disposition) + "\x00" + effect.Precondition.ExistingSHA256 + "\x00"))
		}
	}
	return "delivery-" + hex.EncodeToString(hash.Sum(nil))[:24]
}

func effectID(workID, target string, effect Effect) string {
	sum := sha256.Sum256([]byte(workID + "\x00" + target + "\x00" + string(effect.Kind) + "\x00" + effect.SourcePath + "\x00" + effect.SourceSHA256 + "\x00" + effect.CandidateID))
	return "effect-" + hex.EncodeToString(sum[:])[:24]
}

func validateRecord(record Record) error {
	if record.Version != Version || validateID(record.ID) != nil || validateID(record.WorkID) != nil || !filepath.IsAbs(record.BundlePath) || !filepath.IsAbs(record.TargetPath) || len(record.Effects) == 0 || len(record.Effects) > 256 || record.CreatedAt.IsZero() || record.UpdatedAt.IsZero() || record.UpdatedAt.Before(record.CreatedAt) {
		return errors.New("delivery record identity is invalid")
	}
	seen := map[string]bool{}
	for _, effect := range record.Effects {
		if err := validateEffect(effect); err != nil {
			return err
		}
		if seen[effect.ID] {
			return errors.New("delivery record repeats an effect")
		}
		seen[effect.ID] = true
	}
	return nil
}

func validateEffect(effect Effect) error {
	if validateID(effect.ID) != nil || (effect.Kind != ArtifactFile && effect.Kind != CodeCandidate) || !safeRelative(effect.SourcePath) || !digest(effect.SourceSHA256) {
		return errors.New("delivery effect identity is invalid")
	}
	switch effect.Kind {
	case ArtifactFile:
		if effect.CandidateID != "" || effect.Precondition == nil || (effect.Precondition.Disposition != artifact.ApplyCreate && effect.Precondition.Disposition != artifact.ApplyReplace && effect.Precondition.Disposition != artifact.ApplyUnchanged && effect.Precondition.Disposition != artifact.ApplyConflict) || effect.Precondition.ExistingSHA256 != "" && !digest(effect.Precondition.ExistingSHA256) {
			return errors.New("artifact delivery effect is invalid")
		}
	case CodeCandidate:
		if validateID(effect.CandidateID) != nil || effect.Precondition != nil {
			return errors.New("Code delivery effect is invalid")
		}
	}
	if effect.Status != Pending && effect.Status != Applied && effect.Status != Failed && effect.Status != Unknown && effect.Status != Superseded {
		return errors.New("delivery effect status is invalid")
	}
	for _, attempt := range effect.Attempts {
		if attempt.StartedAt.IsZero() || attempt.Status != Pending && attempt.Status != Applied && attempt.Status != Failed && attempt.Status != Unknown {
			return errors.New("delivery attempt is invalid")
		}
		if attempt.Status == Pending {
			if !attempt.FinishedAt.IsZero() || attempt.Error != "" {
				return errors.New("pending delivery attempt is invalid")
			}
		} else if attempt.FinishedAt.IsZero() || attempt.FinishedAt.Before(attempt.StartedAt) || attempt.Error != "" && !safeError(attempt.Error) {
			return errors.New("terminal delivery attempt is invalid")
		}
	}
	return nil
}

func validateID(value string) error {
	if value == "" || len(value) > 96 || strings.ContainsAny(value, "/\\\x00\r\n") {
		return errors.New("invalid delivery identity")
	}
	return nil
}

func safeRelative(value string) bool {
	clean := filepath.ToSlash(filepath.Clean(value))
	return value != "" && clean == value && clean != "." && clean != ".." && !strings.HasPrefix(clean, "../") && !filepath.IsAbs(value)
}

func digest(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func boundedError(err error) string {
	if err == nil {
		return ""
	}
	message := strings.TrimSpace(strings.ToValidUTF8(err.Error(), "�"))
	message = strings.ReplaceAll(message, "\x00", "�")
	if message == "" {
		message = "delivery failed"
	}
	for len(message) > 512 {
		_, size := utf8.DecodeLastRuneInString(message)
		message = message[:len(message)-size]
	}
	return message
}

func safeError(value string) bool {
	return strings.TrimSpace(value) != "" && len(value) <= 512 && !strings.ContainsRune(value, 0)
}

func readJSON(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > 1024*1024 {
		return nil, errors.New("delivery state file is not a bounded regular file")
	}
	return os.ReadFile(path)
}

func writeJSON(path string, value any) error {
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".delivery-*")
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
	directoryHandle, err := os.Open(directory)
	if err == nil {
		defer directoryHandle.Close()
		if err := directoryHandle.Sync(); err != nil {
			return err
		}
	}
	return nil
}
