package knowledge

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/filelock"
	"github.com/gongahkia/gator/internal/retrieval"
)

func (s Store) lock(readOnly bool) (func(), error) {
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return nil, err
	}
	file, err := filelock.OpenRootFile(root, ".write-lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		root.Close()
		return nil, err
	}
	mode := filelock.Exclusive
	if readOnly {
		mode = filelock.Shared
	}
	if err := filelock.Lock(file, mode); err != nil {
		file.Close()
		root.Close()
		return nil, err
	}
	return func() { filelock.Unlock(file); file.Close(); root.Close() }, nil
}

func (s Store) nextUpdate(previous time.Time) time.Time {
	now := s.clock()
	if !now.After(previous) {
		return previous.Add(time.Nanosecond)
	}
	return now
}

// Revision identifies both metadata and the body because a note's content.md
// can also be edited by a user without updating its metadata timestamp.
func (d Document) Revision() string {
	metadata, _ := json.Marshal(d)
	hash := sha256.New()
	hash.Write(metadata)
	hash.Write([]byte("\x00" + d.Body))
	return hex.EncodeToString(hash.Sum(nil))
}

// ReviewNote applies a correction or deletion to exactly the note reviewed by
// the user. All application note writers share this lock, including tool edits.
func (s Store) ReviewNote(id, goalID, revision, action, text string) (Document, error) {
	release, err := s.lock(false)
	if err != nil {
		return Document{}, err
	}
	defer release()
	d, err := s.load("note", id)
	if err != nil {
		return Document{}, err
	}
	if d.GoalID != "" && d.GoalID != goalID {
		return Document{}, errors.New("saved note is outside this goal")
	}
	if revision == "" || d.Revision() != revision {
		return Document{}, errors.New("this note changed; reopen its current saved version before editing")
	}
	switch action {
	case "correct":
		d.Body, d.UpdatedAt = strings.TrimSpace(text), s.nextUpdate(d.UpdatedAt)
		if err := validate(d); err != nil {
			return Document{}, err
		}
		if err := s.write(d); err != nil {
			return Document{}, err
		}
	case "forget":
		if err := s.remove("note", id); err != nil {
			return Document{}, err
		}
	default:
		return Document{}, errors.New("unsupported saved note action")
	}
	return d, retrieval.DropID(filepath.Dir(filepath.Dir(s.root)), id)
}
