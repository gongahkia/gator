package memory

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/agent"
)

type semanticFixture struct {
	response string
	calls    int
	inspect  func(agent.TurnRequest)
	tool     bool
}

func (m *semanticFixture) Complete(_ context.Context, request agent.TurnRequest) (agent.Turn, error) {
	m.calls++
	if m.inspect != nil {
		m.inspect(request)
	}
	turn := agent.Turn{Text: m.response}
	if m.tool {
		turn.ToolCalls = []agent.ToolCall{{Name: "memory_forget"}}
	}
	return turn, nil
}

func TestSemanticAnalysisUsesMeaningCandidatesWithoutPublishing(t *testing.T) {
	s, _ := Open(t.TempDir())
	current := reconciliationRecord(t, s, "custom.detail", "I prefer detailed replies.", "work-first", "")
	_ = reconciliationRecord(t, s, "private.other_goal", "Do not reveal other-goal words.", "work-other", "goal-0123456789abcdef01234567")
	before, _ := os.ReadFile(s.path(current.ID))
	m := &semanticFixture{response: `{"relation":"contradictory","match_ids":["` + current.ID + `"],"suggested_subject":"custom.detail","reason":"The statements concern the same reply detail preference with incompatible values."}`, inspect: func(request agent.TurnRequest) {
		if len(request.Tools) != 0 || len(request.Messages) != 1 || strings.Contains(request.Messages[0].Content, "other-goal words") || !strings.Contains(request.Messages[0].Content, current.Text) {
			t.Fatal("invalid semantic scope", request)
		}
	}}
	input := ChatInput{Subject: "communication.verbosity", Kind: "preference", Quote: "Keep my replies concise."}
	plan, err := s.AnalyzeChat(context.Background(), m, input, input.Quote)
	if err != nil || plan.Relation != Contradictory || plan.MatchIDs[0] != current.ID || plan.Snapshot.Token == "" || plan.StatementSHA256 != textDigest(input.Quote) {
		t.Fatal(plan, err)
	}
	after, _ := os.ReadFile(s.path(current.ID))
	if string(before) != string(after) {
		t.Fatal("classifier modified saved memory")
	}
	if _, err := os.Stat(s.root + "/.concepts"); !os.IsNotExist(err) {
		t.Fatal("classifier published aliases", err)
	}
}

func TestSemanticAnalysisRejectsForgedMalformedAndInconsistentResults(t *testing.T) {
	s, _ := Open(t.TempDir())
	for _, response := range []string{
		`{"relation":"equivalent","match_ids":["mem-unavailable"],"suggested_subject":"profile.city","reason":"same"}`,
		`{"relation":"distinct","match_ids":["mem-unavailable"],"suggested_subject":"profile.city","reason":"new"}`,
		`{"relation":"equivalent","match_ids":[],"suggested_subject":"profile.city","reason":"same"}`,
		`{"relation":"probably","match_ids":[],"suggested_subject":"profile.city","reason":"guess"}`,
		`{"relation":"distinct","match_ids":[],"suggested_subject":"profile.city","reason":"new","confidence":100}`,
		`{"relation":"distinct","match_ids":[],"suggested_subject":"profile.city","reason":"new"}{}`,
	} {
		m := &semanticFixture{response: response}
		if _, err := s.AnalyzeChat(context.Background(), m, ChatInput{Subject: "profile.city", Kind: "fact", Quote: "I live in Harbor City."}, "I live in Harbor City."); err == nil {
			t.Fatal("invalid plan accepted", response)
		}
	}
	m := &semanticFixture{response: `{"relation":"distinct","match_ids":[],"suggested_subject":"profile.city","reason":"new"}`, tool: true}
	if _, err := s.AnalyzeChat(context.Background(), m, ChatInput{Subject: "profile.city", Quote: "I live in Harbor City."}, "I live in Harbor City."); err == nil {
		t.Fatal("classifier tool invocation accepted")
	}
}

func TestSemanticCoverageExclusionsAndCancellationDoNotCallProvider(t *testing.T) {
	s, _ := Open(t.TempDir())
	record := reconciliationRecord(t, s, "custom.property", "I prefer short replies.", "work-first", "")
	m := &semanticFixture{}
	input := ChatInput{Subject: "preference.response_length", Kind: "preference", Quote: "I prefer concise answers.", ExcludedIDs: []string{record.ID}}
	if _, err := s.AnalyzeChat(context.Background(), m, input, input.Quote); err != ErrSemanticCoverage || m.calls != 0 {
		t.Fatal("incomplete comparison called provider", err, m.calls)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	input.ExcludedIDs = nil
	if _, err := s.AnalyzeChat(ctx, m, input, input.Quote); err != context.Canceled || m.calls != 0 {
		t.Fatal(err, m.calls)
	}
}

func TestSemanticCoverageLimitsDoNotSilentlySampleCandidates(t *testing.T) {
	for _, tc := range []struct {
		name        string
		count, size int
	}{{"count", 129, 20}, {"bytes", 18, 4096}} {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := Open(t.TempDir())
			for i := 0; i < tc.count; i++ {
				if _, err := s.Add("fact", strings.Repeat("a", tc.size), "", "fixture", time.Time{}); err != nil {
					t.Fatal(err)
				}
			}
			m := &semanticFixture{}
			input := ChatInput{Subject: "custom.property", Kind: "fact", Quote: "An enduring statement."}
			if _, err := s.AnalyzeChat(context.Background(), m, input, input.Quote); err != ErrSemanticCoverage || m.calls != 0 {
				t.Fatal("partial candidate set treated as complete", err, m.calls)
			}
		})
	}
}

func TestSemanticSnapshotDetectsDirectChangesWithoutRevisionBump(t *testing.T) {
	s, _ := Open(t.TempDir())
	record := reconciliationRecord(t, s, "custom.property", "I prefer short replies.", "work-first", "")
	first, err := s.SemanticSnapshot("", nil)
	if err != nil {
		t.Fatal(err)
	}
	record.Text = "I prefer detailed replies."
	data, _ := json.Marshal(record)
	if err := os.WriteFile(s.path(record.ID), data, 0600); err != nil {
		t.Fatal(err)
	}
	second, err := s.SemanticSnapshot("", nil)
	if err != nil || first.Token == second.Token {
		t.Fatal("direct edit did not invalidate analysis snapshot", err)
	}
	s.now = func() time.Time { return record.CreatedAt.Add(time.Hour) }
	third, err := s.SemanticSnapshot("", nil)
	if err != nil || third.Token != second.Token {
		t.Fatal("stable snapshot changed without evidence", err)
	}
}
