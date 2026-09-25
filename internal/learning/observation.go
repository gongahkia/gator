package learning

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const observationVersion = 1

// Signal is one learning-relevant, observable event. It deliberately omits
// ambiguous conversational behavior and raw model interpretation.
type Signal string

const (
	WorkFailed             Signal = "work_failed"
	VerificationFailed     Signal = "verification_failed"
	DeliveryFailed         Signal = "delivery_failed"
	DeliveryUnknown        Signal = "delivery_unknown"
	ExternalOutcomeUnknown Signal = "external_outcome_unknown"
	UserAccepted           Signal = "user_accepted"
	UserRejected           Signal = "user_rejected"
	UserCorrected          Signal = "user_corrected"
	UserRemembered         Signal = "user_remembered"
	UserDeclinedLearning   Signal = "user_declined_learning"
)

type Reliability string

const (
	HighReliability Reliability = "high"
)

// Observation records a bounded fact that may later support a candidate
// learning. It is separate from the mutable learning itself and references
// existing Work/delivery evidence instead of copying it.
type Observation struct {
	Version      int         `json:"version"`
	ID           string      `json:"id"`
	Signal       Signal      `json:"signal"`
	Reliability  Reliability `json:"reliability"`
	WorkID       string      `json:"work_id"`
	Scope        *Scope      `json:"scope,omitempty"`
	Summary      string      `json:"summary,omitempty"`
	EvidenceRefs []string    `json:"evidence_refs,omitempty"`
	LearningIDs  []string    `json:"learning_ids,omitempty"`
	CreatedAt    time.Time   `json:"created_at"`
	UpdatedAt    time.Time   `json:"updated_at"`
}

type ObservationInput struct {
	ID           string
	Signal       Signal
	Reliability  Reliability
	WorkID       string
	Scope        *Scope
	Summary      string
	EvidenceRefs []string
	LearningIDs  []string
}

// Proposal describes a deterministic candidate derived from one bounded
// observation. Creating a proposal never activates it.
type Proposal struct {
	Observation Observation
	Type        Type
	Key         string
	Content     string
	Scope       Scope
	Confidence  int
}

func (s Store) RecordObservation(input ObservationInput) (Observation, error) {
	if input.ID == "" {
		id, err := newObservationID()
		if err != nil {
			return Observation{}, err
		}
		input.ID = id
	}
	if input.Reliability == "" {
		input.Reliability = HighReliability
	}
	if input.Scope != nil {
		normalized, err := normalizeScope(*input.Scope)
		if err != nil {
			return Observation{}, err
		}
		input.Scope = &normalized
	}
	now := s.clock()().UTC()
	observation := Observation{
		Version: observationVersion, ID: input.ID, Signal: input.Signal,
		Reliability: input.Reliability, WorkID: input.WorkID, Scope: input.Scope,
		Summary: input.Summary, EvidenceRefs: append([]string(nil), input.EvidenceRefs...),
		LearningIDs: append([]string(nil), input.LearningIDs...), CreatedAt: now, UpdatedAt: now,
	}
	if err := validateObservation(observation); err != nil {
		return Observation{}, err
	}
	if _, err := s.observationsRoot(); err != nil {
		return Observation{}, err
	}
	if err := writeExclusive(s.observationPath(observation.ID), observation); err != nil {
		return Observation{}, err
	}
	return observation, nil
}

// UpsertObservation records an idempotent operational signal. Delivery and
// external-state reconciliation can update one existing observation while the
// authoritative attempt history stays in its delivery/action record.
func (s Store) UpsertObservation(input ObservationInput) (Observation, error) {
	if strings.TrimSpace(input.ID) == "" {
		return s.RecordObservation(input)
	}
	existing, err := s.LoadObservation(input.ID)
	if errors.Is(err, os.ErrNotExist) {
		return s.RecordObservation(input)
	}
	if err != nil {
		return Observation{}, err
	}
	if existing.WorkID != input.WorkID || existing.Signal != input.Signal {
		return Observation{}, errors.New("observation identity does not match existing evidence")
	}
	if input.Reliability != "" {
		existing.Reliability = input.Reliability
	}
	if input.Scope != nil {
		normalized, normalizeErr := normalizeScope(*input.Scope)
		if normalizeErr != nil {
			return Observation{}, normalizeErr
		}
		existing.Scope = &normalized
	}
	if strings.TrimSpace(input.Summary) != "" {
		existing.Summary = input.Summary
	}
	existing.EvidenceRefs = mergeUnique(existing.EvidenceRefs, input.EvidenceRefs)
	existing.LearningIDs = mergeUnique(existing.LearningIDs, input.LearningIDs)
	existing.UpdatedAt = s.clock()().UTC()
	if err := validateObservation(existing); err != nil {
		return Observation{}, err
	}
	if err := writeAtomic(s.observationPath(existing.ID), existing); err != nil {
		return Observation{}, err
	}
	return existing, nil
}

func (s Store) LoadObservation(id string) (Observation, error) {
	if err := validateID(id); err != nil {
		return Observation{}, err
	}
	var observation Observation
	if err := readJSONRecord(s.observationPath(id), &observation); err != nil {
		return Observation{}, err
	}
	if observation.ID != id || validateObservation(observation) != nil {
		return Observation{}, errors.New("observation record is invalid")
	}
	return observation, nil
}

func (s Store) ListObservations(workID string) ([]Observation, error) {
	if err := validateID(workID); err != nil {
		return nil, err
	}
	root, err := s.observationsRoot()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	observations := make([]Observation, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		observation, loadErr := s.LoadObservation(strings.TrimSuffix(entry.Name(), ".json"))
		if loadErr != nil {
			return nil, fmt.Errorf("read observation %q: %w", entry.Name(), loadErr)
		}
		if observation.WorkID == workID {
			observations = append(observations, observation)
		}
	}
	sort.Slice(observations, func(i, j int) bool {
		if observations[i].CreatedAt.Equal(observations[j].CreatedAt) {
			return observations[i].ID < observations[j].ID
		}
		return observations[i].CreatedAt.Before(observations[j].CreatedAt)
	})
	return observations, nil
}

// Propose creates a candidate from an explicit bounded observation, or adds
// evidence to an existing non-rejected inferred rule with the same identity.
// It never changes the candidate's status to active.
func (s Store) Propose(input Proposal) (Record, error) {
	if err := validateObservation(input.Observation); err != nil {
		return Record{}, err
	}
	if input.Observation.Signal != UserCorrected {
		return Record{}, errors.New("only an explicit user correction can propose a learning in this tranche")
	}
	normalizedScope, err := normalizeScope(input.Scope)
	if err != nil {
		return Record{}, err
	}
	if input.Confidence == 0 {
		input.Confidence = 100
	}
	if input.Confidence < 1 || input.Confidence > 100 {
		return Record{}, errors.New("candidate confidence must be 1 through 100")
	}
	records, err := s.List()
	if err != nil {
		return Record{}, err
	}
	for _, record := range records {
		if record.Origin != Inferred || record.Status == Rejected || record.Type != input.Type || record.Key != input.Key || record.Content != input.Content || record.Scope != normalizedScope {
			continue
		}
		record.Provenance.WorkIDs = mergeUnique(record.Provenance.WorkIDs, []string{input.Observation.WorkID})
		record.Provenance.EvidenceRefs = mergeUnique(record.Provenance.EvidenceRefs, []string{s.observationReference(input.Observation.ID)})
		if input.Confidence > record.Confidence {
			record.Confidence = input.Confidence
		}
		return s.save(record)
	}
	return s.Create(Create{
		Type: input.Type, Key: input.Key, Content: input.Content, Scope: normalizedScope,
		Origin: Inferred, Confidence: input.Confidence,
		Provenance: Provenance{WorkIDs: []string{input.Observation.WorkID}, EvidenceRefs: []string{s.observationReference(input.Observation.ID)}},
	})
}

// LinkObservation records that a candidate or explicit learning was created
// from this observation without copying its content into the learning record.
func (s Store) LinkObservation(id, learningID string) (Observation, error) {
	observation, err := s.LoadObservation(id)
	if err != nil {
		return Observation{}, err
	}
	if err := validateID(learningID); err != nil {
		return Observation{}, err
	}
	observation.LearningIDs = mergeUnique(observation.LearningIDs, []string{learningID})
	observation.UpdatedAt = s.clock()().UTC()
	if err := validateObservation(observation); err != nil {
		return Observation{}, err
	}
	if err := writeAtomic(s.observationPath(observation.ID), observation); err != nil {
		return Observation{}, err
	}
	return observation, nil
}

func (s Store) observationsRoot() (string, error) {
	root := filepath.Join(filepath.Dir(s.root), "observations")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", err
	}
	if err := os.Chmod(root, 0o700); err != nil {
		return "", err
	}
	return root, nil
}

func (s Store) observationPath(id string) string {
	return filepath.Join(filepath.Dir(s.root), "observations", id+".json")
}

func (s Store) observationReference(id string) string {
	return filepath.ToSlash(filepath.Join("gator", "observations", id+".json"))
}

func validateObservation(observation Observation) error {
	if observation.Version != observationVersion || validateID(observation.ID) != nil || !validSignal(observation.Signal) || observation.Reliability != HighReliability || validateID(observation.WorkID) != nil {
		return errors.New("observation identity is invalid")
	}
	if observation.Scope != nil && validateScope(*observation.Scope) != nil {
		return errors.New("observation scope is invalid")
	}
	if len(observation.Summary) > 1024 || strings.ContainsRune(observation.Summary, 0) || observation.CreatedAt.IsZero() || observation.UpdatedAt.IsZero() || observation.UpdatedAt.Before(observation.CreatedAt) {
		return errors.New("observation content is invalid")
	}
	if len(observation.EvidenceRefs) > 64 || len(observation.LearningIDs) > 64 {
		return errors.New("observation has too many references")
	}
	if err := validateReferences(observation.EvidenceRefs); err != nil {
		return err
	}
	for _, id := range observation.LearningIDs {
		if validateID(id) != nil {
			return errors.New("observation learning reference is invalid")
		}
	}
	return nil
}

func validSignal(signal Signal) bool {
	switch signal {
	case WorkFailed, VerificationFailed, DeliveryFailed, DeliveryUnknown, ExternalOutcomeUnknown, UserAccepted, UserRejected, UserCorrected, UserRemembered, UserDeclinedLearning:
		return true
	default:
		return false
	}
}

func validateReferences(references []string) error {
	for _, reference := range references {
		if len(reference) == 0 || len(reference) > 512 || strings.ContainsRune(reference, 0) {
			return errors.New("observation evidence reference is invalid")
		}
	}
	return nil
}

func mergeUnique(values, additions []string) []string {
	result := append([]string(nil), values...)
	seen := make(map[string]bool, len(result)+len(additions))
	for _, value := range result {
		seen[value] = true
	}
	for _, value := range additions {
		if value != "" && !seen[value] {
			result = append(result, value)
			seen[value] = true
		}
	}
	return result
}

func newObservationID() (string, error) {
	id, err := newID()
	if err != nil {
		return "", err
	}
	return "observation-" + strings.TrimPrefix(id, "learning-"), nil
}
