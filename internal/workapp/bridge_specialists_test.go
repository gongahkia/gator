package workapp

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/workrun"
	"github.com/gongahkia/gator/internal/worksession"
)

func specialistModel(t *testing.T) Model {
	t.Helper()
	m := New(Config{CurrentFolder: t.TempDir()})
	m.running, m.conversation = true, "chat-one"
	m.operation = &workrun.Operation{RunID: "run-one"}
	m.recordBridgeSpecialist(agent.Event{TaskID: "run-one/subagent-001", Text: `{"id":"subagent-001","parent_run":"run-one","role":"timeline_scout","description":"Check dated claims","assignment":"Find the deadline","status":"running","steerable":true,"configuration":{"secret":"PRIVATE_CONFIG"},"source_capture":"PRIVATE_PATH","profile":{"instructions":"PRIVATE_PROMPT"},"native":{"secret":"PRIVATE_SESSION"}}`})
	return m
}

func TestBridgeSpecialistsProjectOnlyCurrentPublicFieldsAndCapabilities(t *testing.T) {
	m := specialistModel(t)
	cards := m.BridgeState().Specialists
	if len(cards) != 1 || !cards[0].CanStop || !cards[0].CanGuide || len(cards[0].Token) != 64 || cards[0].Assignment != "Find the deadline" {
		t.Fatalf("live card = %+v", cards)
	}
	data, err := json.Marshal(cards)
	if err != nil || strings.Contains(string(data), "PRIVATE") || strings.Contains(string(data), "parentRun") || strings.Contains(string(data), "configuration") {
		t.Fatalf("private state leaked: %s %v", data, err)
	}
	// A previous run's event cannot give the next run a card/control identity.
	m.recordBridgeSpecialist(agent.Event{TaskID: "run-old/subagent-002", Text: `{"id":"subagent-002","parent_run":"run-old","role":"old","status":"running"}`})
	m.recordBridgeSpecialist(agent.Event{TaskID: "forged/subagent-003", Text: `{"id":"subagent-003","parent_run":"run-one","role":"forged","status":"running"}`})
	if len(m.bridgeSpecialists()) != 1 {
		t.Fatal("stale or mismatched event produced a current card")
	}
	m.activeRequest = "plan"
	m.requests.Requests = []worksession.Request{{ID: "plan", Plan: &worksession.TaskPlan{}}}
	if card := m.bridgeSpecialists()[0]; card.CanGuide || !card.CanStop {
		t.Fatal("reviewed plan should permit stop, but not new guidance")
	}
	m.cancelling = true
	if card := m.bridgeSpecialists()[0]; card.CanGuide || card.CanStop || card.Token != "" {
		t.Fatal("cancelling parent retained controls")
	}
	m.running = false
	if len(m.bridgeSpecialists()) != 0 {
		t.Fatal("idle parent retained live cards")
	}
	if text := cardText(strings.Repeat("日", 1000), 2048); !utf8.ValidString(text) || len(text) > 2051 {
		t.Fatal("card truncation split UTF-8 or exceeded bound")
	}
}

func TestBridgeSpecialistActionsRejectStaleOwnersAndReplayOnlyExactRequests(t *testing.T) {
	m := specialistModel(t)
	action := BridgeSpecialistAction{ID: "subagent-001", Token: m.bridgeSpecialists()[0].Token, RequestID: "guide-one", Action: "guide", Text: "Use primary evidence"}
	for _, change := range []func(*Model, *BridgeSpecialistAction){
		func(m *Model, _ *BridgeSpecialistAction) { m.conversation = "chat-other" },
		func(m *Model, _ *BridgeSpecialistAction) { m.loadingRun++ },
		func(m *Model, _ *BridgeSpecialistAction) { m.operation = &workrun.Operation{RunID: "other-run"} },
		func(m *Model, _ *BridgeSpecialistAction) { m.running = false },
		func(m *Model, _ *BridgeSpecialistAction) { m.cancelling = true },
		func(m *Model, _ *BridgeSpecialistAction) { m.section = "settings" },
		func(_ *Model, a *BridgeSpecialistAction) { a.ID = "subagent-002" },
		func(_ *Model, a *BridgeSpecialistAction) { a.Text = strings.Repeat("x", 8193) },
		func(_ *Model, a *BridgeSpecialistAction) { a.Text = "\x00" },
		func(_ *Model, a *BridgeSpecialistAction) { a.Action = "stop" },
	} {
		copy, changed := m, action
		change(&copy, &changed)
		next, _ := copy.ApplyBridgeSpecialist(changed)
		if next.(Model).bridgeSpecialistResult.Error == "" {
			t.Fatal("invalid or stale control succeeded")
		}
	}
	// A retained acknowledgement must not dispatch duplicate guidance. This
	// operation has no supervisor, so redispatch would fail instead of succeed.
	m.specialistActions = map[string]specialistActionReceipt{action.RequestID: {action, BridgeSpecialistResult{RequestID: action.RequestID}}}
	next, _ := m.ApplyBridgeSpecialist(action)
	if next.(Model).bridgeSpecialistResult.Error != "" {
		t.Fatal("exact replay redispatched the guidance")
	}
	action.Text = "Different guidance"
	next, _ = m.ApplyBridgeSpecialist(action)
	if next.(Model).bridgeSpecialistResult.Error == "" {
		t.Fatal("nonce reuse accepted a different action")
	}
	m.activeRequest = "plan"
	m.requests.Requests = []worksession.Request{{ID: "plan", Plan: &worksession.TaskPlan{}}}
	action.RequestID = "new-guidance"
	next, _ = m.ApplyBridgeSpecialist(action)
	if !strings.Contains(next.(Model).bridgeSpecialistResult.Error, "review a new plan") {
		t.Fatal("guidance bypassed reviewed-plan restriction")
	}
}

func TestStoppedSpecialistDoesNotShowFailureWarning(t *testing.T) {
	m := specialistModel(t)
	before := len(m.messages)
	next, _ := m.updateAgentEvent(agent.Event{Kind: agent.EventSubagent, TaskID: "run-one/subagent-001", Text: `{"id":"subagent-001","parent_run":"run-one","role":"timeline_scout","status":"cancelled","error":"context canceled"}`})
	m = next.(Model)
	if len(m.messages) != before || m.bridgeSpecialists()[0].Status != "cancelled" {
		t.Fatal("stopping a specialist was presented as an unexpected failure")
	}
	next, _ = m.updateAgentEvent(agent.Event{Kind: agent.EventSubagent, TaskID: "run-one/subagent-001", Text: `{"id":"subagent-001","parent_run":"run-one","role":"timeline_scout","status":"failed","error":"model request failed"}`})
	if len(next.(Model).messages) != before+1 {
		t.Fatal("unexpected worker failure lost its warning")
	}
}
