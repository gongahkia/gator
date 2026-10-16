// Package goal stores the user's ongoing responsibilities separately from
// individual Work runs. A goal is explicit user-owned context, not a model
// instruction or an authority grant.
package goal

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
)

const Version = 1

type Status string

const (
	Active    Status = "active"
	Paused    Status = "paused"
	Completed Status = "completed"
)

type Record struct {
	Version        int       `json:"version"`
	ID             string    `json:"id"`
	Title          string    `json:"title"`
	Context        string    `json:"context,omitempty"`
	SourcePath     string    `json:"source_path"`
	ConversationID string    `json:"conversation_id,omitempty"`
	Status         Status    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type Store struct {
	root string
	now  func() time.Time
}

func Open(stateDir string) (Store, error) {
	if strings.TrimSpace(stateDir) == "" {
		return Store{}, errors.New("goal state directory is required")
	}
	abs, err := filepath.Abs(stateDir)
	if err != nil {
		return Store{}, err
	}
	root := filepath.Join(abs, "gator", "goals")
	if err := os.MkdirAll(root, 0700); err != nil {
		return Store{}, err
	}
	if err := os.Chmod(root, 0700); err != nil {
		return Store{}, err
	}
	return Store{root: root, now: time.Now}, nil
}

func (s Store) Add(title, context, sourcePath string) (Record, error) {
	title, context = strings.TrimSpace(title), strings.TrimSpace(context)
	if title == "" || len(title) > 256 || len(context) > 8192 || strings.ContainsRune(title+context, 0) {
		return Record{}, errors.New("invalid goal title or context")
	}
	var bytes [12]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return Record{}, err
	}
	id := "goal-" + hex.EncodeToString(bytes[:])
	if strings.TrimSpace(sourcePath) == "" {
		sourcePath = filepath.Join(s.root, "sources", id)
		if err := os.MkdirAll(sourcePath, 0700); err != nil {
			return Record{}, err
		}
	}
	abs, err := filepath.Abs(sourcePath)
	if err != nil {
		return Record{}, err
	}
	abs, err = filepath.EvalSymlinks(abs)
	if err != nil {
		return Record{}, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return Record{}, err
	}
	if !info.IsDir() {
		return Record{}, errors.New("goal source must be a directory")
	}
	now := s.now().UTC()
	record := Record{Version: Version, ID: id, Title: title, Context: context, SourcePath: filepath.Clean(abs), Status: Active, CreatedAt: now, UpdatedAt: now}
	if err := validate(record); err != nil {
		return Record{}, err
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return Record{}, err
	}
	file, err := os.OpenFile(s.path(record.ID), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return Record{}, err
	}
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(s.path(record.ID))
		return Record{}, errors.Join(writeErr, closeErr)
	}
	return record, nil
}

func (s Store) Load(id string) (Record, error) {
	if !validID(id) {
		return Record{}, errors.New("invalid goal ID")
	}
	path := s.path(id)
	info, err := os.Lstat(path)
	if err != nil {
		return Record{}, err
	}
	if !info.Mode().IsRegular() || info.Size() > 16*1024 {
		return Record{}, errors.New("goal record is not a bounded regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Record{}, err
	}
	var record Record
	if err := json.Unmarshal(data, &record); err != nil {
		return Record{}, err
	}
	if record.ID != id {
		return Record{}, errors.New("goal record ID mismatch")
	}
	if err := validate(record); err != nil {
		return Record{}, err
	}
	return record, nil
}

func (s Store) List() ([]Record, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, err
	}
	var records []Record
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		record, err := s.Load(strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			return nil, fmt.Errorf("load goal %s: %w", entry.Name(), err)
		}
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].UpdatedAt.Equal(records[j].UpdatedAt) {
			return records[i].ID < records[j].ID
		}
		return records[i].UpdatedAt.After(records[j].UpdatedAt)
	})
	return records, nil
}

func (s Store) SetContext(id, context string) (Record, error) {
	record, err := s.Load(id)
	if err != nil {
		return Record{}, err
	}
	record.Context = strings.TrimSpace(context)
	return s.save(record)
}

func (s Store) SetStatus(id string, status Status) (Record, error) {
	record, err := s.Load(id)
	if err != nil {
		return Record{}, err
	}
	if status != Active && status != Paused && status != Completed {
		return Record{}, errors.New("invalid goal status")
	}
	record.Status = status
	return s.save(record)
}

func (s Store) AttachConversation(id, conversationID string) (Record, error) {
	record, err := s.Load(id)
	if err != nil {
		return Record{}, err
	}
	if record.ConversationID != "" && record.ConversationID != conversationID {
		return Record{}, errors.New("goal already has a different conversation")
	}
	record.ConversationID = conversationID
	return s.save(record)
}

func (s Store) save(record Record) (Record, error) {
	record.UpdatedAt = s.now().UTC()
	if err := validate(record); err != nil {
		return Record{}, err
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return Record{}, err
	}
	temp, err := os.CreateTemp(s.root, ".goal-*")
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
	if err := os.Rename(temp.Name(), s.path(record.ID)); err != nil {
		return Record{}, err
	}
	return record, nil
}

func (s Store) path(id string) string { return filepath.Join(s.root, id+".json") }

func validID(id string) bool {
	if !strings.HasPrefix(id, "goal-") || len(id) != 29 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(id, "goal-"))
	return err == nil
}

func validate(record Record) error {
	if record.Version != Version || !validID(record.ID) || record.CreatedAt.IsZero() || record.UpdatedAt.IsZero() {
		return errors.New("invalid goal record identity")
	}
	if record.Title == "" || len(record.Title) > 256 || len(record.Context) > 8192 || strings.ContainsRune(record.Title+record.Context, 0) {
		return errors.New("invalid goal title or context")
	}
	if !filepath.IsAbs(record.SourcePath) || record.SourcePath != filepath.Clean(record.SourcePath) {
		return errors.New("invalid goal source")
	}
	if record.Status != Active && record.Status != Paused && record.Status != Completed {
		return errors.New("invalid goal status")
	}
	if record.ConversationID != "" && !strings.HasPrefix(record.ConversationID, "work-") {
		return errors.New("invalid goal conversation")
	}
	return nil
}
