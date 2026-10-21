package worktui

import (
	"reflect"
	"strings"
	"testing"
)

func TestBridgeQueueEditsReordersAndRunsCapturedOptions(t *testing.T) {
	var prompts []string
	var options []RunOptions
	m := New(Config{CurrentFolder: t.TempDir(), Run: func(_, _, prompt string, captured RunOptions) RunResult {
		prompts = append(prompts, prompt)
		options = append(options, captured)
		return RunResult{FinalText: "done"}
	}})
	next, _ := m.SubmitText("first")
	m = next.(Model)
	m.options.Attachments = []string{"original.md"}
	next, _ = m.SubmitText("duplicate")
	m = next.(Model)
	next, _ = m.SubmitText("duplicate")
	m = next.(Model)
	queue := m.BridgeState().Queue
	if len(queue) != 2 || queue[0].ID == queue[1].ID || queue[0].Text != "duplicate" {
		t.Fatalf("queue projection: %#v", queue)
	}
	apply := func(id, action, text string) {
		t.Helper()
		next, _ := m.ApplyBridgeQueue(BridgeQueueAction{RequestID: action, ID: id, Action: action, Text: text})
		m = next.(Model)
		if result := m.BridgeState().QueueResult; result.RequestID != action || result.Error != "" {
			t.Fatalf("%s: %#v", action, result)
		}
	}
	apply(queue[1].ID, "earlier", "")
	apply(queue[0].ID, "edit", "edited")
	changed := m.BridgeState().Queue
	if changed[0].ID != queue[1].ID || changed[1].ID == queue[0].ID {
		t.Fatalf("identities after reorder/edit: %#v", changed)
	}
	next, _ = m.ApplyBridgeQueue(BridgeQueueAction{RequestID: "stale", ID: queue[0].ID, Action: "remove"})
	m = next.(Model)
	if m.BridgeState().QueueResult.Error == "" || len(m.queue) != 2 {
		t.Fatal("old edit identity changed the new revision")
	}
	apply(changed[0].ID, "remove", "")
	// A failed run pauses the existing queue. Sending a selected item resumes it
	// with the original captured options, even after composer settings change.
	next, _ = m.updateRunDone(runDone{Error: "failed"})
	m = next.(Model)
	m.options.Attachments = []string{"new.md"}
	next, run := m.ApplyBridgeQueue(BridgeQueueAction{RequestID: "run", ID: changed[1].ID, Action: "run"})
	m = next.(Model)
	if run == nil || !m.running || len(m.queue) != 0 {
		t.Fatal("paused message did not start")
	}
	next, _ = m.Update(run())
	m = next.(Model)
	if m.running || !reflect.DeepEqual(prompts, []string{"edited"}) || !reflect.DeepEqual(options[0].Attachments, []string{"original.md"}) {
		t.Fatalf("prompts=%v options=%#v", prompts, options)
	}
	next, _ = m.ApplyBridgeQueue(BridgeQueueAction{RequestID: "late", ID: changed[1].ID, Action: "edit", Text: "too late"})
	if next.(Model).BridgeState().QueueResult.Error == "" {
		t.Fatal("editing a consumed message succeeded")
	}
}

func TestBridgeQueueRejectsInvalidEditsAndLateGuidance(t *testing.T) {
	m := New(Config{CurrentFolder: t.TempDir()})
	m.running = true
	next, _ := m.SubmitText("kept")
	m = next.(Model)
	id := m.BridgeState().Queue[0].ID
	for _, action := range []BridgeQueueAction{
		{Action: "edit", Text: " "}, {Action: "edit", Text: strings.Repeat("x", 128*1024+1)},
		{Action: "run"}, {Action: "execute"}, {Action: "guide", Text: "late guidance"},
	} {
		action.RequestID, action.ID = "invalid", id
		next, cmd := m.ApplyBridgeQueue(action)
		m = next.(Model)
		if cmd != nil || m.BridgeState().QueueResult.Error == "" || m.queue[0].prompt != "kept" {
			t.Fatalf("invalid action mutated queue: %#v", action)
		}
	}
}
