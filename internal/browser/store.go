package browser

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	sessionsFile  = "sessions.json"
	schemaVersion = 1
)

// Store persists display-safe browser-session metadata beneath Gator's private
// state root. Live browser transport credentials are intentionally owned by a
// running local controller, never this store.
type Store struct {
	mu        sync.Mutex
	directory string
	path      string
	now       func() time.Time
	random    io.Reader
}

type storedSessions struct {
	Version  int       `json:"version"`
	Sessions []Session `json:"sessions"`
}

// Open creates (if necessary) the private browser state directory. stateDir
// must already be an absolute Gator state directory resolved by the caller.
func Open(stateDir string) (*Store, error) {
	if !filepath.IsAbs(stateDir) {
		return nil, errors.New("browser state directory must be absolute")
	}
	directory := filepath.Join(stateDir, "gator", "browser")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, fmt.Errorf("create browser state directory: %w", err)
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return nil, fmt.Errorf("protect browser state directory: %w", err)
	}
	return &Store{directory: directory, path: filepath.Join(directory, sessionsFile), now: time.Now, random: rand.Reader}, nil
}

// Directory returns the private browser state directory for controller-owned
// runtime sockets and artifacts. Callers must not place repository data here.
func (s *Store) Directory() string {
	if s == nil {
		return ""
	}
	return s.directory
}

func (s *Store) List() ([]Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, err := s.loadLocked()
	if err != nil {
		return nil, err
	}
	sessions := cloneSessions(record.Sessions)
	sort.Slice(sessions, func(left, right int) bool { return sessions[left].CreatedAt.After(sessions[right].CreatedAt) })
	return sessions, nil
}

func (s *Store) Get(id string) (Session, error) {
	if err := sessionError(id); err != nil {
		return Session{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, err := s.loadLocked()
	if err != nil {
		return Session{}, err
	}
	index := sessionIndex(record.Sessions, id)
	if index < 0 {
		return Session{}, ErrNotFound
	}
	return cloneSession(record.Sessions[index]), nil
}

func (s *Store) Start(options StartOptions) (Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, err := s.loadLocked()
	if err != nil {
		return Session{}, err
	}
	id, err := s.newIDLocked(record.Sessions)
	if err != nil {
		return Session{}, err
	}
	session := Session{ID: id, Mode: ModeManaged, State: StateRunning, Headed: options.Headed, VisualCapture: options.VisualCapture, CreatedAt: s.now().UTC()}
	record.Sessions = append(record.Sessions, session)
	if err := s.saveLocked(record); err != nil {
		return Session{}, err
	}
	return cloneSession(session), nil
}

func (s *Store) Attach(options AttachOptions) (Session, error) {
	if err := ValidateCDPEndpoint(options.CDPEndpoint); err != nil {
		return Session{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, err := s.loadLocked()
	if err != nil {
		return Session{}, err
	}
	id, err := s.newIDLocked(record.Sessions)
	if err != nil {
		return Session{}, err
	}
	session := Session{ID: id, Mode: ModeAttached, State: StateRunning, VisualCapture: options.VisualCapture, CreatedAt: s.now().UTC()}
	record.Sessions = append(record.Sessions, session)
	if err := s.saveLocked(record); err != nil {
		return Session{}, err
	}
	return cloneSession(session), nil
}

// SelectTabs replaces the selected-tab set. The caller must get the candidate
// tabs from the local controller and must never forward unselected candidates
// to the agent-facing tool layer.
func (s *Store) SelectTabs(id string, tabs []Tab) (Session, error) {
	if len(tabs) == 0 {
		return Session{}, errors.New("select at least one browser tab")
	}
	seen := make(map[string]struct{}, len(tabs))
	for _, tab := range tabs {
		if err := validateTab(tab); err != nil {
			return Session{}, err
		}
		if _, exists := seen[tab.ID]; exists {
			return Session{}, errors.New("browser tab was selected more than once")
		}
		seen[tab.ID] = struct{}{}
	}
	return s.update(id, func(session *Session) error {
		session.SelectedTabs = append([]Tab(nil), tabs...)
		return nil
	})
}

func (s *Store) AddOrigin(ctx context.Context, id, value string, resolver interface {
	LookupIPAddr(context.Context, string) ([]net.IPAddr, error)
}) (Session, error) {
	origin, err := ResolveOrigin(ctx, value, resolver)
	if err != nil {
		return Session{}, err
	}
	return s.update(id, func(session *Session) error {
		for _, existing := range session.Origins {
			if existing.URL == origin.URL {
				return nil
			}
		}
		session.Origins = append(session.Origins, origin)
		sort.Slice(session.Origins, func(left, right int) bool { return session.Origins[left].URL < session.Origins[right].URL })
		return nil
	})
}

func (s *Store) RemoveOrigin(id, value string) (Session, error) {
	origin, err := CanonicalOrigin(value)
	if err != nil {
		return Session{}, err
	}
	return s.update(id, func(session *Session) error {
		filtered := session.Origins[:0]
		for _, existing := range session.Origins {
			if existing.URL != origin.URL {
				filtered = append(filtered, existing)
			}
		}
		session.Origins = filtered
		return nil
	})
}

func (s *Store) SetVisualCapture(id string, allowed bool) (Session, error) {
	return s.update(id, func(session *Session) error {
		session.VisualCapture = allowed
		return nil
	})
}

// AllowUpload registers a display-safe opaque upload ID. The controller keeps
// the validated real path in memory for the lifetime of the session.
func (s *Store) AllowUpload(id, name string) (Session, Upload, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 512 || strings.ContainsAny(name, "\r\n") {
		return Session{}, Upload{}, errors.New("browser upload name is invalid")
	}
	var created Upload
	session, err := s.update(id, func(session *Session) error {
		uploadID, err := s.newUploadIDLocked(session.AllowedUploads)
		if err != nil {
			return err
		}
		created = Upload{ID: uploadID, Name: name}
		session.AllowedUploads = append(session.AllowedUploads, created)
		return nil
	})
	return session, created, err
}

func (s *Store) Stop(id string) (Session, error) {
	return s.update(id, func(session *Session) error {
		if session.State == StateStopped {
			return nil
		}
		now := s.now().UTC()
		session.State = StateStopped
		session.StoppedAt = &now
		session.SelectedTabs = nil
		session.AllowedUploads = nil
		return nil
	})
}

func (s *Store) update(id string, mutate func(*Session) error) (Session, error) {
	if err := sessionError(id); err != nil {
		return Session{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, err := s.loadLocked()
	if err != nil {
		return Session{}, err
	}
	index := sessionIndex(record.Sessions, id)
	if index < 0 {
		return Session{}, ErrNotFound
	}
	if record.Sessions[index].State != StateRunning {
		return Session{}, ErrStopped
	}
	if err := mutate(&record.Sessions[index]); err != nil {
		return Session{}, err
	}
	if err := s.saveLocked(record); err != nil {
		return Session{}, err
	}
	return cloneSession(record.Sessions[index]), nil
}

func (s *Store) loadLocked() (storedSessions, error) {
	contents, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return storedSessions{Version: schemaVersion}, nil
	}
	if err != nil {
		return storedSessions{}, fmt.Errorf("read browser session state: %w", err)
	}
	info, err := os.Stat(s.path)
	if err != nil {
		return storedSessions{}, fmt.Errorf("stat browser session state: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return storedSessions{}, errors.New("browser session state must be a private regular file")
	}
	var record storedSessions
	if err := json.Unmarshal(contents, &record); err != nil {
		return storedSessions{}, fmt.Errorf("decode browser session state: %w", err)
	}
	if record.Version != schemaVersion {
		return storedSessions{}, errors.New("browser session state has an unsupported version")
	}
	for _, session := range record.Sessions {
		if err := validateStoredSession(session); err != nil {
			return storedSessions{}, fmt.Errorf("browser session state is invalid: %w", err)
		}
	}
	return record, nil
}

func (s *Store) saveLocked(record storedSessions) error {
	record.Version = schemaVersion
	payload, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("encode browser session state: %w", err)
	}
	temporary, err := os.CreateTemp(s.directory, ".sessions-*")
	if err != nil {
		return fmt.Errorf("create browser session state: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("protect browser session state: %w", err)
	}
	if _, err := temporary.Write(append(payload, '\n')); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write browser session state: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync browser session state: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close browser session state: %w", err)
	}
	if err := os.Rename(temporaryPath, s.path); err != nil {
		return fmt.Errorf("replace browser session state: %w", err)
	}
	return nil
}

func (s *Store) newIDLocked(existing []Session) (string, error) {
	for range 8 {
		bytes := make([]byte, 10)
		if _, err := io.ReadFull(s.random, bytes); err != nil {
			return "", fmt.Errorf("generate browser session id: %w", err)
		}
		id := "browser-" + hex.EncodeToString(bytes)
		if sessionIndex(existing, id) < 0 {
			return id, nil
		}
	}
	return "", errors.New("could not generate a unique browser session id")
}

func (s *Store) newUploadIDLocked(existing []Upload) (string, error) {
	for range 8 {
		bytes := make([]byte, 8)
		if _, err := io.ReadFull(s.random, bytes); err != nil {
			return "", fmt.Errorf("generate browser upload id: %w", err)
		}
		id := "upload-" + hex.EncodeToString(bytes)
		exists := false
		for _, upload := range existing {
			if upload.ID == id {
				exists = true
				break
			}
		}
		if !exists {
			return id, nil
		}
	}
	return "", errors.New("could not generate a unique browser upload id")
}

func sessionIndex(sessions []Session, id string) int {
	for index, session := range sessions {
		if session.ID == id {
			return index
		}
	}
	return -1
}

func validateStoredSession(session Session) error {
	if err := validateSessionID(session.ID); err != nil {
		return err
	}
	if session.Mode != ModeManaged && session.Mode != ModeAttached {
		return errors.New("browser session mode is invalid")
	}
	if session.State != StateRunning && session.State != StateStopped {
		return errors.New("browser session state is invalid")
	}
	if session.CreatedAt.IsZero() {
		return errors.New("browser session creation time is required")
	}
	seenTabs := make(map[string]struct{}, len(session.SelectedTabs))
	for _, tab := range session.SelectedTabs {
		if err := validateTab(tab); err != nil {
			return err
		}
		if _, exists := seenTabs[tab.ID]; exists {
			return errors.New("browser session selected duplicate tabs")
		}
		seenTabs[tab.ID] = struct{}{}
	}
	for _, origin := range session.Origins {
		canonical, err := CanonicalOrigin(origin.URL)
		if err != nil || canonical.URL != origin.URL {
			return errors.New("browser session origin is invalid")
		}
	}
	for _, upload := range session.AllowedUploads {
		if err := validateUpload(upload); err != nil {
			return err
		}
	}
	return nil
}

func cloneSessions(source []Session) []Session {
	if source == nil {
		return nil
	}
	result := make([]Session, len(source))
	for index, session := range source {
		result[index] = cloneSession(session)
	}
	return result
}

func cloneSession(source Session) Session {
	result := source
	result.SelectedTabs = append([]Tab(nil), source.SelectedTabs...)
	result.Origins = append([]Origin(nil), source.Origins...)
	result.AllowedUploads = append([]Upload(nil), source.AllowedUploads...)
	if source.StoppedAt != nil {
		stopped := *source.StoppedAt
		result.StoppedAt = &stopped
	}
	return result
}
