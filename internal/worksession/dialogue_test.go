package worksession

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestDialogueAdmissionIsIndependentOfExecutionAndIdempotent(t *testing.T) {
	stateDir := t.TempDir()
	store, err := Open(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := store.CreateChat(t.TempDir(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	lease, err := store.AcquireExecution(conversation.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	options := json.RawMessage(`{ "Uploads": ["owned-file"], "ConnectorIDs": ["mail"], "ModelName": "A&B" }`)
	first, err := store.AdmitDialogue(conversation.ID, "client-dialogue-0001", "Actually, keep it under $2,000", options, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	reopened, _ := Open(stateDir)
	duplicate, err := reopened.AdmitDialogue(conversation.ID, first.ClientID, first.Text, options, time.Now())
	if err != nil || duplicate.ID != first.ID || duplicate.Sequence != first.Sequence || duplicate.Revision != 1 {
		t.Fatalf("retry changed message identity: %+v %v", duplicate, err)
	}
	if _, err := store.AdmitDialogue(conversation.ID, first.ClientID, "Different instructions", options, time.Now()); err == nil {
		t.Fatal("same identity changed its message")
	}
	if _, err := store.AdmitDialogue(conversation.ID, first.ClientID, first.Text, json.RawMessage(`{"Uploads":["different-file"]}`), time.Now()); err == nil {
		t.Fatal("same identity changed captured inputs")
	}
	chat, err := store.Load(conversation.ID)
	requests, requestErr := store.LoadRequests(conversation.ID)
	if err != nil || requestErr != nil || chat.HeadRevision != "" || len(requests.Requests) != 0 {
		t.Fatal("conversation admission mutated execution/replay state")
	}
	info, err := os.Stat(filepath.Join(store.root, conversation.ID, "dialogue", dialogueFilename(first)))
	if err != nil || info.Mode().Perm()&0077 != 0 {
		t.Fatal("conversation message is not private")
	}
}

func TestDialoguePagesRetainOlderMessagesAndUseSequenceInsteadOfClockOrder(t *testing.T) {
	store, _ := Open(t.TempDir())
	conversation, _ := store.CreateChat(t.TempDir(), time.Now())
	now := time.Now()
	for i := 0; i < 132; i++ {
		_, err := store.AdmitDialogue(conversation.ID, fmt.Sprintf("client-message-%06d", i), fmt.Sprintf("Message %d", i), json.RawMessage(`{}`), now.Add(-time.Duration(i)*time.Hour))
		if err != nil {
			t.Fatal(err)
		}
	}
	var before uint64
	total := 0
	for {
		page, more, err := store.DialoguePage(conversation.ID, before, 17)
		if err != nil || len(page) == 0 || len(page) > 17 {
			t.Fatalf("bad page: %d %v", len(page), err)
		}
		for index := 1; index < len(page); index++ {
			if page[index].Sequence <= page[index-1].Sequence {
				t.Fatal("clock changes reordered conversation")
			}
		}
		total += len(page)
		before = page[0].Sequence
		if !more {
			break
		}
	}
	if total != 132 {
		t.Fatal("older conversation messages were dropped")
	}
	if _, _, err := store.DialoguePage("missing-conversation", 0, 17); err == nil {
		t.Fatal("missing owner looked like an empty conversation")
	}
}

func TestDialogueDispatchIsVersionedAndNeverReplayedByReading(t *testing.T) {
	store, _ := Open(t.TempDir())
	conversation, _ := store.CreateChat(t.TempDir(), time.Now())
	turn, err := store.AdmitDialogue(conversation.ID, "client-guidance-0001", "Change the budget", json.RawMessage(`{"target":"live-task"}`), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	stale := turn
	turn.State = "routing"
	turn, err = store.UpdateDialogue(turn, turn.Revision)
	if err != nil {
		t.Fatal(err)
	}
	stale.State = "routing"
	if _, err := store.UpdateDialogue(stale, stale.Revision); !errors.Is(err, ErrDialogueChanged) {
		t.Fatal("stale router replaced a newer message state")
	}
	for _, field := range []string{"text", "options"} {
		changed := turn
		changed.State = "ready"
		changed.Decision = json.RawMessage(`{"kind":"task"}`)
		if field == "text" {
			changed.Text = "Send instead"
		} else {
			changed.Options = json.RawMessage(`{"target":"other-task"}`)
		}
		if _, err := store.UpdateDialogue(changed, changed.Revision); !errors.Is(err, ErrDialogueChanged) {
			t.Fatal("routing changed frozen admission")
		}
	}
	turn.State, turn.Decision = "ready", json.RawMessage(`{"kind":"guide","target":"live-task","instruction":"Change the budget"}`)
	turn, err = store.UpdateDialogue(turn, turn.Revision)
	if err != nil {
		t.Fatal(err)
	}
	turn.State = "dispatching"
	turn, err = store.UpdateDialogue(turn, turn.Revision)
	if err != nil {
		t.Fatal(err)
	}
	page, _, err := store.DialoguePage(conversation.ID, 0, 1)
	if err != nil || page[0].State != "dispatching" || page[0].Revision != turn.Revision {
		t.Fatal("reading pretended an unconfirmed dispatch had completed")
	}
	turn.State, turn.Response = "guided", "Your correction was accepted by that task."
	turn, err = store.UpdateDialogue(turn, turn.Revision)
	if err != nil {
		t.Fatal(err)
	}
	turn.State = "dispatching"
	if _, err := store.UpdateDialogue(turn, turn.Revision); err == nil {
		t.Fatal("completed guidance was revived for replay")
	}
}

func TestConcurrentDialogueRetriesRetainOneMessage(t *testing.T) {
	store, _ := Open(t.TempDir())
	conversation, _ := store.CreateChat(t.TempDir(), time.Now())
	var workers sync.WaitGroup
	results, failures := make(chan DialogueTurn, 8), make(chan error, 8)
	for index := 0; index < 8; index++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			turn, err := store.AdmitDialogue(conversation.ID, "client-concurrent-0001", "One message", json.RawMessage(`{}`), time.Now())
			if err != nil {
				failures <- err
				return
			}
			results <- turn
		}()
	}
	workers.Wait()
	close(results)
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	for result := range results {
		if result.Sequence != 1 || result.Revision != 1 {
			t.Fatal("a retry admitted different work")
		}
	}
	page, more, err := store.DialoguePage(conversation.ID, 0, 10)
	if err != nil || more || len(page) != 1 {
		t.Fatalf("duplicate message admitted: %d %v", len(page), err)
	}
}

func TestDialogueRejectsLinkedStorageAndTamperedMessages(t *testing.T) {
	store, _ := Open(t.TempDir())
	conversation, _ := store.CreateChat(t.TempDir(), time.Now())
	directory := filepath.Join(store.root, conversation.ID, "dialogue")
	if err := os.Symlink(t.TempDir(), directory); err != nil {
		t.Skip(err)
	}
	if _, err := store.AdmitDialogue(conversation.ID, "client-storage-0001", "Hello", json.RawMessage(`{}`), time.Now()); err == nil {
		t.Fatal("accepted linked dialogue directory")
	}
	_ = os.Remove(directory)
	turn, err := store.AdmitDialogue(conversation.ID, "client-storage-0001", "Hello", json.RawMessage(`{}`), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, dialogueFilename(turn))
	turn.ConversationID = "different-owner"
	data, _ := json.Marshal(turn)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.DialoguePage(conversation.ID, 0, 1); err == nil {
		t.Fatal("accepted a message from another owner")
	}
}
