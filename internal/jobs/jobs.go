// Package jobs stores and schedules durable unattended Gator Work definitions.
package jobs

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/robfig/cron/v3"
)

const Version = 1

type Definition struct {
	Version         int               `json:"version"`
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	Enabled         bool              `json:"enabled"`
	Schedule        string            `json:"schedule"`
	Timezone        string            `json:"timezone"`
	Missed          string            `json:"missed"`
	SourcePath      string            `json:"source_path"`
	RefreshSnapshot bool              `json:"refresh_snapshot"`
	Objective       string            `json:"objective"`
	Provider        string            `json:"provider,omitempty"`
	Model           string            `json:"model,omitempty"`
	Mode            action.Mode       `json:"mode"`
	Contract        artifact.Contract `json:"contract"`
	ConnectorIDs    []string          `json:"connector_ids,omitempty"`
	MaxSteps        int               `json:"max_steps"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
	LastScheduledAt time.Time         `json:"last_scheduled_at,omitempty"`
}

type Attempt struct {
	Version      int       `json:"version"`
	ID           string    `json:"id"`
	JobID        string    `json:"job_id"`
	ScheduledAt  time.Time `json:"scheduled_at"`
	StartedAt    time.Time `json:"started_at"`
	FinishedAt   time.Time `json:"finished_at"`
	Try          int       `json:"try"`
	Status       string    `json:"status"`
	Conversation string    `json:"conversation_id,omitempty"`
	Revision     string    `json:"revision_id,omitempty"`
	BundlePath   string    `json:"bundle_path,omitempty"`
	Error        string    `json:"error,omitempty"`
}

type Store struct{ root string }

func Open(stateDir string) (Store, error) {
	if strings.TrimSpace(stateDir) == "" {
		return Store{}, errors.New("job state directory is required")
	}
	absolute, err := filepath.Abs(stateDir)
	if err != nil {
		return Store{}, err
	}
	root := filepath.Join(absolute, "gator", "jobs")
	for _, directory := range []string{root, filepath.Join(root, "definitions"), filepath.Join(root, "history")} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return Store{}, err
		}
		if err := os.Chmod(directory, 0o700); err != nil {
			return Store{}, err
		}
	}
	return Store{root: root}, nil
}

func (s Store) Root() string { return s.root }

func (s Store) Save(definition Definition) (Definition, error) {
	now := time.Now().UTC()
	if definition.Version == 0 {
		definition.Version = Version
	}
	if definition.ID == "" {
		definition.ID = newID("job")
	}
	if definition.Timezone == "" {
		definition.Timezone = "Local"
	}
	if definition.Missed == "" {
		definition.Missed = "skip"
	}
	if definition.Mode == "" {
		definition.Mode = action.Draft
	}
	if definition.MaxSteps == 0 {
		definition.MaxSteps = 24
	}
	if definition.CreatedAt.IsZero() {
		definition.CreatedAt = now
	}
	definition.UpdatedAt = now
	if err := definition.Validate(); err != nil {
		return Definition{}, err
	}
	if err := writeJSON(filepath.Join(s.root, "definitions", definition.ID+".json"), definition); err != nil {
		return Definition{}, err
	}
	return definition, nil
}

func (d Definition) Validate() error {
	if d.Version != Version || invalidID(d.ID) || strings.TrimSpace(d.Name) == "" || len(d.Name) > 128 || strings.TrimSpace(d.SourcePath) == "" || strings.TrimSpace(d.Objective) == "" || len(d.Objective) > 64*1024 {
		return errors.New("job identity is invalid")
	}
	if d.Mode != action.Inspect && d.Mode != action.Draft {
		return errors.New("scheduled jobs may only inspect or draft")
	}
	if d.Contract.Normalize().ExternalActions == action.Approve {
		return errors.New("scheduled jobs cannot approve external actions")
	}
	if err := d.Contract.Validate(); err != nil {
		return fmt.Errorf("job outcome contract: %w", err)
	}
	if d.Missed != "skip" && d.Missed != "run_once" {
		return errors.New("job missed policy must be skip or run_once")
	}
	if d.MaxSteps < 1 || d.MaxSteps > 1_000 || len(d.ConnectorIDs) > 16 {
		return errors.New("job execution limits are invalid")
	}
	if _, err := Schedule(d); err != nil {
		return err
	}
	return nil
}

func (s Store) Load(id string) (Definition, error) {
	if invalidID(id) {
		return Definition{}, errors.New("invalid job ID")
	}
	var definition Definition
	if err := readJSON(filepath.Join(s.root, "definitions", id+".json"), &definition); err != nil {
		return Definition{}, err
	}
	if definition.ID != id {
		return Definition{}, errors.New("job file identity does not match")
	}
	return definition, definition.Validate()
}

func (s Store) List() ([]Definition, error) {
	entries, err := os.ReadDir(filepath.Join(s.root, "definitions"))
	if err != nil {
		return nil, err
	}
	var definitions []Definition
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		definition, err := s.Load(strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			return nil, err
		}
		definitions = append(definitions, definition)
	}
	sort.Slice(definitions, func(i, j int) bool { return definitions[i].Name < definitions[j].Name })
	return definitions, nil
}

func (s Store) SetEnabled(id string, enabled bool) (Definition, error) {
	definition, err := s.Load(id)
	if err != nil {
		return Definition{}, err
	}
	definition.Enabled = enabled
	return s.Save(definition)
}

func (s Store) Claim(id string, scheduled time.Time) (Definition, bool, error) {
	definition, err := s.Load(id)
	if err != nil {
		return Definition{}, false, err
	}
	scheduled = scheduled.UTC().Truncate(time.Minute)
	if !definition.LastScheduledAt.IsZero() && !definition.LastScheduledAt.Before(scheduled) {
		return definition, false, nil
	}
	definition.LastScheduledAt = scheduled
	definition, err = s.Save(definition)
	return definition, err == nil, err
}

func (s Store) Remove(id string) error {
	if _, err := s.Load(id); err != nil {
		return err
	}
	return os.Remove(filepath.Join(s.root, "definitions", id+".json"))
}

func (s Store) Record(attempt Attempt) error {
	if attempt.Version == 0 {
		attempt.Version = Version
	}
	if attempt.ID == "" {
		attempt.ID = newID("attempt")
	}
	if invalidID(attempt.ID) || invalidID(attempt.JobID) || attempt.StartedAt.IsZero() || attempt.FinishedAt.Before(attempt.StartedAt) || attempt.Try < 1 || strings.TrimSpace(attempt.Status) == "" {
		return errors.New("job attempt is invalid")
	}
	directory := filepath.Join(s.root, "history", attempt.JobID)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	return writeJSON(filepath.Join(directory, attempt.ID+".json"), attempt)
}

func (s Store) History(id string, limit int) ([]Attempt, error) {
	if _, err := s.Load(id); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(s.root, "history", id))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var attempts []Attempt
	for _, entry := range entries {
		var attempt Attempt
		if !entry.IsDir() && readJSON(filepath.Join(s.root, "history", id, entry.Name()), &attempt) == nil {
			attempts = append(attempts, attempt)
		}
	}
	sort.Slice(attempts, func(i, j int) bool { return attempts[i].StartedAt.After(attempts[j].StartedAt) })
	if limit > 0 && len(attempts) > limit {
		attempts = attempts[:limit]
	}
	return attempts, nil
}

func Schedule(definition Definition) (cron.Schedule, error) {
	location, err := time.LoadLocation(definition.Timezone)
	if err != nil {
		return nil, fmt.Errorf("load job timezone: %w", err)
	}
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	schedule, err := parser.Parse(definition.Schedule)
	if err != nil {
		return nil, fmt.Errorf("parse five-field job schedule: %w", err)
	}
	return locatedSchedule{Schedule: schedule, Location: location}, nil
}

type locatedSchedule struct {
	cron.Schedule
	Location *time.Location
}

func (s locatedSchedule) Next(after time.Time) time.Time {
	return s.Schedule.Next(after.In(s.Location)).In(after.Location())
}

func Due(definition Definition, now time.Time) (time.Time, bool, error) {
	schedule, err := Schedule(definition)
	if err != nil {
		return time.Time{}, false, err
	}
	minute := now.Truncate(time.Minute)
	currentDue := schedule.Next(minute.Add(-time.Minute)).Equal(minute)
	if currentDue && (definition.LastScheduledAt.IsZero() || definition.LastScheduledAt.Before(minute)) {
		return minute, true, nil
	}
	if definition.Missed == "run_once" && !definition.LastScheduledAt.IsZero() {
		next := schedule.Next(definition.LastScheduledAt)
		if !next.After(minute) {
			return next, true, nil
		}
	}
	return time.Time{}, false, nil
}

func newID(prefix string) string {
	value := make([]byte, 8)
	_, _ = rand.Read(value)
	return prefix + "-" + time.Now().UTC().Format("20060102T150405") + "-" + hex.EncodeToString(value)
}

func invalidID(id string) bool {
	return id == "" || len(id) > 128 || strings.ContainsAny(id, "/\\\x00\r\n")
}

func readJSON(path string, target any) error {
	payload, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func writeJSON(path string, value any) error {
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".job-*")
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
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}
