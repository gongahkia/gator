package inbox

import (
	"errors"
	"time"
)

// ForConversation selects by the requesting chat, never by the separate
// conversation used to execute a scheduled job. Older unlinked entries remain
// available in the inbox; they are not assigned to a guessed chat.
func (s Store) ForConversation(conversationID string, limit int) ([]Entry, error) {
	if !idPattern.MatchString(conversationID) {
		return nil, errors.New("a valid conversation is required")
	}
	entries, err := s.List(false, 0)
	if err != nil {
		return nil, err
	}
	result := make([]Entry, 0)
	for _, entry := range entries {
		if entry.OriginConversationID == conversationID {
			result = append(result, entry)
			if limit > 0 && len(result) == limit {
				break
			}
		}
	}
	return result, nil
}

// Receipt only changes attention state. Delivery means the client displayed
// the card, reading is explicit, and resolution records that the user is done
// with this update. None of these authorize or execute a job or external action.
// A resolution version prevents a stale client from undoing a later decision.
func (s Store) Receipt(conversationID, entryID, action string, version uint64, now time.Time) (Entry, error) {
	if !idPattern.MatchString(conversationID) || now.IsZero() {
		return Entry{}, errors.New("a valid conversation and receipt time are required")
	}
	unlock, err := s.lock()
	if err != nil {
		return Entry{}, err
	}
	defer unlock()
	entry, err := s.Load(entryID)
	if err != nil {
		return Entry{}, err
	}
	if entry.OriginConversationID != conversationID {
		return Entry{}, errors.New("this update belongs to another conversation")
	}
	switch action {
	case "delivered":
		if !entry.DeliveredAt.IsZero() {
			return entry, nil
		}
		entry.DeliveredAt = now.UTC()
	case "read":
		if !entry.ReadAt.IsZero() {
			return entry, nil
		}
		entry.ReadAt = now.UTC()
	case "resolve", "reopen":
		resolved := action == "resolve"
		if resolved == !entry.ResolvedAt.IsZero() {
			return entry, nil
		}
		if entry.ResolutionVersion != version {
			return Entry{}, errors.New("this update changed; refresh it before changing its status")
		}
		if entry.ResolutionVersion == ^uint64(0) {
			return Entry{}, errors.New("update version exhausted")
		}
		entry.ResolutionVersion++
		if resolved {
			entry.ResolvedAt = now.UTC()
		} else {
			entry.ResolvedAt = time.Time{}
		}
	default:
		return Entry{}, errors.New("unknown update receipt")
	}
	return s.write(entry)
}
