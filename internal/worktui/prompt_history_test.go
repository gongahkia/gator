package worktui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestComposerHistoryRecallsPromptsAndRestoresDraft(t *testing.T) {
	model := New(Config{CurrentFolder: "/work", Run: func(_, _, prompt string, _ RunOptions) RunResult {
		return RunResult{FinalText: "finished " + prompt}
	}})
	model.home = false
	for _, prompt := range []string{"first prompt", "second prompt"} {
		model.input = prompt
		updated, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if command == nil {
			t.Fatal("prompt did not start a run")
		}
		updated, _ = updated.(Model).Update(command())
		model = updated.(Model)
	}

	model.input = "unfinished draft"
	for _, step := range []struct {
		key  tea.KeyType
		want string
	}{
		{tea.KeyUp, "second prompt"},
		{tea.KeyUp, "first prompt"},
		{tea.KeyDown, "second prompt"},
		{tea.KeyDown, "unfinished draft"},
	} {
		updated, _ := model.Update(tea.KeyMsg{Type: step.key})
		model = updated.(Model)
		if model.input != step.want {
			t.Fatalf("history navigation = %q, want %q", model.input, step.want)
		}
	}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyUp})
	model = updated.(Model)
	if model.input != "second prompt" {
		t.Fatalf("up did not recall latest prompt: %q", model.input)
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("!")})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(Model)
	if model.input != "second prompt!" {
		t.Fatalf("editing recalled prompt did not leave history: %q", model.input)
	}
}

func TestRetainedConversationPromptsPopulateHistory(t *testing.T) {
	model := New(Config{
		StartConversationID: "work-one",
		LoadConversation: func(string) (ConversationState, error) {
			return ConversationState{Messages: []TranscriptMessage{
				{Role: "You", Text: "retained first"},
				{Role: "Gator", Text: "answer"},
				{Role: "You", Text: "retained second"},
			}}, nil
		},
	})
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyUp})
	model = updated.(Model)
	if model.input != "retained second" {
		t.Fatalf("latest retained prompt = %q", model.input)
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyUp})
	if updated.(Model).input != "retained first" {
		t.Fatalf("earlier retained prompt = %q", updated.(Model).input)
	}
}

func TestQueuedPromptIsAvailableInHistory(t *testing.T) {
	model := New(Config{CurrentFolder: "/work"})
	model.home = false
	model.running = true
	model.input = "queued prompt"
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	model.input = ""
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyUp})
	if updated.(Model).input != "queued prompt" {
		t.Fatalf("queued prompt was not recalled: %q", updated.(Model).input)
	}
}
