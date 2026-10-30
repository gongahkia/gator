package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/memory"
)

type memoryRelationModel struct {
	response string
	calls    int
}

func (m *memoryRelationModel) Complete(context.Context, agent.TurnRequest) (agent.Turn, error) {
	m.calls++
	return agent.Turn{Text: m.response}, nil
}

func TestSemanticMemoryToolNormalizesNovelLabelsAndAvoidsFalseIntentReview(t *testing.T) {
	s, _ := memory.Open(t.TempDir())
	m := &memoryRelationModel{response: `{"relation":"distinct","match_ids":[],"suggested_subject":"preference.response_length","reason":"An enduring response-detail preference."}`}
	reviews := 0
	tool := rememberChat{ChatMemory{Store: s, UserMessage: "I prefer concise answers.", ConversationID: "work-current", RunID: "run-current", Automatic: true, Semantics: m, Review: func(context.Context, any) (bool, error) { reviews++; return false, nil }}}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"subject":"novel.answer_length","kind":"preference","quote":"I prefer concise answers.","scope":"personal","explicit":true}`))
	if err != nil || reviews != 0 || m.calls != 1 || !strings.Contains(result.Content, `"saved":true`) {
		t.Fatal(result.Content, err, reviews, m.calls)
	}
	records, err := s.List("", true)
	if err != nil || len(records) != 1 || records[0].Subject != "preference.response_length" {
		t.Fatal(records, err)
	}
}

func TestSemanticMemoryToolSharesIdentityAcrossUnregisteredParaphrases(t *testing.T) {
	s, _ := memory.Open(t.TempDir())
	record, err := s.RememberChat(memory.ChatInput{Subject: "custom.communication", Kind: "preference", Quote: "I prefer brief replies.", ConversationID: "work-first", RunID: "run-first"})
	if err != nil {
		t.Fatal(err)
	}
	m := &memoryRelationModel{response: fmt.Sprintf(`{"relation":"equivalent","match_ids":[%q],"suggested_subject":"preference.response_length","reason":"Equivalent preference under different labels."}`, record.ID)}
	tool := rememberChat{ChatMemory{Store: s, UserMessage: "I prefer concise answers.", ConversationID: "work-current", RunID: "run-current", Automatic: true, Semantics: m}}
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"subject":"communication.detail_level","kind":"preference","quote":"I prefer concise answers.","scope":"personal"}`)); err != nil {
		t.Fatal(err)
	}
	list, err := s.List("", true)
	if err != nil || len(list) != 1 || list[0].ID != record.ID || list[0].Text != "I prefer concise answers." {
		t.Fatal(list, err)
	}
}

func TestSemanticMemoryPreconditionsPreventUnneededProviderCalls(t *testing.T) {
	for _, message := range []string{"Don't save this: I prefer concise answers.", "My API key is SYNTHETIC_ONLY. I prefer concise answers."} {
		s, _ := memory.Open(t.TempDir())
		m := &memoryRelationModel{}
		tool := rememberChat{ChatMemory{Store: s, UserMessage: message, ConversationID: "work-current", RunID: "run-current", Automatic: true, Semantics: m}}
		if _, err := tool.Execute(context.Background(), json.RawMessage(`{"subject":"preference.response_length","kind":"preference","quote":"I prefer concise answers.","scope":"personal"}`)); err == nil || m.calls != 0 {
			t.Fatal("denied text reached provider", err, m.calls)
		}
	}
}

func TestSemanticConsolidationReviewRejectsConcurrentChanges(t *testing.T) {
	s, _ := memory.Open(t.TempDir())
	first, _ := s.RememberChat(memory.ChatInput{Subject: "custom.first", Kind: "preference", Quote: "I prefer short replies.", ConversationID: "work-first", RunID: "run-first"})
	second, _ := s.RememberChat(memory.ChatInput{Subject: "custom.second", Kind: "preference", Quote: "I prefer brief answers.", ConversationID: "work-second", RunID: "run-second"})
	m := &memoryRelationModel{response: fmt.Sprintf(`{"relation":"equivalent","match_ids":[%q,%q],"suggested_subject":"preference.response_length","reason":"Equivalent preference duplicates."}`, first.ID, second.ID)}
	tool := rememberChat{ChatMemory{Store: s, UserMessage: "I prefer concise answers.", ConversationID: "work-current", RunID: "run-current", Automatic: true, Semantics: m, Review: func(_ context.Context, preview any) (bool, error) {
		p := preview.(map[string]any)
		if p["action"] != "reconcile" || len(p["existing_memories"].([]memory.SemanticCandidate)) != 2 {
			t.Fatal("review omitted exact inputs", p)
		}
		_, err := s.Correct(first.ID, "A concurrent user edit.", "user")
		return true, err
	}}}
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"subject":"preference.response_length","kind":"preference","quote":"I prefer concise answers.","scope":"personal"}`)); err == nil {
		t.Fatal("stale consolidation approved")
	}
	list, err := s.List("", true)
	if err != nil || len(list) != 2 {
		t.Fatal(list, err)
	}
}

func TestSemanticConflictRequiresActualInspection(t *testing.T) {
	s, _ := memory.Open(t.TempDir())
	record, _ := s.RememberChat(memory.ChatInput{Subject: "custom.verbosity", Kind: "preference", Quote: "I prefer detailed replies.", ConversationID: "work-first", RunID: "run-first"})
	m := &memoryRelationModel{response: fmt.Sprintf(`{"relation":"contradictory","match_ids":[%q],"suggested_subject":"preference.response_length","reason":"Different values for the same preference."}`, record.ID)}
	tool := rememberChat{ChatMemory{Store: s, UserMessage: "I prefer short replies.", ConversationID: "work-current", RunID: "run-current", Automatic: true, Semantics: m}}
	args := map[string]any{"subject": "novel.reply_style", "kind": "preference", "quote": "I prefer short replies.", "scope": "personal"}
	raw, _ := json.Marshal(args)
	if _, err := tool.Execute(context.Background(), raw); err == nil {
		t.Fatal("uninspected semantic contradiction overwritten")
	}
	args["replace_id"], args["expected_updated"] = record.ID, record.UpdatedAt.Format(time.RFC3339Nano)
	raw, _ = json.Marshal(args)
	if _, err := tool.Execute(context.Background(), raw); err != nil {
		t.Fatal(err)
	}
}
