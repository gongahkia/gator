package actionlog

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/executiontarget"
	"github.com/gongahkia/gator/internal/filelock"
	"github.com/gongahkia/gator/internal/privatefs"
	"github.com/gongahkia/gator/internal/workspace"
)

// TerminalReceipt is an additive terminal receipt in the existing action store.
// It never contains terminal input bytes, or credentials.
type TerminalReceipt struct {
	Version   int                       `json:"version"`
	ID        string                    `json:"id"`
	Target    executiontarget.Binding   `json:"target"`
	Actor     executiontarget.Actor     `json:"actor"`
	Operation executiontarget.Operation `json:"operation"`
	Scope     string                    `json:"scope"`
	Digest    string                    `json:"digest"`
	Phase     string                    `json:"phase"`
	Delivery  executiontarget.Delivery  `json:"delivery"`
	Note      string                    `json:"note"`
	CreatedAt time.Time                 `json:"created_at"`
	UpdatedAt time.Time                 `json:"updated_at"`
}

func (r TerminalReceipt) validate() error {
	if r.Version != 1 || !ControlID.MatchString(r.ID) || r.Target.Validate() != nil || r.Target.Backend != executiontarget.Tmux || !controlHash.MatchString(r.Digest) || len(r.Actor.ID) > 256 || r.Actor.ID == "" || r.Actor.Kind != executiontarget.Human || r.Actor.Origin != executiontarget.Remote || len(r.Scope) > 256 || len(r.Note) > 256 || r.CreatedAt.IsZero() || r.UpdatedAt.Before(r.CreatedAt) {
		return errors.New("invalid terminal control receipt")
	}
	switch r.Operation {
	case executiontarget.TextInput, executiontarget.KeyInput, executiontarget.Interrupt:
	default:
		return errors.New("invalid terminal control operation")
	}
	if r.Phase == "claimed" && r.Delivery == executiontarget.OutcomeUnknown {
		return nil
	}
	if r.Phase != "settled" {
		return errors.New("invalid terminal control phase")
	}
	switch r.Delivery {
	case executiontarget.NotDispatched, executiontarget.DispatchAcknowledged, executiontarget.OutcomeUnknown:
		return nil
	}
	return errors.New("invalid terminal delivery")
}

// Existing history deletion/backup owns external-actions. Live
// profile guards prevent removal while the owner executes. No tombstones are
// pruned here; reaching the per-target bound refuses further admission.
func (s Store) terminalRoot(target executiontarget.Binding, create bool) (workspace.Root, error) {
	if target.Validate() != nil || target.Backend != executiontarget.Tmux {
		return workspace.Root{}, errors.New("invalid terminal receipt target")
	}
	dir := filepath.Join(s.path, "terminal-"+target.ID)
	info, err := os.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) && create {
		if err = privatefs.MkdirAll(dir); err != nil {
			return workspace.Root{}, err
		}
		parent, e := os.Open(s.path)
		if e != nil {
			return workspace.Root{}, e
		}
		e = privatefs.SyncDir(parent)
		parent.Close()
		if e != nil {
			return workspace.Root{}, e
		}
	} else if err != nil {
		return workspace.Root{}, err
	} else if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || privatefs.Check(dir) != nil {
		return workspace.Root{}, errors.New("unsafe terminal receipts")
	}
	return workspace.Open(dir)
}

// Terminal action IDs are globally claimed inside the existing action store;
// reuse against another target is a conflict, not a new dispatch intention.
func (s Store) terminalRecords(create bool) (workspace.Root, error) {
	dir := filepath.Join(s.path, "terminal-claims")
	info, err := os.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) && create {
		if err = privatefs.MkdirAll(dir); err != nil {
			return workspace.Root{}, err
		}
		parent, e := os.Open(s.path)
		if e != nil {
			return workspace.Root{}, e
		}
		e = privatefs.SyncDir(parent)
		parent.Close()
		if e != nil {
			return workspace.Root{}, e
		}
	} else if err != nil {
		return workspace.Root{}, err
	} else if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || privatefs.Check(dir) != nil {
		return workspace.Root{}, errors.New("unsafe terminal receipts")
	}
	return workspace.Open(dir)
}

// LockTerminal serializes all Gator-originated dispatches to this exact target
// across helpers. Closing or crashing releases the lock, never its durable claim.
func (s Store) LockTerminal(target executiontarget.Binding) (func(), error) {
	root, err := s.terminalRoot(target, true)
	if err != nil {
		return nil, err
	}
	dir, err := os.OpenRoot(root.Path())
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	f, err := filelock.OpenRootFile(dir, ".dispatch.lock", os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return nil, err
	}
	if err = filelock.Lock(f, filelock.Exclusive); err != nil {
		f.Close()
		return nil, err
	}
	return func() { f.Close() }, nil
}

func readTerminal(root workspace.Root, id string) (TerminalReceipt, error) {
	if !ControlID.MatchString(id) {
		return TerminalReceipt{}, errors.New("invalid action ID")
	}
	data, err := root.ReadRegularFile(id+".json", 8192)
	if err != nil {
		return TerminalReceipt{}, err
	}
	var r TerminalReceipt
	if json.Unmarshal(data, &r) != nil || r.ID != id || r.validate() != nil {
		return TerminalReceipt{}, errors.New("corrupt terminal receipt")
	}
	return r, nil
}

// ClaimTerminal flushes uncertainty before dispatch. Matching duplicates never
// receive permission to dispatch, including a claimed receipt after owner death.
func (s Store) ClaimTerminal(r TerminalReceipt) (TerminalReceipt, bool, error) {
	r.Version = 1
	r.Phase = "claimed"
	r.Delivery = executiontarget.OutcomeUnknown
	r.Note = "Admitted; dispatch outcome is not durably confirmed."
	r.UpdatedAt = r.CreatedAt
	if err := r.validate(); err != nil {
		return TerminalReceipt{}, false, err
	}
	root, err := s.terminalRecords(true)
	if err != nil {
		return TerminalReceipt{}, false, err
	}
	lock, err := controlLock(root)
	if err != nil {
		return TerminalReceipt{}, false, err
	}
	defer lock.Close()
	prior, err := readTerminal(root, r.ID)
	if err == nil {
		if prior.Digest != r.Digest || prior.Target != r.Target || prior.Actor != r.Actor || prior.Operation != r.Operation || prior.Scope != r.Scope {
			return TerminalReceipt{}, false, errors.New("action ID conflicts with retained request")
		}
		return prior, false, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return TerminalReceipt{}, false, err
	}
	dir, err := os.Open(root.Path())
	if err != nil {
		return TerminalReceipt{}, false, err
	}
	entries, e := dir.ReadDir(4098)
	dir.Close()
	if e != nil && e != io.EOF {
		return TerminalReceipt{}, false, e
	}
	if len(entries) >= 4097 {
		return TerminalReceipt{}, false, errors.New("terminal action admission limit reached")
	}
	// A crashed or ambiguous predecessor blocks new intentions. No recovery
	// button can establish that native input did or did not occur.
	targetCount := 0
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		prior, e := readTerminal(root, strings.TrimSuffix(entry.Name(), ".json"))
		if e != nil {
			return TerminalReceipt{}, false, e
		}
		if prior.Target != r.Target {
			continue
		}
		targetCount++
		if prior.Delivery == executiontarget.OutcomeUnknown {
			return TerminalReceipt{}, false, errors.New("unresolved terminal action; no follow-up admitted")
		}
	}
	if targetCount >= 128 {
		return TerminalReceipt{}, false, errors.New("target action limit reached")
	}
	data, _ := json.Marshal(r)
	if err = root.WriteRegularFileAtomic(r.ID+".json", data, 8192); err != nil {
		return TerminalReceipt{}, false, err
	}
	return r, true, nil
}

func (s Store) LookupTerminalID(id string) (TerminalReceipt, error) {
	root, err := s.terminalRecords(false)
	if err != nil {
		return TerminalReceipt{}, err
	}
	return readTerminal(root, id)
}

// ReadTerminal never dispatches and remains readable after target disappearance.
func (s Store) ReadTerminal(target executiontarget.Binding, id string) (TerminalReceipt, error) {
	root, err := s.terminalRecords(false)
	if err != nil {
		return TerminalReceipt{}, err
	}
	r, err := readTerminal(root, id)
	if err == nil && r.Target != target {
		return TerminalReceipt{}, errors.New("receipt target changed")
	}
	return r, err
}

func (s Store) FinishTerminal(r TerminalReceipt, delivery executiontarget.Delivery, note string) (TerminalReceipt, error) {
	root, err := s.terminalRecords(false)
	if err != nil {
		return r, err
	}
	lock, err := controlLock(root)
	if err != nil {
		return r, err
	}
	defer lock.Close()
	prior, err := readTerminal(root, r.ID)
	if err != nil {
		return r, err
	}
	if prior.Digest != r.Digest || prior.Target != r.Target || prior.Actor != r.Actor {
		return r, errors.New("receipt changed")
	}
	if prior.Phase == "settled" {
		return prior, nil
	}
	next := prior
	next.Phase = "settled"
	next.Delivery = delivery
	next.Note = note
	next.UpdatedAt = time.Now().UTC()
	if err = next.validate(); err != nil {
		return prior, err
	}
	data, _ := json.Marshal(next)
	if err = root.WriteRegularFileAtomic(next.ID+".json", data, 8192); err != nil {
		return prior, err
	}
	return next, nil
}

// PrepareTerminal binds durable admission to this enrollment. Only initial local
// control review may create the marker. If history/receipts were cleared later,
// this enrollment cannot regain mutation authority; fresh enrollment is required.
func (s Store) PrepareTerminal(target executiontarget.Binding, initial bool) error {
	if _, err := s.terminalRecords(initial); err != nil {
		return err
	}
	root, err := s.terminalRoot(target, initial)
	if err != nil {
		return err
	}
	lock, err := controlLock(root)
	if err != nil {
		return err
	}
	defer lock.Close()
	want, _ := json.Marshal(target)
	data, err := root.ReadRegularFile(".identity", 2048)
	if err == nil {
		if string(data) != string(want) {
			return errors.New("terminal admission identity changed")
		}
		return nil
	}
	if !initial || !errors.Is(err, os.ErrNotExist) {
		return errors.New("terminal admission history missing; reenroll target")
	}
	return root.WriteRegularFileAtomic(".identity", want, 2048)
}
