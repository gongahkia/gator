// Package external owns explicit external target enrollment and Inspect grants.
// It has no Work execution, model, terminal input, or daemon dependency.
package external

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/gongahkia/gator/internal/companionaccess"
	"github.com/gongahkia/gator/internal/executiontarget"
	"github.com/gongahkia/gator/internal/executiontarget/tmux"
	"github.com/gongahkia/gator/internal/filelock"
	"github.com/gongahkia/gator/internal/privatefs"
	"github.com/gongahkia/gator/internal/workspace"
)

var idPattern = regexp.MustCompile(`^target-[a-f0-9]{32}$`)
var actorPattern = regexp.MustCompile(`^inspection-[a-f0-9]{64}$`)
var ErrDenied = errors.New("external target unavailable or access withdrawn")

type NativeReader interface {
	Resolve(context.Context, string) (tmux.Native, error)
	Capture(context.Context, tmux.Native, int) (tmux.Capture, error)
}
type InspectGrant struct {
	Grant     executiontarget.Grant     `json:"grant"`
	Authority companionaccess.Reference `json:"authority"`
}
type Entry struct {
	ControlInitialized bool                       `json:"control_initialized,omitempty"`
	ControlGrants      []InspectGrant             `json:"control_grants,omitempty"`
	Target             executiontarget.Target     `json:"target"`
	Enrollment         executiontarget.Enrollment `json:"enrollment"`
	CreatedAt          time.Time                  `json:"created_at"`
	Config             tmux.Config                `json:"config"`
	ExecutableDigest   string                     `json:"executable_digest"`
	Native             tmux.Native                `json:"native"`
	Invalid            bool                       `json:"invalid"`
	Grants             []InspectGrant             `json:"grants"`
}
type catalog struct {
	Version int     `json:"version"`
	Entries []Entry `json:"entries"`
}
type Store struct {
	stateDir    string
	root        workspace.Root
	OpenReader  func(tmux.Config) (NativeReader, error)
	Fingerprint func(string) (string, error)
}
type Snapshot struct {
	Controls   []executiontarget.Operation `json:"controls,omitempty"`
	Target     executiontarget.Target      `json:"target"`
	CapturedAt *time.Time                  `json:"captured_at,omitempty"`
	Text       string                      `json:"text"`
	Truncated  bool                        `json:"truncated"`
	Stale      bool                        `json:"stale"`
}
type Response struct {
	Targets     []Snapshot `json:"targets"`
	Unavailable bool       `json:"unavailable,omitempty"`
	Reason      string     `json:"reason,omitempty"`
}

func Open(stateDir string) (Store, error) {
	parent := filepath.Join(stateDir, "gator")
	if info, err := os.Lstat(parent); err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
		return Store{}, ErrDenied
	}
	base := filepath.Join(parent, "external-targets")
	if info, err := os.Lstat(base); err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
		return Store{}, ErrDenied
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Store{}, ErrDenied
	}
	if err := privatefs.MkdirAll(base); err != nil {
		return Store{}, err
	}
	root, err := workspace.Open(base)
	return Store{root: root, stateDir: stateDir, OpenReader: func(c tmux.Config) (NativeReader, error) { return tmux.New(c) }, Fingerprint: tmux.ExecutableFingerprint}, err
}
func (s Store) transaction(write bool, f func(*catalog) error) error {
	if privatefs.Check(s.root.Path()) != nil {
		return ErrDenied
	}
	if err := privatefs.Check(filepath.Join(s.root.Path(), "catalog.json")); err != nil && !errors.Is(err, os.ErrNotExist) {
		return ErrDenied
	}
	dir, err := os.OpenRoot(s.root.Path())
	if err != nil {
		return ErrDenied
	}
	defer dir.Close()
	lock, err := filelock.OpenRootFile(dir, ".catalog.lock", os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return ErrDenied
	}
	defer lock.Close()
	mode := filelock.Shared
	if write {
		mode = filelock.Exclusive
	}
	if filelock.Lock(lock, mode) != nil {
		return ErrDenied
	}
	value := catalog{Version: 1, Entries: []Entry{}}
	data, err := s.root.ReadRegularFile("catalog.json", 512*1024)
	if err == nil {
		d := json.NewDecoder(strings.NewReader(string(data)))
		d.DisallowUnknownFields()
		if d.Decode(&value) != nil || d.Decode(new(any)) != io.EOF || value.Version != 1 || len(value.Entries) > 32 {
			return ErrDenied
		}
		for _, e := range value.Entries {
			if e.Target.Validate() != nil || e.Target.Binding.Backend != executiontarget.Tmux || e.Enrollment.Target != e.Target.Binding || len(e.Grants) > 8 || len(e.ControlGrants) > 24 {
				return ErrDenied
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return ErrDenied
	}
	if err = f(&value); err != nil {
		return err
	}
	if write {
		data, err = json.Marshal(value)
		if err != nil {
			return err
		}
		return s.root.WriteRegularFileAtomic("catalog.json", data, 512*1024)
	}
	return nil
}
func (s Store) entry(id string) (Entry, error) {
	var result Entry
	if !idPattern.MatchString(id) {
		return result, ErrDenied
	}
	err := s.transaction(false, func(c *catalog) error {
		for _, e := range c.Entries {
			if e.Target.Binding.ID == id {
				result = e
				return nil
			}
		}
		return ErrDenied
	})
	return result, err
}
func (s Store) LocalEntries() ([]Entry, error) {
	var entries []Entry
	err := s.transaction(false, func(c *catalog) error { entries = c.Entries; return nil })
	return entries, err
}
func (s Store) Enroll(ctx context.Context, config tmux.Config, pane, label string) (Entry, error) {
	if !config.ReadConfigReviewed || !config.Review.Valid() || strings.TrimSpace(label) != label || label == "" || len(label) > 256 || !utf8.ValidString(label) || strings.IndexFunc(label, unicode.IsControl) >= 0 {
		return Entry{}, ErrDenied
	}
	digest, err := s.Fingerprint(config.Executable)
	if err != nil {
		return Entry{}, ErrDenied
	}
	reader, err := s.OpenReader(config)
	if err != nil {
		return Entry{}, ErrDenied
	}
	native, err := reader.Resolve(ctx, pane)
	if err != nil {
		return Entry{}, ErrDenied
	}
	if config.Review.SocketEvidence != native.SocketEvidence || config.Review.ExecutableDigest != digest {
		return Entry{}, ErrDenied
	}
	if after, e := s.Fingerprint(config.Executable); e != nil || after != digest {
		return Entry{}, ErrDenied
	}
	ownerHash := sha256.Sum256([]byte(config.Socket + digest + native.ServerStart + native.SocketEvidence))
	binding, err := executiontarget.NewBinding(executiontarget.Tmux, "tmux-instance-"+hex.EncodeToString(ownerHash[:16]), native.Incarnation())
	if err != nil {
		return Entry{}, err
	}
	review := *config.Review
	review.ServerPID = native.ServerPID
	review.ServerStart = native.ServerStart
	config.Review = &review
	now := time.Now().UTC()
	e := Entry{Target: executiontarget.Target{Binding: binding, Label: label, Available: true, Capabilities: []executiontarget.Operation{executiontarget.Inspect}}, Enrollment: executiontarget.Enrollment{Target: binding, EnrolledBy: executiontarget.Actor{ID: "local-profile-owner", Kind: executiontarget.Human, Origin: executiontarget.Local}, EnrolledAt: now}, CreatedAt: now, Config: config, ExecutableDigest: digest, Native: native, Grants: []InspectGrant{}}
	err = s.transaction(true, func(c *catalog) error {
		if len(c.Entries) >= 32 {
			return errors.New("external enrollment limit reached")
		}
		c.Entries = append(c.Entries, e)
		return nil
	})
	return e, err
}
func (s Store) Revoke(id string) error {
	return s.transaction(true, func(c *catalog) error {
		for i := range c.Entries {
			if c.Entries[i].Target.Binding.ID == id {
				c.Entries[i].Invalid = true
				c.Entries[i].Config.Review = nil
				c.Entries[i].Config.ReadConfigReviewed = false
				c.Entries[i].Grants = nil
				c.Entries[i].ControlGrants = nil
				c.Entries[i].Config.ControlReview = nil
				return nil
			}
		}
		return ErrDenied
	})
}
func (s Store) Grant(ctx context.Context, id string, authority companionaccess.Reference, seconds int) error {
	if seconds < 0 || seconds > 900 {
		return ErrDenied
	}
	actorID, err := authority.Actor()
	if err != nil {
		return ErrDenied
	}
	e, err := s.entry(id)
	if err != nil || e.Invalid {
		return ErrDenied
	}
	if seconds > 0 {
		if _, err = s.verify(ctx, e); err != nil {
			s.Revoke(id)
			return ErrDenied
		}
	}
	return s.transaction(true, func(c *catalog) error {
		for i := range c.Entries {
			e := &c.Entries[i]
			if e.Target.Binding.ID != id {
				continue
			}
			if e.Invalid {
				return ErrDenied
			}
			grants := []InspectGrant{}
			now := time.Now().UTC()
			for _, g := range e.Grants {
				if g.Grant.Actor.ID != actorID && now.Before(g.Grant.ExpiresAt) {
					grants = append(grants, g)
				}
			}
			if seconds > 0 {
				if len(grants) >= 8 {
					return ErrDenied
				}
				grants = append(grants, InspectGrant{Grant: executiontarget.Grant{Actor: executiontarget.Actor{ID: actorID, Kind: executiontarget.Human, Origin: executiontarget.Remote}, Target: e.Target.Binding, Operation: executiontarget.Inspect, ExpiresAt: now.Add(time.Duration(seconds) * time.Second)}, Authority: authority})
			}
			e.Grants = grants
			return nil
		}
		return ErrDenied
	})
}
func (s Store) verify(ctx context.Context, e Entry) (NativeReader, error) {
	if !e.Config.Review.Valid() || !e.Config.ReadConfigReviewed || e.Config.Review.ServerPID != e.Native.ServerPID || e.Config.Review.ServerStart != e.Native.ServerStart || e.Config.Review.SocketEvidence != e.Native.SocketEvidence || e.Config.Review.ExecutableDigest != e.ExecutableDigest {
		return nil, ErrDenied
	}
	digest, err := s.Fingerprint(e.Config.Executable)
	if err != nil || digest != e.ExecutableDigest {
		return nil, ErrDenied
	}
	r, err := s.OpenReader(e.Config)
	if err != nil {
		return nil, ErrDenied
	}
	if controller, ok := r.(*tmux.Controller); ok {
		controller.ReviewCurrent = func() bool {
			fresh, err := s.entry(e.Target.Binding.ID)
			return err == nil && !fresh.Invalid && fresh.Target.Binding == e.Target.Binding && fresh.Config.Review.Valid() && fresh.Config.ReadConfigReviewed
		}
	}
	n, err := r.Resolve(ctx, e.Native.Pane)
	if err != nil || n.Incarnation() != e.Native.Incarnation() {
		return nil, ErrDenied
	}
	return r, nil
}
func (s Store) authorized(e Entry, actor executiontarget.Actor, expected executiontarget.Binding) error {
	if e.Invalid || !e.Config.ReadConfigReviewed || !e.Config.Review.Valid() || !actorPattern.MatchString(actor.ID) || actor.Kind != executiontarget.Human || actor.Origin != executiontarget.Remote {
		return ErrDenied
	}
	for _, g := range e.Grants {
		if g.Grant.Actor != actor {
			continue
		}
		if id, err := g.Authority.Actor(); err != nil || id != actor.ID {
			return ErrDenied
		}
		if executiontarget.Check(time.Now(), actor, expected, e.Target, executiontarget.Inspect, g.Grant, &e.Enrollment) == nil {
			return nil
		}
	}
	return ErrDenied
}
func (s Store) Read(ctx context.Context, actor executiontarget.Actor, id, generation string, lines int, capture bool) (Snapshot, error) {
	e, err := s.entry(id)
	if err != nil || e.Target.Binding.Generation != generation || s.authorized(e, actor, e.Target.Binding) != nil {
		return Snapshot{}, ErrDenied
	}
	r, err := s.verify(ctx, e)
	if err != nil {
		s.Revoke(id)
		return Snapshot{}, ErrDenied
	}
	value := Snapshot{Target: e.Target}
	// Native validation can block. Withdrawal during that wait must prevent
	// starting the capture as well as prevent returning its bytes.
	currentBeforeCapture, checkErr := s.entry(id)
	if checkErr != nil || currentBeforeCapture.Target.Binding != e.Target.Binding || s.authorized(currentBeforeCapture, actor, e.Target.Binding) != nil {
		return Snapshot{}, ErrDenied
	}
	if capture {
		result, err := r.Capture(ctx, e.Native, lines)
		if err != nil {
			s.Revoke(id)
			return Snapshot{}, ErrDenied
		}
		if len(result.Text) > tmux.MaxBytes || !utf8.ValidString(result.Text) {
			return Snapshot{}, ErrDenied
		}
		value.Text = result.Text
		value.Truncated = result.Truncated
		value.CapturedAt = &result.CapturedAt
	}
	// Capture holds no catalog lock, so local enrollment/grant revocation can
	// proceed. Re-read all authority before releasing any sensitive bytes.
	current, err := s.entry(id)
	if err != nil || current.Target.Binding != e.Target.Binding || s.authorized(current, actor, e.Target.Binding) != nil {
		return Snapshot{}, ErrDenied
	}
	if _, err = s.verify(ctx, current); err != nil {
		s.Revoke(id)
		return Snapshot{}, ErrDenied
	}
	err = s.transaction(false, func(c *catalog) error {
		for _, fresh := range c.Entries {
			if fresh.Target.Binding == e.Target.Binding {
				value.Controls = s.ControlOffers(fresh, actor)
				return s.authorized(fresh, actor, e.Target.Binding)
			}
		}
		return ErrDenied
	})
	if err != nil {
		return Snapshot{}, ErrDenied
	}
	return value, nil
}
func (s Store) List(ctx context.Context, actor executiontarget.Actor) (Response, error) {
	response := Response{Targets: []Snapshot{}}
	entries, err := s.LocalEntries()
	if err != nil {
		return response, err
	}
	for _, e := range entries {
		if s.authorized(e, actor, e.Target.Binding) != nil {
			continue
		}
		v, err := s.Read(ctx, actor, e.Target.Binding.ID, e.Target.Binding.Generation, 0, false)
		if err == nil {
			response.Targets = append(response.Targets, v)
		}
		if len(response.Targets) >= 16 {
			break
		}
	}
	return response, nil
}

// LocalStatus validates retained metadata; it never captures terminal output.
func (s Store) LocalStatus(ctx context.Context) ([]Entry, error) {
	entries, err := s.LocalEntries()
	if err != nil {
		return nil, err
	}
	for i := range entries {
		e := &entries[i]
		if e.Invalid {
			e.Target.Available = false
			e.Target.UnavailableReason = "Enrollment revoked or identity invalidated"
			continue
		}
		if _, err := s.verify(ctx, *e); err != nil {
			s.Revoke(e.Target.Binding.ID)
			e.Invalid = true
			e.Target.Available = false
			e.Target.UnavailableReason = "Native identity cannot be re-established; enroll again"
			e.Grants = nil
			e.ControlGrants = nil
			e.Config.ControlReview = nil
		}
	}
	return entries, nil
}

// Unreview withdraws backend trust for ALL enrollments of this server instance.
// Pure catalog update: no tmux invocation, including when the server is gone.
// Renewal requires fresh enrollment/generation and new device grants.
func (s Store) Unreview(id string) error {
	return s.transaction(true, func(c *catalog) error {
		var selected *Entry
		for i := range c.Entries {
			if c.Entries[i].Target.Binding.ID == id {
				selected = &c.Entries[i]
				break
			}
		}
		if selected == nil {
			return ErrDenied
		}
		native := selected.Native
		for i := range c.Entries {
			e := &c.Entries[i]
			if e.Native.SocketEvidence == native.SocketEvidence && e.Native.ServerPID == native.ServerPID && e.Native.ServerStart == native.ServerStart {
				e.Invalid = true
				e.Config.Review = nil
				e.Config.ReadConfigReviewed = false
				e.Grants = nil
				e.ControlGrants = nil
				e.Config.ControlReview = nil
			}
		}
		return nil
	})
}
