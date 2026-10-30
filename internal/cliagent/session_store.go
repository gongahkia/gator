package cliagent

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
	"regexp"
	"sort"
	"sync"
	"time"

	"github.com/gongahkia/gator/internal/filelock"
	"github.com/gongahkia/gator/internal/privatefs"
	"github.com/gongahkia/gator/internal/sandbox"
)

var ErrSessionReconciliation = errors.New("CLI session owner stopped with uncertain work; reconcile before resuming")

// SessionBinding is host-selected authority, never a model-supplied native ID.
// Capture identifies the immutable source; Owner identifies its conversation or
// goal within a profile. Dynamic memory/prompt contents deliberately do not form
// part of this permission boundary.
type SessionBinding struct {
	Owner   string   `json:"owner"`
	Capture string   `json:"capture"`
	Config  Config   `json:"config"`
	Edit    bool     `json:"edit"`
	Scopes  []string `json:"scopes,omitempty"`
}

type SessionCommand struct {
	ID      string    `json:"id"`
	Kind    string    `json:"kind"`
	Digest  string    `json:"digest"`
	Status  string    `json:"status"`
	Created time.Time `json:"created"`
}

type SessionRecord struct {
	Version        int              `json:"version"`
	ID             string           `json:"id"`
	BindingSHA256  string           `json:"binding_sha256"`
	Initialized    bool             `json:"initialized"`
	ProcessOpen    bool             `json:"process_open"`
	Native         NativeState      `json:"native"`
	Commands       []SessionCommand `json:"commands,omitempty"`
	Base           string           `json:"base,omitempty"`
	SelectedBase   string           `json:"selected_base,omitempty"`
	BaselineSHA256 string           `json:"baseline_sha256,omitempty"`
	Updated        time.Time        `json:"updated"`
}

// sessionLease is held for the entire native process lifetime, including child
// shutdown and patch export. Metadata lives outside both sandbox-writable roots.
// A lease alone does not prove an orphaned child stopped after a host crash:
// ProcessOpen makes those cases fail closed until the supervisor reconciles them.
type sessionLease struct {
	mu     sync.Mutex
	root   *os.Root
	lock   *os.File
	path   string
	record SessionRecord
	closed bool
	failed error
}

var sessionIDPattern = regexp.MustCompile(`^cli-[a-f0-9]{32}$`)

func sessionBindingDigest(b SessionBinding) (string, error) {
	if b.Owner == "" || b.Capture == "" || len(b.Owner) > 512 || len(b.Capture) > 512 {
		return "", errors.New("CLI session requires a bounded owner and source capture")
	}
	if err := b.Config.Validate(); err != nil {
		return "", err
	}
	if b.Edit && !b.Config.AllowEdits {
		return "", errors.New("CLI edits are not enabled")
	}
	if err := (sandbox.Policy{Mode: sandbox.Strict, WritablePaths: b.Scopes}).Validate(); err != nil {
		return "", err
	}
	b.Scopes = append([]string(nil), b.Scopes...)
	b.Config.ReadOnlyRoots = append([]string(nil), b.Config.ReadOnlyRoots...)
	sort.Strings(b.Scopes)
	sort.Strings(b.Config.ReadOnlyRoots)
	data, err := json.Marshal(b)
	if err != nil {
		return "", err
	}
	return sessionDigest(data), nil
}

func sessionDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func sessionToken() string {
	var token [16]byte
	// crypto/rand.Read never returns an error on supported Go runtimes.
	_, _ = rand.Read(token[:])
	return hex.EncodeToString(token[:])
}

// openSession creates a host-generated session when id is empty. An existing
// ID must come from a retained task owned by this binding, not model arguments.
func openSession(directory, id string, binding SessionBinding) (*sessionLease, error) {
	digest, err := sessionBindingDigest(binding)
	if err != nil {
		return nil, err
	}
	if !filepath.IsAbs(directory) {
		return nil, errors.New("CLI session store must be absolute")
	}
	if err := privatefs.MkdirAll(directory); err != nil {
		return nil, err
	}
	canonical, err := filepath.EvalSymlinks(directory)
	if err != nil || canonical != filepath.Clean(directory) {
		return nil, errors.New("CLI session store must be canonical")
	}
	store, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	defer store.Close()
	create := id == ""
	if create {
		id = "cli-" + sessionToken()
		if err := store.Mkdir(id, 0700); err != nil {
			return nil, err
		}
	} else if !sessionIDPattern.MatchString(id) {
		return nil, errors.New("invalid Gator CLI session ID")
	}
	info, err := store.Lstat(id)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("CLI session must be an ordinary directory")
	}
	root, err := store.OpenRoot(id)
	if err != nil {
		return nil, err
	}
	f, err := filelock.OpenRootFile(root, "lease", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		root.Close()
		return nil, err
	}
	if err := filelock.Lock(f, filelock.Exclusive|filelock.Nonblocking); err != nil {
		f.Close()
		root.Close()
		return nil, fmt.Errorf("CLI session is already owned: %w", err)
	}
	s := &sessionLease{root: root, lock: f, path: filepath.Join(directory, id)}
	fail := func(err error) (*sessionLease, error) { s.Close(); return nil, err }
	if create {
		s.record = SessionRecord{Version: 1, ID: id, BindingSHA256: digest, Native: NativeState{Status: "initializing"}}
		if err := root.Mkdir("home", 0700); err != nil {
			return fail(err)
		}
		if err := s.saveLocked(); err != nil {
			return fail(err)
		}
	} else {
		file, err := filelock.OpenRootFile(root, "session.json", os.O_RDONLY, 0)
		if err != nil {
			return fail(err)
		}
		data, err := io.ReadAll(io.LimitReader(file, 1024*1024+1))
		file.Close()
		if err != nil || len(data) > 1024*1024 || json.Unmarshal(data, &s.record) != nil || s.record.Version != 1 || s.record.ID != id {
			return fail(errors.New("invalid retained CLI session"))
		}
		if s.record.BindingSHA256 != digest {
			return fail(errors.New("CLI session owner, source or permission policy differs"))
		}
		if !s.record.Initialized || s.record.ProcessOpen {
			return fail(ErrSessionReconciliation)
		}
		for _, command := range s.record.Commands {
			if command.Status != "completed" && command.Status != "rejected" {
				return fail(ErrSessionReconciliation)
			}
		}
	}
	return s, nil
}

func (s *sessionLease) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.closed = true
		_ = filelock.Unlock(s.lock)
		_ = s.lock.Close()
		_ = s.root.Close()
	}
}

func (s *sessionLease) saveLocked() (err error) {
	if s.closed {
		return errors.New("CLI session lease closed")
	}
	if s.failed != nil {
		return s.failed
	}
	defer func() {
		if err != nil {
			s.failed = fmt.Errorf("CLI journal write failed; owner must stop: %w", err)
		}
	}()
	s.record.Updated = time.Now().UTC()
	data, err := json.MarshalIndent(s.record, "", "  ")
	if err != nil {
		return err
	}
	tmp := ".record-" + sessionToken()
	f, err := s.root.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer s.root.Remove(tmp)
	_, writeErr := f.Write(data)
	syncErr := f.Sync()
	closeErr := f.Close()
	if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
		return err
	}
	if err := s.root.Rename(tmp, "session.json"); err != nil {
		return err
	}
	dir, err := s.root.Open(".")
	if err != nil {
		return err
	}
	defer dir.Close()
	return privatefs.SyncDir(dir)
}

// admit records intent before sending bytes. A duplicate ID never sends again,
// even if acknowledgement was lost. Payload hashes detect accidental reuse
// without retaining prompts, answers or credentials in the admission journal.
func (s *sessionLease) admit(id, kind string, payload []byte) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.failed != nil {
		return false, errors.New("CLI session journal unavailable")
	}
	if id == "" || len(id) > 128 {
		return false, errors.New("invalid CLI command ID")
	}
	switch kind {
	case "send", "steer", "answer", "interrupt":
	default:
		return false, errors.New("invalid CLI command kind")
	}
	digest := sessionDigest(payload)
	for _, command := range s.record.Commands {
		if command.ID == id {
			if command.Kind != kind || command.Digest != digest {
				return false, errors.New("CLI command ID reused with different input")
			}
			return false, nil
		}
	}
	if len(s.record.Commands) >= 256 {
		return false, errors.New("CLI session command budget exhausted")
	}
	s.record.Commands = append(s.record.Commands, SessionCommand{ID: id, Kind: kind, Digest: digest, Status: "admitted", Created: time.Now().UTC()})
	return true, s.saveLocked()
}

func (s *sessionLease) commandStatus(id, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.failed != nil {
		return errors.New("CLI session journal unavailable")
	}
	for i := range s.record.Commands {
		c := &s.record.Commands[i]
		if c.ID != id {
			continue
		}
		allowed := c.Status == status || c.Status == "admitted" && (status == "acknowledged" || status == "delivered" || status == "completed" || status == "rejected") || (c.Status == "acknowledged" || c.Status == "delivered") && status == "completed"
		if !allowed {
			return errors.New("invalid CLI command state transition")
		}
		c.Status = status
		return s.saveLocked()
	}
	return errors.New("unknown CLI command")
}
