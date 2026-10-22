package workapp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/uisnapshot"
)

func TestControlEditsRequireReviewAndSecretsStayOutOfBridge(t *testing.T) {
	mutations := 0
	m := New(Config{Controls: func(_ context.Context, _, action string, args []string) (ControlScreen, error) {
		if action == "save" {
			mutations++
			if len(args) != 1 || args[0] != "secret-token" {
				t.Fatal("wrong secret payload")
			}
		}
		return ControlScreen{Panel: uisnapshot.Panel{Kind: "recovery", Title: "Reconnect"}, Actions: []ControlAction{{Key: "T", Label: "Replace token", Action: "save", Fields: []ControlField{{Label: "Token", Secret: true}}, Review: true}}}, nil
	}})
	cmd := m.openControls("recovery", nil)
	next, _ := m.Update(cmd())
	m = next.(Model)
	m, _ = productKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	m, _ = productKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("secret-token")})
	encoded, _ := json.Marshal(m.BridgeState())
	if strings.Contains(string(encoded), "secret-token") {
		t.Fatal("secret leaked into display state")
	}
	m, _ = productKey(m, tea.KeyMsg{Type: tea.KeyCtrlR})
	encoded, _ = json.Marshal(m.BridgeState())
	if strings.Contains(string(encoded), "secret-token") || mutations != 0 {
		t.Fatal("review leaked or saved the credential")
	}
	m, cmd = productKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if mutations != 0 || cmd == nil {
		t.Fatal("save was synchronous or missing")
	}
	next, _ = m.Update(cmd())
	m = next.(Model)
	if mutations != 1 {
		t.Fatal("confirmation did not save")
	}
}
func TestRemovedDataClearsVisibleHistoryAndGoalOnlyWhenDeleted(t *testing.T) {
	cleared := 0
	m := New(Config{ClearGoal: func() { cleared++ }, ResetPrivateState: func() (GoalState, error) { return GoalState{SourcePath: "/project", Title: "Work in project"}, nil }})
	m.goalID = "goal-current"
	m.conversation = "conv-old"
	m.messages = []message{{role: "Gator", text: "private excerpt"}}
	m.contextState.Items = []KnowledgeItem{{Text: "retained preference"}}
	m.clearRemovedData(ControlScreen{ResetConversation: true})
	if len(m.messages) != 0 || len(m.contextState.Items) != 0 || m.conversation != "" || m.goalID != "goal-current" || cleared != 0 {
		t.Fatal("history cleanup lost the goal or retained old inputs")
	}
	m.clearRemovedData(ControlScreen{ResetConversation: true, ClearGoal: true})
	if m.goalID != "" || m.source != "/project" || cleared != 1 {
		t.Fatal("all-content deletion retained the deleted goal binding")
	}
}

func TestConnectionsRefreshKeepsDetailAndRoutesWithoutTerminal(t *testing.T) {
	pending := "google"
	m := New(Config{ManagedSessions: true, PendingConnection: func() string { return pending }, Controls: func(_ context.Context, family, action string, args []string) (ControlScreen, error) {
		if family != "connections" {
			t.Fatalf("unexpected family %s", family)
		}
		id := ""
		if len(args) > 0 {
			id = args[0]
		}
		return ControlScreen{Location: id, Panel: uisnapshot.Panel{Kind: "connections", Title: "Google", Notice: "Credentials saved"}}, nil
	}})
	next, cmd := m.NavigateBridge("/connections")
	m = next.(Model)
	next, poll := m.Update(cmd())
	m = next.(Model)
	if poll == nil || m.BridgeState().PendingConnection != "google" || m.BridgeState().Panel.Notice != "Credentials saved" {
		t.Fatal("pending sign-in or notice missing")
	}
	m.controlRootArgs = []string{"google"}
	next, _ = m.Update(m.refreshControlBackground()())
	m = next.(Model)
	if len(m.controlRootArgs) != 1 || m.controlRootArgs[0] != "google" {
		t.Fatal("refresh left connection detail")
	}
	next, _ = m.NavigateBridge("/new")
	m = next.(Model)
	if m.sessionRequest == nil || !m.sessionRequest.New {
		t.Fatal("sign-in owner was reused for new chat")
	}
	pending = ""
	next, _ = m.Update(connectionTick{})
	m = next.(Model)
	if m.BridgeState().PendingConnection != "" || m.connectionPolling {
		t.Fatal("completed sign-in retained busy state")
	}
}
