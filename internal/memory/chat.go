package memory

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/filelock"
	"github.com/gongahkia/gator/internal/retrieval"
)

var subjectPattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{1,95}$`)

func (s Store) lock() (func(), error) {
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return nil, err
	}
	file, err := filelock.OpenRootFile(root, ".write-lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		root.Close()
		return nil, err
	}
	if err = filelock.Lock(file, filelock.Exclusive); err != nil {
		file.Close()
		root.Close()
		return nil, err
	}
	return func() { filelock.Unlock(file); file.Close(); root.Close() }, nil
}
func (s Store) subjectPath(goalID, subject string) string {
	hash := sha256.Sum256([]byte(goalID + "\n" + subject))
	return filepath.Join(s.root, ".forgotten", hex.EncodeToString(hash[:]))
}
func (s Store) blockSubject(goalID, subject string) error {
	path := s.subjectPath(goalID, subject)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	// Retain only a subject hash, never the forgotten fact or its source quote.
	return os.WriteFile(path, []byte("blocked\n"), 0600)
}

type ChatInput struct {
	Subject, Kind, GoalID, Quote, ConversationID, RunID, ReplaceID, ExpectedUpdated string
	Explicit                                                                        bool
}

// RememberChat stores the user's exact words. A model cannot manufacture a
// rewritten fact. Conflicting updates need the current record and timestamp.
func (s Store) RememberChat(input ChatInput) (Record, error) {
	if !subjectPattern.MatchString(input.Subject) || len(input.Quote) > 2000 || strings.TrimSpace(input.Quote) == "" || !strings.HasPrefix(input.ConversationID, "work-") || input.RunID == "" {
		return Record{}, errors.New("invalid chat memory")
	}
	if input.Kind != "fact" && input.Kind != "preference" && !(input.Kind == "decision" && input.GoalID != "") {
		return Record{}, errors.New("save personal facts/preferences, or decisions scoped to the current goal")
	}
	release, err := s.lock()
	if err != nil {
		return Record{}, err
	}
	defer release()
	blocked := s.subjectPath(input.GoalID, input.Subject)
	if _, err := os.Lstat(blocked); err == nil && !input.Explicit {
		return Record{}, errors.New("this memory subject was forgotten; only an explicit new request to remember it may restore it")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Record{}, err
	}
	records, err := s.List(input.GoalID, false)
	if err != nil {
		return Record{}, err
	}
	var current Record
	creating := false
	for _, record := range records {
		if record.GoalID == input.GoalID && record.Subject == input.Subject {
			current = record
			break
		}
	}
	if current.ID != "" {
		if current.Text == strings.TrimSpace(input.Quote) {
			return current, nil
		}
		if current.ID != input.ReplaceID || current.UpdatedAt.Format(time.RFC3339Nano) != input.ExpectedUpdated {
			return Record{}, fmt.Errorf("memory conflict: inspect %s (updated %s) before replacing it", current.ID, current.UpdatedAt.Format(time.RFC3339Nano))
		}
		current.Text, current.Kind, current.Source, current.SourceSHA256, current.SourceStatus, current.UpdatedAt = strings.TrimSpace(input.Quote), input.Kind, "Chat "+input.ConversationID, "", "", s.nextUpdate(current.UpdatedAt)
	} else {
		if input.ReplaceID != "" {
			return Record{}, errors.New("the memory was removed; inspect current memory before saving")
		}
		creating = true
		current, err = s.newRecord(input.Kind, input.Quote, input.GoalID, "Chat "+input.ConversationID, "", time.Time{})
		if err != nil {
			return Record{}, err
		}
	}
	current.Subject = input.Subject
	current.Origin = &ChatOrigin{ConversationID: input.ConversationID, RunID: input.RunID, Quote: strings.TrimSpace(input.Quote), Basis: "user-stated"}
	if err := validate(current); err != nil {
		return Record{}, err
	}
	if err := s.write(current, creating); err != nil {
		return Record{}, err
	}
	if input.Explicit {
		if err := os.Remove(blocked); err != nil && !errors.Is(err, os.ErrNotExist) {
			return Record{}, err
		}
	}
	// A changed record must not keep a stale embedding under the same ID.
	if err := retrieval.DropID(filepath.Dir(filepath.Dir(s.root)), current.ID); err != nil {
		return current, err
	}
	return current, nil
}

// Profile is a small always-available capsule. Relevant facts are separately
// retrieved by query; project/goal facts never enter this global capsule.
func (s Store) Profile(excluded []string) ([]Record, error) {
	records, err := s.List("", false)
	if err != nil {
		return nil, err
	}
	var selected []Record
	for _, record := range records {
		skip := false
		for _, id := range excluded {
			skip = skip || id == record.ID
		}
		if skip || record.SourceStatus == "changed" || record.SourceStatus == "unavailable" {
			continue
		}
		if record.Kind != "preference" && !(record.Subject == "profile.name" || record.Subject == "profile.timezone" || record.Subject == "profile.language") {
			continue
		}
		record.MatchScope, record.MatchMethod = "global", "profile"
		selected = append(selected, record)
		if len(selected) == 4 {
			break
		}
	}
	return selected, nil
}

// Revisions stay ordered even on clocks with coarse resolution or after a
// clock adjustment. This makes optimistic updates reject an old inspection.
func (s Store) nextUpdate(prior time.Time) time.Time {
	now := s.clock()
	if !now.After(prior) {
		return prior.Add(time.Nanosecond)
	}
	return now
}
