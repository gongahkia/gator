package worksession

import (
	"errors"
	"os"
	"path/filepath"
	"time"
)

// Active is navigation state, independent of a conversation's model replay.
// Older primary designations migrate by selection without merging histories.
func (s Store) Active() (string, error) {
	var value primaryConversation
	err := readJSON(filepath.Join(s.root, "active.json"), &value)
	if err == nil {
		if value.Version != 1 {
			return "", errors.New("last chat metadata is invalid")
		}
		if _, err = s.Load(value.ID); err == nil {
			return value.ID, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	} else if id, err := s.Primary(); err != nil || id != "" {
		return id, err
	}
	items, err := s.List(1)
	if err != nil || len(items) == 0 {
		return "", err
	}
	return items[0].ID, nil
}
func (s Store) SetActive(id string) error {
	if _, err := s.Load(id); err != nil {
		return err
	}
	return writeJSON(filepath.Join(s.root, "active.json"), primaryConversation{Version: 1, ID: id})
}

// CreateChat gives even an unsent chat its own identity and draft. No source
// capture, model request, synthetic revision or permissions are created here.
func (s Store) CreateChat(source string, now time.Time) (Conversation, error) {
	return s.create("New chat", source, "", now, true)
}

// RetainedActive returns an explicit last selection, without migration or a
// recency fallback. Automatic retention uses this to preserve unsent drafts.
func (s Store) RetainedActive() (string, error) {
	var value primaryConversation
	if err := readJSON(filepath.Join(s.root, "active.json"), &value); errors.Is(err, os.ErrNotExist) {
		return "", nil
	} else if err != nil {
		return "", err
	}
	if value.Version != 1 {
		return "", errors.New("last chat metadata is invalid")
	}
	if _, err := s.Load(value.ID); errors.Is(err, os.ErrNotExist) {
		return "", nil
	} else if err != nil {
		return "", err
	}
	return value.ID, nil
}
