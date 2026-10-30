package goal

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gongahkia/gator/internal/filelock"
	"github.com/gongahkia/gator/internal/privatefs"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

const PlanVersion = 1

type StepStatus string

const (
	StepPending StepStatus = "pending"
	StepDoing   StepStatus = "doing"
	StepBlocked StepStatus = "blocked"
	StepDone    StepStatus = "done"
)

// Plan is the durable, user-reviewable state of an ongoing responsibility.
// Conversation text can inform a change, but does not mutate this record.
type Plan struct {
	LastChangeID   string    `json:"last_change_id,omitempty"`
	LastChangeHash string    `json:"last_change_hash,omitempty"`
	Version        int       `json:"version"`
	GoalID         string    `json:"goal_id"`
	Revision       int       `json:"revision"`
	Outcome        string    `json:"outcome,omitempty"`
	Steps          []Step    `json:"steps,omitempty"`
	LastReviewAt   time.Time `json:"last_review_at,omitempty"`
	NextReviewAt   time.Time `json:"next_review_at,omitempty"`
	UpdatedAt      time.Time `json:"updated_at,omitempty"`
}

type Step struct {
	StartsAt  time.Time  `json:"starts_at,omitempty"`
	EndsAt    time.Time  `json:"ends_at,omitempty"`
	DependsOn []string   `json:"depends_on,omitempty"`
	ID        string     `json:"id"`
	Title     string     `json:"title"`
	Status    StepStatus `json:"status"`
	DueAt     time.Time  `json:"due_at,omitempty"`
	Evidence  []string   `json:"evidence,omitempty"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// PlanChange is a single typed edit. ExpectedRevision prevents a stale agent
// proposal from overwriting a later user correction.
type PlanChange struct {
	StartsAt          time.Time  `json:"starts_at,omitempty"`
	EndsAt            time.Time  `json:"ends_at,omitempty"`
	ClearSchedule     bool       `json:"clear_schedule,omitempty"`
	DependsOn         []string   `json:"depends_on,omitempty"`
	ClearDependencies bool       `json:"clear_dependencies,omitempty"`
	Kind              string     `json:"kind"`
	ExpectedRevision  int        `json:"expected_revision"`
	StepID            string     `json:"step_id,omitempty"`
	Title             string     `json:"title,omitempty"`
	Status            StepStatus `json:"status,omitempty"`
	DueAt             time.Time  `json:"due_at,omitempty"`
	ClearDue          bool       `json:"clear_due,omitempty"`
	Evidence          []string   `json:"evidence,omitempty"`
	ClearEvidence     bool       `json:"clear_evidence,omitempty"`
	Outcome           string     `json:"outcome,omitempty"`
	NextReviewAt      time.Time  `json:"next_review_at,omitempty"`
}

func (s Store) LoadPlan(goalID string) (Plan, error) {
	if _, err := s.Load(goalID); err != nil {
		return Plan{}, err
	}
	return s.loadPlan(goalID)
}

// InitializePlan publishes an entire reviewed starting plan in one file write.
// Reapplying the same proposal returns the existing plan without resetting any
// later user edits to its steps or review time.
func (s Store) InitializePlan(goalID, outcome string, titles []string) (Plan, error) {
	if _, err := s.Load(goalID); err != nil {
		return Plan{}, err
	}
	outcome = strings.TrimSpace(outcome)
	if outcome == "" || len(outcome) > 2048 || !utf8.ValidString(outcome) || strings.ContainsRune(outcome, 0) || len(titles) == 0 || len(titles) > 8 {
		return Plan{}, errors.New("invalid initial goal plan")
	}
	for _, title := range titles {
		if !validStepTitle(title) {
			return Plan{}, errors.New("invalid initial goal step")
		}
	}
	lock, err := s.lockPlan(goalID)
	if err != nil {
		return Plan{}, err
	}
	defer func() { _ = filelock.Unlock(lock); _ = lock.Close() }()
	current, err := s.loadPlan(goalID)
	if err != nil {
		return Plan{}, err
	}
	if current.Revision > 0 {
		if current.Outcome != outcome || len(current.Steps) < len(titles) {
			return Plan{}, errors.New("goal already has a different plan")
		}
		for i, title := range titles {
			if current.Steps[i].Title != strings.TrimSpace(title) {
				return Plan{}, errors.New("goal already has a different plan")
			}
		}
		return current, nil
	}
	now := s.now().UTC()
	plan := Plan{Version: PlanVersion, GoalID: goalID, Revision: 1, Outcome: outcome, UpdatedAt: now}
	for _, title := range titles {
		var random [8]byte
		if _, err := rand.Read(random[:]); err != nil {
			return Plan{}, err
		}
		plan.Steps = append(plan.Steps, Step{ID: "step-" + hex.EncodeToString(random[:]), Title: strings.TrimSpace(title), Status: StepPending, UpdatedAt: now})
	}
	if err := validatePlan(plan); err != nil {
		return Plan{}, err
	}
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return Plan{}, err
	}
	if err := writePrivatePlan(s.planPath(goalID), data); err != nil {
		return Plan{}, err
	}
	return plan, nil
}

func (s Store) loadPlan(goalID string) (Plan, error) {
	plan := Plan{Version: PlanVersion, GoalID: goalID}
	path := s.planPath(goalID)
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return plan, nil
	}
	if err != nil {
		return Plan{}, err
	}
	if !info.Mode().IsRegular() || info.Size() > 64*1024 {
		return Plan{}, errors.New("goal plan must be a bounded regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return Plan{}, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) || !opened.Mode().IsRegular() {
		return Plan{}, errors.New("goal plan changed during read")
	}
	data, err := io.ReadAll(io.LimitReader(file, 64*1024+1))
	if err != nil || len(data) > 64*1024 {
		return Plan{}, errors.New("goal plan exceeds size limit")
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&plan); err != nil {
		return Plan{}, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return Plan{}, errors.New("goal plan has trailing data")
	}
	if plan.GoalID != goalID {
		return Plan{}, errors.New("goal plan identity mismatch")
	}
	return plan, validatePlan(plan)
}

func (s Store) ApplyPlanChange(goalID string, change PlanChange) (Plan, error) {
	return s.applyPlanChange(goalID, change, "")
}

// ApplyReviewedPlanChange safely retries one accepted proposal after a crash.
func (s Store) ApplyReviewedPlanChange(goalID string, change PlanChange, mutationID string) (Plan, error) {
	if !strings.HasPrefix(mutationID, "suggestion-") || len(mutationID) != 35 {
		return Plan{}, errors.New("invalid plan proposal identity")
	}
	if _, err := hex.DecodeString(mutationID[11:]); err != nil {
		return Plan{}, err
	}
	return s.applyPlanChange(goalID, change, mutationID)
}

func (s Store) applyPlanChange(goalID string, change PlanChange, mutationID string) (Plan, error) {
	if _, err := s.Load(goalID); err != nil {
		return Plan{}, err
	}
	if err := validateChange(change); err != nil {
		return Plan{}, err
	}
	graphLock, err := s.lockPlan("dependency-graph")
	if err != nil {
		return Plan{}, err
	}
	defer func() { _ = filelock.Unlock(graphLock); _ = graphLock.Close() }()
	lock, err := s.lockPlan(goalID)
	if err != nil {
		return Plan{}, err
	}
	defer func() { _ = filelock.Unlock(lock); _ = lock.Close() }()
	plan, err := s.loadPlan(goalID)
	if err != nil {
		return Plan{}, err
	}
	input, _ := json.Marshal(change)
	digest := sha256.Sum256(input)
	changeHash := hex.EncodeToString(digest[:])
	if mutationID != "" && plan.LastChangeID == mutationID {
		if plan.LastChangeHash == changeHash && plan.Revision == change.ExpectedRevision+1 {
			return plan, nil
		}
		return Plan{}, errors.New("accepted plan changed; inspect it again")
	}
	if plan.Revision != change.ExpectedRevision {
		return Plan{}, fmt.Errorf("goal plan changed: expected revision %d, now %d", change.ExpectedRevision, plan.Revision)
	}
	now := s.now().UTC()
	switch change.Kind {
	case "set_outcome":
		plan.Outcome = strings.TrimSpace(change.Outcome)
	case "add_step":
		if len(plan.Steps) >= 32 {
			return Plan{}, errors.New("goal plan has 32 steps; complete or remove one first")
		}
		var random [8]byte
		if _, err := rand.Read(random[:]); err != nil {
			return Plan{}, err
		}
		plan.Steps = append(plan.Steps, Step{StartsAt: change.StartsAt.UTC(), EndsAt: change.EndsAt.UTC(), DependsOn: append([]string(nil), change.DependsOn...), ID: "step-" + hex.EncodeToString(random[:]), Title: strings.TrimSpace(change.Title), Status: StepPending, DueAt: change.DueAt.UTC(), Evidence: append([]string(nil), change.Evidence...), UpdatedAt: now})
	case "update_step":
		found := false
		for i := range plan.Steps {
			if plan.Steps[i].ID == change.StepID {
				if change.Title != "" {
					plan.Steps[i].Title = strings.TrimSpace(change.Title)
				}
				if change.Status != "" {
					plan.Steps[i].Status = change.Status
				}
				if !change.DueAt.IsZero() {
					plan.Steps[i].DueAt = change.DueAt.UTC()
				}
				if !change.StartsAt.IsZero() {
					plan.Steps[i].StartsAt, plan.Steps[i].EndsAt = change.StartsAt.UTC(), change.EndsAt.UTC()
				}
				if change.ClearSchedule {
					plan.Steps[i].StartsAt, plan.Steps[i].EndsAt = time.Time{}, time.Time{}
				}
				if change.DependsOn != nil {
					plan.Steps[i].DependsOn = append([]string(nil), change.DependsOn...)
				}
				if change.ClearDependencies {
					plan.Steps[i].DependsOn = nil
				}
				if change.ClearDue {
					plan.Steps[i].DueAt = time.Time{}
				}
				if change.Evidence != nil {
					plan.Steps[i].Evidence = append([]string(nil), change.Evidence...)
				}
				if change.ClearEvidence {
					plan.Steps[i].Evidence = nil
				}
				plan.Steps[i].UpdatedAt = now
				found = true
				break
			}
		}
		if !found {
			return Plan{}, errors.New("goal step not found")
		}
	case "remove_step":
		found := false
		for i := range plan.Steps {
			if plan.Steps[i].ID == change.StepID {
				plan.Steps = append(plan.Steps[:i], plan.Steps[i+1:]...)
				found = true
				break
			}
		}
		if !found {
			return Plan{}, errors.New("goal step not found")
		}
	case "set_next_review":
		plan.NextReviewAt = change.NextReviewAt.UTC()
	case "clear_next_review":
		plan.NextReviewAt = time.Time{}
	case "mark_reviewed":
		plan.LastReviewAt = now
		if !plan.NextReviewAt.IsZero() && !plan.NextReviewAt.After(now) {
			plan.NextReviewAt = time.Time{}
		}
	}
	plan.LastChangeID, plan.LastChangeHash = mutationID, ""
	if mutationID != "" {
		plan.LastChangeHash = changeHash
	}
	plan.Revision++
	plan.UpdatedAt = now
	if err := validatePlan(plan); err != nil {
		return Plan{}, err
	}
	if err := s.validateDependencyGraph(plan); err != nil {
		return Plan{}, err
	}
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return Plan{}, err
	}
	if len(data) > 64*1024 {
		return Plan{}, errors.New("goal plan exceeds size limit")
	}
	if err := writePrivatePlan(s.planPath(goalID), data); err != nil {
		return Plan{}, err
	}
	return plan, nil
}

func (s Store) planPath(goalID string) string { return filepath.Join(s.root, "plans", goalID+".json") }

func (s Store) lockPlan(goalID string) (*os.File, error) {
	path := filepath.Join(s.root, "plans", goalID+".lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(path)
	opened, statErr := file.Stat()
	if err != nil || statErr != nil || !info.Mode().IsRegular() || !os.SameFile(info, opened) {
		_ = file.Close()
		return nil, errors.New("goal plan lock is not a regular file")
	}
	if err := filelock.Lock(file, filelock.Exclusive); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

func writePrivatePlan(path string, data []byte) error {
	temp, err := os.CreateTemp(filepath.Dir(path), ".plan-")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if err := temp.Chmod(0600); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(temp.Name(), path); err != nil {
		return err
	}
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer directory.Close()
	return privatefs.SyncDir(directory)
}

func validateChange(change PlanChange) error {
	if err := validateSchedule(change.StartsAt, change.EndsAt, change.DependsOn); err != nil {
		return err
	}
	if (change.ClearSchedule && !change.StartsAt.IsZero()) || (change.ClearDependencies && change.DependsOn != nil) {
		return errors.New("cannot set and clear schedule or dependencies together")
	}
	if change.Kind != "add_step" && change.Kind != "update_step" && (!change.StartsAt.IsZero() || change.ClearSchedule || change.DependsOn != nil || change.ClearDependencies) {
		return errors.New("scheduling fields require a step change")
	}

	if change.ExpectedRevision < 0 || change.ExpectedRevision > 1000000 {
		return errors.New("invalid expected plan revision")
	}
	switch change.Kind {
	case "set_outcome":
		if strings.TrimSpace(change.Outcome) == "" || len(change.Outcome) > 2048 || !utf8.ValidString(change.Outcome) || strings.ContainsRune(change.Outcome, 0) {
			return errors.New("invalid goal outcome")
		}
	case "add_step":
		if !validStepTitle(change.Title) || !validEvidence(change.Evidence) {
			return errors.New("invalid new goal step")
		}
	case "update_step":
		if !validStepID(change.StepID) || (change.Title != "" && !validStepTitle(change.Title)) || (change.Status != "" && !validStepStatus(change.Status)) || !validEvidence(change.Evidence) || (change.ClearDue && !change.DueAt.IsZero()) || (change.ClearEvidence && change.Evidence != nil) {
			return errors.New("invalid goal step update")
		}
		if change.Title == "" && change.Status == "" && change.DueAt.IsZero() && change.Evidence == nil && !change.ClearDue && !change.ClearEvidence && change.StartsAt.IsZero() && !change.ClearSchedule && change.DependsOn == nil && !change.ClearDependencies {
			return errors.New("goal step update is empty")
		}
	case "remove_step":
		if !validStepID(change.StepID) {
			return errors.New("invalid goal step ID")
		}
	case "set_next_review":
		if change.NextReviewAt.IsZero() {
			return errors.New("next review time is required")
		}
	case "clear_next_review":
	case "mark_reviewed":
	default:
		return errors.New("unknown goal plan change")
	}
	return nil
}

// ValidatePlanChange checks a proposed edit without applying it.
func ValidatePlanChange(change PlanChange) error { return validateChange(change) }

func validatePlan(plan Plan) error {
	if plan.Version != PlanVersion || !validID(plan.GoalID) || plan.Revision < 0 || plan.Revision > 1000000 || len(plan.Steps) > 32 || len(plan.Outcome) > 2048 || !utf8.ValidString(plan.Outcome) || strings.ContainsRune(plan.Outcome, 0) {
		return errors.New("invalid goal plan")
	}
	seen := map[string]bool{}
	for _, step := range plan.Steps {
		if !validStepID(step.ID) || seen[step.ID] || !validStepTitle(step.Title) || !validStepStatus(step.Status) || !validEvidence(step.Evidence) || step.UpdatedAt.IsZero() {
			return errors.New("invalid goal plan step")
		}
		if err := validateSchedule(step.StartsAt, step.EndsAt, step.DependsOn); err != nil {
			return err
		}
		seen[step.ID] = true
	}
	return validateDependencies(plan.Steps)
}

func validStepID(id string) bool {
	if !strings.HasPrefix(id, "step-") || len(id) != 21 {
		return false
	}
	_, err := hex.DecodeString(id[5:])
	return err == nil
}
func validStepStatus(status StepStatus) bool {
	return status == StepPending || status == StepDoing || status == StepBlocked || status == StepDone
}
func validStepTitle(title string) bool {
	return strings.TrimSpace(title) != "" && len(title) <= 512 && utf8.ValidString(title) && !strings.ContainsRune(title, 0)
}
func validEvidence(items []string) bool {
	if len(items) > 8 {
		return false
	}
	for _, item := range items {
		if strings.TrimSpace(item) == "" || len(item) > 256 || !utf8.ValidString(item) || strings.ContainsRune(item, 0) {
			return false
		}
	}
	return true
}
