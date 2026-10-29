package workapp

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/uisnapshot"
)

func TestBrowserGrantRequiresReviewAndIsConsumedByOneTask(t *testing.T) {
	var calls []RunOptions
	m := New(Config{Run: func(_, _, _ string, options RunOptions) RunResult { calls = append(calls, options); return RunResult{} }, Controls: func(_ context.Context, family, action string, args []string) (ControlScreen, error) {
		if family != "browser" {
			t.Fatal(family)
		}
		screen := ControlScreen{Panel: uisnapshot.Panel{Kind: "browser", Title: "Browser"}, Actions: []ControlAction{{Key: "G", Label: "Use for next task", Action: "grant", Review: true}}}
		if action == "grant" {
			screen.BrowserGrant = &BrowserGrant{SessionID: "browser-test", Policy: "reviewed-policy"}
		}
		return screen, nil
	}})
	m.conversation = "work-one"
	next, cmd := m.runLocalCommand("/browser")
	m = next.(Model)
	next, _ = m.Update(cmd())
	m = next.(Model)
	m, cmd = productKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	if cmd != nil || m.options.BrowserSession != "" || !m.controlReview {
		t.Fatal("grant escaped review")
	}
	m, cmd = productKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	next, _ = m.Update(cmd())
	m = next.(Model)
	if m.options.BrowserSession != "browser-test" {
		t.Fatal("reviewed grant missing")
	}
	if m.BridgeState().WorkDetails.BrowserSession != "browser-test" {
		t.Fatal("selection hidden")
	}
	m.input = "Inspect the page"
	next, cmd = m.submitComposerInput()
	m = next.(Model)
	if m.options.BrowserSession != "" || m.options.BrowserPolicy != "" {
		t.Fatal("grant remained selected")
	}
	_ = cmd()
	m.input = "An unrelated follow-up"
	next, cmd = m.submitComposerInput()
	m = next.(Model)
	_ = cmd()
	if len(calls) != 2 || calls[0].BrowserSession != "browser-test" || calls[0].BrowserPolicy != "reviewed-policy" || calls[1].BrowserSession != "" {
		t.Fatalf("wrong per-task grants: %#v", calls)
	}
}

func TestBrowserGrantQueueAndChatBoundary(t *testing.T) {
	m := New(Config{})
	m.conversation = "work-one"
	m.options.BrowserSession, m.options.BrowserPolicy = "browser-test", "policy"
	m.running = true
	m.input = "Inspect browser when ready"
	next, _ := m.submitRunningInput()
	m = next.(Model)
	if len(m.queue) != 1 || m.queue[0].options.BrowserSession != "browser-test" || m.options.BrowserSession != "" {
		t.Fatal("queue did not capture and consume grant")
	}
	m.input = "Then summarize"
	next, _ = m.submitRunningInput()
	m = next.(Model)
	if len(m.queue) != 2 || m.queue[1].options.BrowserSession != "" {
		t.Fatal("grant leaked to another queued task")
	}
	m.controlFamily = "browser"
	next, _ = m.Update(controlDone{Generation: m.controlGeneration, Screen: ControlScreen{BrowserGrant: &BrowserGrant{SessionID: "browser-other", Policy: "policy", ConversationID: "work-other"}}})
	m = next.(Model)
	if m.options.BrowserSession != "" {
		t.Fatal("another chat's delayed review granted access")
	}
}

func TestBrowserTakeoverPanelStaysInRunningChat(t *testing.T) {
	m := New(Config{ManagedSessions: true, Controls: func(context.Context, string, string, []string) (ControlScreen, error) { return ControlScreen{}, nil }})
	m.running = true
	m.conversation = "work-active"
	next, cmd := m.NavigateBridge("/browser")
	m = next.(Model)
	if m.sessionRequest != nil || m.controlFamily != "browser" || cmd == nil {
		t.Fatal("takeover navigated away from active work")
	}
}
