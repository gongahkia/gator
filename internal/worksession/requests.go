package worksession

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
	"strings"
	"sync"
	"time"

	"github.com/gongahkia/gator/internal/filelock"
)

// Requests retain user admission independently of model replay and revisions.
// Running means execution was admitted, not that an external effect succeeded.
// An owner that disappears leaves an interrupted request for explicit review;
// reading this journal never starts work.
type Request struct {
	ID         string          `json:"id"`
	Edit       uint64          `json:"edit"`
	Text       string          `json:"text"`
	Options    json.RawMessage `json:"options"`
	State      string          `json:"state"`
	WorkID     string          `json:"work_id,omitempty"`
	RevisionID string          `json:"revision_id,omitempty"`
	Issue      string          `json:"issue,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
}

type RequestJournal struct {
	Version    int       `json:"version"`
	Generation uint64    `json:"generation"`
	Requests   []Request `json:"requests"`
}

const MaxRetainedRequests = 128
const maxRequestJournalBytes = 34 * 1024 * 1024

var ErrRequestsChanged = errors.New("messages changed in another session; reopen this chat before trying again")
var requestsMu sync.Mutex

func NewRequest(text string, options json.RawMessage, now time.Time) (Request, error) {
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return Request{}, err
	}
	if now.IsZero() {
		now = time.Now()
	}
	request := Request{ID: hex.EncodeToString(id), Edit: 1, Text: strings.TrimSpace(text), Options: bytes.Clone(options), State: "queued", CreatedAt: now.UTC(), UpdatedAt: now.UTC()}
	return request, validateRequest(request)
}

func (r Request) Pending() bool {
	return r.State == "queued" || r.State == "running" || r.State == "interrupted"
}

func validateRequest(r Request) error {
	id, err := hex.DecodeString(r.ID)
	if err != nil || len(id) != 16 || r.Edit == 0 || strings.TrimSpace(r.Text) == "" || len(r.Text) > 128*1024 || len(r.Options) > 128*1024 || !json.Valid(r.Options) || len(r.Options) == 0 || r.Options[0] != '{' || r.CreatedAt.IsZero() || r.UpdatedAt.Before(r.CreatedAt) || len(r.Issue) > 4096 {
		return errors.New("invalid retained message")
	}
	switch r.State {
	case "queued", "running", "interrupted", "completed", "failed", "stopped", "discarded":
		return nil
	default:
		return errors.New("invalid message state")
	}
}

func validateRequests(j RequestJournal) error {
	if j.Version != 1 || len(j.Requests) > MaxRetainedRequests {
		return errors.New("invalid message journal")
	}
	seen, pending, running := map[string]bool{}, 0, 0
	for _, r := range j.Requests {
		if err := validateRequest(r); err != nil {
			return err
		}
		if seen[r.ID] {
			return errors.New("duplicate message identity")
		}
		seen[r.ID] = true
		if r.Pending() {
			pending++
		}
		if r.State == "running" {
			running++
		}
	}
	if pending > 17 || running > 1 {
		return errors.New("too many pending messages in this chat")
	}
	return nil
}

func (s Store) LoadRequests(id string) (RequestJournal, error) {
	if _, err := s.Load(id); err != nil {
		return RequestJournal{}, err
	}
	root, err := os.OpenRoot(filepath.Join(s.root, id))
	if err != nil {
		return RequestJournal{}, err
	}
	defer root.Close()
	file, err := filelock.OpenRootFile(root, "requests.json", os.O_RDONLY, 0)
	if errors.Is(err, os.ErrNotExist) {
		return RequestJournal{Version: 1}, nil
	} else if err != nil {
		return RequestJournal{}, err
	}
	defer file.Close()
	payload, err := io.ReadAll(io.LimitReader(file, maxRequestJournalBytes+1))
	if err != nil || len(payload) > maxRequestJournalBytes {
		return RequestJournal{}, errors.New("could not read bounded message journal")
	}
	var journal RequestJournal
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&journal); err != nil {
		return RequestJournal{}, fmt.Errorf("read retained messages: %w", err)
	}
	if decoder.Decode(new(any)) != io.EOF {
		return RequestJournal{}, errors.New("invalid message journal trailer")
	}
	return journal, validateRequests(journal)
}

// SaveRequests performs one atomic compare-and-swap, including reorder/edit and
// queued-to-running transitions. A stale UI cannot resurrect a consumed request.
func (s Store) SaveRequests(id string, next RequestJournal) (RequestJournal, error) {
	requestsMu.Lock()
	defer requestsMu.Unlock()
	if err := validateRequests(next); err != nil {
		return RequestJournal{}, err
	}
	lock, err := s.requestLock(id, ".requests.lock", false)
	if err != nil {
		return RequestJournal{}, err
	}
	defer lock.Close()
	prior, err := s.LoadRequests(id)
	if err != nil {
		return RequestJournal{}, err
	}
	if prior.Generation != next.Generation {
		return RequestJournal{}, ErrRequestsChanged
	}
	if next.Generation == ^uint64(0) {
		return RequestJournal{}, errors.New("message journal generation exhausted")
	}
	next.Generation++
	if err := writeJSON(filepath.Join(s.root, id, "requests.json"), next); err != nil {
		return RequestJournal{}, err
	}
	return next, nil
}

// AcquireExecution is held for the actual run, not just its dispatch. Process
// exit releases it. Recovery must acquire this before declaring an old run
// interrupted, so a second window cannot replay work that is still executing.
func (s Store) AcquireExecution(id string) (*os.File, error) {
	return s.requestLock(id, ".execution.lock", true)
}

func (s Store) requestLock(id, name string, nonblocking bool) (*os.File, error) {
	if _, err := s.Load(id); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(filepath.Join(s.root, id))
	if err != nil {
		return nil, err
	}
	defer root.Close()
	lock, err := filelock.OpenRootFile(root, name, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	mode := filelock.Exclusive
	if nonblocking {
		mode |= filelock.Nonblocking
	}
	if err := filelock.Lock(lock, mode); err != nil {
		lock.Close()
		return nil, err
	}
	return lock, nil
}
