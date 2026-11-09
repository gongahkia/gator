package worksession

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func readyTask(t *testing.T, s Store, conversation string) DialogueTurn {
	t.Helper()
	turn, err := s.AdmitDialogue(conversation, "client-independent-task-0001", "Prepare a separate brief", json.RawMessage(`{"Uploads":["private-file"],"ConnectorIDs":["selected-mail"],"ModelName":"A&B"}`), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	turn.State = "routing"
	turn, err = s.UpdateDialogue(turn, turn.Revision)
	if err != nil {
		t.Fatal(err)
	}
	turn.State, turn.Decision = "ready", json.RawMessage(`{ "kind": "task" }`)
	turn, err = s.UpdateDialogue(turn, turn.Revision)
	if err != nil {
		t.Fatal(err)
	}
	return turn
}

func TestDialogueTaskUsesItsOwnOwnerWithoutChangingParentExecutionOrNavigation(t *testing.T) {
	s, _ := Open(t.TempDir())
	parent, _ := s.CreateChat(t.TempDir(), time.Now())
	if err := s.SetActive(parent.ID); err != nil {
		t.Fatal(err)
	}
	lease, err := s.AcquireExecution(parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	turn := readyTask(t, s, parent.ID)
	bound, child, claimed, err := s.BindDialogueTask(parent.ID, turn.ID, parent.SourcePath, turn.Revision, time.Now())
	if err != nil || !claimed || bound.State != "dispatching" || bound.TaskChat != child.ID || child.ID == parent.ID || child.SourcePath != parent.SourcePath || child.HeadRevision != "" {
		t.Fatalf("binding = %+v %+v %v %v", bound, child, claimed, err)
	}
	childLease, err := s.AcquireExecution(child.ID)
	if err != nil {
		t.Fatal("child cannot own independent execution:", err)
	}
	childLease.Close()
	origin, err := s.DialogueTask(child.ID)
	if err != nil || origin == nil || origin.ParentConversationID != parent.ID || origin.TurnID != turn.ID || origin.OptionsSHA256 != dialogueHash(turn.Options) || origin.TextSHA256 != dialogueHash([]byte(turn.Text)) {
		t.Fatalf("origin = %+v, %v", origin, err)
	}
	for _, id := range []string{parent.ID, child.ID} {
		requests, err := s.LoadRequests(id)
		if err != nil || len(requests.Requests) != 0 {
			t.Fatal("binding executed or admitted work")
		}
	}
	active, err := s.Active()
	if err != nil || active != parent.ID {
		t.Fatal("delegation changed visible navigation")
	}
	reopened, _ := Open(filepath.Dir(filepath.Dir(s.root)))
	retried, same, claimed, err := reopened.BindDialogueTask(parent.ID, turn.ID, parent.SourcePath, turn.Revision, time.Now())
	if err != nil || claimed || retried.Revision != bound.Revision || same.ID != child.ID {
		t.Fatalf("retry claimed a second dispatch: %+v %v %v", retried, claimed, err)
	}
	bound.State, bound.Response = "delegated", "Independent work accepted."
	bound, err = s.UpdateDialogue(bound, bound.Revision)
	if err != nil {
		t.Fatal(err)
	}
	_, same, claimed, err = s.BindDialogueTask(parent.ID, turn.ID, parent.SourcePath, turn.Revision, time.Now())
	if err != nil || claimed || same.ID != child.ID {
		t.Fatal("completed delegation was relaunched")
	}
}

func TestDialogueTaskConcurrentRetriesClaimOnlyOneDispatch(t *testing.T) {
	s, _ := Open(t.TempDir())
	parent, _ := s.CreateChat(t.TempDir(), time.Now())
	turn := readyTask(t, s, parent.ID)
	var group sync.WaitGroup
	var mutex sync.Mutex
	claims := 0
	ids := make(map[string]bool)
	for i := 0; i < 8; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			_, child, claimed, err := s.BindDialogueTask(parent.ID, turn.ID, parent.SourcePath, turn.Revision, time.Now())
			if err != nil {
				t.Error(err)
				return
			}
			mutex.Lock()
			defer mutex.Unlock()
			ids[child.ID] = true
			if claimed {
				claims++
			}
		}()
	}
	group.Wait()
	if claims != 1 || len(ids) != 1 {
		t.Fatalf("dispatch claims=%d owners=%v", claims, ids)
	}
}

func TestDialogueTaskRejectsChangedScopeDecisionAndDeletedOwner(t *testing.T) {
	s, _ := Open(t.TempDir())
	parent, _ := s.CreateChat(t.TempDir(), time.Now())
	turn := readyTask(t, s, parent.ID)
	if _, _, _, err := s.BindDialogueTask(parent.ID, turn.ID, t.TempDir(), turn.Revision, time.Now()); err == nil {
		t.Fatal("task retargeted to a different source")
	}
	changed := turn
	changed.State = "dispatching"
	changed.Decision = json.RawMessage(`{"kind":"guide","target":"different-task"}`)
	if _, err := s.UpdateDialogue(changed, turn.Revision); err == nil {
		t.Fatal("retained routing decision changed")
	}
	bound, child, _, err := s.BindDialogueTask(parent.ID, turn.ID, parent.SourcePath, turn.Revision, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	changed = bound
	changed.State, changed.Response, changed.TaskChat = "delegated", "Accepted", "work-other"
	if _, err := s.UpdateDialogue(changed, bound.Revision); err == nil {
		t.Fatal("task owner changed")
	}
	if err := os.Remove(filepath.Join(s.root, child.ID, "task-origin.json")); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := s.BindDialogueTask(parent.ID, turn.ID, parent.SourcePath, turn.Revision, time.Now()); err == nil {
		t.Fatal("missing origin was treated as a verified task")
	}
	if err := os.RemoveAll(filepath.Join(s.root, child.ID)); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := s.BindDialogueTask(parent.ID, turn.ID, parent.SourcePath, turn.Revision, time.Now()); err == nil {
		t.Fatal("deleted task was recreated")
	}
	if _, err := os.Stat(filepath.Join(s.root, child.ID)); !os.IsNotExist(err) {
		t.Fatal("deleted owner returned")
	}
}

func TestDialogueTaskRejectsForgedOriginAndNeverDelegatesReplies(t *testing.T) {
	s, _ := Open(t.TempDir())
	parent, _ := s.CreateChat(t.TempDir(), time.Now())
	turn := readyTask(t, s, parent.ID)
	_, child, _, err := s.BindDialogueTask(parent.ID, turn.ID, parent.SourcePath, turn.Revision, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	origin, _ := s.DialogueTask(child.ID)
	origin.OptionsSHA256 = dialogueHash([]byte("other inputs"))
	if err := writeJSON(filepath.Join(s.root, child.ID, "task-origin.json"), origin); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DialogueTask(child.ID); err == nil {
		t.Fatal("forged source/input inheritance accepted")
	}
	path := filepath.Join(s.root, child.ID, "task-origin.json")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := writeJSON(outside, origin); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, path); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DialogueTask(child.ID); err == nil {
		t.Fatal("linked task metadata was trusted")
	}
	ordinary, _ := s.CreateChat(t.TempDir(), time.Now())
	if origin, err := s.DialogueTask(ordinary.ID); err != nil || origin != nil {
		t.Fatal("ordinary chat acquired a task origin")
	}
	reply, _ := s.AdmitDialogue(ordinary.ID, "client-conversation-reply-001", "Hello", json.RawMessage(`{}`), time.Now())
	if _, _, _, err := s.BindDialogueTask(ordinary.ID, reply.ID, ordinary.SourcePath, reply.Revision, time.Now()); err == nil {
		t.Fatal("unrouted greeting became executable work")
	}
}
