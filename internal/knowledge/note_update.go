package knowledge

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gongahkia/gator/internal/privatefs"
	"github.com/gongahkia/gator/internal/retrieval"
)

// ApplyNoteUpdate edits one inspected note and preserves its identity and scope.
// The receipt makes retrying acceptance safe if suggestion resolution was interrupted.
func (s Store) ApplyNoteUpdate(id, goalID, revision, mutationID, title, body string, refs []string) (Document, error) {
	if !strings.HasPrefix(mutationID, "suggestion-") || len(mutationID) != 35 {
		return Document{}, errors.New("note update requires a suggestion identity")
	}
	if _, err := hex.DecodeString(mutationID[11:]); err != nil {
		return Document{}, errors.New("invalid suggestion identity")
	}
	release, err := s.lock(false)
	if err != nil {
		return Document{}, err
	}
	defer release()
	d, err := s.load("note", id)
	if err != nil {
		return Document{}, err
	}
	if d.GoalID != goalID {
		return Document{}, errors.New("note update scope changed")
	}
	input, _ := json.Marshal([]any{id, goalID, revision, title, body, refs})
	digest := sha256.Sum256(input)
	inputHash := hex.EncodeToString(digest[:])
	if d.LastUpdate != nil && d.LastUpdate.ID == mutationID {
		if d.LastUpdate.InputHash == inputHash && d.LastUpdate.ContentHash == noteContentRevision(d) {
			return d, nil
		}
		return Document{}, errors.New("accepted note changed; inspect it again before proposing an update")
	}
	if revision == "" || d.Revision() != revision {
		return Document{}, errors.New("this note changed; read its current saved revision before updating")
	}
	d.Title, d.Body, d.UpdatedAt = strings.TrimSpace(title), strings.TrimSpace(body), s.nextUpdate(d.UpdatedAt)
	for _, ref := range refs {
		if !slices.Contains(d.SourceRefs, ref) {
			d.SourceRefs = append(d.SourceRefs, ref)
		}
	}
	d.LastUpdate = &NoteUpdateReceipt{ID: mutationID, InputHash: inputHash, ContentHash: noteContentRevision(d)}
	if err := validate(d); err != nil {
		return Document{}, err
	}
	if err := s.write(d); err != nil {
		return Document{}, err
	}
	return d, nil
}

func noteContentRevision(d Document) string { d.LastUpdate = nil; return d.Revision() }

// The journal covers both files: a crash between content.md and meta.json must
// not expose mixed revisions. All note access is serialized by the store lock.
type noteUpdateJournal struct {
	Before     Document `json:"before"`
	BeforeBody string   `json:"before_body"`
	After      Document `json:"after"`
	AfterBody  string   `json:"after_body"`
}

func (s Store) writeNoteUpdate(before, after Document) error {
	journal := noteUpdateJournal{Before: before, BeforeBody: before.Body, After: after, AfterBody: after.Body}
	data, err := json.Marshal(journal)
	if err != nil {
		return err
	}
	dir := s.dir("note", after.ID)
	if err := writeAtomic(filepath.Join(dir, "update.json"), data); err != nil {
		return err
	}
	if err := syncNoteDirectory(dir); err != nil {
		return err
	}
	return s.recoverNoteUpdate(after.ID)
}

func (s Store) recoverNoteUpdate(id string) error {
	dir := s.dir("note", id)
	path := filepath.Join(dir, "update.json")
	data, err := readRegular(path, 2*maxNoteBytes+32*1024)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var j noteUpdateJournal
	if err := json.Unmarshal(data, &j); err != nil {
		return err
	}
	j.Before.Body, j.After.Body = j.BeforeBody, j.AfterBody
	if j.Before.ID != id || j.After.ID != id || j.Before.Kind != "note" || j.After.Kind != "note" || j.Before.GoalID != j.After.GoalID || validate(j.Before) != nil || validate(j.After) != nil {
		return errors.New("invalid pending note update")
	}
	current, err := s.loadRaw("note", id)
	if err != nil {
		return err
	}
	partial := j.Before
	partial.Body = j.After.Body
	reversePartial := j.After
	reversePartial.Body = j.Before.Body
	// A direct file edit during downtime is never overwritten by recovery.
	if current.Revision() != j.Before.Revision() && current.Revision() != partial.Revision() && current.Revision() != reversePartial.Revision() && current.Revision() != j.After.Revision() {
		return errors.New("note changed outside an interrupted update; preserve the files and resolve update.json before continuing")
	}
	if err := writeFiles(dir, j.After); err != nil {
		return err
	}
	if err := syncNoteDirectory(dir); err != nil {
		return err
	}
	if err := retrieval.InvalidateID(filepath.Dir(filepath.Dir(s.root)), id); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	return syncNoteDirectory(dir)
}

func syncNoteDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return privatefs.SyncDir(dir)
}
