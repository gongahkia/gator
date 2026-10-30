// Package knowledge stores user-owned long notes and reusable text skills.
// It never executes skill content or treats note text as tool authority.
package knowledge

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gongahkia/gator/internal/privatefs"
	"github.com/gongahkia/gator/internal/retrieval"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const Version = 1
const maxNoteBytes = 64 * 1024
const maxSkillBytes = 32 * 1024
const maxEntries = 2048

var skillIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)
var noteIDPattern = regexp.MustCompile(`^note-[0-9a-f]{24}$`)
var goalIDPattern = regexp.MustCompile(`^goal-[0-9a-f]{24}$`)

type NoteUpdateReceipt struct {
	ID          string `json:"id"`
	InputHash   string `json:"input_hash"`
	ContentHash string `json:"content_hash"`
}

type Document struct {
	LastUpdate   *NoteUpdateReceipt `json:"last_update,omitempty"`
	Version      int                `json:"version"`
	ID           string             `json:"id"`
	Kind         string             `json:"kind"`
	Title        string             `json:"title"`
	Description  string             `json:"description,omitempty"`
	SourceRefs   []string           `json:"source_refs,omitempty"`
	GoalID       string             `json:"goal_id,omitempty"`
	Enabled      bool               `json:"enabled,omitempty"`
	CreatedAt    time.Time          `json:"created_at"`
	UpdatedAt    time.Time          `json:"updated_at"`
	Body         string             `json:"-"`
	MatchedTerms []string           `json:"-"`
	MatchScope   string             `json:"-"`
	MatchMethod  string             `json:"-"`
	Excerpt      string             `json:"-"`
}

type Store struct {
	root string
	now  func() time.Time
}

func Open(stateDir string) (Store, error) {
	if strings.TrimSpace(stateDir) == "" {
		return Store{}, errors.New("knowledge state directory is required")
	}
	abs, err := filepath.Abs(stateDir)
	if err != nil {
		return Store{}, err
	}
	if err := os.MkdirAll(abs, 0700); err != nil {
		return Store{}, err
	}
	gatorDir := filepath.Join(abs, "gator")
	root := filepath.Join(gatorDir, "knowledge")
	for _, dir := range []string{gatorDir, root, filepath.Join(root, "notes"), filepath.Join(root, "skills")} {
		if err := privateDirectory(dir); err != nil {
			return Store{}, err
		}
	}
	return Store{root: root, now: time.Now}, nil
}

func privateDirectory(path string) error {
	if err := os.Mkdir(path, 0700); err != nil && !os.IsExist(err) {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("knowledge path %s is not a directory", path)
	}
	return privatefs.Protect(path)
}

func (s Store) Root() string { return s.root }
func (s Store) clock() time.Time {
	if s.now != nil {
		return s.now().UTC()
	}
	return time.Now().UTC()
}

func (s Store) AddNote(title, body, goalID string) (Document, error) {
	return s.AddNoteWithRefs(title, body, goalID, nil)
}

func (s Store) AddNoteWithRefs(title, body, goalID string, refs []string) (Document, error) {
	var random [12]byte
	if _, err := rand.Read(random[:]); err != nil {
		return Document{}, err
	}
	return s.AddNoteWithIDAndRefs("note-"+hex.EncodeToString(random[:]), title, body, goalID, refs)
}

// AddNoteWithID supports idempotent, user-approved suggestion application.
// The caller chooses a valid random-looking ID bound to the proposal identity.
func (s Store) AddNoteWithID(id, title, body, goalID string) (Document, error) {
	return s.AddNoteWithIDAndRefs(id, title, body, goalID, nil)
}

func (s Store) AddNoteWithIDAndRefs(id, title, body, goalID string, refs []string) (Document, error) {
	release, err := s.lock(false)
	if err != nil {
		return Document{}, err
	}
	defer release()
	now := s.clock()
	d := Document{Version: Version, ID: id, Kind: "note", Title: strings.TrimSpace(title), GoalID: strings.TrimSpace(goalID), SourceRefs: append([]string(nil), refs...), CreatedAt: now, UpdatedAt: now, Body: strings.TrimSpace(body)}
	if err := validate(d); err != nil {
		return Document{}, err
	}
	return d, s.create(d)
}

func (s Store) AddSkill(id, description, body string) (Document, error) {
	now := s.clock()
	d := Document{Version: Version, ID: strings.TrimSpace(id), Kind: "skill", Title: strings.TrimSpace(id), Description: strings.TrimSpace(description), Enabled: true, CreatedAt: now, UpdatedAt: now, Body: strings.TrimSpace(body)}
	if err := validate(d); err != nil {
		return Document{}, err
	}
	return d, s.create(d)
}

func (s Store) LoadNote(id string) (Document, error) {
	release, err := s.lock(true)
	if err != nil {
		return Document{}, err
	}
	defer release()
	return s.load("note", id)
}
func (s Store) LoadSkill(id string) (Document, error) { return s.load("skill", id) }

func (s Store) ListNotes(goalID string) ([]Document, error) {
	release, err := s.lock(true)
	if err != nil {
		return nil, err
	}
	defer release()
	if goalID != "" && !goalIDPattern.MatchString(goalID) {
		return nil, errors.New("invalid goal ID")
	}
	documents, err := s.list("note")
	if err != nil {
		return nil, err
	}
	result := documents[:0]
	for _, d := range documents {
		if d.GoalID == "" || d.GoalID == goalID {
			result = append(result, d)
		}
	}
	return result, nil
}

func (s Store) ListSkills(enabledOnly bool) ([]Document, error) {
	documents, err := s.list("skill")
	if err != nil {
		return nil, err
	}
	if !enabledOnly {
		return documents, nil
	}
	result := documents[:0]
	for _, d := range documents {
		if d.Enabled {
			result = append(result, d)
		}
	}
	return result, nil
}

func (s Store) SearchNotes(query, goalID string, limit int) ([]Document, error) {
	documents, err := s.ListNotes(goalID)
	if err != nil {
		return nil, err
	}
	terms := tokenize(query)
	if len(terms) == 0 {
		return nil, nil
	}
	type scored struct {
		d     Document
		score int
	}
	var matches []scored
	for _, d := range documents {
		d, err = s.LoadNote(d.ID)
		if err != nil {
			return nil, err
		}
		words := tokenize(d.Title + " " + d.Body + " " + strings.Join(d.SourceRefs, " "))
		score := 0
		var matched []string
		for term := range terms {
			if words[term] {
				score++
				matched = append(matched, term)
			}
		}
		if score >= min(2, len(terms)) {
			sort.Strings(matched)
			d.MatchedTerms = matched
			d.MatchScope = "global"
			if goalID != "" && d.GoalID == goalID {
				score += 2
				d.MatchScope = "goal"
			}
			d.Excerpt = noteMatchExcerpt(d.Body, matched, 240)
			d.Body = ""
			matches = append(matches, scored{d, score})
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		return matches[i].d.UpdatedAt.After(matches[j].d.UpdatedAt)
	})
	if limit < 0 {
		limit = 0
	}
	if limit > len(matches) {
		limit = len(matches)
	}
	result := make([]Document, 0, limit)
	for _, m := range matches[:limit] {
		result = append(result, m.d)
	}
	return result, nil
}

func noteMatchExcerpt(body string, terms []string, maxRunes int) string {
	for _, line := range strings.Split(body, "\n") {
		lower := strings.ToLower(line)
		lineWords := tokenize(line)
		for _, term := range terms {
			if !lineWords[term] {
				continue
			}
			index := strings.Index(lower, term)
			if index < 0 {
				continue
			}
			runes := []rune(line)
			start := max(0, utf8.RuneCountInString(line[:min(index, len(line))])-80)
			end := min(len(runes), start+maxRunes)
			return strings.Join(strings.Fields(string(runes[start:end])), " ")
		}
	}
	return ""
}

func (s Store) UpdateNote(id, title, body string) (Document, error) {
	return s.updateNote(id, title, body, nil, false)
}

func (s Store) UpdateNoteWithRefs(id, title, body string, refs []string) (Document, error) {
	return s.updateNote(id, title, body, refs, true)
}

func (s Store) updateNote(id, title, body string, refs []string, replaceRefs bool) (Document, error) {
	release, err := s.lock(false)
	if err != nil {
		return Document{}, err
	}
	defer release()
	d, err := s.load("note", id)
	if err != nil {
		return Document{}, err
	}
	d.Title, d.Body, d.UpdatedAt = strings.TrimSpace(title), strings.TrimSpace(body), s.nextUpdate(d.UpdatedAt)
	if replaceRefs {
		d.SourceRefs = append([]string(nil), refs...)
	}
	if err := validate(d); err != nil {
		return Document{}, err
	}
	if err := s.write(d); err != nil {
		return Document{}, err
	}
	return d, retrieval.InvalidateID(filepath.Dir(filepath.Dir(s.root)), id)
}

func (s Store) UpdateSkill(id, description, body string) (Document, error) {
	d, err := s.LoadSkill(id)
	if err != nil {
		return Document{}, err
	}
	d.Description, d.Body, d.UpdatedAt = strings.TrimSpace(description), strings.TrimSpace(body), s.clock()
	if err := validate(d); err != nil {
		return Document{}, err
	}
	return d, s.write(d)
}

func (s Store) SetSkillEnabled(id string, enabled bool) (Document, error) {
	d, err := s.LoadSkill(id)
	if err != nil {
		return Document{}, err
	}
	d.Enabled, d.UpdatedAt = enabled, s.clock()
	if err := s.write(d); err != nil {
		return Document{}, err
	}
	return d, nil
}

func (s Store) RemoveNote(id string) error {
	release, err := s.lock(false)
	if err != nil {
		return err
	}
	defer release()
	if err := s.remove("note", id); err != nil {
		return err
	}
	return retrieval.DropID(filepath.Dir(filepath.Dir(s.root)), id)
}
func (s Store) RemoveSkill(id string) error { return s.remove("skill", id) }
func (s Store) NotePath(id string) (string, error) {
	if _, err := s.LoadNote(id); err != nil {
		return "", err
	}
	return filepath.Join(s.dir("note", id), "content.md"), nil
}
func (s Store) SkillPath(id string) (string, error) {
	if _, err := s.LoadSkill(id); err != nil {
		return "", err
	}
	return filepath.Join(s.dir("skill", id), "SKILL.md"), nil
}

func (s Store) list(kind string) ([]Document, error) {
	entries, err := os.ReadDir(s.parent(kind))
	if err != nil {
		return nil, err
	}
	if len(entries) > maxEntries {
		return nil, errors.New("knowledge directory has too many entries")
	}
	var result []Document
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".tmp-") {
			continue
		}
		d, err := s.load(kind, entry.Name())
		if err != nil {
			return nil, fmt.Errorf("load %s %s: %w", kind, entry.Name(), err)
		}
		d.Body = ""
		result = append(result, d)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].UpdatedAt.Equal(result[j].UpdatedAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].UpdatedAt.After(result[j].UpdatedAt)
	})
	return result, nil
}

func (s Store) load(kind, id string) (Document, error) {
	if kind == "note" && validID(kind, id) {
		if err := s.recoverNoteUpdate(id); err != nil {
			return Document{}, err
		}
	}
	return s.loadRaw(kind, id)
}

func (s Store) loadRaw(kind, id string) (Document, error) {
	if !validID(kind, id) {
		return Document{}, errors.New("invalid knowledge ID")
	}
	dir := s.dir(kind, id)
	info, err := os.Lstat(dir)
	if err != nil {
		return Document{}, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return Document{}, errors.New("knowledge entry is not a regular directory")
	}
	meta, err := readRegular(filepath.Join(dir, "meta.json"), 16*1024)
	if err != nil {
		return Document{}, err
	}
	var d Document
	decoder := json.NewDecoder(strings.NewReader(string(meta)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&d); err != nil {
		return Document{}, err
	}
	bodyName := "content.md"
	maxBytes := maxNoteBytes
	if kind == "skill" {
		bodyName = "SKILL.md"
		maxBytes = maxSkillBytes
	}
	body, err := readRegular(filepath.Join(dir, bodyName), int64(maxBytes))
	if err != nil {
		return Document{}, err
	}
	d.Body = string(body)
	if d.ID != id || d.Kind != kind {
		return Document{}, errors.New("knowledge identity mismatch")
	}
	return d, validate(d)
}

func (s Store) create(d Document) error {
	parent := s.parent(d.Kind)
	temp, err := os.MkdirTemp(parent, ".tmp-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp)
	if err := os.Chmod(temp, 0700); err != nil {
		return err
	}
	if err := writeFiles(temp, d); err != nil {
		return err
	}
	return os.Rename(temp, s.dir(d.Kind, d.ID))
}

func (s Store) write(d Document) error {
	before, err := s.load(d.Kind, d.ID)
	if err != nil {
		return err
	}
	if d.Kind == "note" {
		return s.writeNoteUpdate(before, d)
	}
	return writeFiles(s.dir(d.Kind, d.ID), d)
}

func writeFiles(dir string, d Document) error {
	metadata := d
	metadata.Body = ""
	meta, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	bodyName := "content.md"
	if d.Kind == "skill" {
		bodyName = "SKILL.md"
	}
	if err := writeAtomic(filepath.Join(dir, bodyName), []byte(d.Body)); err != nil {
		return err
	}
	return writeAtomic(filepath.Join(dir, "meta.json"), meta)
}

func (s Store) remove(kind, id string) error {
	if _, err := s.load(kind, id); err != nil {
		return err
	}
	return os.RemoveAll(s.dir(kind, id))
}

func (s Store) parent(kind string) string {
	if kind == "skill" {
		return filepath.Join(s.root, "skills")
	}
	return filepath.Join(s.root, "notes")
}
func (s Store) dir(kind, id string) string { return filepath.Join(s.parent(kind), id) }
func validID(kind, id string) bool {
	if kind == "skill" {
		return skillIDPattern.MatchString(id)
	}
	return kind == "note" && noteIDPattern.MatchString(id)
}
func validate(d Document) error {
	if d.Version != Version || !validID(d.Kind, d.ID) || d.CreatedAt.IsZero() || d.UpdatedAt.Before(d.CreatedAt) {
		return errors.New("invalid knowledge identity")
	}
	if d.Title == "" || len(d.Title) > 256 || strings.ContainsAny(d.Title, "\x00\r\n") || !utf8.ValidString(d.Title) {
		return errors.New("invalid knowledge title")
	}
	if d.LastUpdate != nil {
		r := d.LastUpdate
		if d.Kind != "note" || !strings.HasPrefix(r.ID, "suggestion-") || len(r.ID) != 35 || len(r.InputHash) != 64 || len(r.ContentHash) != 64 {
			return errors.New("invalid note update receipt")
		}
		for _, value := range []string{r.ID[11:], r.InputHash, r.ContentHash} {
			if _, err := hex.DecodeString(value); err != nil {
				return errors.New("invalid note update receipt digest")
			}
		}
	}
	if d.Kind == "note" {
		if d.GoalID != "" && !goalIDPattern.MatchString(d.GoalID) {
			return errors.New("invalid note goal")
		}
		if d.Description != "" || d.Enabled || len(d.Body) > maxNoteBytes || !validSourceRefs(d.SourceRefs) {
			return errors.New("invalid note metadata or size")
		}
	} else {
		if d.GoalID != "" || len(d.SourceRefs) != 0 || d.Description == "" || len(d.Description) > 512 || strings.ContainsRune(d.Description, 0) || len(d.Body) > maxSkillBytes {
			return errors.New("invalid skill metadata or size")
		}
	}
	if strings.TrimSpace(d.Body) == "" || !utf8.ValidString(d.Body) || strings.ContainsRune(d.Body, 0) {
		return errors.New("knowledge body must be bounded UTF-8 text")
	}
	return nil
}

func validSourceRefs(refs []string) bool {
	if len(refs) > 16 {
		return false
	}
	for _, ref := range refs {
		if strings.TrimSpace(ref) == "" || len(ref) > 512 || !utf8.ValidString(ref) || strings.ContainsAny(ref, "\x00\r\n") {
			return false
		}
	}
	return true
}

func readRegular(path string, max int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > max {
		return nil, errors.New("knowledge file is not a bounded regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !opened.Mode().IsRegular() || !os.SameFile(info, opened) || opened.Size() > max {
		return nil, errors.New("knowledge file changed during read")
	}
	data, err := io.ReadAll(io.LimitReader(file, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, errors.New("knowledge file exceeds size limit")
	}
	return data, nil
}

func writeAtomic(path string, data []byte) error {
	temp, err := os.CreateTemp(filepath.Dir(path), ".tmp-")
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
	return os.Rename(temp.Name(), path)
}

func tokenize(value string) map[string]bool {
	stop := map[string]bool{"the": true, "and": true, "for": true, "with": true, "from": true, "this": true, "that": true, "what": true, "when": true, "where": true, "about": true, "your": true, "please": true, "should": true, "would": true, "could": true, "have": true, "will": true, "need": true}
	terms := map[string]bool{}
	for _, term := range strings.FieldsFunc(strings.ToLower(value), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) }) {
		if len(term) >= 3 && !stop[term] {
			terms[term] = true
		}
	}
	return terms
}
