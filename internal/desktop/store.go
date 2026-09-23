package desktop

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
	"sync"
	"time"
)

const sessionsFile = "sessions.json"

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

func Open(stateDir string) (*Store, error) {
	if !filepath.IsAbs(stateDir) {
		return nil, errors.New("desktop state directory must be absolute")
	}
	directory := filepath.Join(stateDir, "gator", "desktop")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, fmt.Errorf("create desktop state directory: %w", err)
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return nil, fmt.Errorf("protect desktop state directory: %w", err)
	}
	return &Store{directory: directory, path: filepath.Join(directory, sessionsFile), now: time.Now, random: rand.Reader}, nil
}

func (s *Store) Start(options StartOptions) (Session, error) {
	if len(options.Apps) == 0 || len(options.Apps) > 16 {
		return Session{}, errors.New("desktop session requires 1-16 approved applications")
	}
	seen := map[string]bool{}
	for _, app := range options.Apps {
		if err := validateApplication(app); err != nil {
			return Session{}, err
		}
		if seen[app.BundleID] {
			return Session{}, errors.New("desktop application was approved more than once")
		}
		seen[app.BundleID] = true
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
	session := Session{ID: id, State: StateRunning, Apps: append([]Application(nil), options.Apps...), RetainProviderState: options.RetainProviderState, CreatedAt: s.now().UTC()}
	record.Sessions = append(record.Sessions, session)
	if err := s.saveLocked(record); err != nil {
		return Session{}, err
	}
	return cloneSession(session), nil
}

func (s *Store) Get(id string) (Session, error) {
	if err := validateSessionID(id); err != nil {
		return Session{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, err := s.loadLocked()
	if err != nil {
		return Session{}, err
	}
	for _, session := range record.Sessions {
		if session.ID == id {
			return cloneSession(session), nil
		}
	}
	return Session{}, ErrNotFound
}

func (s *Store) List() ([]Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, err := s.loadLocked()
	if err != nil {
		return nil, err
	}
	result := make([]Session, len(record.Sessions))
	for index, session := range record.Sessions {
		result[index] = cloneSession(session)
	}
	sort.Slice(result, func(left, right int) bool { return result[left].CreatedAt.After(result[right].CreatedAt) })
	return result, nil
}

func (s *Store) Stop(id string) (Session, error) {
	if err := validateSessionID(id); err != nil {
		return Session{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, err := s.loadLocked()
	if err != nil {
		return Session{}, err
	}
	for index := range record.Sessions {
		if record.Sessions[index].ID != id {
			continue
		}
		if record.Sessions[index].State == StateRunning {
			now := s.now().UTC()
			record.Sessions[index].State = StateStopped
			record.Sessions[index].StoppedAt = &now
			if err := s.saveLocked(record); err != nil {
				return Session{}, err
			}
		}
		return cloneSession(record.Sessions[index]), nil
	}
	return Session{}, ErrNotFound
}

func (s *Store) loadLocked() (storedSessions, error) {
	contents, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return storedSessions{Version: 1}, nil
	}
	if err != nil {
		return storedSessions{}, fmt.Errorf("read desktop sessions: %w", err)
	}
	info, err := os.Stat(s.path)
	if err != nil {
		return storedSessions{}, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return storedSessions{}, errors.New("desktop session state must be a private regular file")
	}
	var record storedSessions
	if err := json.Unmarshal(contents, &record); err != nil {
		return storedSessions{}, fmt.Errorf("decode desktop sessions: %w", err)
	}
	if record.Version != 1 {
		return storedSessions{}, errors.New("desktop session state has an unsupported version")
	}
	for _, session := range record.Sessions {
		if err := validateStoredSession(session); err != nil {
			return storedSessions{}, err
		}
	}
	return record, nil
}

func (s *Store) saveLocked(record storedSessions) error {
	record.Version = 1
	payload, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(s.directory, ".sessions-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(append(payload, '\n')); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, s.path)
}

func (s *Store) newIDLocked(existing []Session) (string, error) {
	for range 8 {
		data := make([]byte, 10)
		if _, err := io.ReadFull(s.random, data); err != nil {
			return "", err
		}
		id := "desktop-" + hex.EncodeToString(data)
		found := false
		for _, session := range existing {
			found = found || session.ID == id
		}
		if !found {
			return id, nil
		}
	}
	return "", errors.New("could not allocate a unique desktop session id")
}

func validateStoredSession(session Session) error {
	if err := validateSessionID(session.ID); err != nil {
		return err
	}
	if session.State != StateRunning && session.State != StateStopped || session.CreatedAt.IsZero() || len(session.Apps) == 0 || len(session.Apps) > 16 {
		return errors.New("desktop session state is invalid")
	}
	if session.State == StateStopped && session.StoppedAt == nil {
		return errors.New("stopped desktop session has no stop time")
	}
	seen := map[string]bool{}
	for _, app := range session.Apps {
		if err := validateApplication(app); err != nil || seen[app.BundleID] {
			return errors.New("desktop session application policy is invalid")
		}
		seen[app.BundleID] = true
	}
	return nil
}

func cloneSession(value Session) Session {
	value.Apps = append([]Application(nil), value.Apps...)
	if value.StoppedAt != nil {
		copied := *value.StoppedAt
		value.StoppedAt = &copied
	}
	return value
}
