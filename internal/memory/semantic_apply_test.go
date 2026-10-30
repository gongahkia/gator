package memory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func analyzedInput(t *testing.T, s Store, record Record, quote, relation string) (ChatInput, SemanticPlan) {
	t.Helper()
	input := ChatInput{Subject: "novel.reply_style", Kind: "preference", Quote: quote, ConversationID: "work-current", RunID: "run-current"}
	m := &semanticFixture{response: fmt.Sprintf(`{"relation":%q,"match_ids":[%q],"suggested_subject":"model.unrelated_label","reason":"Meaning comparison of the exact current statement."}`, relation, record.ID)}
	plan, err := s.AnalyzeChat(context.Background(), m, input, quote)
	if err != nil {
		t.Fatal(err)
	}
	return input, plan
}

func TestSemanticApplyEquivalentUsesStableIdentityAndCurrentQuote(t *testing.T) {
	s, _ := Open(t.TempDir())
	record := reconciliationRecord(t, s, "custom.verbosity", "I prefer short replies.", "work-first", "")
	input, plan := analyzedInput(t, s, record, "Keep my answers concise.", "equivalent")
	updated, err := s.ApplySemanticChat(context.Background(), input, plan, false)
	if err != nil || updated.ID != record.ID || updated.Subject != record.Subject || updated.Text != input.Quote || len(updated.Evidence) != 2 {
		t.Fatal(updated, err)
	}
	list, err := s.List("", true)
	if err != nil || len(list) != 1 {
		t.Fatal(list, err)
	}
	bindings, err := s.bindings()
	if err != nil || bindings.resolve("", record.Subject) != bindings.resolve("", input.Subject) {
		t.Fatal("new wording was not bound", err)
	}
	if err := s.Forget(record.ID); err != nil {
		t.Fatal(err)
	}
	for _, chat := range []string{"work-first", "work-current"} {
		if blocked, err := s.HistorySuppressed(chat, ""); err != nil || !blocked {
			t.Fatal(chat, err)
		}
	}
}

func TestSemanticConflictRequiresInspectedRevisionOrExactHostReview(t *testing.T) {
	for _, reviewed := range []bool{false, true} {
		s, _ := Open(t.TempDir())
		record := reconciliationRecord(t, s, "custom.verbosity", "I prefer detailed replies.", "work-first", "")
		input, plan := analyzedInput(t, s, record, "I prefer short replies.", "contradictory")
		if !reviewed {
			if _, err := s.ApplySemanticChat(context.Background(), input, plan, false); err == nil {
				t.Fatal("uninspected conflict overwritten")
			}
			if _, err := os.Stat(filepath.Join(s.root, ".concepts")); !os.IsNotExist(err) {
				t.Fatal("refused conflict published aliases", err)
			}
			input.ReplaceID, input.ExpectedUpdated = record.ID, record.UpdatedAt.Format(time.RFC3339Nano)
		}
		updated, err := s.ApplySemanticChat(context.Background(), input, plan, reviewed)
		if err != nil || updated.ID != record.ID || updated.Text != input.Quote {
			t.Fatal(updated, err)
		}
	}
}

func TestSemanticApplicationRejectsStaleScopeQuoteAndAddedRecords(t *testing.T) {
	for _, change := range []string{"scope", "quote", "record", "added"} {
		t.Run(change, func(t *testing.T) {
			s, _ := Open(t.TempDir())
			record := reconciliationRecord(t, s, "custom.verbosity", "I prefer short replies.", "work-first", "")
			input, plan := analyzedInput(t, s, record, "Keep my answers concise.", "equivalent")
			switch change {
			case "scope":
				input.GoalID = "goal-0123456789abcdef01234567"
			case "quote":
				input.Quote = "I prefer detailed replies."
			case "record":
				if _, err := s.Correct(record.ID, "Keep the latest words.", "user"); err != nil {
					t.Fatal(err)
				}
			case "added":
				reconciliationRecord(t, s, "another.preference", "Another current preference.", "work-second", "")
			}
			before, _ := os.ReadFile(s.path(record.ID))
			if _, err := s.ApplySemanticChat(context.Background(), input, plan, true); err == nil {
				t.Fatal("stale or unrelated plan applied")
			}
			after, _ := os.ReadFile(s.path(record.ID))
			if string(before) != string(after) {
				t.Fatal("refused application changed record")
			}
		})
	}
}

func TestSemanticDistinctDoesNotReviveExplicitExpiry(t *testing.T) {
	s, _ := Open(t.TempDir())
	old, err := s.Add("preference", "Old expired user words.", "", "user", time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	input := ChatInput{Subject: "preference.response_length", Kind: "preference", Quote: "I prefer short replies.", ConversationID: "work-current", RunID: "run-current"}
	m := &semanticFixture{response: `{"relation":"distinct","match_ids":[],"suggested_subject":"preference.response_length","reason":"A new current preference."}`}
	plan, err := s.AnalyzeChat(context.Background(), m, input, input.Quote)
	if err != nil || len(plan.Snapshot.Candidates) != 0 {
		t.Fatal(plan, err)
	}
	current, err := s.ApplySemanticChat(context.Background(), input, plan, false)
	if err != nil || current.ID == old.ID {
		t.Fatal(current, err)
	}
	retained, err := s.Load(old.ID)
	if err != nil || retained.ExpiresAt != old.ExpiresAt {
		t.Fatal("old expiry changed", retained, err)
	}
}
