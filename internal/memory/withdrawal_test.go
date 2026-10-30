package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWithdrawalRetainsOnlyAbstractTopicAndBindsSemanticRestoration(t *testing.T) {
	s, _ := Open(t.TempDir())
	in := ChatInput{Subject: "profile.city", Kind: "fact", Quote: "I live in Harbor City.", ConversationID: "work-first", RunID: "run-first"}
	record, err := s.RememberChat(in)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Forget(record.ID); err != nil {
		t.Fatal(err)
	}
	rules, err := s.WithdrawalRules("")
	if err != nil || len(rules) != 1 || rules[0].Topic == "" || strings.Contains(rules[0].Topic, "Harbor") {
		t.Fatal(rules, err)
	}
	data, _ := os.ReadFile(filepath.Join(s.root, ".withdrawals", rules[0].ID+".json"))
	if strings.Contains(string(data), "Harbor") || strings.Contains(string(data), record.Text) {
		t.Fatal("withdrawal retained fact value")
	}
	in.Subject, in.Quote, in.ConversationID, in.RunID = "personal.base_location", "I live in Birch City.", "work-next", "run-next"
	m := &semanticFixture{response: fmt.Sprintf(`{"relation":"withdrawn","match_ids":["withdrawal-%s"],"suggested_subject":"profile.city","reason":"The residence attribute was withdrawn."}`, rules[0].ID)}
	plan, err := s.AnalyzeChat(context.Background(), m, in, in.Quote)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplySemanticChat(context.Background(), in, plan, false); err == nil {
		t.Fatal("semantic relabel bypassed withdrawal")
	}
	in.Explicit = true
	current, err := s.ApplySemanticChat(context.Background(), in, plan, false)
	if err != nil || current.Text != in.Quote {
		t.Fatal(current, err)
	}
	rules, err = s.WithdrawalRules("")
	if err != nil || len(rules) != 1 || rules[0].RestoredAt.IsZero() {
		t.Fatal("restore did not preserve historical barrier", rules, err)
	}
	if blocked, err := s.HistorySuppressed("work-first", ""); err != nil || !blocked {
		t.Fatal("restore reopened source history", err)
	}
}

func TestWithdrawalScopeIsolationAndLegacyCanonicalMigration(t *testing.T) {
	s, _ := Open(t.TempDir())
	goal := "goal-0123456789abcdef01234567"
	record := reconciliationRecord(t, s, "preference.response_length", "I prefer short replies.", "work-goal", goal)
	if err := s.Forget(record.ID); err != nil {
		t.Fatal(err)
	}
	if rules, err := s.WithdrawalRules(""); err != nil || len(rules) != 0 {
		t.Fatal("another goal's rule entered personal scope", rules, err)
	}
	if rules, err := s.WithdrawalRules(goal); err != nil || len(rules) != 1 || rules[0].Scope != goal {
		t.Fatal(rules, err)
	}
	path := s.subjectPath("", "profile.home_city")
	if err := blockMarker(path); err != nil {
		t.Fatal(err)
	}
	rules, err := s.WithdrawalRules("")
	if err != nil || len(rules) != 1 || rules[0].ID != conceptKey("", "profile.city") || rules[0].Topic == "" || rules[0].ScopeUnknown {
		t.Fatal("legacy alias was not attributed", rules, err)
	}
	in := ChatInput{Subject: "profile.city", Kind: "fact", Quote: "I live in Birch City.", ConversationID: "work-new", RunID: "run-new", Explicit: true}
	if _, err := s.RememberChat(in); err != nil {
		t.Fatal(err)
	}
	rules, err = s.WithdrawalRules("")
	if err != nil || len(rules) != 1 || rules[0].RestoredAt.IsZero() {
		t.Fatal("legacy restoration lost barrier", rules, err)
	}
}

func TestUnknownLegacyWithdrawalRequiresTopicReview(t *testing.T) {
	s, _ := Open(t.TempDir())
	if err := blockMarker(s.subjectPath("", "custom.unknown")); err != nil {
		t.Fatal(err)
	}
	rules, err := s.WithdrawalRules("")
	if err != nil || len(rules) != 1 || !rules[0].ScopeUnknown || rules[0].Topic != "" {
		t.Fatal("legacy facts invented", rules, err)
	}
	m := &semanticFixture{}
	in := ChatInput{Subject: "new.preference", Kind: "preference", Quote: "I prefer short answers."}
	if _, err := s.AnalyzeChat(context.Background(), m, in, in.Quote); err != ErrSemanticCoverage || m.calls != 0 {
		t.Fatal("incomplete withdrawal state called provider", err, m.calls)
	}
}

func TestApprovedTopicIsRevisionGuardedAndEntersSnapshotIdentity(t *testing.T) {
	s, _ := Open(t.TempDir())
	record := reconciliationRecord(t, s, "equipment.keyboard", "I use a split keyboard.", "work-first", "")
	if err := s.ForgetWithTopic(record.ID, "old-revision", "the user's keyboard preferences"); err == nil {
		t.Fatal("stale withdrawal accepted")
	}
	if err := s.ForgetWithTopic(record.ID, record.UpdatedAt.Format(time.RFC3339Nano), record.Text); err == nil {
		t.Fatal("forgotten plaintext accepted as topic")
	}
	if err := s.ForgetWithTopic(record.ID, record.UpdatedAt.Format(time.RFC3339Nano), "the user's keyboard preferences"); err != nil {
		t.Fatal(err)
	}
	first, err := s.SemanticSnapshot("", nil)
	if err != nil || len(first.Candidates) != 1 || first.Candidates[0].Kind != "withdrawn" {
		t.Fatal(first, err)
	}
	rules, err := s.WithdrawalRules("")
	if err != nil {
		t.Fatal(err)
	}
	rules[0].Topic = "the user's keyboard type"
	data, _ := json.Marshal(rules[0])
	if err := os.WriteFile(filepath.Join(s.root, ".withdrawals", rules[0].ID+".json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	second, err := s.SemanticSnapshot("", nil)
	if err != nil || first.Token == second.Token {
		t.Fatal("changed withdrawal did not invalidate comparison", err)
	}
}
