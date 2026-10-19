package jobs

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

type CoverageGap struct{ From, To time.Time }
type Availability struct {
	LastSeen, StoppedAt time.Time
	Gaps                []CoverageGap
}

func (s Store) Availability() (Availability, error) {
	var a Availability
	err := readJSON(filepath.Join(s.root, "availability.json"), &a)
	if errors.Is(err, os.ErrNotExist) {
		err = nil
	}
	return a, err
}

// Observe records a bounded history of intervals without a supervisor
// heartbeat. It cannot reconstruct source changes during those intervals.
func (s Store) Observe(now time.Time, stopped bool) error {
	f, err := os.OpenFile(filepath.Join(s.root, ".availability.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	a, err := s.Availability()
	if err != nil {
		return err
	}
	if !a.LastSeen.IsZero() && now.Sub(a.LastSeen) > 45*time.Second {
		a.Gaps = append(a.Gaps, CoverageGap{a.LastSeen, now.UTC()})
		if len(a.Gaps) > 32 {
			a.Gaps = a.Gaps[len(a.Gaps)-32:]
		}
	}
	a.LastSeen = now.UTC()
	a.StoppedAt = time.Time{}
	if stopped {
		a.StoppedAt = now.UTC()
	}
	return writeJSON(filepath.Join(s.root, "availability.json"), a)
}
