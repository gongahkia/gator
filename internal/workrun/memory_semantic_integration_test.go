package workrun

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/memory"
	"github.com/gongahkia/gator/internal/worksession"
)

type foregroundMemoryModel struct {
	calls, comparisons, foreground int
	historyID                      string
	t                              *testing.T
}

func (m *foregroundMemoryModel) Complete(_ context.Context, r agent.TurnRequest) (agent.Turn, error) {
	m.calls++
	if len(r.Tools) == 0 {
		m.comparisons++
		if m.historyID != "" {
			var checks []struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal([]byte(r.Messages[0].Content), &checks); err != nil {
				m.t.Fatal(err)
			}
			var verdicts []map[string]string
			for _, check := range checks {
				verdicts = append(verdicts, map[string]string{"id": check.ID, "verdict": "blocked"})
			}
			data, _ := json.Marshal(map[string]any{"verdicts": verdicts})
			return agent.Turn{Text: string(data)}, nil
		}
		return agent.Turn{Text: `{"relation":"distinct","match_ids":[],"suggested_subject":"preference.response_length","reason":"An enduring preference."}`}, nil
	}
	m.foreground++
	if m.historyID != "" {
		for _, message := range r.Messages {
			if strings.Contains(message.Content, "Harbor City") {
				m.t.Fatal("privacy classifier content reached foreground context")
			}
		}
	}
	if m.foreground > 1 {
		return agent.Turn{Text: "Completed the bounded memory request."}, nil
	}
	call := agent.ToolCall{ID: "memory-call", Name: "memory_remember", Arguments: json.RawMessage(`{"subject":"custom.reply_length","kind":"preference","quote":"I prefer concise answers.","scope":"personal"}`)}
	if m.historyID != "" {
		call.Name = "chat_read"
		call.Arguments, _ = json.Marshal(map[string]string{"conversation_id": m.historyID})
	}
	return agent.Turn{ToolCalls: []agent.ToolCall{call}}, nil
}

func TestForegroundSemanticMemoryUsesProductionBindingAndAggregateBudget(t *testing.T) {
	for _, limit := range []int{8, 1} {
		root := t.TempDir()
		model := &foregroundMemoryModel{t: t}
		_, err := (Executor{Model: model, StateDir: root}).Execute(context.Background(), Request{SourcePath: t.TempDir(), Objective: "I prefer concise answers.", ForegroundApp: true, LearnFromChats: true, DisableBrowser: true, DisableDelegation: true, Mode: action.Inspect, Contract: artifact.InspectionContract(), Limits: agent.Limits{ModelRequests: limit, WallSeconds: 30}})
		store, _ := memory.Open(root)
		records, readErr := store.List("", true)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if limit == 1 {
			if err == nil || len(records) != 0 || model.comparisons != 0 || model.calls != 1 {
				t.Fatal("nested comparison escaped shared budget", records, model, err)
			}
		} else if err != nil || len(records) != 1 || records[0].Subject != "preference.response_length" || model.comparisons != 1 || model.calls != 3 {
			t.Fatal("foreground save bypassed semantic binding", records, model, err)
		}
	}
}

func TestForegroundHistoryPrivacyDoesNotRetainClassifierContextInReplay(t *testing.T) {
	root := t.TempDir()
	sessions, _ := worksession.Open(root)
	chat, err := sessions.Create("Planning", t.TempDir(), "snap-test", time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.AddRevision(chat.ID, worksession.Revision{ID: "rev-old", SnapshotID: "snap-test", Objective: "My home is Harbor City.", BundlePath: "/synthetic/source", Status: "completed"}); err != nil {
		t.Fatal(err)
	}
	store, _ := memory.Open(root)
	record, err := store.RememberChat(memory.ChatInput{Subject: "profile.city", Kind: "fact", Quote: "I live in Harbor City.", ConversationID: "work-different-source", RunID: "run-source"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Forget(record.ID); err != nil {
		t.Fatal(err)
	}
	m := &foregroundMemoryModel{t: t, historyID: chat.ID}
	result, err := (Executor{Model: m, StateDir: root}).Execute(context.Background(), Request{SourcePath: t.TempDir(), Objective: "Recall earlier planning", ForegroundApp: true, DisableBrowser: true, DisableDelegation: true, Mode: action.Inspect, Contract: artifact.InspectionContract(), Limits: agent.Limits{ModelRequests: 8, WallSeconds: 30}})
	if err != nil || m.comparisons == 0 {
		t.Fatal(result, m, err)
	}
	revision, err := sessions.LoadRevision(result.ConversationID, result.RevisionID)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(revision.Replay)
	if strings.Contains(string(encoded), "Harbor City") {
		t.Fatal("private comparison content persisted as foreground replay")
	}
}
