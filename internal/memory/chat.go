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
	"github.com/gongahkia/gator/internal/privatefs"
	"github.com/gongahkia/gator/internal/retrieval"
)

var subjectPattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{1,95}$`)

func (s Store) lock() (func(), error) {
	return s.lockMode(filelock.Exclusive)
}

func (s Store) readLock() (func(), error) {
	return s.lockMode(filelock.Shared)
}

func (s Store) lockMode(mode int) (func(), error) {
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return nil, err
	}
	file, err := filelock.OpenRootFile(root, ".write-lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		root.Close()
		return nil, err
	}
	if err = filelock.Lock(file, mode); err != nil {
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
	bindings, err := s.bindings()
	if err != nil {
		return err
	}
	path := filepath.Join(s.root, ".forgotten", bindings.resolve(goalID, subject))
	return blockMarker(path)
}

func blockMarker(path string) error {
	if err := privateDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	// Retain only a subject hash, never the forgotten fact or its source quote.
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer root.Close()
	file, err := root.OpenFile(filepath.Base(path), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err := file.Write([]byte("blocked\n")); err != nil {
		return errors.Join(err, file.Close())
	}
	if err := file.Sync(); err != nil {
		return errors.Join(err, file.Close())
	}
	if err := file.Close(); err != nil {
		return err
	}
	directory, err := root.Open(".")
	if err != nil {
		return err
	}
	defer directory.Close()
	return privatefs.SyncDir(directory)
}

type ChatInput struct {
	Subject, Kind, GoalID, Quote, ConversationID, RunID, ReplaceID, ExpectedUpdated string
	Explicit                                                                        bool
	ExcludedIDs                                                                     []string
}

// RememberChat stores the user's exact words. A model cannot manufacture a
// rewritten fact. Conflicting updates need the current record and timestamp.
func (s Store) RememberChat(input ChatInput) (Record, error) {
	input.Quote = strings.TrimSpace(input.Quote)
	input.Subject = CanonicalSubject(input.Subject)
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
	blocked, err := s.blockedSubject(input.GoalID, input.Subject)
	if err != nil {
		return Record{}, err
	}
	if blocked && !input.Explicit {
		return Record{}, errors.New("this memory subject was forgotten; only an explicit new request to remember it may restore it")
	}
	records, err := s.listUnlocked(input.GoalID, false)
	if err != nil {
		return Record{}, err
	}
	bindings, err := s.bindings()
	if err != nil {
		return Record{}, err
	}
	var current Record
	creating := false
	for _, record := range records {
		if record.GoalID == input.GoalID && bindings.resolve(record.GoalID, record.Subject) == bindings.resolve(input.GoalID, input.Subject) {
			if current.ID != "" {
				return Record{}, errors.New("multiple records share this concept; reconcile the saved records before remembering it")
			}
			current = record
		}
	}
	if current.ID != "" {
		for _, id := range input.ExcludedIDs {
			if id == current.ID {
				return Record{}, errors.New("this memory subject is excluded from the current run")
			}
		}
		if input.ReplaceID != "" && (current.ID != input.ReplaceID || current.UpdatedAt.Format(time.RFC3339Nano) != input.ExpectedUpdated) {
			return Record{}, errors.New("memory revision changed; inspect current memory before saving")
		}
		if current.Text == input.Quote && current.Kind != input.Kind {
			return Record{}, errors.New("reconfirmation cannot change a memory's kind")
		}
		if current.Text == input.Quote && current.alreadyObserved(input) {
			return current, nil
		}
		if current.Text != strings.TrimSpace(input.Quote) && (current.ID != input.ReplaceID || current.UpdatedAt.Format(time.RFC3339Nano) != input.ExpectedUpdated) {
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
	if current.Subject != "" {
		input.Subject = CanonicalSubject(current.Subject)
	}
	current.Subject = input.Subject
	current.ConceptID = bindings.resolve(input.GoalID, input.Subject)
	current.RecallIssue = ""
	current.Origin = &ChatOrigin{ConversationID: input.ConversationID, RunID: input.RunID, Quote: strings.TrimSpace(input.Quote), Basis: "user-stated"}
	current.observe("user-stated", current.Origin)
	if err := validate(current); err != nil {
		return Record{}, err
	}
	if err := s.retainHistoryOrigin(current); err != nil {
		return Record{}, err
	}
	if err := s.write(current, creating); err != nil {
		return Record{}, err
	}
	if input.Explicit {
		if err := s.unblockSubject(input.GoalID, input.Subject); err != nil {
			return Record{}, err
		}
	}
	// A changed record must not keep a stale embedding under the same ID.
	if err := retrieval.InvalidateID(filepath.Dir(filepath.Dir(s.root)), current.ID); err != nil {
		return current, err
	}
	return current, nil
}

// Profile is a small always-available capsule. Relevant facts are separately
// retrieved by query; project/goal facts never enter this global capsule.
func (s Store) Profile(excluded []string) ([]Record, error) {
	records, err := s.List("", false)
	if err != nil && ReadWarning(err) == "" {
		return nil, err
	}
	var selected []Record
	for _, record := range records {
		skip := false
		for _, id := range excluded {
			skip = skip || id == record.ID
		}
		if skip || !record.Recallable() {
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
	return selected, err
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
