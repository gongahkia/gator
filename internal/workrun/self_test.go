package workrun

import (
	"context"
	"encoding/json"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
)

func readApplicationState(t *testing.T, prompt string) map[string]any {
	t.Helper()
	_, data, found := strings.Cut(prompt, applicationStateHeader)
	if !found {
		t.Fatal("model was not given Gator's current app state")
	}
	var state map[string]any
	if err := json.NewDecoder(strings.NewReader(data)).Decode(&state); err != nil {
		t.Fatal(err)
	}
	return state
}

func TestApplicationContextIncludesOnlyDisplaySafeRunState(t *testing.T) {
	contract := artifact.DefaultContract("brief.md")
	contract.ExternalActions = action.Propose
	requested := time.Date(2026, 10, 22, 14, 0, 0, 0, time.UTC)
	prompt := applicationContext(Request{
		Provider: "team-gateway/routed-model", Mode: action.Draft, Contract: contract,
		ConnectorIDs: []string{"calendar"}, SnapshotID: "snap-current", GoalID: "goal-current",
		RecallStatus: "Memories: basic matching", ContextRefs: []string{"memory-one", "note-two"}, RequestedAt: requested,
		StateDir: "/private/profile-path-must-not-leak", SourcePath: "/private/source-path-must-not-leak",
		System:           "extra-instructions-must-not-enter-app-state",
		OAuthRefreshPins: map[string]string{"calendar": "refresh-pin-must-not-leak"},
		OAuthAccountPins: map[string]string{"calendar": "account-pin-must-not-leak"},
		WebOrigins:       []string{"https://private-endpoint-must-not-leak.example"},
		Images:           []agent.Image{{Data: []byte("attachment-content-must-not-leak")}},
	})
	state := readApplicationState(t, prompt)
	for key, expected := range map[string]any{
		"platform": runtime.GOOS, "provider_model": "team-gateway/routed-model",
		"effective_mode": "draft", "external_actions": "draft",
		"snapshot_id": "snap-current", "goal_id": "goal-current",
		"saved_recall_status": "Memories: basic matching", "supplied_saved_items": float64(2),
		"requested_at": requested.Format(time.RFC3339),
	} {
		if state[key] != expected {
			t.Errorf("%s = %v, want %v", key, state[key], expected)
		}
	}
	connectors, ok := state["selected_connectors"].([]any)
	if !ok || len(connectors) != 1 || connectors[0] != "calendar" {
		t.Fatalf("selected connectors = %#v", state["selected_connectors"])
	}
	if !strings.Contains(state["source_access"].(string), "read-only snapshot") {
		t.Fatal("source access did not describe the read-only snapshot")
	}
	for _, private := range []string{"profile-path-must-not-leak", "source-path-must-not-leak", "extra-instructions-must-not-enter-app-state", "refresh-pin-must-not-leak", "account-pin-must-not-leak", "private-endpoint-must-not-leak", "attachment-content-must-not-leak"} {
		if strings.Contains(prompt, private) {
			t.Errorf("app help leaked private request data %q", private)
		}
	}
}

func TestApplicationContextDoesNotGuessMissingConfiguration(t *testing.T) {
	state := readApplicationState(t, applicationContext(Request{}))
	if state["provider_model"] != "not reported" {
		t.Fatalf("missing provider/model was guessed: %v", state["provider_model"])
	}
	connectors, ok := state["selected_connectors"].([]any)
	if !ok || len(connectors) != 0 {
		t.Fatalf("missing connector selection = %#v", state["selected_connectors"])
	}
	for _, absent := range []string{"snapshot_id", "goal_id", "saved_recall_status", "requested_at"} {
		if _, present := state[absent]; present {
			t.Errorf("app state invented %s", absent)
		}
	}
}

func TestConversationReceivesCurrentAppKnowledgeAfterModelChange(t *testing.T) {
	const earlierReply = "The model is managed by the host platform."
	model := &scriptedModel{turns: []agent.Turn{
		{Text: earlierReply},
		{Text: "Open Settings at the bottom left, then Models, or type /model here."},
	}}
	executor := Executor{Model: model, StateDir: t.TempDir()}
	request := Request{
		SourcePath: t.TempDir(), Provider: "gateway/old-model",
		Objective: "Where can I change the model?", Mode: action.Inspect, Contract: artifact.InspectionContract(),
	}
	first, err := executor.Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	request.ConversationID = first.ConversationID
	request.Provider = "gator-local/current-model"
	request.Objective = "Which model are you using now, and how do I change it in Gator?"
	if _, err := executor.Execute(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if len(model.requests) != 2 {
		t.Fatalf("model requests = %d", len(model.requests))
	}
	firstState := readApplicationState(t, model.requests[0].System)
	current := model.requests[1]
	currentState := readApplicationState(t, current.System)
	if firstState["provider_model"] != "gateway/old-model" || currentState["provider_model"] != "gator-local/current-model" {
		t.Fatalf("app state was stale: first=%v current=%v", firstState, currentState)
	}
	if currentState["snapshot_id"] != first.SnapshotID || currentState["effective_mode"] != "inspect" || currentState["external_actions"] != "forbid" {
		t.Fatalf("app state did not reflect the actual run: %v", currentState)
	}
	for _, guide := range []string{"desktop personal agent", "Settings (the bottom-left sliders icon) → Models", "send /model in Gator's message box", "Correct earlier assistant replies", "Use gator_app_update", "available tools"} {
		if !strings.Contains(current.System, guide) {
			t.Errorf("conversation omitted app guidance %q", guide)
		}
	}
	if strings.Contains(current.System, "terminal-native") || strings.Contains(current.System, "gateway/old-model") {
		t.Fatal("conversation reused outdated app or model identity")
	}
	if len(current.Messages) != 3 || current.Messages[1].Content != earlierReply {
		t.Fatal("app knowledge replaced conversation history instead of supplementing it")
	}
}
