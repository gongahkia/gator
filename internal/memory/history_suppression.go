package memory

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
)

func historyHash(goalID, conversationID string) string {
	digest := sha256.Sum256([]byte(goalID + "\n" + conversationID))
	return hex.EncodeToString(digest[:])
}

func (s Store) originDirectory(record Record) string {
	return filepath.Join(s.root, ".origins", historyHash(record.GoalID, CanonicalSubject(record.Subject)))
}

// The bounded human-readable evidence log may rotate. This separate hash-only
// index retains every source conversation needed to honor a later forget.
// Publish before the record: interruption may conservatively retain an extra
// source association, but cannot publish a memory without its suppression path.
func (s Store) retainHistoryOrigin(record Record) error {
	if record.Origin == nil || record.Subject == "" || record.Origin.ConversationID == "" {
		return nil
	}
	if err := privateDirectory(filepath.Join(s.root, ".origins")); err != nil {
		return err
	}
	return blockMarker(filepath.Join(s.originDirectory(record), historyHash(record.GoalID, record.Origin.ConversationID)))
}

func (s Store) historyOrigins(record Record) (map[string]bool, error) {
	hashes := map[string]bool{}
	if record.Origin != nil && record.Origin.ConversationID != "" {
		hashes[historyHash(record.GoalID, record.Origin.ConversationID)] = true
	}
	for _, evidence := range record.Evidence {
		if evidence.ConversationID != "" {
			hashes[historyHash(record.GoalID, evidence.ConversationID)] = true
		}
	}
	if record.Subject != "" {
		entries, err := os.ReadDir(s.originDirectory(record))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() || len(entry.Name()) != 64 {
				return nil, errors.New("invalid memory history-origin index")
			}
			if _, err := hex.DecodeString(entry.Name()); err != nil {
				return nil, errors.New("invalid memory history-origin digest")
			}
			hashes[entry.Name()] = true
		}
	}
	return hashes, nil
}

func (s Store) suppressHistory(record Record) error {
	hashes, err := s.historyOrigins(record)
	if err != nil {
		return err
	}
	for hash := range hashes {
		if err := blockMarker(filepath.Join(s.root, ".forgotten-history", hash)); err != nil {
			return err
		}
	}
	return nil
}

// Historical transcripts remain available to the user. Agent-driven history
// tools omit a whole known source conversation, since excerpting individual
// lines cannot safely remove their meaning from dependent replies or titles.
// Restoring a saved concept does not silently re-enable its old source chats.
func (s Store) HistorySuppressed(conversationID, goalID string) (bool, error) {
	for _, scope := range []string{"", goalID} {
		_, err := os.Lstat(filepath.Join(s.root, ".forgotten-history", historyHash(scope, conversationID)))
		if err == nil {
			return true, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
	}
	return false, nil
}
