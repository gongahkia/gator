package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/gongahkia/gator/internal/memory"
)

func TestUnknownMemoryCategoryCannotSilentlyBypassConflictOrForget(t *testing.T) {
	for _, forgotten := range []bool{false, true} {
		store, _ := memory.Open(t.TempDir())
		record, err := store.RememberChat(memory.ChatInput{Subject: "preference.response_length", Kind: "preference", Quote: "I prefer detailed replies.", ConversationID: "work-source", RunID: "run-source"})
		if err != nil {
			t.Fatal(err)
		}
		if forgotten {
			if err := store.Forget(record.ID); err != nil {
				t.Fatal(err)
			}
		}
		quote := "I prefer concise replies."
		args, _ := json.Marshal(map[string]any{"subject": "communication.detail_level", "kind": "preference", "quote": quote, "scope": "personal"})
		tool := rememberChat{ChatMemory{Store: store, Automatic: true, UserMessage: quote, ConversationID: "work-next", RunID: "run-next"}}
		if _, err := tool.Execute(context.Background(), args); err == nil {
			t.Fatal("unknown alias silently saved")
		}
		called := false
		tool.Review = func(_ context.Context, value any) (bool, error) {
			called = true
			preview := value.(map[string]any)
			if preview["subject"] != "communication.detail_level" || preview["saved_text"] != quote {
				t.Fatal("unclear review", preview)
			}
			previous := preview["existing_memories"].([]map[string]string)
			if !forgotten && (len(previous) != 1 || previous[0]["id"] != record.ID) {
				t.Fatal("review omitted existing concept", preview)
			}
			if forgotten && len(previous) != 0 {
				t.Fatal("review resurfaced forgotten text", preview)
			}
			return false, nil
		}
		if _, err := tool.Execute(context.Background(), args); err == nil || !called {
			t.Fatal("declined review saved", err, called)
		}
		records, err := store.List("", true)
		want := 1
		if forgotten {
			want = 0
		}
		if err != nil || len(records) != want {
			t.Fatal(records, err)
		}
	}
}

func TestUnknownConceptReviewIsExactAndDoesNotGrantFutureAutomaticAdmission(t *testing.T) {
	store, _ := memory.Open(t.TempDir())
	quote := "I use a split keyboard."
	tool := rememberChat{ChatMemory{Store: store, Automatic: true, UserMessage: quote, ConversationID: "work-first", RunID: "run-first", Review: func(context.Context, any) (bool, error) { return true, nil }}}
	args := json.RawMessage(`{"subject":"equipment.keyboard","kind":"fact","quote":"I use a split keyboard.","scope":"personal"}`)
	if _, err := tool.Execute(context.Background(), args); err != nil {
		t.Fatal(err)
	}
	tool.Review = nil
	tool.RunID = "run-two"
	if _, err := tool.Execute(context.Background(), args); err == nil {
		t.Fatal("earlier review became unrestricted classification authority")
	}
	// A supported category still admits ordinary passive statements.
	tool.UserMessage = "I prefer concise answers."
	args = json.RawMessage(`{"subject":"preference.response_length","kind":"preference","quote":"I prefer concise answers.","scope":"personal"}`)
	if _, err := tool.Execute(context.Background(), args); err != nil {
		t.Fatal("supported passive capture stopped", err)
	}
}
