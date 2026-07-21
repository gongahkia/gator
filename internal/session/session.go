package session

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gongahkia/paw/internal/egress"
	"github.com/gongahkia/paw/internal/workspace"
)

const (
	SchemaVersion       = "paw.session/2"
	LegacySchemaVersion = "paw.session/1"
	maxIDAttempts       = 8
)

var (
	ErrInvalidSessionID   = errors.New("invalid session id")
	ErrInvalidManifest    = errors.New("invalid session manifest")
	ErrMigrationRequired  = errors.New("session state migration required")
	ErrUnsupportedSchema  = errors.New("unsupported session schema")
	ErrSessionIDCollision = errors.New("session id collision")
	journalMu             sync.Mutex
)

type Status string

const (
	StatusActive    Status = "active"
	StatusStopped   Status = "stopped"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
)

type Workspace struct {
	Root         string `json:"root"`
	GitRoot      string `json:"git_root,omitempty"`
	IsGit        bool   `json:"is_git"`
	WritableHint bool   `json:"writable_hint"`
}

type Manifest struct {
	SchemaVersion string    `json:"schema_version"`
	ID            string    `json:"id"`
	Cwd           string    `json:"cwd"`
	Status        Status    `json:"status"`
	Workspace     Workspace `json:"workspace"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type Event struct {
	SchemaVersion string          `json:"schema_version"`
	SessionID     string          `json:"session_id"`
	Sequence      int             `json:"sequence"`
	At            time.Time       `json:"at"`
	Type          string          `json:"type"`
	Data          json.RawMessage `json:"data,omitempty"`
}

type Store struct {
	dir string
}

func NewID(now time.Time, entropy io.Reader) (string, error) {
	if entropy == nil {
		entropy = rand.Reader
	}
	buf := make([]byte, 10)
	if _, err := io.ReadFull(entropy, buf); err != nil {
		return "", fmt.Errorf("read session entropy: %w", err)
	}
	return fmt.Sprintf("session-%s-%s", now.UTC().Format("20060102T150405.000000000Z"), hex.EncodeToString(buf)), nil
}

func NewManifest(id, cwd string, now time.Time) (Manifest, error) {
	state, err := workspace.Inspect(cwd)
	if err != nil {
		return Manifest{}, err
	}
	if err := validateID(id); err != nil {
		return Manifest{}, err
	}
	return Manifest{
		SchemaVersion: SchemaVersion,
		ID:            id,
		Cwd:           state.Root,
		Status:        StatusActive,
		Workspace: Workspace{
			Root:         state.Root,
			GitRoot:      state.GitRoot,
			IsGit:        state.IsGit,
			WritableHint: state.WritableHint,
		},
		CreatedAt: now.UTC(),
		UpdatedAt: now.UTC(),
	}, nil
}

func SessionRoot(cwd string) (string, error) {
	return workspace.ResolvePath(cwd, ".paw/sessions")
}

func Create(cwd string, manifest Manifest) (*Store, error) {
	if err := validateManifest(manifest); err != nil {
		return nil, err
	}
	root, err := SessionRoot(cwd)
	if err != nil {
		return nil, err
	}
	canonical, err := workspace.CanonicalRoot(cwd)
	if err != nil {
		return nil, err
	}
	if canonical != manifest.Cwd || canonical != manifest.Workspace.Root {
		return nil, fmt.Errorf("session manifest workspace does not match target cwd")
	}
	dir := filepath.Join(root, manifest.ID)
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("create session root: %w", err)
	}
	if err := os.Mkdir(dir, 0o700); err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("%w: %q", ErrSessionIDCollision, manifest.ID)
		}
		return nil, fmt.Errorf("create session directory: %w", err)
	}
	store := &Store{dir: dir}
	if err := store.SaveManifest(manifest); err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	return store, nil
}

func CreateNew(cwd string, now time.Time, entropy io.Reader) (*Store, Manifest, error) {
	for attempt := 0; attempt < maxIDAttempts; attempt++ {
		id, err := NewID(now, entropy)
		if err != nil {
			return nil, Manifest{}, err
		}
		manifest, err := NewManifest(id, cwd, now)
		if err != nil {
			return nil, Manifest{}, err
		}
		store, err := Create(cwd, manifest)
		if err == nil {
			return store, manifest, nil
		}
		if !errors.Is(err, ErrSessionIDCollision) {
			return nil, Manifest{}, err
		}
	}
	return nil, Manifest{}, fmt.Errorf("%w after %d attempts", ErrSessionIDCollision, maxIDAttempts)
}

func Open(cwd, id string) (*Store, error) {
	if err := validateID(id); err != nil {
		return nil, err
	}
	root, err := SessionRoot(cwd)
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(root, id)
	info, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("session path is not a directory: %s", dir)
	}
	return &Store{dir: dir}, nil
}

func List(cwd string) ([]Manifest, error) {
	root, err := SessionRoot(cwd)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return []Manifest{}, nil
	}
	if err != nil {
		return nil, err
	}
	manifests := make([]Manifest, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		store, err := Open(cwd, entry.Name())
		if err != nil {
			return nil, err
		}
		manifest, err := store.LoadManifest()
		if err != nil {
			return nil, fmt.Errorf("load session %q: %w", entry.Name(), err)
		}
		manifests = append(manifests, manifest)
	}
	sort.Slice(manifests, func(i, j int) bool {
		return manifests[i].UpdatedAt.After(manifests[j].UpdatedAt)
	})
	return manifests, nil
}

func (s *Store) Dir() string {
	if s == nil {
		return ""
	}
	return s.dir
}

func (s *Store) SaveManifest(manifest Manifest) error {
	if s == nil {
		return errors.New("nil session store")
	}
	if err := validateManifest(manifest); err != nil {
		return err
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session manifest: %w", err)
	}
	data = append(data, '\n')
	return writeAtomic(filepath.Join(s.dir, "manifest.json"), data)
}

func (s *Store) LoadManifest() (Manifest, error) {
	if s == nil {
		return Manifest{}, errors.New("nil session store")
	}
	data, err := os.ReadFile(filepath.Join(s.dir, "manifest.json"))
	if err != nil {
		return Manifest{}, err
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode session manifest: %w", err)
	}
	if err := validateManifest(manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func (s *Store) Append(event Event) (Event, error) {
	if s == nil {
		return Event{}, errors.New("nil session store")
	}
	journalMu.Lock()
	defer journalMu.Unlock()
	manifest, err := s.LoadManifest()
	if err != nil {
		return Event{}, err
	}
	events, err := s.events(manifest.ID)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Event{}, err
	}
	event.SchemaVersion = SchemaVersion
	event.SessionID = manifest.ID
	event.Sequence = len(events) + 1
	if event.At.IsZero() {
		event.At = time.Now().UTC()
	} else {
		event.At = event.At.UTC()
	}
	if strings.TrimSpace(event.Type) == "" {
		return Event{}, errors.New("session event type is required")
	}
	event.Data = egress.ScrubJSON(event.Data)
	data, err := json.Marshal(event)
	if err != nil {
		return Event{}, fmt.Errorf("marshal session event: %w", err)
	}
	file, err := os.OpenFile(filepath.Join(s.dir, "events.ndjson"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return Event{}, err
	}
	defer func() { _ = file.Close() }()
	if _, err := file.Write(append(data, '\n')); err != nil {
		return Event{}, err
	}
	if err := file.Sync(); err != nil {
		return Event{}, err
	}
	return event, nil
}

func (s *Store) Events() ([]Event, error) {
	if s == nil {
		return nil, errors.New("nil session store")
	}
	journalMu.Lock()
	defer journalMu.Unlock()
	manifest, err := s.LoadManifest()
	if err != nil {
		return nil, err
	}
	return s.events(manifest.ID)
}

func (s *Store) events(sessionID string) ([]Event, error) {
	file, err := os.Open(filepath.Join(s.dir, "events.ndjson"))
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	dec := json.NewDecoder(file)
	var events []Event
	for {
		var event Event
		if err := dec.Decode(&event); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("decode session event %d: %w", len(events)+1, err)
		}
		if err := validateSchemaVersion(event.SchemaVersion); err != nil {
			return nil, fmt.Errorf("invalid session event %d: %w", len(events)+1, err)
		}
		if event.SessionID != sessionID || event.Sequence != len(events)+1 || strings.TrimSpace(event.Type) == "" {
			return nil, fmt.Errorf("invalid session event %d", len(events)+1)
		}
		events = append(events, event)
	}
	return events, nil
}

func validateManifest(manifest Manifest) error {
	if err := validateSchemaVersion(manifest.SchemaVersion); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidManifest, err)
	}
	if err := validateID(manifest.ID); err != nil {
		return err
	}
	if manifest.Cwd == "" || manifest.Workspace.Root == "" {
		return fmt.Errorf("%w: cwd and workspace root are required", ErrInvalidManifest)
	}
	if manifest.Workspace.IsGit && manifest.Workspace.GitRoot == "" {
		return fmt.Errorf("%w: git workspace requires git root", ErrInvalidManifest)
	}
	if !manifest.Workspace.IsGit && manifest.Workspace.GitRoot != "" {
		return fmt.Errorf("%w: non-git workspace cannot have git root", ErrInvalidManifest)
	}
	switch manifest.Status {
	case StatusActive, StatusStopped, StatusCompleted, StatusFailed:
	default:
		return fmt.Errorf("%w: status %q", ErrInvalidManifest, manifest.Status)
	}
	if manifest.CreatedAt.IsZero() || manifest.UpdatedAt.IsZero() {
		return fmt.Errorf("%w: timestamps are required", ErrInvalidManifest)
	}
	return nil
}

func validateSchemaVersion(version string) error {
	switch version {
	case SchemaVersion:
		return nil
	case LegacySchemaVersion:
		return fmt.Errorf("%w: %q must remain in the legacy trace workflow", ErrMigrationRequired, version)
	default:
		return fmt.Errorf("%w: %q", ErrUnsupportedSchema, version)
	}
}

func validateID(id string) error {
	if !strings.HasPrefix(id, "session-") || len(id) > 160 || strings.Contains(id, "..") {
		return fmt.Errorf("%w: %q", ErrInvalidSessionID, id)
	}
	for _, r := range id {
		if !validIDRune(r) {
			return fmt.Errorf("%w: %q", ErrInvalidSessionID, id)
		}
	}
	return nil
}

func validIDRune(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.'
}

func writeAtomic(path string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".paw-session-*")
	if err != nil {
		return err
	}
	tmp := file.Name()
	defer func() { _ = os.Remove(tmp) }()
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer func() { _ = dir.Close() }()
	return dir.Sync()
}
