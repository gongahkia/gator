package config

import (
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/gongahkia/gator/internal/filelock"
)

var settingsUpdateMu sync.Mutex

// Update serializes a read/change/save transaction across desktop and chat
// writers. The callback must only edit settings and must not call Update again.
func (s Store) Update(change func(*Settings) error) error {
	settingsUpdateMu.Lock()
	defer settingsUpdateMu.Unlock()
	if s.path == "" || change == nil {
		return errors.New("configuration update is not initialized")
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return err
	}
	root, err := os.OpenRoot(filepath.Dir(s.path))
	if err != nil {
		return err
	}
	defer root.Close()
	lock, err := filelock.OpenRootFile(root, ".settings.lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := filelock.Lock(lock, filelock.Exclusive); err != nil {
		return err
	}
	defer filelock.Unlock(lock)
	settings, err := s.Load()
	if err != nil {
		return err
	}
	if err := change(&settings); err != nil {
		return err
	}
	return s.Save(settings)
}
