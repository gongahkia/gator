package workapp

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestAnswerContextNavigationRejectsForeignAndMalformedRuns(t *testing.T) {
	reads := 0
	m := New(Config{ReadContext: func(id, goal string) (ContextState, error) {
		reads++
		return ContextState{WorkID: id}, nil
	}})
	m.conversation = "chat-one"
	m.messages = []message{{runID: "run-owned"}, {bundle: &BundleSummary{RunID: "run-bundle"}}}
	for _, command := range []string{"/context run-other", "/context ../private", "/context run-owned extra", "/context run-owned;send"} {
		next, cmd := m.NavigateBridge(command)
		if cmd != nil || reads != 0 || next.(Model).section != "" || next.(Model).bridgeNotice == "" {
			t.Fatal("unowned context navigation was accepted", command)
		}
	}
	for _, id := range []string{"run-owned", "run-bundle"} {
		next, cmd := m.NavigateBridge("/context " + id)
		if cmd != nil || next.(Model).contextWorkID != id || next.(Model).section != "context" {
			t.Fatal("owned answer context did not open", id, next.(Model).BridgeState())
		}
	}
	if reads != 2 {
		t.Fatal("unexpected context reads", reads)
	}
}

func TestAnswerContextReadFailureExplainsMissingEvidenceWithoutSubstitution(t *testing.T) {
	m := New(Config{ReadContext: func(string, string) (ContextState, error) {
		return ContextState{}, errors.New("retained knowledge response changed")
	}})
	m.conversation = "chat-one"
	m.messages = []message{{runID: "run-owned"}}
	next, _ := m.NavigateBridge("/context run-owned")
	m = next.(Model)
	if !strings.Contains(m.bridgeNotice, "retained knowledge response changed") || m.section == "context" || len(m.contextState.Items) != 0 {
		t.Fatal("failed historical read substituted saved context or hid its cause")
	}
}

func TestBusyAnswerContextDelegatesViewAndRevalidatesOriginalAncestry(t *testing.T) {
	m := New(Config{ManagedSessions: true})
	m.conversation, m.running, m.input = "original-chat", true, "Keep original input"
	m.messages = []message{{runID: "run-owned"}}
	next, _ := m.NavigateBridge("/context run-owned")
	busy := next.(Model)
	if !busy.running || busy.conversation != m.conversation || busy.input != m.input || busy.sessionRequest == nil || busy.sessionRequest.View != "/context run-owned" || busy.sessionRequest.OriginConversationID != m.conversation {
		t.Fatal("context inspection replaced execution or its origin")
	}
	available := true
	view := New(Config{StartView: true, ManagedSessions: true, ViewOriginConversationID: "original-chat", LoadConversation: func(id string) (ConversationState, error) {
		if id != "original-chat" || !available {
			return ConversationState{}, errors.New("original conversation unavailable")
		}
		return ConversationState{Messages: []TranscriptMessage{{RunID: "run-owned"}}}, nil
	}, ReadContext: func(id, goal string) (ContextState, error) {
		return ContextState{WorkID: id, Items: []KnowledgeItem{{ID: "mem-one", Kind: "memory", Available: true, Text: "Supplied before correction", CurrentText: "Current saved text"}}}, nil
	}})
	next, _ = view.NavigateBridge(busy.sessionRequest.View)
	view = next.(Model)
	if view.contextWorkID != "run-owned" || view.conversation != "" || view.contextState.Items[0].Text != "Supplied before correction" {
		t.Fatal("management view lost the original excerpt or created a chat")
	}
	view.contextDetail = &view.contextState.Items[0]
	next, _ = view.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	view = next.(Model)
	if len(view.options.ExcludedKnowledgeIDs) != 0 || !strings.Contains(view.sectionNotice, "original conversation") {
		t.Fatal("view claimed to alter the executing owner's next-message scope")
	}
	for _, hint := range view.contextPanel().Hints {
		if hint.Shortcut == "Ctrl+O" {
			t.Fatal("unowned next-message setting was offered")
		}
	}
	available = false
	next, _ = view.NavigateBridge("/context run-owned")
	if next.(Model).bridgeNotice == "" {
		t.Fatal("a vanished origin reused stale inspection authority")
	}
}

func TestAnswerContextOwnsItsCtrlActionsBeforeGlobalNavigation(t *testing.T) {
	m := New(Config{})
	m.section = "context"
	m.contextState.Items = []KnowledgeItem{{ID: "mem-one", Kind: "memory", Available: true, Text: "Earlier", CurrentText: "Current"}}
	m, _ = productKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = productKey(m, tea.KeyMsg{Type: tea.KeyCtrlO})
	if m.productHelp || !sliceContains(m.options.ExcludedKnowledgeIDs, "mem-one") {
		t.Fatal("omit opened global help or lost its original owner")
	}
	m, _ = productKey(m, tea.KeyMsg{Type: tea.KeyCtrlX})
	if !m.contextConfirm || m.launcher {
		t.Fatal("forget review opened the global conversation picker")
	}
}
