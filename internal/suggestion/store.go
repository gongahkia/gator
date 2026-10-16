// Package suggestion retains model-proposed changes until a user reviews them.
package suggestion

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/gongahkia/gator/internal/goal"
)

const Version = 1

var skillIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)

type Record struct {
	Version     int             `json:"version"`
	ID          string          `json:"id"`
	Kind        string          `json:"kind"`
	Status      string          `json:"status"`
	Title       string          `json:"title,omitempty"`
	Body        string          `json:"body,omitempty"`
	SourceRefs  []string        `json:"source_refs,omitempty"`
	SkillID     string          `json:"skill_id,omitempty"`
	Description string          `json:"description,omitempty"`
	GoalID      string          `json:"goal_id,omitempty"`
	WorkID      string          `json:"work_id"`
	PlanChange  goal.PlanChange `json:"plan_change,omitempty"`
	ResultID    string          `json:"result_id,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	ResolvedAt  time.Time       `json:"resolved_at,omitempty"`
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
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return Record{}, err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
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
	if err := directory.Sync(); err != nil {
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
	switch record.Kind {
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
