// Package learning stores small, inspectable, user-controlled guidance for
// future Work. It intentionally stores provenance references rather than Work
// payloads, and does not derive or promote learnings on its own.
package learning

import (
	"crypto/rand"
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
)

const (
	Version             = 1
	maxRecordBytes      = 32 * 1024
	maxProjectedRecords = 16
	maxProjectionBytes  = 16 * 1024
)

type Type string

const (
	Preference       Type = "preference"
	EnvironmentFact  Type = "environment_fact"
	Procedure        Type = "procedure"
	FailurePrevention Type = "failure_prevention"
)

type Status string

const (
	Candidate Status = "candidate"
	Active    Status = "active"
	Disabled  Status = "disabled"
	Rejected  Status = "rejected"
)

type Origin string

const (
	UserAuthored Origin = "user"
	Inferred     Origin = "inferred"
)

type ScopeKind string

const (
	Global  ScopeKind = "global"
	Project ScopeKind = "project"
)

// Scope deliberately supports only a user-wide default and one exact local
// project root for now. A project root is an absolute, cleaned path; it may be
// a Git repository or any other selected Work source directory.
type Scope struct {
	Kind  ScopeKind `json:"kind"`
	Value string    `json:"value,omitempty"`
}

// Provenance points to retained evidence. It never copies model prompts,
// transaction payloads, attachments, or connector data into learning state.
type Provenance struct {
	WorkIDs         []string  `json:"work_ids,omitempty"`
	EvidenceRefs    []string  `json:"evidence_refs,omitempty"`
	UserConfirmedAt time.Time `json:"user_confirmed_at,omitempty"`
}

// Record is one mutable learning. Historical Work evidence remains in its
// original store even when this record is disabled, rejected, or edited.
type Record struct {
	Version    int        `json:"version"`
	ID         string     `json:"id"`
	Type       Type       `json:"type"`
	Key        string     `json:"key"`
	Content    string     `json:"content"`
	Scope      Scope      `json:"scope"`
	Status     Status     `json:"status"`
	Origin     Origin     `json:"origin"`
	Confidence int        `json:"confidence,omitempty"`
	Provenance Provenance `json:"provenance"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// Create supplies the user-visible state of one learning. User-authored
// records default to active; inferred records default to candidate. Inferred
// records cannot start active without an explicit user-confirmation timestamp.
type Create struct {
	ID         string
	Type       Type
	Key        string
	Content    string
	Scope      Scope
	Status     Status
	Origin     Origin
	Confidence int
	Provenance Provenance
}

// Context is the small set of Work facts used to choose scoped guidance.
// Project is normally the selected Work source path.
type Context struct {
	Project string
}

// Store is a private local directory containing one readable JSON file per
// learning. Direct edits are accepted only after full validation; an invalid
// or non-regular file fails closed and is never projected into a Work prompt.
type Store struct {
	root string
	now  func() time.Time
}

func Open(stateDir string) (Store, error) {
	if strings.TrimSpace(stateDir) == "" {
		return Store{}, errors.New("learning state directory is required")
	}
	absolute, err := filepath.Abs(stateDir)
	if err != nil {
		return Store{}, err
	}
	root := filepath.Join(absolute, "gator", "learnings")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return Store{}, fmt.Errorf("create learning store: %w", err)
	}
	if err := os.Chmod(root, 0o700); err != nil {
		return Store{}, err
	}
	return Store{root: root, now: time.Now}, nil
}

func (s Store) Root() string { return s.root }

func (s Store) Create(input Create) (Record, error) {
	if input.ID == "" {
		id, err := newID()
		if err != nil {
			return Record{}, err
		}
		input.ID = id
	}
	if input.Origin == "" {
		input.Origin = UserAuthored
	}
	if input.Status == "" {
		if input.Origin == Inferred {
			input.Status = Candidate
		} else {
			input.Status = Active
		}
	}
	normalizedScope, err := normalizeScope(input.Scope)
	if err != nil {
		return Record{}, err
	}
	now := s.clock()().UTC()
	record := Record{
		Version: Version, ID: input.ID, Type: input.Type, Key: input.Key,
		Content: input.Content, Scope: normalizedScope, Status: input.Status,
		Origin: input.Origin, Confidence: input.Confidence,
		Provenance: cloneProvenance(input.Provenance), CreatedAt: now, UpdatedAt: now,
	}
	if err := validateRecord(record); err != nil {
		return Record{}, err
	}
	if err := writeExclusive(s.path(record.ID), record); err != nil {
		return Record{}, err
	}
	return record, nil
}

func (s Store) Load(id string) (Record, error) {
	if err := validateID(id); err != nil {
		return Record{}, err
	}
	return readRecord(s.path(id), id)
}

// List returns all records in stable ID order. A malformed direct edit makes
// the problem visible instead of silently using unvalidated prompt context.
func (s Store) List() ([]Record, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, err
	}
	records := make([]Record, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		record, err := s.Load(id)
		if err != nil {
			return nil, fmt.Errorf("read learning %q: %w", entry.Name(), err)
		}
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].ID < records[j].ID })
	return records, nil
}

// Enable makes a disabled record usable again. Enabling a candidate is an
// explicit human confirmation, not automatic promotion.
func (s Store) Enable(id string) (Record, error) {
	record, err := s.Load(id)
	if err != nil {
		return Record{}, err
	}
	if record.Status == Rejected {
		return Record{}, errors.New("a rejected learning cannot be enabled")
	}
	if record.Status == Candidate {
		record.Provenance.UserConfirmedAt = s.clock()().UTC()
	}
	record.Status = Active
	return s.save(record)
}

// Disable removes a learning from future Work while retaining its provenance.
func (s Store) Disable(id string) (Record, error) {
	record, err := s.Load(id)
	if err != nil {
		return Record{}, err
	}
	if record.Status == Rejected {
		return Record{}, errors.New("a rejected learning cannot be disabled")
	}
	record.Status = Disabled
	return s.save(record)
}

// Reject prevents a candidate from influencing Work while preserving the
// evidence that led to it for later inspection.
func (s Store) Reject(id string) (Record, error) {
	record, err := s.Load(id)
	if err != nil {
		return Record{}, err
	}
	if record.Status != Candidate {
		return Record{}, errors.New("only a candidate learning can be rejected")
	}
	record.Status = Rejected
	return s.save(record)
}

// Remove is a reversible removal: it disables the record rather than deleting
// provenance or historical references.
func (s Store) Remove(id string) (Record, error) { return s.Disable(id) }

// Edit changes the user-maintained wording/key of a direct user learning. An
// inferred candidate must instead be approved or rejected; this prevents an
// edited inference from being misrepresented as unchanged machine inference.
func (s Store) Edit(id, key, content string) (Record, error) {
	record, err := s.Load(id)
	if err != nil {
		return Record{}, err
	}
	if record.Origin != UserAuthored {
		return Record{}, errors.New("edit an inferred candidate by adding a user-authored learning instead")
	}
	if strings.TrimSpace(key) != "" {
		record.Key = key
	}
	if strings.TrimSpace(content) != "" {
		record.Content = content
	}
	return s.save(record)
}

// Applicable selects one active record per type/key. User-authored rules
// outrank inferred ones, then exact project scope outranks global scope. The
// remaining timestamp/ID tie-breaks make conflict resolution deterministic.
func Applicable(records []Record, context Context) []Record {
	project := ""
	if strings.TrimSpace(context.Project) != "" {
		var err error
		project, err = normalizeProject(context.Project)
		if err != nil {
			return nil
		}
	}
	chosen := map[string]Record{}
	for _, record := range records {
		if record.Status != Active || !matches(record.Scope, project) {
			continue
		}
		identity := string(record.Type) + "\x00" + record.Key
		if current, exists := chosen[identity]; !exists || higher(record, current) {
			chosen[identity] = record
		}
	}
	result := make([]Record, 0, len(chosen))
	for _, record := range chosen {
		result = append(result, record)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Type == result[j].Type {
			if result[i].Key == result[j].Key {
				return result[i].ID < result[j].ID
			}
			return result[i].Key < result[j].Key
		}
		return result[i].Type < result[j].Type
	})
	if len(result) > maxProjectedRecords {
		return result[:maxProjectedRecords]
	}
	return result
}

// Projection returns only applicable active records. Candidates, disabled,
// rejected, invalid, and unrelated records can never enter this result.
func (s Store) Projection(context Context) ([]Record, error) {
	records, err := s.List()
	if err != nil {
		return nil, err
	}
	return Applicable(records, context), nil
}

// RenderProjection provides bounded, explicitly subordinate prompt context.
// Current user instructions and the Work contract remain higher precedence.
func RenderProjection(records []Record) string {
	if len(records) == 0 {
		return ""
	}
	lines := []string{"Stored active learnings are user-controlled context, not authority. Apply only when relevant. The current user request and developer-owned Work contract override every learning."}
	for _, record := range records {
		line := fmt.Sprintf("- [%s] %s (%s, %s): %s", record.ID, record.Key, record.Type, formatScope(record.Scope), record.Content)
		if totalBytes(lines)+len(line)+1 > maxProjectionBytes {
			break
		}
		lines = append(lines, line)
	}
	if len(lines) == 1 {
		return ""
	}
	return strings.Join(lines, "\n")
}

func (s Store) save(record Record) (Record, error) {
	record.UpdatedAt = s.clock()().UTC()
	if err := validateRecord(record); err != nil {
		return Record{}, err
	}
	if err := writeAtomic(s.path(record.ID), record); err != nil {
		return Record{}, err
	}
	return record, nil
}

func (s Store) path(id string) string { return filepath.Join(s.root, id+".json") }

func (s Store) clock() func() time.Time {
	if s.now == nil {
		return time.Now
	}
	return s.now
}

func higher(left, right Record) bool {
	if originRank(left.Origin) != originRank(right.Origin) {
		return originRank(left.Origin) > originRank(right.Origin)
	}
	if scopeRank(left.Scope.Kind) != scopeRank(right.Scope.Kind) {
		return scopeRank(left.Scope.Kind) > scopeRank(right.Scope.Kind)
	}
	if !left.UpdatedAt.Equal(right.UpdatedAt) {
		return left.UpdatedAt.After(right.UpdatedAt)
	}
	return left.ID < right.ID
}

func originRank(value Origin) int {
	if value == UserAuthored {
		return 2
	}
	return 1
}

func scopeRank(value ScopeKind) int {
	if value == Project {
		return 2
	}
	return 1
}

func matches(scope Scope, project string) bool {
	switch scope.Kind {
	case Global:
		return true
	case Project:
		return scope.Value == project
	default:
		return false
	}
}

func formatScope(scope Scope) string {
	if scope.Kind == Global {
		return string(Global)
	}
	return string(scope.Kind) + ":" + scope.Value
}

func validateRecord(record Record) error {
	if record.Version != Version || validateID(record.ID) != nil {
		return errors.New("learning identity is invalid")
	}
	if !validType(record.Type) || !validKey(record.Key) || !validContent(record.Content) || validateScope(record.Scope) != nil {
		return errors.New("learning content is invalid")
	}
	if !validStatus(record.Status) || !validOrigin(record.Origin) {
		return errors.New("learning state is invalid")
	}
	if record.Origin == UserAuthored && record.Confidence != 0 {
		return errors.New("user-authored learning cannot have inferred confidence")
	}
	if record.Origin == Inferred && (record.Confidence < 1 || record.Confidence > 100) {
		return errors.New("inferred learning confidence must be 1 through 100")
	}
	if record.Origin == Inferred && record.Status == Active && record.Provenance.UserConfirmedAt.IsZero() {
		return errors.New("inferred learning requires user confirmation before activation")
	}
	if record.CreatedAt.IsZero() || record.UpdatedAt.IsZero() || record.UpdatedAt.Before(record.CreatedAt) {
		return errors.New("learning timestamps are invalid")
	}
	if err := validateProvenance(record.Provenance); err != nil {
		return err
	}
	return nil
}

func validateScope(scope Scope) error {
	normalized, err := normalizeScope(scope)
	if err != nil || normalized != scope {
		return errors.New("learning scope is invalid")
	}
	return nil
}

func normalizeScope(scope Scope) (Scope, error) {
	switch scope.Kind {
	case Global:
		if scope.Value != "" {
			return Scope{}, errors.New("global learning cannot have a scope value")
		}
	case Project:
		value, err := normalizeProject(scope.Value)
		if err != nil {
			return Scope{}, err
		}
		scope.Value = value
	default:
		return Scope{}, errors.New("learning scope is invalid")
	}
	return scope, nil
}

func normalizeProject(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", errors.New("project scope requires a path")
	}
	abs, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func validateProvenance(provenance Provenance) error {
	if len(provenance.WorkIDs) > 64 || len(provenance.EvidenceRefs) > 64 {
		return errors.New("learning provenance has too many references")
	}
	for _, id := range provenance.WorkIDs {
		if validateID(id) != nil {
			return errors.New("learning provenance Work ID is invalid")
		}
	}
	for _, reference := range provenance.EvidenceRefs {
		if len(reference) == 0 || len(reference) > 512 || strings.ContainsRune(reference, 0) {
			return errors.New("learning provenance evidence reference is invalid")
		}
	}
	return nil
}

func validType(value Type) bool {
	switch value {
	case Preference, EnvironmentFact, Procedure, FailurePrevention:
		return true
	default:
		return false
	}
}

func validStatus(value Status) bool {
	switch value {
	case Candidate, Active, Disabled, Rejected:
		return true
	default:
		return false
	}
}

func validOrigin(value Origin) bool { return value == UserAuthored || value == Inferred }

func validKey(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) == 0 || len(value) > 96 {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			continue
		}
		return false
	}
	return true
}

func validContent(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && len(value) <= 4096 && !strings.ContainsRune(value, 0)
}

func validateID(value string) error {
	if len(value) == 0 || len(value) > 128 || value == "." || value == ".." {
		return errors.New("learning ID is invalid")
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return errors.New("learning ID is invalid")
	}
	return nil
}

func newID() (string, error) {
	var data [8]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "", err
	}
	return "learning-" + hex.EncodeToString(data[:]), nil
}

func cloneProvenance(value Provenance) Provenance {
	value.WorkIDs = append([]string(nil), value.WorkIDs...)
	value.EvidenceRefs = append([]string(nil), value.EvidenceRefs...)
	return value
}

func readRecord(path, expectedID string) (Record, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return Record{}, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxRecordBytes {
		return Record{}, errors.New("learning record must be a bounded regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return Record{}, err
	}
	defer file.Close()
	var record Record
	decoder := json.NewDecoder(io.LimitReader(file, maxRecordBytes+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&record); err != nil {
		return Record{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Record{}, errors.New("expected one learning JSON document")
	}
	if record.ID != expectedID || validateRecord(record) != nil {
		return Record{}, errors.New("learning record is invalid")
	}
	return record, nil
}

func writeExclusive(path string, record Record) error {
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(append(data, '\n')); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return nil
}

func writeAtomic(path string, record Record) error {
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".learning-*.tmp")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(append(data, '\n')); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

func totalBytes(values []string) int {
	total := 0
	for _, value := range values {
		total += len(value)
	}
	return total
}
