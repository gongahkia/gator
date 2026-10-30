package memory

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/agent"
)

type historyModelFunc func(context.Context, agent.TurnRequest) (agent.Turn, error)

func (f historyModelFunc) Complete(ctx context.Context, r agent.TurnRequest) (agent.Turn, error) {
	return f(ctx, r)
}

func historyVerdicts(t *testing.T, r agent.TurnRequest, verdict func(historyCheck) string) agent.Turn {
	t.Helper()
	if len(r.Tools) != 0 || len(r.Messages) != 1 {
		t.Fatal("privacy filter received an action surface", r)
	}
	var checks []historyCheck
	if err := json.Unmarshal([]byte(r.Messages[0].Content), &checks); err != nil {
		t.Fatal(err)
	}
	var results []map[string]string
	for _, check := range checks {
		results = append(results, map[string]string{"id": check.ID, "verdict": verdict(check)})
	}
	data, _ := json.Marshal(map[string]any{"verdicts": results})
	return agent.Turn{Text: string(data)}
}

func withdrawResidence(t *testing.T, s Store) {
	t.Helper()
	r, err := s.RememberChat(ChatInput{Subject: "profile.city", Kind: "fact", Quote: "I live in Harbor City.", ConversationID: "work-source", RunID: "run-source"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Forget(r.ID); err != nil {
		t.Fatal(err)
	}
}

func TestHistoryScreenPreservesUnrelatedRowsAndChecksAllReturnedFields(t *testing.T) {
	s, _ := Open(t.TempDir())
	withdrawResidence(t, s)
	calls := 0
	seen := map[string]bool{}
	m := historyModelFunc(func(_ context.Context, r agent.TurnRequest) (agent.Turn, error) {
		calls++
		return historyVerdicts(t, r, func(c historyCheck) string {
			seen[c.ID] = true
			if len(c.Topics) != 1 || strings.Contains(c.Topics[0], "Harbor") {
				t.Fatal("rule retained a fact value", c.Topics)
			}
			switch c.ID {
			case "private/body", "title/metadata", "folder/metadata", "reply/body":
				return "blocked"
			case "ambiguous/body":
				return "uncertain"
			default:
				return "clear"
			}
		}), nil
	})
	rows := []HistoryFragment{
		{ID: "private", User: "My home is in Harbor City.", Title: "Travel"},
		{ID: "title", Title: "Home: Harbor City", User: "My plan is ready."},
		{ID: "folder", Folder: "Harbor-City-resident", User: "My plan is ready."},
		{ID: "reply", User: "Where do I live?", Assistant: "Your home city is Harbor City."},
		{ID: "ambiguous", User: "It's where I stay."},
		{ID: "safe", User: "The plan is to use Birch Hall.", Title: "Venue"},
	}
	got, err := s.ScreenHistory(context.Background(), m, "", rows)
	if err != nil || got.Withheld != 5 || !got.Allowed["safe"] || got.MigrationPending || calls != 3 {
		t.Fatal(got, calls, err)
	}
	for _, id := range []string{"private/body", "title/metadata", "folder/metadata", "reply/body", "safe/body", "safe/metadata"} {
		if !seen[id] {
			t.Fatal("returned field skipped", id)
		}
	}
}

func TestHistoryScreenRestorationKeepsOldMetadataBehindItsOwnBarrier(t *testing.T) {
	s, _ := Open(t.TempDir())
	before := time.Now().UTC().Add(-time.Hour)
	s.now = func() time.Time { return before }
	withdrawResidence(t, s)
	restored := before.Add(time.Minute)
	s.now = func() time.Time { return restored }
	_, err := s.RememberChat(ChatInput{Subject: "profile.city", Kind: "fact", Quote: "I live in Birch City.", Explicit: true, ConversationID: "work-restored", RunID: "run-restored"})
	if err != nil {
		t.Fatal(err)
	}
	var seen []string
	m := historyModelFunc(func(_ context.Context, r agent.TurnRequest) (agent.Turn, error) {
		return historyVerdicts(t, r, func(c historyCheck) string { seen = append(seen, c.ID); return "blocked" }), nil
	})
	got, err := s.ScreenHistory(context.Background(), m, "", []HistoryFragment{
		{ID: "new-body-old-title", At: restored.Add(time.Second), MetadataAt: before, Title: "Home Harbor City", User: "I live in Birch City."},
		{ID: "old-body", At: before, MetadataAt: restored.Add(time.Second), Title: "New title", User: "My home is Harbor City."},
		{ID: "new", At: restored.Add(time.Second), MetadataAt: restored.Add(time.Second), Title: "New home", User: "I live in Birch City."},
	})
	if err != nil || got.Withheld != 2 || !got.Allowed["new"] || strings.Join(seen, ",") != "new-body-old-title/metadata,old-body/body" {
		t.Fatal(got, seen, err)
	}
}

func TestHistoryScreenNeverReturnsPartialResultsOnClassifierFailure(t *testing.T) {
	s, _ := Open(t.TempDir())
	withdrawResidence(t, s)
	rows := []HistoryFragment{{ID: "row", User: "Untrusted historical text."}}
	for _, response := range []string{
		`{"verdicts":[]}`, `{"verdicts":[{"id":"row/body","verdict":"clear","reason":"private"}]}`,
		`{"verdicts":[{"id":"row/body","verdict":"clear"}]}{}`,
		`{"verdicts":[{"id":"invented","verdict":"clear"}]}`,
		`{"verdicts":[{"id":"row/body","verdict":"probably"}]}`,
	} {
		got, err := s.ScreenHistory(context.Background(), &semanticFixture{response: response}, "", rows)
		if err == nil || len(got.Allowed) != 0 || got.Withheld != 1 {
			t.Fatal("malformed response released text", response, got, err)
		}
	}
	for _, m := range []agent.Model{nil, historyModelFunc(func(context.Context, agent.TurnRequest) (agent.Turn, error) {
		return agent.Turn{}, errors.New("offline")
	}), &semanticFixture{tool: true}} {
		got, err := s.ScreenHistory(context.Background(), m, "", rows)
		if err == nil || len(got.Allowed) != 0 {
			t.Fatal("unavailable classifier released text", got, err)
		}
	}
	count := 0
	m := historyModelFunc(func(_ context.Context, r agent.TurnRequest) (agent.Turn, error) {
		count++
		if count > 1 {
			return agent.Turn{}, errors.New("offline after first batch")
		}
		return historyVerdicts(t, r, func(historyCheck) string { return "clear" }), nil
	})
	rows = []HistoryFragment{{ID: "one", User: "A", Title: "A"}, {ID: "two", User: "B", Title: "B"}, {ID: "three", User: "C"}}
	if got, err := s.ScreenHistory(context.Background(), m, "", rows); err == nil || len(got.Allowed) != 0 || count != 2 {
		t.Fatal("first batch leaked on later failure", got, count, err)
	}
}

func TestHistoryScreenDetectsWithdrawalChangesAndRefusesOversizedOrIncompleteContext(t *testing.T) {
	s, _ := Open(t.TempDir())
	withdrawResidence(t, s)
	m := historyModelFunc(func(_ context.Context, r agent.TurnRequest) (agent.Turn, error) {
		rules, _ := s.WithdrawalRules("")
		rules[0].Topic += " or region"
		if err := s.writeWithdrawal(rules[0]); err != nil {
			t.Fatal(err)
		}
		return historyVerdicts(t, r, func(historyCheck) string { return "clear" }), nil
	})
	if got, err := s.ScreenHistory(context.Background(), m, "", []HistoryFragment{{ID: "one", User: "Plan"}}); err == nil || len(got.Allowed) != 0 {
		t.Fatal("stale rule snapshot released history", got, err)
	}
	fixture := &semanticFixture{}
	if _, err := s.ScreenHistory(context.Background(), fixture, "", []HistoryFragment{{ID: "one", User: strings.Repeat("a", 65536)}}); err == nil || fixture.calls != 0 {
		t.Fatal("incomplete excerpt was sampled", err, fixture.calls)
	}
	if err := blockMarker(s.subjectPath("", "unknown.attribute")); err != nil {
		t.Fatal(err)
	}
	got, err := s.ScreenHistory(context.Background(), fixture, "", []HistoryFragment{{ID: "one", User: "Plan"}})
	if err != nil || !got.MigrationPending || got.Withheld != 1 || fixture.calls != 0 {
		t.Fatal("unknown legacy rule was ignored or content was sent", got, fixture.calls, err)
	}
}

func TestHistoryScreenDoesNotCallModelWithoutApplicableWithdrawals(t *testing.T) {
	s, _ := Open(t.TempDir())
	rows := []HistoryFragment{{ID: "one", User: "Plan"}}
	if got, err := s.ScreenHistory(context.Background(), nil, "", rows); err != nil || !got.Allowed["one"] {
		t.Fatal(got, err)
	}
	r := reconciliationRecord(t, s, "profile.city", "I live in Harbor City.", "work-source", "goal-0123456789abcdef01234567")
	if err := s.Forget(r.ID); err != nil {
		t.Fatal(err)
	}
	if got, err := s.ScreenHistory(context.Background(), nil, "", rows); err != nil || !got.Allowed["one"] {
		t.Fatal("other goal entered comparison", got, err)
	}
}
