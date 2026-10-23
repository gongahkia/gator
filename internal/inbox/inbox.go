// Package inbox stores durable local notifications for Work and scheduled jobs.
package inbox

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gongahkia/gator/internal/filelock"
	"github.com/gongahkia/gator/internal/privatefs"
	"github.com/gongahkia/gator/internal/workspace"
)

const Version = 1

var idPattern = regexp.MustCompile(`\A[a-zA-Z0-9][a-zA-Z0-9_-]{0,127}\z`)

type Entry struct {
	NextStep             string    `json:"next_step,omitempty"`
	Evidence             []string  `json:"evidence,omitempty"`
	OriginConversationID string    `json:"origin_conversation_id,omitempty"`
	AttemptID            string    `json:"attempt_id,omitempty"`
	DeliveredAt          time.Time `json:"delivered_at,omitempty"`
	ResolvedAt           time.Time `json:"resolved_at,omitempty"`
	ResolutionVersion    uint64    `json:"resolution_version,omitempty"`
	Version              int       `json:"version"`
	ID                   string    `json:"id"`
	Kind                 string    `json:"kind"`
	Title                string    `json:"title"`
	Summary              string    `json:"summary"`
	JobID                string    `json:"job_id,omitempty"`
	GoalID               string    `json:"goal_id,omitempty"`
	ConversationID       string    `json:"conversation_id,omitempty"`
	RevisionID           string    `json:"revision_id,omitempty"`
	Status               string    `json:"status"`
	Urgent               bool      `json:"urgent,omitempty"`
	CreatedAt            time.Time `json:"created_at"`
	ReadAt               time.Time `json:"read_at,omitempty"`
	Rating               string    `json:"rating,omitempty"`
	RatedAt              time.Time `json:"rated_at,omitempty"`
	RatingVersion        uint64    `json:"rating_version,omitempty"`
}

type RatingSummary struct {
	HealthNotices int `json:"health_notices"`
	Total         int `json:"total"`
	Useful        int `json:"useful"`
	Noise         int `json:"noise"`
	Wrong         int `json:"wrong"`
	Unrated       int `json:"unrated"`
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
	unlock, err := s.lock()
	if err != nil {
		return Entry{}, err
	}
	defer unlock()
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
	prior, err := s.Load(entry.ID)
	if err == nil {
		if !sameEvent(prior, entry) {
			return Entry{}, errors.New("this update identity already belongs to a different result")
		}
		return prior, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return Entry{}, err
	}
	return s.write(entry)
}

func (s Store) write(entry Entry) (Entry, error) {
	if err := validateEntry(entry); err != nil {
		return Entry{}, err
	}
	payload, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return Entry{}, err
	}
	if len(payload) >= 16*1024 {
		return Entry{}, errors.New("inbox entry is too large")
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
	err = privatefs.SyncDir(directory)
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
	unlock, err := s.lock()
	if err != nil {
		return err
	}
	defer unlock()
	entry, err := s.Load(entryID)
	if err != nil {
		return err
	}
	if now.IsZero() {
		return errors.New("read time is required")
	}
	if entry.ReadAt.IsZero() {
		entry.ReadAt = now.UTC()
	}
	_, err = s.write(entry)
	return err
}

// Rate records explicit feedback about a surfaced job alert. It does not infer
// that reading or dismissing an alert means the alert was noise.
func (s Store) Rate(entryID, rating string, now time.Time) (Entry, error) {
	return s.rate(entryID, rating, "", nil, now)
}

// RateReviewed protects an assessment shown in the inbox from later edits.
func (s Store) RateReviewed(entryID, rating string, version uint64, now time.Time) (Entry, error) {
	return s.rate(entryID, rating, "", &version, now)
}

// RateForConversation checks chat ownership and the displayed feedback version
// under the same lock as the write. Feedback never changes attention receipts.
func (s Store) RateForConversation(conversationID, entryID, rating string, version uint64, now time.Time) (Entry, error) {
	if !idPattern.MatchString(conversationID) {
		return Entry{}, errors.New("a valid conversation is required")
	}
	return s.rate(entryID, rating, conversationID, &version, now)
}

func (e Entry) CanRate() bool { return e.Kind == "job" && e.Status == "completed" }

func (s Store) rate(entryID, rating, conversationID string, version *uint64, now time.Time) (Entry, error) {
	if rating != "useful" && rating != "noise" && rating != "wrong" && rating != "clear" {
		return Entry{}, errors.New("rating must be useful, noise, wrong, or clear")
	}
	if now.IsZero() {
		return Entry{}, errors.New("rating time is required")
	}
	unlock, err := s.lock()
	if err != nil {
		return Entry{}, err
	}
	defer unlock()
	entry, err := s.Load(entryID)
	if err != nil {
		return Entry{}, err
	}
	if conversationID != "" && entry.OriginConversationID != conversationID {
		return Entry{}, errors.New("this update belongs to another conversation")
	}
	if !entry.CanRate() {
		return Entry{}, errors.New("only completed job alerts can be rated; failed checks are health notices")
	}
	if version != nil && entry.RatingVersion != *version {
		return Entry{}, errors.New("feedback changed; review the latest assessment before changing it")
	}
	if entry.RatingVersion == ^uint64(0) {
		return Entry{}, errors.New("feedback version exhausted")
	}
	entry.RatingVersion++
	if rating == "clear" {
		entry.Rating = ""
		entry.RatedAt = time.Time{}
	} else {
		entry.Rating = rating
		entry.RatedAt = now.UTC()
	}
	return s.write(entry)
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
		if !entry.CanRate() {
			result.HealthNotices++
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
	if len(entry.NextStep) > 1024 || len(entry.Evidence) > 8 {
		return errors.New("update evidence is too large")
	}
	for _, reference := range entry.Evidence {
		if len(reference) > 512 || strings.ContainsRune(reference, 0) {
			return errors.New("invalid update evidence reference")
		}
	}
	for _, id := range []string{entry.OriginConversationID, entry.AttemptID, entry.ConversationID, entry.RevisionID, entry.JobID} {
		if id != "" && !idPattern.MatchString(id) {
			return errors.New("invalid update reference")
		}
	}
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

// JobUpdateID makes retrying notification delivery idempotent without merging
// different attempts or erasing the user's read/resolution receipts.
func JobUpdateID(jobID, attemptID string) string {
	return "inbox-job-" + eventDigest(jobID+"\x00"+attemptID)
}

func eventDigest(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func sameEvent(a, b Entry) bool {
	return a.Kind == b.Kind && a.Title == b.Title && a.Summary == b.Summary && a.NextStep == b.NextStep && slices.Equal(a.Evidence, b.Evidence) && a.JobID == b.JobID && a.GoalID == b.GoalID && a.OriginConversationID == b.OriginConversationID && a.AttemptID == b.AttemptID && a.ConversationID == b.ConversationID && a.RevisionID == b.RevisionID && a.Status == b.Status && a.Urgent == b.Urgent
}

var inboxMu sync.Mutex

func (s Store) lock() (func(), error) {
	inboxMu.Lock()
	root, err := os.OpenRoot(s.root)
	if err != nil {
		inboxMu.Unlock()
		return nil, err
	}
	defer root.Close()
	file, err := filelock.OpenRootFile(root, ".inbox.lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		inboxMu.Unlock()
		return nil, err
	}
	if err := filelock.Lock(file, filelock.Exclusive); err != nil {
		file.Close()
		inboxMu.Unlock()
		return nil, err
	}
	return func() { _ = file.Close(); inboxMu.Unlock() }, nil
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
