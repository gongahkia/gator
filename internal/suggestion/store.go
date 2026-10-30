// Package suggestion retains model-proposed changes until a user reviews them.
package suggestion

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gongahkia/gator/internal/filelock"
	"github.com/gongahkia/gator/internal/privatefs"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gongahkia/gator/internal/goal"
	"github.com/robfig/cron/v3"
)

const Version = 1

var skillIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)

type Record struct {
	NoteID               string          `json:"note_id,omitempty"`
	ExpectedNoteRevision string          `json:"expected_note_revision,omitempty"`
	Version              int             `json:"version"`
	ID                   string          `json:"id"`
	Kind                 string          `json:"kind"`
	Status               string          `json:"status"`
	Title                string          `json:"title,omitempty"`
	Body                 string          `json:"body,omitempty"`
	SourceRefs           []string        `json:"source_refs,omitempty"`
	SkillID              string          `json:"skill_id,omitempty"`
	Description          string          `json:"description,omitempty"`
	GoalID               string          `json:"goal_id,omitempty"`
	GoalSourcePath       string          `json:"goal_source_path,omitempty"`
	GoalOutcome          string          `json:"goal_outcome,omitempty"`
	GoalSteps            []string        `json:"goal_steps,omitempty"`
	WorkID               string          `json:"work_id"`
	PlanChange           goal.PlanChange `json:"plan_change,omitempty"`
	Cron                 string          `json:"cron,omitempty"`
	Task                 string          `json:"task,omitempty"`
	Timezone             string          `json:"timezone,omitempty"`
	ChangeOnly           bool            `json:"change_only,omitempty"`
	ResultID             string          `json:"result_id,omitempty"`
	CreatedAt            time.Time       `json:"created_at"`
	ResolvedAt           time.Time       `json:"resolved_at,omitempty"`
}

type Store struct{ root string }

func Open(stateDir string) (Store, error) {
	if strings.TrimSpace(stateDir) == "" {
		return Store{}, errors.New("suggestion state directory is required")
	}
	root := filepath.Join(stateDir, "gator", "suggestions")
	if err := os.MkdirAll(root, 0700); err != nil {
		return Store{}, err
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return Store{}, errors.New("suggestion directory is not a regular directory")
	}
	if err := os.Chmod(root, 0700); err != nil {
		return Store{}, err
	}
	return Store{root: root}, nil
}

func (s Store) Create(record Record) (Record, error) {
	var random [12]byte
	if _, err := rand.Read(random[:]); err != nil {
		return Record{}, err
	}
	record.Version, record.ID, record.Status = Version, "suggestion-"+hex.EncodeToString(random[:]), "pending"
	record.CreatedAt = time.Now().UTC()
	if err := validate(record); err != nil {
		return Record{}, err
	}
	entries, err := s.List("pending")
	if err != nil {
		return Record{}, err
	}
	if len(entries) >= 256 {
		return Record{}, errors.New("too many pending suggestions; review or reject some first")
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return Record{}, err
	}
	file, err := os.OpenFile(s.path(record.ID), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return Record{}, err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return Record{}, err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return Record{}, err
	}
	return record, file.Close()
}

func (s Store) Load(id string) (Record, error) {
	if !validID(id) {
		return Record{}, errors.New("invalid suggestion ID")
	}
	path := s.path(id)
	info, err := os.Lstat(path)
	if err != nil {
		return Record{}, err
	}
	if !info.Mode().IsRegular() || info.Size() > 32*1024 {
		return Record{}, errors.New("suggestion is not a bounded regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return Record{}, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) || !opened.Mode().IsRegular() {
		return Record{}, errors.New("suggestion changed during read")
	}
	data, err := io.ReadAll(io.LimitReader(file, 32*1024+1))
	if err != nil || len(data) > 32*1024 {
		return Record{}, errors.New("suggestion read failed or exceeded size limit")
	}
	var record Record
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&record); err != nil {
		return Record{}, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return Record{}, errors.New("suggestion has trailing data")
	}
	if record.ID != id {
		return Record{}, errors.New("suggestion identity mismatch")
	}
	return record, validate(record)
}

func (s Store) List(status string) ([]Record, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, err
	}
	if len(entries) > 4096 {
		return nil, errors.New("suggestion store has too many entries")
	}
	var records []Record
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		record, err := s.Load(strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			return nil, err
		}
		if status == "" || record.Status == status {
			records = append(records, record)
		}
	}
	sort.Slice(records, func(i, j int) bool { return records[i].CreatedAt.After(records[j].CreatedAt) })
	return records, nil
}

// Resolve holds a process lock while applying and retaining a decision. A
// failed application leaves the suggestion pending, including a stale plan
// revision that needs user correction before acceptance.
func (s Store) Resolve(id, decision string, apply func(Record) (string, error)) (Record, error) {
	if !validID(id) || decision != "accept" && decision != "reject" {
		return Record{}, errors.New("invalid suggestion decision")
	}
	lock, err := os.OpenFile(filepath.Join(s.root, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return Record{}, err
	}
	defer lock.Close()
	info, statErr := os.Lstat(filepath.Join(s.root, ".lock"))
	opened, openErr := lock.Stat()
	if statErr != nil || openErr != nil || !info.Mode().IsRegular() || !os.SameFile(info, opened) {
		return Record{}, errors.New("suggestion lock is not a regular file")
	}
	if err := filelock.Lock(lock, filelock.Exclusive); err != nil {
		return Record{}, err
	}
	defer filelock.Unlock(lock)
	record, err := s.Load(id)
	if err != nil {
		return Record{}, err
	}
	if record.Status != "pending" {
		return Record{}, errors.New("suggestion is already resolved")
	}
	if decision == "accept" {
		if apply == nil {
			return Record{}, errors.New("suggestion accept requires an application callback")
		}
		record.ResultID, err = apply(record)
		if err != nil {
			return Record{}, err
		}
		record.Status = "accepted"
	} else {
		record.Status = "rejected"
	}
	record.ResolvedAt = time.Now().UTC()
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return Record{}, err
	}
	temp, err := os.CreateTemp(s.root, ".suggestion-")
	if err != nil {
		return Record{}, err
	}
	defer os.Remove(temp.Name())
	if err := temp.Chmod(0600); err != nil {
		_ = temp.Close()
		return Record{}, err
	}
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return Record{}, err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return Record{}, err
	}
	if err := temp.Close(); err != nil {
		return Record{}, err
	}
	if err := os.Rename(temp.Name(), s.path(id)); err != nil {
		return Record{}, err
	}
	directory, err := os.Open(s.root)
	if err != nil {
		return Record{}, err
	}
	defer directory.Close()
	if err := privatefs.SyncDir(directory); err != nil {
		return Record{}, err
	}
	return record, nil
}

func (s Store) path(id string) string { return filepath.Join(s.root, id+".json") }

func validID(id string) bool {
	if !strings.HasPrefix(id, "suggestion-") || len(id) != 35 {
		return false
	}
	_, err := hex.DecodeString(id[11:])
	return err == nil
}

func validate(record Record) error {
	if record.Version != Version || !validID(record.ID) || record.CreatedAt.IsZero() || record.WorkID == "" || len(record.WorkID) > 128 || strings.ContainsAny(record.WorkID, "\x00\r\n") {
		return errors.New("invalid suggestion identity")
	}
	if record.Status != "pending" && record.Status != "accepted" && record.Status != "rejected" {
		return errors.New("invalid suggestion status")
	}
	if record.Status == "pending" && !record.ResolvedAt.IsZero() || record.Status != "pending" && record.ResolvedAt.IsZero() {
		return errors.New("invalid suggestion resolution time")
	}
	if len(record.Title) > 256 || len(record.Body) > 24*1024 || len(record.Description) > 512 || !utf8.ValidString(record.Title+record.Body+record.Description) || strings.ContainsRune(record.Title+record.Body+record.Description, 0) || len(record.SourceRefs) > 15 {
		return errors.New("invalid suggestion content")
	}
	for _, ref := range record.SourceRefs {
		if strings.TrimSpace(ref) == "" || len(ref) > 512 || !utf8.ValidString(ref) || strings.ContainsAny(ref, "\x00\r\n") {
			return errors.New("invalid suggestion source reference")
		}
	}
	if record.Kind != "goal" && (record.GoalSourcePath != "" || record.GoalOutcome != "" || len(record.GoalSteps) != 0) {
		return errors.New("goal proposal fields require a goal suggestion")
	}
	if record.NoteID != "" || record.ExpectedNoteRevision != "" {
		if record.Kind != "note" || !strings.HasPrefix(record.NoteID, "note-") || len(record.NoteID) != 29 || len(record.ExpectedNoteRevision) != 64 {
			return errors.New("note updates require note_id and an inspected expected_note_revision")
		}
		if _, err := hex.DecodeString(record.NoteID[5:]); err != nil {
			return errors.New("invalid note identity")
		}
		if _, err := hex.DecodeString(record.ExpectedNoteRevision); err != nil {
			return errors.New("invalid note revision")
		}
	}
	switch record.Kind {
	case "goal":
		if strings.TrimSpace(record.Title) == "" || len(record.Body) > 8192 || record.GoalID != "" || len(record.SourceRefs) != 0 || strings.TrimSpace(record.GoalOutcome) == "" || len(record.GoalOutcome) > 2048 || !utf8.ValidString(record.GoalOutcome) || strings.ContainsRune(record.GoalOutcome, 0) || len(record.GoalSteps) == 0 || len(record.GoalSteps) > 8 {
			return errors.New("goal suggestion needs a title, outcome, and 1-8 plan steps")
		}
		if record.GoalSourcePath != "" && (!filepath.IsAbs(record.GoalSourcePath) || filepath.Clean(record.GoalSourcePath) != record.GoalSourcePath || len(record.GoalSourcePath) > 4096) {
			return errors.New("goal suggestion source must be a bounded absolute path")
		}
		for _, step := range record.GoalSteps {
			if strings.TrimSpace(step) == "" || len(step) > 512 || !utf8.ValidString(step) || strings.ContainsRune(step, 0) {
				return errors.New("invalid suggested goal step")
			}
		}
		if record.Cron == "" {
			if record.Task != "" || record.Timezone != "" || record.ChangeOnly {
				return errors.New("goal follow-up task requires a schedule")
			}
		} else {
			if strings.TrimSpace(record.Task) == "" || len(record.Task) > 8192 || !utf8.ValidString(record.Task) || strings.ContainsRune(record.Task, 0) || len(record.Cron) > 256 || len(record.Timezone) > 128 {
				return errors.New("invalid suggested goal follow-up")
			}
			parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
			if _, err := parser.Parse(record.Cron); err != nil {
				return fmt.Errorf("invalid suggested goal cron: %w", err)
			}
			if _, err := time.LoadLocation(record.Timezone); err != nil {
				return fmt.Errorf("invalid suggested goal timezone: %w", err)
			}
		}
	case "note":
		if strings.TrimSpace(record.Title) == "" || strings.TrimSpace(record.Body) == "" || record.GoalID != "" && !validGoalID(record.GoalID) {
			return errors.New("note suggestion needs title and body")
		}
	case "skill":
		if !skillIDPattern.MatchString(record.SkillID) || strings.TrimSpace(record.Description) == "" || strings.TrimSpace(record.Body) == "" || record.GoalID != "" || len(record.SourceRefs) != 0 {
			return errors.New("skill suggestion needs a global name, description, and procedure")
		}
	case "plan":
		if !validGoalID(record.GoalID) || goal.ValidatePlanChange(record.PlanChange) != nil || len(record.SourceRefs) != 0 {
			return errors.New("plan suggestion needs a goal and typed change")
		}
	case "schedule":
		if !validGoalID(record.GoalID) || strings.TrimSpace(record.Task) == "" || len(record.Task) > 8192 || !utf8.ValidString(record.Task) || strings.ContainsRune(record.Task, 0) || len(record.Cron) > 256 || len(record.Timezone) > 128 || len(record.SourceRefs) != 0 {
			return errors.New("schedule suggestion needs a goal, bounded task, and cron")
		}
		parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
		if _, err := parser.Parse(record.Cron); err != nil {
			return fmt.Errorf("invalid suggested cron: %w", err)
		}
		if _, err := time.LoadLocation(record.Timezone); err != nil {
			return fmt.Errorf("invalid suggested timezone: %w", err)
		}
	default:
		return fmt.Errorf("invalid suggestion kind %q", record.Kind)
	}
	return nil
}

func validGoalID(id string) bool {
	if !strings.HasPrefix(id, "goal-") || len(id) != 29 {
		return false
	}
	_, err := hex.DecodeString(id[5:])
	return err == nil
}
