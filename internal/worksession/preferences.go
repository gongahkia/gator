package worksession

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"

	"github.com/gongahkia/gator/internal/filelock"
)

type Preferences struct {
	ConnectorIDs *[]string `json:"connector_ids,omitempty"`
}

var preferencesMu sync.Mutex

func (s Store) LoadPreferences(id string) (Preferences, error) {
	if _, err := s.Load(id); err != nil {
		return Preferences{}, err
	}
	var p Preferences
	err := readJSON(filepath.Join(s.root, id, "preferences.json"), &p)
	if errors.Is(err, os.ErrNotExist) {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	return p, validatePreferences(p)
}

func validatePreferences(p Preferences) error {
	if p.ConnectorIDs == nil {
		return nil
	}
	if len(*p.ConnectorIDs) > 32 {
		return errors.New("at most 32 connections may be selected")
	}
	seen := map[string]bool{}
	for _, id := range *p.ConnectorIDs {
		if id == "" || len(id) > 128 || strings.ContainsAny(id, "/\\\x00\r\n") || seen[id] {
			return errors.New("invalid or repeated connection ID")
		}
		seen[id] = true
	}
	return nil
}

// SavePreferences stores new-request selections separately from sealed replay
// evidence. A supplied expected value prevents replacing a newer selection.
func (s Store) SavePreferences(id string, next Preferences, expected *Preferences) error {
	preferencesMu.Lock()
	defer preferencesMu.Unlock()
	if err := validatePreferences(next); err != nil {
		return err
	}
	if _, err := s.Load(id); err != nil {
		return err
	}
	root, err := os.OpenRoot(filepath.Join(s.root, id))
	if err != nil {
		return err
	}
	defer root.Close()
	lock, err := filelock.OpenRootFile(root, ".preferences.lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := filelock.Lock(lock, filelock.Exclusive); err != nil {
		return err
	}
	defer filelock.Unlock(lock)
	prior, err := s.LoadPreferences(id)
	if err != nil {
		return err
	}
	if expected != nil && !reflect.DeepEqual(prior, *expected) {
		return errors.New("connection selection changed; inspect connections and try again")
	}
	return writeJSON(filepath.Join(s.root, id, "preferences.json"), next)
}
