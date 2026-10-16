package goal

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
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
	Version      int       `json:"version"`
	GoalID       string    `json:"goal_id"`
	Revision     int       `json:"revision"`
	Outcome      string    `json:"outcome,omitempty"`
	Steps        []Step    `json:"steps,omitempty"`
	LastReviewAt time.Time `json:"last_review_at,omitempty"`
	NextReviewAt time.Time `json:"next_review_at,omitempty"`
	UpdatedAt    time.Time `json:"updated_at,omitempty"`
}

type Step struct {
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
	Kind             string     `json:"kind"`
	ExpectedRevision int        `json:"expected_revision"`
	StepID           string     `json:"step_id,omitempty"`
	Title            string     `json:"title,omitempty"`
	Status           StepStatus `json:"status,omitempty"`
	DueAt            time.Time  `json:"due_at,omitempty"`
	ClearDue         bool       `json:"clear_due,omitempty"`
	Evidence         []string   `json:"evidence,omitempty"`
	ClearEvidence    bool       `json:"clear_evidence,omitempty"`
	Outcome          string     `json:"outcome,omitempty"`
	NextReviewAt     time.Time  `json:"next_review_at,omitempty"`
}

func (s Store) LoadPlan(goalID string) (Plan, error) {
	if _, err := s.Load(goalID); err != nil {
		return Plan{}, err
	}
	return s.loadPlan(goalID)
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
	if _, err := s.Load(goalID); err != nil {
		return Plan{}, err
	}
	if err := validateChange(change); err != nil {
		return Plan{}, err
	}
	lock, err := s.lockPlan(goalID)
	if err != nil {
		return Plan{}, err
	}
	defer func() { _ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN); _ = lock.Close() }()
	plan, err := s.loadPlan(goalID)
	if err != nil {
		return Plan{}, err
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
		plan.Steps = append(plan.Steps, Step{ID: "step-" + hex.EncodeToString(random[:]), Title: strings.TrimSpace(change.Title), Status: StepPending, DueAt: change.DueAt.UTC(), Evidence: append([]string(nil), change.Evidence...), UpdatedAt: now})
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
	}
	plan.Revision++
	plan.UpdatedAt = now
	if err := validatePlan(plan); err != nil {
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
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
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
	return directory.Sync()
}

func validateChange(change PlanChange) error {
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
		if change.Title == "" && change.Status == "" && change.DueAt.IsZero() && change.Evidence == nil && !change.ClearDue && !change.ClearEvidence {
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
		seen[step.ID] = true
	}
	return nil
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
