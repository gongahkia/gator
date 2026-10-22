package memory

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/retrieval"
)

// ReviewChat changes exactly the saved revision the user inspected. The origin
// and goal checks stay under the write lock so another chat cannot rebind it
// between review and mutation. Undo is available only for an unchanged new save.
func (s Store) ReviewChat(id, conversationID, goalID, updated, action, text string) error {
	release, err := s.lock()
	if err != nil {
		return err
	}
	defer release()
	record, err := s.Load(id)
	if err != nil {
		return err
	}
	if record.Origin == nil || record.Origin.ConversationID != conversationID || record.GoalID != "" && record.GoalID != goalID {
		return errors.New("this memory belongs to another chat or goal")
	}
	if updated == "" || record.UpdatedAt.Format(time.RFC3339Nano) != updated {
		return errors.New("this memory changed; reopen its current saved version before editing")
	}
	switch action {
	case "undo":
		if !record.CreatedAt.Equal(record.UpdatedAt) || record.Origin.Basis != "user-stated" {
			return errors.New("this memory has changed; review it before forgetting")
		}
		fallthrough
	case "forget":
		if record.Subject != "" {
			if err := s.blockSubject(record.GoalID, record.Subject); err != nil {
				return err
			}
		}
		if err := os.Remove(s.path(id)); err != nil {
			return err
		}
	case "correct":
		record.Text, record.UpdatedAt = strings.TrimSpace(text), s.nextUpdate(record.UpdatedAt)
		record.Origin = &ChatOrigin{ConversationID: record.Origin.ConversationID, RunID: record.Origin.RunID, Basis: "user-corrected"}
		if err := validate(record); err != nil {
			return err
		}
		if err := s.write(record, false); err != nil {
			return err
		}
	default:
		return errors.New("unsupported memory review action")
	}
	return retrieval.DropID(filepath.Dir(filepath.Dir(s.root)), id)
}
