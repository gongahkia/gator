// Package inbox stores durable local notifications for Work and scheduled jobs.
package inbox

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const Version = 1

type Entry struct {
	Version        int       `json:"version"`
	ID             string    `json:"id"`
	Kind           string    `json:"kind"`
	Title          string    `json:"title"`
	Summary        string    `json:"summary"`
	JobID          string    `json:"job_id,omitempty"`
	ConversationID string    `json:"conversation_id,omitempty"`
	RevisionID     string    `json:"revision_id,omitempty"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
	ReadAt         time.Time `json:"read_at,omitempty"`
}

type Store struct{ root string }

func Open(stateDir string) (Store, error) {
	if strings.TrimSpace(stateDir) == "" {
		return Store{}, errors.New("inbox state directory is required")
	}
	root := filepath.Join(stateDir, "gator", "inbox")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return Store{}, err
	}
	return Store{root: root}, os.Chmod(root, 0o700)
}

func (s Store) Add(entry Entry) (Entry, error) {
	if entry.Version == 0 {
		entry.Version = Version
	}
	if entry.ID == "" {
		entry.ID = id()
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}
	if entry.Version != Version || strings.TrimSpace(entry.Title) == "" || strings.TrimSpace(entry.Status) == "" || len(entry.Summary) > 4096 {
		return Entry{}, errors.New("inbox entry is invalid")
	}
	payload, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return Entry{}, err
	}
	temporary, err := os.CreateTemp(s.root, ".inbox-*")
	if err != nil {
		return Entry{}, err
	}
	path := temporary.Name()
	defer os.Remove(path)
	_ = temporary.Chmod(0o600)
	if _, err := temporary.Write(append(payload, '\n')); err != nil {
		_ = temporary.Close()
		return Entry{}, err
	}
	if err := temporary.Close(); err != nil {
		return Entry{}, err
	}
	if err := os.Rename(path, filepath.Join(s.root, entry.ID+".json")); err != nil {
		return Entry{}, err
	}
	return entry, nil
}

func (s Store) List(unreadOnly bool, limit int) ([]Entry, error) {
	files, err := os.ReadDir(s.root)
	if err != nil {
		return nil, err
	}
	var entries []Entry
	for _, file := range files {
		if file.IsDir() || filepath.Ext(file.Name()) != ".json" {
			continue
		}
		payload, err := os.ReadFile(filepath.Join(s.root, file.Name()))
		if err != nil {
			return nil, err
		}
		var entry Entry
		if json.Unmarshal(payload, &entry) == nil && (!unreadOnly || entry.ReadAt.IsZero()) {
			entries = append(entries, entry)
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].CreatedAt.After(entries[j].CreatedAt) })
	if limit > 0 && len(entries) > limit {
		entries = entries[:limit]
	}
	return entries, nil
}

func (s Store) MarkRead(entryID string, now time.Time) error {
	path := filepath.Join(s.root, entryID+".json")
	payload, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var entry Entry
	if err := json.Unmarshal(payload, &entry); err != nil {
		return err
	}
	entry.ReadAt = now.UTC()
	_, err = s.Add(entry)
	return err
}

func id() string {
	value := make([]byte, 8)
	_, _ = rand.Read(value)
	return "inbox-" + time.Now().UTC().Format("20060102T150405") + "-" + hex.EncodeToString(value)
}
