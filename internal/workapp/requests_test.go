package workapp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/worksession"
)

func requestConfig(t *testing.T, run func(string, string, string, RunOptions) RunResult) (Config, worksession.Store) {
	t.Helper()
	store, err := worksession.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	config := Config{CurrentFolder: t.TempDir(), SessionStore: &store, Run: run}
	config.CreateConversation = func(source string) (worksession.Conversation, error) { return store.CreateChat(source, time.Now()) }
	config.LoadConversation = func(id string) (ConversationState, error) {
		c, err := store.Load(id)
		return ConversationState{Title: c.Title, SourcePath: c.SourcePath}, err
	}
	return config, store
}

func TestRetainedQueueRestoresOptionsAndNeverAutomaticallyReplaysInterruptedWork(t *testing.T) {
	var prompts []string
	var captured []RunOptions
	config, store := requestConfig(t, func(_, _, text string, options RunOptions) RunResult {
		prompts = append(prompts, text)
		captured = append(captured, options)
		return RunResult{FinalText: "done", WorkID: "work-result", RevisionID: "rev-result"}
	})
	m := New(config)
	conversation := m.conversation
	request := BridgeUploadSubmission{RequestID: "first-message", Context: "conversation:" + conversation, Text: "Prepare the report"}
	next, first := m.SubmitBridgeUploads(request)
	m = next.(Model)
	if first == nil || m.bridgeUploadResult.Error != "" {
		t.Fatalf("admission: %v", m.bridgeUploadResult)
	}
	m.options.Uploads = []string{strings.Repeat("a", 32) + "/brief.txt"}
	m.options.ConnectorIDs = []string{"selected-account"}
	m.options.MaxSteps = 12
	m.options.BrowserSession, m.options.BrowserPolicy = "browser-reviewed", "policy-reviewed"
	next, _ = m.SubmitText("Use the brief")
	m = next.(Model)
	next, _ = m.SubmitText("Then summarize")
	m = next.(Model)
	items := m.BridgeState().Queue
	next, _ = m.ApplyBridgeQueue(BridgeQueueAction{RequestID: "move", ID: items[1].ID, Action: "move", TargetID: items[0].ID, Placement: "before"})
	m = next.(Model)
	if m.bridgeQueueResult.Error != "" {
		t.Fatal(m.bridgeQueueResult.Error)
	}
	// Execution returned, but its completion event was lost with the UI process.
	// This deliberately exercises the uncertain window, rather than pretending
	// retained conversation text proves an external action was never attempted.
	_ = first()
	config.StartConversationID = conversation
	restored := New(config)
	state := restored.BridgeState()
	if len(state.Queue) != 3 || !state.Queue[0].Interrupted || state.Queue[1].Text != "Then summarize" || !state.RequestsRecovered || state.Running || state.RequestsIssue != "" {
		t.Fatalf("recovery: %+v %s", state.Queue, state.RequestsIssue)
	}
	if len(prompts) != 1 {
		t.Fatal("opening the chat replayed work")
	}
	if state.Queue[2].ID != items[0].ID {
		t.Fatal("retained queue identity changed")
	}
	next, command := restored.ApplyBridgeQueue(BridgeQueueAction{RequestID: "send", ID: state.Queue[2].ID, Action: "run"})
	restored = next.(Model)
	if command == nil || restored.bridgeQueueResult.Error != "" {
		t.Fatal("could not explicitly send retained message")
	}
	next, auto := restored.Update(command())
	restored = next.(Model)
	if auto != nil || len(prompts) != 2 || prompts[1] != "Use the brief" {
		t.Fatal("recovered queue ran without an explicit request", prompts)
	}
	if captured[1].BrowserSession != "browser-reviewed" || captured[1].BrowserPolicy != "policy-reviewed" || captured[1].MaxSteps != 12 || len(captured[1].Uploads) != 1 || captured[1].ConnectorIDs[0] != "selected-account" {
		t.Fatalf("captured options lost: %+v", captured[1])
	}
	j, err := store.LoadRequests(conversation)
	if err != nil {
		t.Fatal(err)
	}
	var completed bool
	for _, r := range j.Requests {
		if r.Text == "Use the brief" {
			completed = r.State == "completed" && r.WorkID == "work-result" && r.RevisionID == "rev-result"
		}
	}
	if !completed {
		t.Fatal("completion receipt missing")
	}
	interruptedID := restored.queue[0].requestID
	next, retry := restored.ApplyBridgeQueue(BridgeQueueAction{RequestID: "retry", ID: restored.BridgeState().Queue[0].ID, Action: "run"})
	restored = next.(Model)
	if retry == nil {
		t.Fatal("explicit retry did not start")
	}
	next, auto = restored.Update(retry())
	restored = next.(Model)
	if auto != nil || len(prompts) != 3 {
		t.Fatal("retry ran another recovered message")
	}
	j, err = store.LoadRequests(conversation)
	if err != nil {
		t.Fatal(err)
	}
	oldPreserved, newAttempt := false, false
	for _, r := range j.Requests {
		if r.ID == interruptedID {
			oldPreserved = r.State == "retried" && r.Issue != ""
		}
		if r.RetryOf == interruptedID {
			newAttempt = r.ID != interruptedID && r.State == "completed"
		}
	}
	if !oldPreserved || !newAttempt {
		t.Fatal("retry overwrote uncertain execution evidence")
	}
	for len(restored.queue) > 0 {
		next, _ = restored.ApplyBridgeQueue(BridgeQueueAction{RequestID: "discard", ID: restored.BridgeState().Queue[0].ID, Action: "remove"})
		restored = next.(Model)
		if restored.bridgeQueueResult.Error != "" {
			t.Fatal(restored.bridgeQueueResult.Error)
		}
	}
	if len(New(config).queue) != 0 {
		t.Fatal("discarded messages returned after reopening")
	}
}

func TestMessageAdmissionDeduplicatesRetriesAndProtectsLiveOwner(t *testing.T) {
	config, _ := requestConfig(t, func(_, _, _ string, _ RunOptions) RunResult { return RunResult{FinalText: "done"} })
	m := New(config)
	request := BridgeUploadSubmission{RequestID: "one", Context: "conversation:" + m.conversation, Text: "Only once"}
	next, command := m.SubmitBridgeUploads(request)
	m = next.(Model)
	defer func() {
		if command != nil {
			command()
		}
	}()
	next, duplicate := m.SubmitBridgeUploads(request)
	m = next.(Model)
	if duplicate != nil || len(m.queue) != 0 || len(m.requests.Requests) != 1 || m.bridgeUploadResult.Error != "" {
		t.Fatal("transport retry became new work")
	}
	request.Text = "Different payload"
	next, duplicate = m.SubmitBridgeUploads(request)
	if duplicate != nil || next.(Model).bridgeUploadResult.Error == "" {
		t.Fatal("accepted identity reuse for another payload")
	}
	for i := range 16 {
		request.RequestID, request.Text = fmt.Sprintf("queued-%d", i), fmt.Sprintf("Follow-up %d", i)
		next, _ = m.SubmitBridgeUploads(request)
		m = next.(Model)
		if m.bridgeUploadResult.Error != "" {
			t.Fatal(m.bridgeUploadResult.Error)
		}
	}
	next, duplicate = m.SubmitBridgeUploads(request)
	m = next.(Model)
	if duplicate != nil || len(m.queue) != 16 || m.bridgeUploadResult.Error != "" {
		t.Fatal("a full queue rejected acknowledgement of an already admitted request")
	}
	config.StartConversationID = m.conversation
	other := New(config)
	if !strings.Contains(other.requestsIssue, "another session") || len(other.queue) != 0 {
		t.Fatal("live work was treated as interrupted")
	}
	next, duplicate = other.SubmitText("Do not duplicate")
	if duplicate != nil || next.(Model).running {
		t.Fatal("second session started another task")
	}
}

func TestFailedAdmissionPreservesQueueAndAcknowledgesFailure(t *testing.T) {
	config, store := requestConfig(t, func(_, _, _ string, _ RunOptions) RunResult { t.Fatal("unadmitted work executed"); return RunResult{} })
	m := New(config)
	// A valid journal advanced by another session makes this UI stale.
	j, err := store.LoadRequests(m.conversation)
	if err != nil {
		t.Fatal(err)
	}
	r, err := worksession.NewRequest("Another accepted message", json.RawMessage(`{}`), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	j.Requests = append(j.Requests, r)
	if _, err := store.SaveRequests(m.conversation, j); err != nil {
		t.Fatal(err)
	}
	request := BridgeUploadSubmission{RequestID: "stale", Context: "conversation:" + m.conversation, Text: "Keep my draft"}
	next, command := m.SubmitBridgeUploads(request)
	m = next.(Model)
	if command != nil || m.running || m.bridgeUploadResult.Error == "" || len(m.messages) != 0 {
		t.Fatal("failed admission was acknowledged as sent")
	}
	retained, err := store.LoadRequests(m.conversation)
	if err != nil || len(retained.Requests) != 1 || retained.Requests[0].ID != r.ID {
		t.Fatal("stale admission overwrote accepted work")
	}
}

func TestUnreadableRequestsNeverStartWithAnEmptyQueue(t *testing.T) {
	root := t.TempDir()
	store, err := worksession.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	c, err := store.CreateChat(t.TempDir(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "gator", "conversations", c.ID, "requests.json"), []byte("damaged"), 0600); err != nil {
		t.Fatal(err)
	}
	m := New(Config{CurrentFolder: c.SourcePath, StartConversationID: c.ID, SessionStore: &store, LoadConversation: func(string) (ConversationState, error) { return ConversationState{SourcePath: c.SourcePath}, nil }, Run: func(_, _, _ string, _ RunOptions) RunResult {
		t.Fatal("work executed without reading admission evidence")
		return RunResult{}
	}})
	next, command := m.SubmitText("Don't lose the earlier requests")
	if command != nil || next.(Model).BridgeState().RequestsIssue == "" {
		t.Fatal("ignored corrupt admission state")
	}
}
