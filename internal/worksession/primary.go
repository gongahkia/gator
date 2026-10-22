package worksession

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gongahkia/gator/internal/filelock"
)

type primaryConversation struct {
	Version int    `json:"version"`
	ID      string `json:"conversation_id"`
}

// Primary returns the explicitly designated personal conversation. It never
// adopts the most recent topic or combines independent conversation histories.
// Removing a conversation also makes a dangling designation unavailable.
func (s Store) Primary() (string, error) {
	var value primaryConversation
	if err := readJSON(filepath.Join(s.root, "primary.json"), &value); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("read main conversation: %w", err)
	}
	if value.Version != 1 {
		return "", errors.New("main conversation designation is invalid")
	}
	if _, err := s.Load(value.ID); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("load main conversation: %w", err)
	}
	return value.ID, nil
}

// BindPrimary designates the first retained main conversation. A process lease
// prevents two newly opened windows from replacing each other's designation.
// The losing window keeps its independent topic and can open the existing main.
func (s Store) BindPrimary(id string) error {
	if _, err := s.Load(id); err != nil {
		return err
	}
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return err
	}
	defer root.Close()
	lock, err := filelock.OpenRootFile(root, ".primary-lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := filelock.Lock(lock, filelock.Exclusive); err != nil {
		return err
	}
	defer filelock.Unlock(lock)
	current, err := s.Primary()
	if err != nil {
		return err
	}
	if current != "" && current != id {
		return errors.New("another window saved the main conversation first; this conversation remains a separate topic")
	}
	return writeJSON(filepath.Join(s.root, "primary.json"), primaryConversation{Version: 1, ID: id})
}
