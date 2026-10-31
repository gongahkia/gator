// Package actionlog retains approved external requests before dispatch. A
// process-held lease distinguishes a current request from a crashed owner;
// neither reading nor reopening this store can execute or retry a request.
package actionlog

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/filelock"
	"github.com/gongahkia/gator/internal/privatefs"
	"github.com/gongahkia/gator/internal/workspace"
)

const MaxRunEntries = 128
const maxEntryBytes = 192 * 1024
const maxRuns = 4096
const maxScanEntries = 16384
const maxScanBytes = 32 * 1024 * 1024

var identity = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,95}$`)
var entryIdentity = regexp.MustCompile(`^effect-[a-f0-9]{32}$`)

type Entry struct {
	Version        int           `json:"version"`
	ID             string        `json:"id"`
	ConversationID string        `json:"conversation_id"`
	RunID          string        `json:"run_id"`
	Phase          string        `json:"phase"`
	StartedAt      time.Time     `json:"started_at"`
	FinishedAt     time.Time     `json:"finished_at,omitempty"`
	Action         action.Record `json:"action"`
	Live           bool          `json:"-"`
}

func (e Entry) Validate() error {
	if e.Version != 1 || !entryIdentity.MatchString(e.ID) || !identity.MatchString(e.ConversationID) || !identity.MatchString(e.RunID) || e.StartedAt.IsZero() {
		return errors.New("invalid retained external request identity")
	}
	if err := e.Action.Validate(); err != nil {
		return err
	}
	switch e.Phase {
	case "executing":
		if e.Action.Status != action.Unknown || !e.FinishedAt.IsZero() {
			return errors.New("invalid executing request outcome")
		}
	case "settled":
		if e.FinishedAt.Before(e.StartedAt) || e.Action.Status != action.Executed && e.Action.Status != action.Failed && e.Action.Status != action.Unknown {
			return errors.New("invalid retained external request outcome")
		}
	default:
		return errors.New("invalid retained external request phase")
	}
	return nil
}

type Store struct{ path string }

func Open(stateDir string) (Store, error) {
	if strings.TrimSpace(stateDir) == "" {
		return Store{}, errors.New("external request state directory is required")
	}
	base, err := filepath.Abs(filepath.Join(stateDir, "gator", "external-actions"))
	if err != nil {
		return Store{}, err
	}
	// This is private application state, never a selected source directory.
	parentPath := filepath.Dir(base)
	if info, err := os.Lstat(parentPath); err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
		return Store{}, errors.New("private Gator state must be an ordinary directory")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Store{}, err
	}
	if info, err := os.Lstat(base); err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
		return Store{}, errors.New("external request store must be an ordinary directory")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Store{}, err
	}
	if err := privatefs.MkdirAll(base); err != nil {
		return Store{}, err
	}
	parent, err := os.Open(filepath.Dir(base))
	if err != nil {
		return Store{}, err
	}
	err = privatefs.SyncDir(parent)
	parent.Close()
	if err != nil {
		return Store{}, err
	}
	return Store{path: base}, nil
}

type Attempt struct {
	mu               sync.Mutex
	root             workspace.Root
	lease            *os.File
	entry            Entry
	closed, finished bool
}

// Begin is called only inside the broker's approved execution closure. Its
// successful return means the initial uncertain receipt was flushed before
// any external mutation. A write or flush failure prevents dispatch.
func (s Store) Begin(conversation, run string, proposal action.Proposal, now time.Time) (*Attempt, error) {
	if !identity.MatchString(conversation) || !identity.MatchString(run) {
		return nil, errors.New("invalid external request owner")
	}
	if err := proposal.Validate(); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(s.path)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	if info, err := root.Lstat(run); err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
		return nil, errors.New("external request run must be an ordinary directory")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err := root.MkdirAll(run, 0700); err != nil {
		return nil, err
	}
	if err := privatefs.Protect(filepath.Join(s.path, run)); err != nil {
		return nil, err
	}
	parent, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	err = privatefs.SyncDir(parent)
	parent.Close()
	if err != nil {
		return nil, err
	}
	runRoot, err := root.OpenRoot(run)
	if err != nil {
		return nil, err
	}
	defer runRoot.Close()
	admission, err := filelock.OpenRootFile(runRoot, ".begin.lock", os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, os.ErrExist) {
		admission, err = filelock.OpenRootFile(runRoot, ".begin.lock", os.O_RDWR, 0)
	}
	if err != nil {
		return nil, err
	}
	defer admission.Close()
	if err := filelock.Lock(admission, filelock.Exclusive); err != nil {
		return nil, err
	}
	directory, err := root.Open(run)
	if err != nil {
		return nil, err
	}
	entries, readErr := directory.ReadDir(MaxRunEntries*3 + 1)
	directory.Close()
	if readErr != nil && readErr != io.EOF {
		return nil, readErr
	}
	if len(entries) > MaxRunEntries*3 {
		return nil, errors.New("external request run file limit reached")
	}
	count := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			count++
		}
	}
	if count >= MaxRunEntries {
		return nil, errors.New("external request run limit reached")
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return nil, err
	}
	id := "effect-" + hex.EncodeToString(random[:])
	lease, err := filelock.OpenRootFile(root, filepath.Join(run, id+".lock"), os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, err
	}
	if err := filelock.Lock(lease, filelock.Exclusive|filelock.Nonblocking); err != nil {
		lease.Close()
		return nil, err
	}
	boundary, err := workspace.Open(filepath.Join(s.path, run))
	if err != nil {
		lease.Close()
		return nil, err
	}
	if now.IsZero() {
		now = time.Now()
	}
	e := Entry{Version: 1, ID: id, ConversationID: conversation, RunID: run, Phase: "executing", StartedAt: now.UTC(), Action: action.Record{Proposal: proposal, Status: action.Unknown, Error: "An approved request was admitted; its external outcome is not durably confirmed. Verify the service before retrying."}}
	attempt := &Attempt{root: boundary, lease: lease, entry: e}
	if err := attempt.write(e); err != nil {
		attempt.Close()
		return nil, err
	}
	return attempt, nil
}
func (a *Attempt) write(e Entry) error {
	if err := e.Validate(); err != nil {
		return err
	}
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	return a.root.WriteRegularFileAtomic(e.ID+".json", append(data, '\n'), maxEntryBytes)
}
func (a *Attempt) Finish(record action.Record) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return errors.New("external request lease is closed")
	}
	if !reflect.DeepEqual(a.entry.Action.Proposal, record.Proposal) {
		return errors.New("external request differs from the approved proposal")
	}
	if a.finished {
		if reflect.DeepEqual(a.entry.Action, record) {
			return nil
		}
		return errors.New("external request outcome is immutable")
	}
	next := a.entry
	next.Phase = "settled"
	next.FinishedAt = time.Now().UTC()
	next.Action = record
	next.Action.BatchSteps = append([]action.BatchStep(nil), record.BatchSteps...)
	if next.FinishedAt.Before(next.StartedAt) {
		next.FinishedAt = next.StartedAt
	}
	if err := a.write(next); err != nil {
		return err
	}
	a.entry = next
	a.finished = true
	return nil
}
func (a *Attempt) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return nil
	}
	a.closed = true
	return a.lease.Close()
}

func readEntry(root *os.Root, run, id string) (Entry, int, error) {
	var e Entry
	f, err := filelock.OpenRootFile(root, filepath.Join(run, id+".json"), os.O_RDONLY, 0)
	if err != nil {
		return e, 0, err
	}
	data, err := io.ReadAll(io.LimitReader(f, maxEntryBytes+1))
	f.Close()
	if err != nil {
		return e, 0, err
	}
	if len(data) > maxEntryBytes {
		return e, 0, errors.New("external request exceeds read limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&e); err != nil {
		return e, 0, err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return e, 0, errors.New("invalid external request trailer")
	}
	if e.RunID != run || e.ID != id {
		return e, 0, errors.New("external request path does not match its identity")
	}
	if err := e.Validate(); err != nil {
		return e, 0, err
	}
	if e.Phase == "executing" {
		lease, err := filelock.OpenRootFile(root, filepath.Join(run, id+".lock"), os.O_RDWR, 0)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return e, 0, err
		}
		if err == nil {
			lockErr := filelock.Lock(lease, filelock.Exclusive|filelock.Nonblocking)
			if errors.Is(lockErr, filelock.ErrWouldBlock) {
				e.Live = true
			} else if lockErr != nil {
				lease.Close()
				return e, 0, lockErr
			} else {
				_ = filelock.Unlock(lease)
			}
			lease.Close()
		}
	}
	return e, len(data), nil
}

// List reads bounded immutable outcomes and leases. It does not resume a
// request. A stopped "executing" entry is an uncertain outcome, not a failure.
// Empty run scans retained runs; a nonempty run selects exactly that owner run.
func (s Store) List(run string) ([]Entry, error) {
	if run != "" && !identity.MatchString(run) {
		return nil, errors.New("invalid external request run")
	}
	root, err := os.OpenRoot(s.path)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	// Terminal receipts have no Work run/conversation and are not connected-
	// service Entry records. Keep their durable namespace out of Work retention.
	if run == "terminal-claims" {
		return []Entry{}, nil
	}
	if strings.HasPrefix(run, "terminal-target-") && ControlID.MatchString(strings.TrimPrefix(run, "terminal-target-")) {
		return []Entry{}, nil
	}
	runs := []string{run}
	if run == "" {
		f, err := root.Open(".")
		if err != nil {
			return nil, err
		}
		dirs, readErr := f.ReadDir(maxRuns + 1)
		f.Close()
		if readErr != nil && readErr != io.EOF {
			return nil, readErr
		}
		if len(dirs) > maxRuns {
			return nil, errors.New("external request run scan limit reached")
		}
		runs = nil
		for _, d := range dirs {
			if !d.IsDir() || !identity.MatchString(d.Name()) {
				return nil, errors.New("invalid external request run directory")
			}
			if d.Name() == "terminal-claims" {
				continue
			}
			if strings.HasPrefix(d.Name(), "terminal-target-") && ControlID.MatchString(strings.TrimPrefix(d.Name(), "terminal-target-")) {
				continue
			}
			runs = append(runs, d.Name())
		}
	}
	var out []Entry
	count, total := 0, 0
	for _, run := range runs {
		f, err := root.Open(run)
		if errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return nil, err
		}
		files, readErr := f.ReadDir(MaxRunEntries*3 + 1)
		f.Close()
		if readErr != nil && readErr != io.EOF {
			return nil, readErr
		}
		if len(files) > MaxRunEntries*3 {
			return nil, errors.New("external request run scan limit reached")
		}
		for _, file := range files {
			if strings.HasSuffix(file.Name(), ".lock") || strings.HasPrefix(file.Name(), ".gator-write-") {
				continue
			}
			id := strings.TrimSuffix(file.Name(), ".json")
			if !file.Type().IsRegular() || !strings.HasSuffix(file.Name(), ".json") || !entryIdentity.MatchString(id) {
				return nil, errors.New("invalid external request file")
			}
			count++
			if count > maxScanEntries {
				return nil, errors.New("external request record scan limit reached")
			}
			e, n, err := readEntry(root, run, id)
			if err != nil {
				return nil, fmt.Errorf("read external request %s/%s: %w", run, id, err)
			}
			total += n
			if total > maxScanBytes {
				return nil, errors.New("external request byte scan limit reached")
			}
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].StartedAt.Equal(out[j].StartedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].StartedAt.Before(out[j].StartedAt)
	})
	return out, nil
}
