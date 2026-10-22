// Package usage accounts for provider requests across Gator processes. The
// ledger contains counters and provider labels, never prompts or credentials.
package usage

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/filelock"
	"github.com/gongahkia/gator/internal/state"
)

var ErrLimit = errors.New("shared usage limit reached; open /usage to review limits")

type Policy struct {
	Paused             bool  `json:"paused"`
	DailyRequests      int   `json:"daily_requests"`
	DailyTokens        int64 `json:"daily_tokens"`
	ConcurrentRequests int   `json:"concurrent_requests"`
	ResponseReserve    int64 `json:"response_reserve"`
}

func (p Policy) Validate() error {
	if p.DailyRequests < 0 || p.DailyRequests > 1000000 || p.DailyTokens < 0 || p.ConcurrentRequests < 0 || p.ConcurrentRequests > 256 || p.ResponseReserve < 0 || p.ResponseReserve > 1048576 {
		return errors.New("invalid shared usage limits")
	}
	return nil
}

type Charge struct {
	ID, Category, Provider string
	StartedAt              time.Time
	FinishedAt             time.Time
	ReservedTokens         int64
	Tokens                 int64
	Estimated              bool
	Failed                 bool
	CostUSD                *float64 `json:",omitempty"`
}
type Day struct {
	Date    string
	Charges map[string]Charge
}
type Snapshot struct {
	Day                                                 string
	Policy                                              Policy
	Requests, Failed, EstimatedRequests, ActiveRequests int
	Tokens                                              int64
	Categories                                          map[string]int
	KnownCostUSD                                        *float64
}
type Store struct{ root string }
type Reservation struct {
	store       Store
	day, id     string
	lease       *os.File
	releaseData func()
}

func Open(stateDir string) (Store, error) {
	if strings.TrimSpace(stateDir) == "" {
		return Store{}, errors.New("usage state directory required")
	}
	root := filepath.Join(stateDir, "gator", "usage")
	for _, path := range []string{root, filepath.Join(root, "days"), filepath.Join(root, "active")} {
		if err := os.MkdirAll(path, 0700); err != nil {
			return Store{}, err
		}
	}
	return Store{root}, nil
}
func (s Store) lock() (func(), error) {
	f, err := os.OpenFile(filepath.Join(s.root, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = filelock.Lock(f, filelock.Exclusive); err != nil {
		f.Close()
		return nil, err
	}
	return func() { _ = filelock.Unlock(f); _ = f.Close() }, nil
}
func read(path string, value any) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > 64<<20 {
		return errors.New("invalid usage document")
	}
	return json.NewDecoder(f).Decode(value)
}
func write(path string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".usage-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(append(data, '\n'))
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), path)
}
func (s Store) Policy() (Policy, error) {
	var p Policy
	err := read(filepath.Join(s.root, "policy.json"), &p)
	if errors.Is(err, os.ErrNotExist) {
		err = nil
	}
	if err != nil {
		return p, err
	}
	return p, p.Validate()
}
func (s Store) SavePolicy(p Policy) error {
	if err := p.Validate(); err != nil {
		return err
	}
	unlock, err := s.lock()
	if err != nil {
		return err
	}
	defer unlock()
	return write(filepath.Join(s.root, "policy.json"), p)
}
func (s Store) SaveReviewedPolicy(p, expected Policy) error {
	if err := p.Validate(); err != nil {
		return err
	}
	unlock, err := s.lock()
	if err != nil {
		return err
	}
	defer unlock()
	prior, err := s.Policy()
	if err != nil {
		return err
	}
	if prior != expected {
		return errors.New("shared limits changed while you reviewed them; refresh and review again")
	}
	return write(filepath.Join(s.root, "policy.json"), p)
}
func (s Store) day(date string) (Day, error) {
	d := Day{Date: date, Charges: map[string]Charge{}}
	err := read(filepath.Join(s.root, "days", date+".json"), &d)
	if errors.Is(err, os.ErrNotExist) {
		err = nil
	}
	if d.Charges == nil {
		d.Charges = map[string]Charge{}
	}
	return d, err
}
func (s Store) active() (int, error) {
	entries, err := os.ReadDir(filepath.Join(s.root, "active"))
	if err != nil {
		return 0, err
	}
	count := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(s.root, "active", entry.Name())
		f, openErr := os.OpenFile(path, os.O_RDWR, 0600)
		if errors.Is(openErr, os.ErrNotExist) {
			continue
		}
		if openErr != nil {
			return 0, openErr
		}
		lockErr := filelock.Lock(f, filelock.Exclusive|filelock.Nonblocking)
		if lockErr == nil {
			_ = filelock.Unlock(f)
			_ = f.Close()
			// Windows cannot delete an open lease. The store lock serializes
			// cleanup with reservation creation on every platform.
			_ = os.Remove(path)
			continue
		} else if errors.Is(lockErr, filelock.ErrWouldBlock) {
			count++
		} else {
			f.Close()
			return 0, lockErr
		}
		f.Close()
	}
	return count, nil
}
func totals(d Day) Snapshot {
	s := Snapshot{Day: d.Date, Categories: map[string]int{}}
	known := 0.0
	allKnown := true
	for _, c := range d.Charges {
		s.Requests++
		s.Tokens += c.Tokens
		s.Categories[c.Category]++
		if c.Estimated {
			s.EstimatedRequests++
		}
		if c.Failed {
			s.Failed++
		}
		if c.CostUSD == nil {
			allKnown = false
		} else {
			known += *c.CostUSD
		}
	}
	if allKnown && s.Requests > 0 {
		s.KnownCostUSD = &known
	}
	return s
}
func (s Store) Snapshot(now time.Time) (Snapshot, error) {
	unlock, err := s.lock()
	if err != nil {
		return Snapshot{}, err
	}
	defer unlock()
	d, err := s.day(now.UTC().Format("2006-01-02"))
	if err != nil {
		return Snapshot{}, err
	}
	result := totals(d)
	result.Policy, err = s.Policy()
	if err != nil {
		return result, err
	}
	result.ActiveRequests, err = s.active()
	return result, err
}

// Admission checks known exhausted limits before preparing a scheduled Work.
// Actual provider calls still make their own atomic reservation.
func (s Store) Admission(now time.Time) error {
	v, err := s.Snapshot(now)
	if err != nil {
		return err
	}
	p := v.Policy
	if p.Paused || (p.DailyRequests > 0 && v.Requests >= p.DailyRequests) || (p.DailyTokens > 0 && v.Tokens >= p.DailyTokens) || (p.ConcurrentRequests > 0 && v.ActiveRequests >= p.ConcurrentRequests) {
		return ErrLimit
	}
	return nil
}
func (s Store) Reserve(now time.Time, category, provider string, tokens int64) (*Reservation, error) {
	release, guardErr := state.Guard(filepath.Dir(filepath.Dir(s.root)), false)
	if guardErr != nil {
		return nil, guardErr
	}
	accepted := false
	defer func() {
		if !accepted {
			release()
		}
	}()
	if tokens < 0 || len(provider) > 256 || strings.ContainsAny(provider, "\x00\r\n") {
		return nil, errors.New("invalid usage reservation")
	}
	switch category {
	case "chat", "background", "routing", "retrieval", "work", "code", "voice":
	default:
		category = "work"
	}
	unlock, err := s.lock()
	if err != nil {
		return nil, err
	}
	defer unlock()
	p, err := s.Policy()
	if err != nil {
		return nil, err
	}
	d, err := s.day(now.UTC().Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	t := totals(d)
	active, err := s.active()
	if err != nil {
		return nil, err
	}
	if p.Paused || (p.DailyRequests > 0 && t.Requests >= p.DailyRequests) || (p.DailyTokens > 0 && (tokens > p.DailyTokens-t.Tokens)) || (p.ConcurrentRequests > 0 && active >= p.ConcurrentRequests) {
		return nil, ErrLimit
	}
	var b [16]byte
	if _, err = rand.Read(b[:]); err != nil {
		return nil, err
	}
	id := hex.EncodeToString(b[:])
	lease, err := os.OpenFile(filepath.Join(s.root, "active", id), os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = filelock.Lock(lease, filelock.Exclusive); err != nil {
		lease.Close()
		os.Remove(lease.Name())
		return nil, err
	}
	d.Charges[id] = Charge{ID: id, Category: category, Provider: provider, StartedAt: now.UTC(), Tokens: tokens, ReservedTokens: tokens, Estimated: true}
	if err = write(filepath.Join(s.root, "days", d.Date+".json"), d); err != nil {
		lease.Close()
		os.Remove(lease.Name())
		return nil, err
	}
	accepted = true
	return &Reservation{store: s, day: d.Date, id: id, lease: lease, releaseData: release}, nil
}

// Finish preserves a conservative charge if the provider omits token usage or
// the process stops. Such charges are shown as estimates, never billed cost.
func (r *Reservation) Finish(tokens int64, reported, failed bool, cost *float64) error {
	if r == nil || r.lease == nil {
		return nil
	}
	defer func() {
		if r.releaseData != nil {
			r.releaseData()
			r.releaseData = nil
		}
	}()
	closeLease := func() {
		if r.lease != nil {
			_ = filelock.Unlock(r.lease)
			_ = r.lease.Close()
			r.lease = nil
		}
	}
	defer closeLease()
	unlock, err := r.store.lock()
	if err != nil {
		return err
	}
	defer unlock()
	d, err := r.store.day(r.day)
	if err != nil {
		return err
	}
	c, ok := d.Charges[r.id]
	if !ok {
		return errors.New("usage reservation disappeared")
	}
	if reported && tokens >= 0 {
		c.Tokens = tokens
		c.Estimated = false
	}
	c.Failed = failed
	c.FinishedAt = time.Now().UTC()
	c.CostUSD = cost
	d.Charges[r.id] = c
	if err = write(filepath.Join(r.store.root, "days", r.day+".json"), d); err != nil {
		return fmt.Errorf("record shared usage: %w", err)
	}
	closeLease()
	return os.Remove(filepath.Join(r.store.root, "active", r.id))
}
