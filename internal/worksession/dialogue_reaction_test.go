package worksession

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestDialogueReactionUsesUnchangedVersionOneReplyEnvelope(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)
	chat, _ := s.CreateChat(t.TempDir(), time.Now())
	turn, err := s.AdmitDialogue(chat.ID, "reaction-store-0001", "Thanks!", json.RawMessage(`{}`), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	turn.State = "routing"
	turn, err = s.UpdateDialogue(turn, turn.Revision)
	if err != nil {
		t.Fatal(err)
	}
	turn.State = "answered"
	turn.Response = "You're welcome."
	turn.Decision = json.RawMessage(`{"kind":"reply","text":"You're welcome.","reaction":"appreciation"}`)
	turn, err = s.UpdateDialogue(turn, turn.Revision)
	if err != nil {
		t.Fatal(err)
	}
	reopened, _ := Open(dir)
	stored, err := reopened.ReadDialogue(chat.ID, turn.ID)
	if err != nil || stored.Version != 1 || stored.State != "answered" || stored.Response == "" || string(stored.Decision) != string(turn.Decision) {
		t.Fatal("reply metadata broke version-one storage", err)
	}
	stale := turn
	stale.Revision--
	if _, err = s.UpdateDialogue(stale, stale.Revision); !errors.Is(err, ErrDialogueChanged) {
		t.Fatal("stale revision accepted", err)
	}
	// Unknown top-level fields still fail the original strict store reader. The
	// optional metadata belongs inside the already opaque decision JSON only.
	path := filepath.Join(s.root, chat.ID, "dialogue", dialogueFilename(turn))
	data, _ := os.ReadFile(path)
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(data, &fields)
	fields["reaction"] = json.RawMessage(`{}`)
	data, _ = json.Marshal(fields)
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ReadDialogue(chat.ID, turn.ID); err == nil {
		t.Fatal("store weakened unknown-field rejection")
	}
}
func TestDialogueReactionCompetingOutcomesHaveOneCASWinner(t *testing.T) {
	s, _ := Open(t.TempDir())
	chat, _ := s.CreateChat(t.TempDir(), time.Now())
	turn, _ := s.AdmitDialogue(chat.ID, "reaction-race-0001", "Thanks", json.RawMessage(`{}`), time.Now())
	turn.State = "routing"
	turn, _ = s.UpdateDialogue(turn, turn.Revision)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, state := range []string{"answered", "failed"} {
		wg.Add(1)
		go func(state string) {
			defer wg.Done()
			next := turn
			next.State = state
			if state == "answered" {
				next.Response = "You're welcome."
				next.Decision = json.RawMessage(`{"kind":"reply","text":"You're welcome.","reaction":"appreciation"}`)
			} else {
				next.Issue = "inert failure"
			}
			_, err := s.UpdateDialogue(next, next.Revision)
			results <- err
		}(state)
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else if !errors.Is(err, ErrDialogueChanged) {
			t.Fatal(err)
		}
	}
	if success != 1 {
		t.Fatal("multiple outcomes committed")
	}
	stored, err := s.ReadDialogue(chat.ID, turn.ID)
	if err != nil || stored.Revision != turn.Revision+1 {
		t.Fatal("invalid retained winner")
	}
	retry, err := s.AdmitDialogue(chat.ID, turn.ClientID, turn.Text, turn.Options, time.Now())
	if err != nil || retry.Revision != stored.Revision {
		t.Fatal("retry replayed the winner")
	}
	other, _ := s.CreateChat(t.TempDir(), time.Now())
	second, err := s.AdmitDialogue(other.ID, turn.ClientID, turn.Text, turn.Options, time.Now())
	if err != nil || second.State != "received" || second.ConversationID == stored.ConversationID {
		t.Fatal("same client key leaked outcome across conversations")
	}
}
