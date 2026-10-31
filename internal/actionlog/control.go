package actionlog

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/gongahkia/gator/internal/executiontarget"
	"github.com/gongahkia/gator/internal/filelock"
	"github.com/gongahkia/gator/internal/privatefs"
	"github.com/gongahkia/gator/internal/workspace"
)

var ControlID = regexp.MustCompile(`^[a-f0-9]{32}$`)
var controlHash = regexp.MustCompile(`^[a-f0-9]{64}$`)

// ControlReceipt is an additive semantic receipt in the existing action store.
// It never contains steering/answer bytes, approval previews, or credentials.
type ControlReceipt struct {
	Version   int                       `json:"version"`
	ID        string                    `json:"id"`
	RunID     string                    `json:"run_id"`
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

func (r ControlReceipt) validate() error {
	if r.Version != 1 || !ControlID.MatchString(r.ID) || !identity.MatchString(r.RunID) || r.Target.Validate() != nil || r.Target.Backend != executiontarget.Work || !controlHash.MatchString(r.Digest) || len(r.Actor.ID) > 256 || r.Actor.ID == "" || r.Actor.Kind != executiontarget.Human || r.Actor.Origin != executiontarget.Remote || len(r.Scope) > 256 || len(r.Note) > 256 || r.CreatedAt.IsZero() || r.UpdatedAt.Before(r.CreatedAt) {
		return errors.New("invalid semantic control receipt")
	}
	switch r.Operation {
	case executiontarget.Cancel, executiontarget.Steer, executiontarget.Answer, executiontarget.Decision:
	default:
		return errors.New("invalid semantic control operation")
	}
	if r.Phase == "claimed" && r.Delivery == executiontarget.OutcomeUnknown {
		return nil
	}
	if r.Phase != "settled" {
		return errors.New("invalid semantic control phase")
	}
	switch r.Delivery {
	case executiontarget.NotDispatched, executiontarget.DispatchAcknowledged, executiontarget.OutcomeUnknown:
		return nil
	}
	return errors.New("invalid semantic delivery")
}

// Existing history deletion/retention/backup owns external-actions/RUN. Live
// profile guards prevent removal while the owner executes. No tombstones are
// pruned here; reaching the per-target bound refuses further admission.
func (s Store) controlRoot(run string, target executiontarget.Binding, create bool) (workspace.Root, error) {
	if !identity.MatchString(run) || target.Validate() != nil || target.Backend != executiontarget.Work {
		return workspace.Root{}, errors.New("invalid semantic receipt owner")
	}
	dir := filepath.Join(s.path, run, "semantic-"+target.ID)
	for _, path := range []string{filepath.Join(s.path, run), dir} {
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) && create {
			if err = privatefs.MkdirAll(path); err != nil {
				return workspace.Root{}, err
			}
			parent, e := os.Open(filepath.Dir(path))
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
		} else if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || privatefs.Check(path) != nil {
			return workspace.Root{}, errors.New("unsafe semantic receipt directory")
		}
	}
	return workspace.Open(dir)
}

func readControl(root workspace.Root, id string) (ControlReceipt, error) {
	if !ControlID.MatchString(id) {
		return ControlReceipt{}, errors.New("invalid action ID")
	}
	data, err := root.ReadRegularFile(id+".json", 8192)
	if err != nil {
		return ControlReceipt{}, err
	}
	var r ControlReceipt
	if json.Unmarshal(data, &r) != nil || r.ID != id || r.validate() != nil {
		return ControlReceipt{}, errors.New("corrupt semantic receipt")
	}
	return r, nil
}

func controlLock(root workspace.Root) (*os.File, error) {
	dir, err := os.OpenRoot(root.Path())
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	lock, err := filelock.OpenRootFile(dir, ".admission.lock", os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return nil, err
	}
	if err = filelock.Lock(lock, filelock.Exclusive); err != nil {
		lock.Close()
		return nil, err
	}
	return lock, nil
}

// ClaimControl flushes uncertainty before dispatch. Matching duplicates never
// receive permission to dispatch, including a claimed receipt after owner death.
func (s Store) ClaimControl(r ControlReceipt) (ControlReceipt, bool, error) {
	r.Version = 1
	r.Phase = "claimed"
	r.Delivery = executiontarget.OutcomeUnknown
	r.Note = "Admitted; dispatch outcome is not durably confirmed."
	r.UpdatedAt = r.CreatedAt
	if err := r.validate(); err != nil {
		return ControlReceipt{}, false, err
	}
	root, err := s.controlRoot(r.RunID, r.Target, true)
	if err != nil {
		return ControlReceipt{}, false, err
	}
	lock, err := controlLock(root)
	if err != nil {
		return ControlReceipt{}, false, err
	}
	defer lock.Close()
	prior, err := readControl(root, r.ID)
	if err == nil {
		if prior.Digest != r.Digest || prior.Target != r.Target || prior.Actor != r.Actor || prior.Operation != r.Operation || prior.Scope != r.Scope {
			return ControlReceipt{}, false, errors.New("action ID conflicts with retained request")
		}
		return prior, false, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return ControlReceipt{}, false, err
	}
	dir, err := os.Open(root.Path())
	if err != nil {
		return ControlReceipt{}, false, err
	}
	entries, e := dir.ReadDir(130)
	dir.Close()
	if e != nil && e != io.EOF {
		return ControlReceipt{}, false, e
	}
	if len(entries) >= 129 {
		return ControlReceipt{}, false, errors.New("semantic action admission limit reached")
	}
	data, _ := json.Marshal(r)
	if err = root.WriteRegularFileAtomic(r.ID+".json", data, 8192); err != nil {
		return ControlReceipt{}, false, err
	}
	return r, true, nil
}

func (s Store) ReadControl(run string, target executiontarget.Binding, id string) (ControlReceipt, error) {
	root, err := s.controlRoot(run, target, false)
	if err != nil {
		return ControlReceipt{}, err
	}
	r, err := readControl(root, id)
	if err == nil && (r.Target != target || r.RunID != run) {
		return ControlReceipt{}, errors.New("receipt owner mismatch")
	}
	return r, err
}

// LookupControl is read-only and does not need a surviving execution owner.
// Callers must check the returned actor before disclosing even this metadata.
func (s Store) LookupControl(run, target, id string) (ControlReceipt, error) {
	if !identity.MatchString(run) || !regexp.MustCompile(`^target-[a-f0-9]{32}$`).MatchString(target) || !ControlID.MatchString(id) {
		return ControlReceipt{}, errors.New("invalid receipt lookup")
	}
	dir := filepath.Join(s.path, run, "semantic-"+target)
	for _, p := range []string{filepath.Join(s.path, run), dir} {
		info, err := os.Lstat(p)
		if err != nil {
			return ControlReceipt{}, err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || privatefs.Check(p) != nil {
			return ControlReceipt{}, errors.New("unsafe receipt path")
		}
	}
	root, err := workspace.Open(dir)
	if err != nil {
		return ControlReceipt{}, err
	}
	r, err := readControl(root, id)
	if err == nil && (r.RunID != run || r.Target.ID != target) {
		return ControlReceipt{}, errors.New("receipt scope mismatch")
	}
	return r, err
}

func (s Store) FinishControl(r ControlReceipt, delivery executiontarget.Delivery, note string) (ControlReceipt, error) {
	root, err := s.controlRoot(r.RunID, r.Target, false)
	if err != nil {
		return r, err
	}
	lock, err := controlLock(root)
	if err != nil {
		return r, err
	}
	defer lock.Close()
	prior, err := readControl(root, r.ID)
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
