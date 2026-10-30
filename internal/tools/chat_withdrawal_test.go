package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/memory"
)

func TestCustomWithdrawalRetainsOnlyApprovedTopicAndStillForgetsWhenDeclined(t *testing.T) {
	for _, approve := range []bool{true, false} {
		s, _ := memory.Open(t.TempDir())
		r, err := s.RememberChat(memory.ChatInput{Subject: "equipment.keyboard", Kind: "fact", Quote: "I use a split keyboard.", ConversationID: "work-source", RunID: "run-source"})
		if err != nil {
			t.Fatal(err)
		}
		m := &memoryRelationModel{response: `{"topic":"the user's keyboard preferences"}`}
		reviews := 0
		tool := forgetChat{ChatMemory{Store: s, UserMessage: "Forget my keyboard information.", Semantics: m, Review: func(_ context.Context, preview any) (bool, error) {
			reviews++
			p := preview.(map[string]any)
			if p["action"] != "withdrawal_topic" || p["retained_topic"] != "the user's keyboard preferences" || strings.Contains(p["retained_topic"].(string), "split") {
				t.Fatal(preview)
			}
			return approve, nil
		}}}
		raw, _ := json.Marshal(map[string]string{"id": r.ID, "quote": "Forget my keyboard information.", "expected_updated": r.UpdatedAt.Format(time.RFC3339Nano)})
		result, err := tool.Execute(context.Background(), raw)
		if err != nil || reviews != 1 || !strings.Contains(result.Content, `"forgotten"`) {
			t.Fatal(result.Content, reviews, err)
		}
		if _, err := s.Load(r.ID); err == nil {
			t.Fatal("forget depended on topic approval")
		}
		rules, err := s.WithdrawalRules("")
		if err != nil || len(rules) != 1 || (rules[0].Topic != "") != approve {
			t.Fatal(rules, err)
		}
	}
}

func TestCustomWithdrawalDoesNotDeleteConcurrentEditDuringTopicReview(t *testing.T) {
	s, _ := memory.Open(t.TempDir())
	r, _ := s.RememberChat(memory.ChatInput{Subject: "equipment.keyboard", Kind: "fact", Quote: "I use a split keyboard.", ConversationID: "work-source", RunID: "run-source"})
	tool := forgetChat{ChatMemory{Store: s, UserMessage: "Forget my keyboard information.", Semantics: &memoryRelationModel{response: `{"topic":"the user's keyboard preferences"}`}, Review: func(context.Context, any) (bool, error) {
		_, err := s.Correct(r.ID, "A newer owner statement.", "user")
		return true, err
	}}}
	raw, _ := json.Marshal(map[string]string{"id": r.ID, "quote": "Forget my keyboard information.", "expected_updated": r.UpdatedAt.Format(time.RFC3339Nano)})
	if _, err := tool.Execute(context.Background(), raw); err == nil {
		t.Fatal("concurrent edit deleted")
	}
	if current, err := s.Load(r.ID); err != nil || current.Text != "A newer owner statement." {
		t.Fatal(current, err)
	}
}
