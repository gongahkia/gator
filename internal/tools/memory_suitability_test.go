package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/gongahkia/gator/internal/memory"
)

func TestPassiveMemorySuitabilityUsesWholeMessage(t *testing.T) {
	for _, tc := range []struct{ name, message, quote, action string }{
		{"preference", "I prefer brief answers.", "I prefer brief answers.", "save"},
		{"fact", "I live in Harbor City.", "I live in Harbor City.", "save"},
		{"credential", "My password is SYNTHETIC_NOT_A_REAL_SECRET_123!", "SYNTHETIC_NOT_A_REAL_SECRET_123!", "deny"},
		{"cropped secret", "My API key is SYNTHETIC_ONLY. I prefer concise answers.", "I prefer concise answers.", "deny"},
		{"third party", "My colleague said: I prefer brief answers.", "I prefer brief answers.", "review"},
		{"quotation", "Translate: \"I live in Harbor City.\"", "I live in Harbor City.", "review"},
		{"temporary", "For this reply only, I prefer brief answers.", "I prefer brief answers.", "review"},
		{"hypothetical", "Imagine I live in Harbor City.", "I live in Harbor City.", "review"},
		{"sensitive", "I have a medical diagnosis.", "I have a medical diagnosis.", "review"},
		{"unfamiliar language", "Me gustan las respuestas breves.", "Me gustan las respuestas breves.", "review"},
		{"Chinese opt out", "不要保存：I prefer brief answers.", "I prefer brief answers.", "deny"},
		{"Spanish opt out", "No guardes esto: I prefer brief answers.", "I prefer brief answers.", "deny"},
		{"Malay opt out", "Jangan simpan: I prefer brief answers.", "I prefer brief answers.", "deny"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, approval := range []bool{false, true} {
				store, _ := memory.Open(t.TempDir())
				calls := 0
				tool := rememberChat{ChatMemory{Store: store, UserMessage: tc.message, ConversationID: "work-suitability", RunID: "run-one", Automatic: true, Review: func(_ context.Context, preview any) (bool, error) {
					calls++
					p := preview.(map[string]any)
					if p["saved_text"] != tc.quote || p["reason"] == "" {
						t.Fatal("review omitted exact text or reason")
					}
					return approval, nil
				}}}
				subject, kind := "preference.response_length", "preference"
				if tc.name == "fact" {
					subject, kind = "profile.city", "fact"
				}
				raw, _ := json.Marshal(map[string]any{"subject": subject, "kind": kind, "quote": tc.quote, "scope": "personal"})
				_, err := tool.Execute(context.Background(), raw)
				wantSave := tc.action == "save" || tc.action == "review" && approval
				if wantSave != (err == nil) {
					t.Fatalf("approval %t action %s error %v", approval, tc.action, err)
				}
				if (tc.action == "review") != (calls == 1) {
					t.Fatalf("review count %d for %s", calls, tc.action)
				}
				records, err := store.List("", true)
				if err != nil || (len(records) == 1) != wantSave {
					t.Fatal("wrong durable result", records, err)
				}
			}
		})
	}
}

func TestSuitabilityApprovalDoesNotRestoreForgottenMemoryOrEnableCapture(t *testing.T) {
	store, _ := memory.Open(t.TempDir())
	input := memory.ChatInput{Subject: "preference.response", Kind: "preference", Quote: "I prefer brief answers.", ConversationID: "work-first", RunID: "run-first"}
	record, err := store.RememberChat(input)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Forget(record.ID); err != nil {
		t.Fatal(err)
	}
	calls := 0
	m := ChatMemory{Store: store, UserMessage: "For this reply only, I prefer brief answers.", ConversationID: "work-next", RunID: "run-next", Automatic: true, Review: func(context.Context, any) (bool, error) { calls++; return true, nil }}
	raw := json.RawMessage(`{"subject":"preference.response","kind":"preference","quote":"I prefer brief answers.","scope":"personal"}`)
	if _, err := (rememberChat{m}).Execute(context.Background(), raw); err == nil || calls != 1 {
		t.Fatal("review silently restored forgotten subject", err, calls)
	}
	m.Automatic = false
	calls = 0
	if _, err := (rememberChat{m}).Execute(context.Background(), raw); err == nil || calls != 0 {
		t.Fatal("review bypassed disabled capture", err, calls)
	}
	records, err := store.List("", true)
	if err != nil || len(records) != 0 {
		t.Fatal("unexpected memory", records, err)
	}
}
