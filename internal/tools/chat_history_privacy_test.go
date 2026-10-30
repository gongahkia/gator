package tools

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/memory"
	"github.com/gongahkia/gator/internal/worksession"
)

type privacyHistoryModel struct{ calls int }

func (m *privacyHistoryModel) Complete(_ context.Context, r agent.TurnRequest) (agent.Turn, error) {
	m.calls++
	var checks []struct{ ID, User, Assistant, Title, Folder string }
	if err := json.Unmarshal([]byte(r.Messages[0].Content), &checks); err != nil {
		return agent.Turn{}, err
	}
	var verdicts []map[string]string
	for _, check := range checks {
		verdict := "clear"
		if strings.Contains(check.User+check.Assistant+check.Title+check.Folder, "Harbor") {
			verdict = "blocked"
		}
		verdicts = append(verdicts, map[string]string{"id": check.ID, "verdict": verdict})
	}
	data, _ := json.Marshal(map[string]any{"verdicts": verdicts})
	return agent.Turn{Text: string(data)}, nil
}

func TestChatHistoryFiltersUntrackedCopiesAcrossSearchReadAndRestart(t *testing.T) {
	root := t.TempDir()
	sessions, _ := worksession.Open(root)
	old := time.Now().Add(-time.Hour)
	c, err := sessions.Create("Planning", t.TempDir(), "snap-test", old)
	if err != nil {
		t.Fatal(err)
	}
	for _, revision := range []worksession.Revision{
		{ID: "rev-private", Objective: "Planning: my home is Harbor City.", FinalText: "I'll use your Harbor City home for the plan.", CreatedAt: old},
		{ID: "rev-safe", ParentRevisionID: "rev-private", Objective: "Planning: the venue is Birch Hall.", FinalText: "Noted Birch Hall.", CreatedAt: old.Add(time.Minute)},
	} {
		revision.SnapshotID, revision.BundlePath, revision.Status = "snap-test", "/bundle/source", "completed"
		if _, err := sessions.AddRevision(c.ID, revision); err != nil {
			t.Fatal(err)
		}
	}
	before, _ := sessions.LoadRevision(c.ID, "rev-private")
	memories, _ := memory.Open(root)
	r, err := memories.RememberChat(memory.ChatInput{Subject: "profile.city", Kind: "fact", Quote: "I live in Harbor City.", ConversationID: "work-tracked-elsewhere", RunID: "run-tracked"})
	if err != nil {
		t.Fatal(err)
	}
	if err := memories.Forget(r.ID); err != nil {
		t.Fatal(err)
	}
	if blocked, err := memories.HistorySuppressed(c.ID, ""); err != nil || blocked {
		t.Fatal("fixture was already tracked", blocked, err)
	}
	for restart := 0; restart < 2; restart++ {
		model := &privacyHistoryModel{}
		h := ChatHistory{StateDir: root, CurrentConversation: "work-current", Semantics: model}
		for _, request := range []struct {
			tool agent.Tool
			args any
			safe bool
		}{
			{chatSearch{h}, map[string]string{"query": "Planning"}, true},
			{chatSearch{h}, map[string]string{"query": "Planning", "conversation_id": c.ID, "before_revision": "rev-private"}, false},
			{chatRead{h}, map[string]string{"conversation_id": c.ID}, true},
			{chatRead{h}, map[string]string{"conversation_id": c.ID, "before_revision": "rev-private"}, false},
		} {
			raw, _ := json.Marshal(request.args)
			result, err := request.tool.Execute(context.Background(), raw)
			if err != nil || strings.Contains(result.Content, "Harbor") || strings.Contains(result.Content, "rev-private\"") || (request.safe && !strings.Contains(result.Content, "Birch Hall")) || !strings.Contains(result.Content, `"withheld_historical_turns":1`) {
				t.Fatal("untracked copy leaked or unrelated history lost", result.Content, err)
			}
		}
		if model.calls == 0 {
			t.Fatal("history bypassed semantic screening")
		}
	}
	after, _ := sessions.LoadRevision(c.ID, "rev-private")
	if !reflect.DeepEqual(before, after) {
		t.Fatal("agent filter changed user-visible transcript")
	}
	args, _ := json.Marshal(map[string]string{"conversation_id": c.ID})
	if result, err := (chatRead{ChatHistory{StateDir: root}}).Execute(context.Background(), args); err == nil || result.Content != "" {
		t.Fatal("offline classifier fell back to raw history", result, err)
	}
}
