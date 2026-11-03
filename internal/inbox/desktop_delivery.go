package inbox

import (
	"errors"
	"os"
	"time"

	"github.com/gongahkia/gator/internal/filelock"
)

const maxDesktopAttempts = 5

// DesktopDelivery records OS handoff, not display or user acknowledgement.
// A crash after handoff but before settlement can duplicate a notification;
// retries never rerun the job or any external action.
type DesktopDelivery struct {
	State         string    `json:"state"`
	Attempts      int       `json:"attempts"`
	AttemptedAt   time.Time `json:"attempted_at"`
	NextAttemptAt time.Time `json:"next_attempt_at"`
}

func (d DesktopDelivery) validate() error {
	if (d.State != "pending" && d.State != "failed" && d.State != "handed_off") || d.Attempts < 1 || d.Attempts > maxDesktopAttempts || d.AttemptedAt.IsZero() || d.NextAttemptAt.Before(d.AttemptedAt) {
		return errors.New("invalid desktop notification delivery")
	}
	return nil
}

func (e Entry) DesktopRetryDue(now time.Time) bool {
	d := e.DesktopDelivery
	return d != nil && d.State != "handed_off" && d.Attempts < maxDesktopAttempts && !now.Before(d.NextAttemptAt) && e.DesktopNotifiedAt.IsZero() && e.ReadAt.IsZero() && e.ResolvedAt.IsZero()
}

func (e Entry) DesktopDeliveryStatus() string {
	if e.DesktopDelivery == nil {
		if !e.DesktopNotifiedAt.IsZero() {
			return "Previous OS handoff recorded; display unverified"
		}
		return "No desktop handoff recorded"
	}
	d := e.DesktopDelivery
	if d.State == "handed_off" {
		return "Accepted by the OS; display and reading unverified"
	}
	if !e.ReadAt.IsZero() || !e.ResolvedAt.IsZero() {
		return "Handoff unconfirmed; retries stopped after reading or resolution"
	}
	if d.Attempts == maxDesktopAttempts {
		return "Handoff unconfirmed after 5 attempts; retained in Inbox"
	}
	return "Handoff unconfirmed; retry eligible after " + d.NextAttemptAt.Local().Format("2 Jan 15:04:05")
}

// SendDesktopNotification serializes only this entry's OS handoff. Other inbox
// reads and receipt writes stay available while the bounded sender runs.
func (s Store) SendDesktopNotification(entryID string, now time.Time, send func(Entry) error) (bool, error) {
	if !idPattern.MatchString(entryID) || now.IsZero() || send == nil {
		return false, errors.New("notification needs an entry, time and sender")
	}
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return false, err
	}
	defer root.Close()
	lease, err := filelock.OpenRootFile(root, ".desktop-"+entryID+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return false, err
	}
	defer lease.Close()
	if err := filelock.Lock(lease, filelock.Exclusive|filelock.Nonblocking); err != nil {
		if errors.Is(err, filelock.ErrWouldBlock) {
			return false, nil
		}
		return false, err
	}
	unlock, err := s.lock()
	if err != nil {
		return false, err
	}
	e, err := s.Load(entryID)
	if err != nil {
		unlock()
		return false, err
	}
	if !e.DesktopNotifiedAt.IsZero() || !e.ReadAt.IsZero() || !e.ResolvedAt.IsZero() || e.DesktopDelivery != nil && !e.DesktopRetryDue(now) {
		unlock()
		return false, nil
	}
	attempt := 1
	if e.DesktopDelivery != nil {
		attempt += e.DesktopDelivery.Attempts
	}
	backoff := []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute, time.Hour, 6 * time.Hour}[attempt-1]
	e.DesktopDelivery = &DesktopDelivery{State: "pending", Attempts: attempt, AttemptedAt: now.UTC(), NextAttemptAt: now.Add(backoff).UTC()}
	_, err = s.write(e)
	unlock()
	if err != nil {
		return false, err
	}
	// Persist pending before dispatch. An error or process exit retains the
	// retry boundary; raw sender errors may contain private content, so do not save them.
	sendErr := send(e)
	unlock, err = s.lock()
	if err != nil {
		return true, err
	}
	defer unlock()
	current, err := s.Load(entryID)
	if err != nil {
		return true, err
	}
	if !sameEvent(e, current) || current.DesktopDelivery == nil || *current.DesktopDelivery != *e.DesktopDelivery {
		return true, errors.New("notification changed during OS handoff; outcome unconfirmed")
	}
	current.DesktopDelivery.State = "failed"
	if sendErr == nil {
		current.DesktopDelivery.State, current.DesktopNotifiedAt = "handed_off", now.UTC()
	}
	_, err = s.write(current)
	if err != nil {
		return true, err
	}
	if sendErr != nil {
		return true, errors.New("desktop notification handoff unconfirmed; retry retained")
	}
	return true, nil
}
