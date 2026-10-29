package browser

import (
	"errors"
	"fmt"
	"os"

	"github.com/gongahkia/gator/internal/filelock"
)

// Controller leases are deliberately separate from socket readiness. An
// unlocked, previously created lease proves its owner exited; a timeout does
// not. Legacy controllers without leases remain subject to explicit revocation.
func (s *Store) controllerLease(id string, create bool) (*os.File, error) {
	if err := sessionError(id); err != nil {
		return nil, err
	}
	// Serialize publication and acquisition with probes from other processes.
	// Otherwise a probe could mistake a just-created, not-yet-locked file for a
	// dead controller during startup.
	unlock, err := s.lock()
	if err != nil {
		return nil, err
	}
	defer unlock()
	root, err := os.OpenRoot(s.directory)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	flags := os.O_RDWR
	if create {
		flags |= os.O_CREATE
	}
	file, err := filelock.OpenRootFile(root, ".controller-"+id+".lock", flags, 0o600)
	if err != nil {
		return nil, err
	}
	if err := filelock.Lock(file, filelock.Exclusive|filelock.Nonblocking); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

// recoverController revokes only a controller whose process lease has been
// released. It never restarts a task or restores a former browser grant.
func (s *Store) recoverController(id string) (bool, error) {
	lease, err := s.controllerLease(id, false)
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, filelock.ErrWouldBlock) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check browser controller owner: %w", err)
	}
	defer lease.Close()
	if _, err := s.Stop(id); err != nil && !errors.Is(err, ErrStopped) {
		return false, err
	}
	if err := RemoveToken(s, id); err != nil {
		return false, err
	}
	if err := os.Remove(socketPath(s, id)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	return true, nil
}
