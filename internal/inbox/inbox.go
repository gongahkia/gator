// Package inbox stores durable local notifications for Work and scheduled jobs.
package inbox

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/workspace"
)

const Version = 1

var idPattern = regexp.MustCompile(`\A[a-zA-Z0-9][a-zA-Z0-9_-]{0,127}\z`)

type Entry struct {
	Version        int       `json:"version"`
	ID             string    `json:"id"`
	Kind           string    `json:"kind"`
	Title          string    `json:"title"`
	Summary        string    `json:"summary"`
	JobID          string    `json:"job_id,omitempty"`
	GoalID         string    `json:"goal_id,omitempty"`
	ConversationID string    `json:"conversation_id,omitempty"`
	RevisionID     string    `json:"revision_id,omitempty"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
	ReadAt         time.Time `json:"read_at,omitempty"`
	Rating         string    `json:"rating,omitempty"`
	RatedAt        time.Time `json:"rated_at,omitempty"`
}

type RatingSummary struct {
	Total   int `json:"total"`
	Useful  int `json:"useful"`
	Noise   int `json:"noise"`
	Wrong   int `json:"wrong"`
	Unrated int `json:"unrated"`
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
	if err := validateEntry(entry); err != nil {
		return Entry{}, err
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
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return Entry{}, err
	}
	if _, err := temporary.Write(append(payload, '\n')); err != nil {
		_ = temporary.Close()
		return Entry{}, err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return Entry{}, err
	}
	if err := temporary.Close(); err != nil {
		return Entry{}, err
	}
	if err := os.Rename(path, filepath.Join(s.root, entry.ID+".json")); err != nil {
		return Entry{}, err
	}
	directory, err := os.Open(s.root)
	if err != nil {
		return Entry{}, err
	}
	err = directory.Sync()
	_ = directory.Close()
	if err != nil {
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
		entry, err := s.Load(strings.TrimSuffix(file.Name(), ".json"))
		if err != nil {
			return nil, err
		}
		if !unreadOnly || entry.ReadAt.IsZero() {
			entries = append(entries, entry)
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].CreatedAt.After(entries[j].CreatedAt) })
	if limit > 0 && len(entries) > limit {
		entries = entries[:limit]
	}
	return entries, nil
}

func (s Store) Load(entryID string) (Entry, error) {
	if !idPattern.MatchString(entryID) {
		return Entry{}, errors.New("invalid inbox entry ID")
	}
	root, err := workspace.Open(s.root)
	if err != nil {
		return Entry{}, err
	}
	payload, err := root.ReadRegularFile(entryID+".json", 16*1024)
	if err != nil {
		return Entry{}, err
	}
	var entry Entry
	if err := json.Unmarshal(payload, &entry); err != nil {
		return Entry{}, err
	}
	if entry.ID != entryID {
		return Entry{}, errors.New("inbox entry identity does not match its file")
	}
	if err := validateEntry(entry); err != nil {
		return Entry{}, err
	}
	return entry, nil
}

func (s Store) MarkRead(entryID string, now time.Time) error {
	entry, err := s.Load(entryID)
	if err != nil {
		return err
	}
	entry.ReadAt = now.UTC()
	_, err = s.Add(entry)
	return err
}

// Rate records explicit feedback about a surfaced job alert. It does not infer
// that reading or dismissing an alert means the alert was noise.
func (s Store) Rate(entryID, rating string, now time.Time) (Entry, error) {
	if rating != "useful" && rating != "noise" && rating != "wrong" && rating != "clear" {
		return Entry{}, errors.New("rating must be useful, noise, wrong, or clear")
	}
	if now.IsZero() {
		return Entry{}, errors.New("rating time is required")
	}
	entry, err := s.Load(entryID)
	if err != nil {
		return Entry{}, err
	}
	if entry.Kind != "job" {
		return Entry{}, errors.New("only job alerts can be rated")
	}
	if rating == "clear" {
		entry.Rating = ""
		entry.RatedAt = time.Time{}
	} else {
		entry.Rating = rating
		entry.RatedAt = now.UTC()
	}
	return s.Add(entry)
}

// RatingsForGoal counts explicitly rated job alerts for one goal. Unrated
// alerts are not counted as noise, and quiet checks have no inbox entry.
func (s Store) RatingsForGoal(goalID string) (RatingSummary, error) {
	if !strings.HasPrefix(goalID, "goal-") || len(goalID) != 29 {
		return RatingSummary{}, errors.New("invalid goal ID")
	}
	entries, err := s.List(false, 0)
	if err != nil {
		return RatingSummary{}, err
	}
	var result RatingSummary
	for _, entry := range entries {
		if entry.GoalID != goalID || entry.Kind != "job" {
			continue
		}
		result.Total++
		switch entry.Rating {
		case "useful":
			result.Useful++
		case "noise":
			result.Noise++
		case "wrong":
			result.Wrong++
		default:
			result.Unrated++
		}
	}
	return result, nil
}

func validateEntry(entry Entry) error {
	if entry.Version != Version || !idPattern.MatchString(entry.ID) || (entry.GoalID != "" && (!strings.HasPrefix(entry.GoalID, "goal-") || len(entry.GoalID) != 29)) || strings.TrimSpace(entry.Title) == "" || len(entry.Title) > 256 || strings.TrimSpace(entry.Status) == "" || len(entry.Status) > 64 || len(entry.Summary) > 4096 || strings.ContainsRune(entry.Summary, 0) {
		return errors.New("inbox entry is invalid")
	}
	if (entry.Rating != "" && entry.Rating != "useful" && entry.Rating != "noise" && entry.Rating != "wrong") ||
		(entry.Rating == "" && !entry.RatedAt.IsZero()) ||
		(entry.Rating != "" && entry.RatedAt.IsZero()) {
		return errors.New("inbox rating is invalid")
	}
	return nil
}

// Has checks a durable entry by exact ID without scanning or truncating the
// inbox. The local supervisor uses this for one reminder per goal review time.
func (s Store) Has(entryID string) (bool, error) {
	if !idPattern.MatchString(entryID) {
		return false, errors.New("invalid inbox entry ID")
	}
	info, err := os.Lstat(filepath.Join(s.root, entryID+".json"))
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return false, errors.New("inbox entry is not a regular file")
	}
	return true, nil
}

func id() string {
	value := make([]byte, 8)
	_, _ = rand.Read(value)
	return "inbox-" + time.Now().UTC().Format("20060102T150405") + "-" + hex.EncodeToString(value)
}
