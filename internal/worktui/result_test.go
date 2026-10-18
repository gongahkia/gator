package worktui

import (
	"encoding/json"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/workrun"
)

func TestCommandResultsCarryTypedStatusAndError(t *testing.T) {
	model := New(Config{CurrentFolder: t.TempDir()})
	next, _ := model.SubmitText("/status")
	state := next.(Model).BridgeState()
	if len(state.Messages) != 1 || state.Messages[0].Result == nil || state.Messages[0].Result.Kind != "details" {
		t.Fatalf("status result = %#v", state.Messages)
	}
	if !strings.Contains(state.Messages[0].Text, "Gator orchestration") || len(state.Messages[0].Result.Fields) < 12 {
		t.Fatalf("status lost original text or fields: %#v", state.Messages[0])
	}
	next, _ = next.(Model).SubmitText("/mode impossible")
	state = next.(Model).BridgeState()
	last := state.Messages[len(state.Messages)-1]
	if last.Result == nil || last.Result.Kind != "notice" || last.Result.Tone != "error" || !strings.Contains(last.Result.Summary, "usage:") {
		t.Fatalf("error result = %#v", last)
	}
}

func TestEffortOpensPickerAndAppliesSelection(t *testing.T) {
	model := New(Config{CurrentFolder: t.TempDir()})
	next, _ := model.SubmitText("/effort")
	model = next.(Model)
	state := model.BridgeState()
	if state.Mode != "panel" || state.Panel == nil || state.Panel.Title != "Choose effort" || len(state.Panel.Rows) != 3 || state.Panel.Selected != 1 {
		t.Fatalf("effort picker = %#v", state.Panel)
	}
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = next.(Model)
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(Model)
	if model.options.MaxSteps != 48 || model.options.Code.MaxSteps != 32 || model.BridgeState().Mode != "chat" {
		t.Fatalf("effort selection = %#v", model.options)
	}
	last := model.BridgeState().Messages[0]
	if last.Result == nil || last.Result.Tone != "success" {
		t.Fatalf("effort notice = %#v", last)
	}
}

func TestApprovalPreviewIsStructuredAndRetainsExactData(t *testing.T) {
	preview := map[string]any{"command": "send", "recipient": "sam@example.test", "metadata": map[string]any{"draft": "Hello, Sam"}}
	model := New(Config{CurrentFolder: t.TempDir()})
	next, _ := model.Update(workrun.Interaction{ID: 7, Kind: "send_email", Preview: preview})
	state := next.(Model).BridgeState()
	if len(state.Messages) != 1 || state.Messages[0].Result == nil || state.Messages[0].Result.Kind != "approval" {
		t.Fatalf("approval result = %#v", state.Messages)
	}
	var got map[string]any
	if err := json.Unmarshal(state.Messages[0].Result.Preview, &got); err != nil {
		t.Fatal(err)
	}
	if got["recipient"] != preview["recipient"] || got["metadata"].(map[string]any)["draft"] != "Hello, Sam" {
		t.Fatalf("approval preview changed: %#v", got)
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"kind":"approval"`) || !strings.Contains(string(encoded), `"recipient":"sam@example.test"`) {
		t.Fatalf("bridge lost structured approval: %s", encoded)
	}
}
