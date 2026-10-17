// Package memory stores explicit, inspectable personal knowledge. Memories are
// reference data, never instructions or tool permissions.
package memory

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
)

const Version = 1
const maxSourceFileBytes = 4 << 20

type Record struct {
	Version      int       `json:"version"`
	ID           string    `json:"id"`
	Kind         string    `json:"kind"`
	Text         string    `json:"text"`
	GoalID       string    `json:"goal_id,omitempty"`
	Source       string    `json:"source,omitempty"`
	SourceSHA256 string    `json:"source_sha256,omitempty"`
	SourceStatus string    `json:"-"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Store struct {
	root string
	now  func() time.Time
}

func Open(stateDir string) (Store, error) {
	if strings.TrimSpace(stateDir) == "" {
		return Store{}, errors.New("memory state directory is required")
	}
	abs, err := filepath.Abs(stateDir)
	if err != nil {
		return Store{}, err
	}
	root := filepath.Join(abs, "gator", "memory")
	if err := os.MkdirAll(root, 0700); err != nil {
		return Store{}, err
	}
	if err := os.Chmod(root, 0700); err != nil {
		return Store{}, err
	}
	return Store{root: root, now: time.Now}, nil
}

func (s Store) clock() time.Time {
	if s.now != nil {
		return s.now().UTC()
	}
	return time.Now().UTC()
}

func (s Store) Add(kind, content, goalID, source string, expires time.Time) (Record, error) {
	return s.add(kind, content, goalID, source, "", expires)
}

// AddFromFile links a memory to a bounded local file. Its current fingerprint
// is checked before the memory is recalled in later Work runs.
func (s Store) AddFromFile(kind, content, goalID, path string, expires time.Time) (Record, error) {
	abs, digest, err := sourceFileFingerprint(path)
	if err != nil {
		return Record{}, err
	}
	return s.add(kind, content, goalID, abs, digest, expires)
}

func (s Store) add(kind, content, goalID, source, sourceSHA256 string, expires time.Time) (Record, error) {
	var random [12]byte
	if _, err := rand.Read(random[:]); err != nil {
		return Record{}, err
	}
	now := s.clock()
	record := Record{Version: Version, ID: "mem-" + hex.EncodeToString(random[:]), Kind: strings.TrimSpace(kind), Text: strings.TrimSpace(content), GoalID: strings.TrimSpace(goalID), Source: strings.TrimSpace(source), SourceSHA256: sourceSHA256, ExpiresAt: expires.UTC(), CreatedAt: now, UpdatedAt: now}
	if expires.IsZero() {
		record.ExpiresAt = time.Time{}
	}
	if err := validate(record); err != nil {
		return Record{}, err
	}
	if err := s.write(record, true); err != nil {
		return Record{}, err
	}
	return record, nil
}

func (s Store) Load(id string) (Record, error) {
	if !validID(id) {
		return Record{}, errors.New("invalid memory ID")
	}
	info, err := os.Lstat(s.path(id))
	if err != nil {
		return Record{}, err
	}
	if !info.Mode().IsRegular() || info.Size() > 16*1024 {
		return Record{}, errors.New("memory must be a bounded regular file")
	}
	data, err := os.ReadFile(s.path(id))
	if err != nil {
		return Record{}, err
	}
	var record Record
	if err := json.Unmarshal(data, &record); err != nil {
		return Record{}, err
	}
	if record.ID != id {
		return Record{}, errors.New("memory ID mismatch")
	}
	if err := validate(record); err != nil {
		return Record{}, err
	}
	record.SourceStatus = sourceFileStatus(record)
	return record, nil
}

func (s Store) List(goalID string, includeExpired bool) ([]Record, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, err
	}
	var result []Record
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		record, err := s.Load(strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			return nil, fmt.Errorf("load %s: %w", entry.Name(), err)
		}
		if goalID != "" && record.GoalID != "" && record.GoalID != goalID {
			continue
		}
		if goalID == "" && record.GoalID != "" {
			continue
		}
		if !includeExpired && !record.ExpiresAt.IsZero() && !record.ExpiresAt.After(s.clock()) {
			continue
		}
		result = append(result, record)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].UpdatedAt.Equal(result[j].UpdatedAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].UpdatedAt.After(result[j].UpdatedAt)
	})
	return result, nil
}

// Search uses exact lexical terms and scope. It intentionally does not invent
// semantic matches; no match is safer than an unrelated remembered fact.
func (s Store) Search(query, goalID string, limit int) ([]Record, error) {
	records, err := s.List(goalID, false)
	if err != nil {
		return nil, err
	}
	terms := tokenize(query)
	if len(terms) == 0 {
		return nil, nil
	}
	type scored struct {
		record Record
		score  int
	}
	var matches []scored
	for _, record := range records {
		if record.SourceStatus == "changed" || record.SourceStatus == "unavailable" {
			continue
		}
		words := tokenize(record.Text + " " + record.Kind + " " + record.Source)
		seen := words
		score := 0
		for term := range terms {
			if seen[term] {
				score++
			}
		}
		if score >= min(2, len(terms)) {
			if goalID != "" && record.GoalID == goalID {
				score += 2
			}
			matches = append(matches, scored{record, score})
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		return matches[i].record.UpdatedAt.After(matches[j].record.UpdatedAt)
	})
	if limit < 0 {
		limit = 0
	}
	if limit > len(matches) {
		limit = len(matches)
	}
	result := make([]Record, 0, limit)
	for _, match := range matches[:limit] {
		result = append(result, match.record)
	}
	return result, nil
}

func (s Store) Correct(id, content, source string) (Record, error) {
	record, err := s.Load(id)
	if err != nil {
		return Record{}, err
	}
	source = strings.TrimSpace(source)
	if source != record.Source {
		record.SourceSHA256, record.SourceStatus = "", ""
	}
	record.Text, record.Source, record.UpdatedAt = strings.TrimSpace(content), source, s.clock()
	if err := validate(record); err != nil {
		return Record{}, err
	}
	return record, s.write(record, false)
}

// CorrectFromFile records a user-reviewed correction against the current file.
func (s Store) CorrectFromFile(id, content, path string) (Record, error) {
	abs, digest, err := sourceFileFingerprint(path)
	if err != nil {
		return Record{}, err
	}
	record, err := s.Load(id)
	if err != nil {
		return Record{}, err
	}
	record.Text, record.Source, record.SourceSHA256, record.SourceStatus, record.UpdatedAt = strings.TrimSpace(content), abs, digest, "current", s.clock()
	if err := validate(record); err != nil {
		return Record{}, err
	}
	return record, s.write(record, false)
}

func (s Store) Forget(id string) error {
	if _, err := s.Load(id); err != nil {
		return err
	}
	return os.Remove(s.path(id))
}

func Render(records []Record) string {
	var lines []string
	remaining := 4000
	for _, record := range records {
		line := fmt.Sprintf("- [%s] %s (%s, updated: %s", record.ID, record.Text, record.Kind, record.UpdatedAt.Format("2006-01-02"))
		if record.Source != "" {
			line += ", source: " + record.Source
		}
		if record.SourceStatus != "" {
			line += ", source status: " + record.SourceStatus
		}
		if !record.ExpiresAt.IsZero() {
			line += ", expires: " + record.ExpiresAt.Format("2006-01-02")
		}
		line += ")"
		if len(line) > remaining {
			break
		}
		lines = append(lines, line)
		remaining -= len(line)
	}
	return strings.Join(lines, "\n")
}

func tokenize(value string) map[string]bool {
	terms := map[string]bool{}
	stop := map[string]bool{"the": true, "and": true, "for": true, "with": true, "from": true, "this": true, "that": true, "what": true, "when": true, "where": true, "which": true, "about": true, "your": true, "user": true, "goal": true, "current": true, "please": true, "should": true, "would": true, "could": true, "into": true, "have": true, "will": true, "need": true, "next": true}
	for _, term := range strings.FieldsFunc(strings.ToLower(value), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) }) {
		if len(term) >= 3 && !stop[term] {
			terms[term] = true
		}
	}
	return terms
}

func (s Store) path(id string) string { return filepath.Join(s.root, id+".json") }
func validID(id string) bool {
	if !strings.HasPrefix(id, "mem-") || len(id) != 28 {
		return false
	}
	_, err := hex.DecodeString(id[4:])
	return err == nil
}
func validate(r Record) error {
	if r.Version != Version || !validID(r.ID) || r.CreatedAt.IsZero() || r.UpdatedAt.Before(r.CreatedAt) {
		return errors.New("invalid memory identity")
	}
	if r.Kind != "fact" && r.Kind != "preference" && r.Kind != "decision" && r.Kind != "note" {
		return errors.New("memory kind must be fact, preference, decision, or note")
	}
	if r.Text == "" || len(r.Text) > 4096 || len(r.Source) > 1024 || strings.ContainsRune(r.Text+r.Source, 0) {
		return errors.New("invalid memory content or source")
	}
	if r.SourceSHA256 != "" {
		if !filepath.IsAbs(r.Source) || len(r.SourceSHA256) != sha256.Size*2 {
			return errors.New("invalid memory source fingerprint")
		}
		if _, err := hex.DecodeString(r.SourceSHA256); err != nil {
			return errors.New("invalid memory source fingerprint")
		}
	}
	if r.GoalID != "" {
		if !strings.HasPrefix(r.GoalID, "goal-") || len(r.GoalID) != 29 {
			return errors.New("invalid memory goal")
		}
		if _, err := hex.DecodeString(r.GoalID[5:]); err != nil {
			return errors.New("invalid memory goal")
		}
	}
	return nil
}

func sourceFileFingerprint(path string) (string, string, error) {
	abs, err := filepath.Abs(strings.TrimSpace(path))
	if err != nil || strings.TrimSpace(path) == "" {
		return "", "", errors.New("source file path is invalid")
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return "", "", fmt.Errorf("inspect source file: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() > maxSourceFileBytes {
		return "", "", errors.New("source file must be a regular file of at most 4 MiB")
	}
	file, err := os.Open(abs)
	if err != nil {
		return "", "", fmt.Errorf("open source file: %w", err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxSourceFileBytes+1))
	if err != nil || len(data) > maxSourceFileBytes {
		return "", "", errors.New("source file changed or exceeded 4 MiB while reading")
	}
	digest := sha256.Sum256(data)
	return abs, hex.EncodeToString(digest[:]), nil
}

func sourceFileStatus(record Record) string {
	if record.SourceSHA256 == "" {
		return ""
	}
	_, digest, err := sourceFileFingerprint(record.Source)
	if err != nil {
		return "unavailable"
	}
	if digest != record.SourceSHA256 {
		return "changed"
	}
	return "current"
}
func (s Store) write(r Record, create bool) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if create {
		file, err := os.OpenFile(s.path(r.ID), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		_, writeErr := file.Write(data)
		return errors.Join(writeErr, file.Close())
	}
	temp, err := os.CreateTemp(s.root, ".memory-*")
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
	return os.Rename(temp.Name(), s.path(r.ID))
}
