package workapp

import (
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/workactivity"
)

func TestLiveActivityMovesToResponseAndRestores(t *testing.T) {
	m := New(Config{CurrentFolder: t.TempDir()})
	m.running = true
	m.activity = workactivity.New(time.Now().Add(-time.Second))
	next, _ := m.updateAgentEvent(agent.Event{Kind: agent.EventText, Text: "Checking notes", At: time.Now()})
	m = next.(Model)
	live := m.BridgeState()
	if live.Activity == nil || len(live.Activity.Entries) != 1 {
		t.Fatal("missing live activity")
	}
	next, _ = m.updateRunDone(runDone{FinalText: "The answer"})
	m = next.(Model)
	state := m.BridgeState()
	if state.Activity != nil || state.Messages[0].Activity == nil || state.Messages[0].Activity.State != "completed" {
		t.Fatal("completed activity was lost")
	}
	m.applyConversationState("reopened", ConversationState{Messages: []TranscriptMessage{{Role: "Gator", Text: "The answer", Activity: state.Messages[0].Activity}}})
	restored := m.BridgeState().Messages[0].Activity
	if restored.Entries[0].Text != "Checking notes" || restored.FinishedAt == nil {
		t.Fatal("restored activity was lost")
	}
}
