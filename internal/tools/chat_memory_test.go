package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/knowledge"
	"github.com/gongahkia/gator/internal/memory"
	"github.com/gongahkia/gator/internal/worksession"
)

func TestChatMemoryRequiresCurrentUserEvidenceAndExplicitForget(t *testing.T) {
	root := t.TempDir()
	store, _ := memory.Open(root)
	m := ChatMemory{Store: store, UserMessage: "I prefer concise answers. Please remember that.", ConversationID: "work-one", RunID: "run-one", Automatic: true}
	raw := json.RawMessage(`{"subject":"preference.response_length","kind":"preference","quote":"I prefer concise answers.","scope":"personal"}`)
	if _, err := (rememberChat{m}).Execute(context.Background(), raw); err != nil {
		t.Fatal(err)
	}
	records, _ := store.List("", false)
	if len(records) != 1 {
		t.Fatal(records)
	}
	m.UserMessage = "What is on my schedule?"
	if _, err := (rememberChat{m}).Execute(context.Background(), raw); err == nil {
		t.Fatal("old chat/file evidence accepted")
	}
	m.UserMessage = "I prefer concise answers."
	m.Automatic = false
	if _, err := (rememberChat{m}).Execute(context.Background(), raw); err == nil {
		t.Fatal("automatic save ignored disabled setting")
	}
	m.UserMessage = "Remember that I prefer concise answers."
	explicit := json.RawMessage(`{"subject":"preference.response_length","kind":"preference","quote":"I prefer concise answers.","scope":"personal","explicit":true}`)
	if _, err := (rememberChat{m}).Execute(context.Background(), explicit); err != nil {
		t.Fatal(err)
	}
	m.UserMessage = "Don't remember this: I prefer concise answers."
	m.Automatic = true
	if _, err := (rememberChat{m}).Execute(context.Background(), explicit); err == nil {
		t.Fatal("negative remember request authorized save")
	}
	forgotten, _ := json.Marshal(map[string]string{"id": records[0].ID, "quote": "Please forget my reply preference."})
	if _, err := (forgetChat{m}).Execute(context.Background(), forgotten); err == nil {
		t.Fatal("fabricated forgetting request accepted")
	}
	m.UserMessage = "Please forget my reply preference."
	if _, err := (forgetChat{m}).Execute(context.Background(), forgotten); err != nil {
		t.Fatal(err)
	}
}
func TestChatHistoryRetrievesSelectedAncestryAndRejectsCurrentAndOtherGoalChats(t *testing.T) {
	root := t.TempDir()
	sessions, _ := worksession.Open(root)
	c, _ := sessions.CreateChat(t.TempDir(), time.Now())
	for _, r := range []worksession.Revision{{ID: "rev-root", Objective: "The venue is Birch Hall.", FinalText: "Noted the venue.", SnapshotID: "snap-test", BundlePath: "/bundle/root", Status: "completed"}, {ID: "rev-hidden", ParentRevisionID: "rev-root", Objective: "Hidden branch mango venue.", SnapshotID: "snap-test", BundlePath: "/bundle/hidden", Status: "completed"}, {ID: "rev-visible", ParentRevisionID: "rev-root", Objective: "Confirm Birch Hall for Friday.", SnapshotID: "snap-test", BundlePath: "/bundle/visible", Status: "completed"}} {
		if _, err := sessions.AddRevision(c.ID, r); err != nil {
			t.Fatal(err)
		}
	}
	h := ChatHistory{StateDir: root, CurrentConversation: "work-current"}
	result, err := (chatSearch{h}).Execute(context.Background(), json.RawMessage(`{"query":"Birch Hall"}`))
	if err != nil || !strings.Contains(result.Content, "Birch") || strings.Contains(result.Content, "mango") {
		t.Fatal(result.Content, err)
	}
	raw, _ := json.Marshal(map[string]string{"conversation_id": c.ID})
	result, err = (chatRead{h}).Execute(context.Background(), raw)
	if err != nil || !strings.Contains(result.Content, "Birch Hall") || strings.Contains(result.Content, "mango") {
		t.Fatal(result.Content, err)
	}
	goals, _ := goal.Open(root)
	g, err := goals.Add("Private goal", "", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = goals.AttachConversation(g.ID, c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = (chatRead{h}).Execute(context.Background(), raw); err == nil {
		t.Fatal("other-goal chat leaked into personal history")
	}
	result, err = (chatSearch{h}).Execute(context.Background(), json.RawMessage(`{"query":"Birch Hall"}`))
	if err != nil || strings.Contains(result.Content, "Birch Hall") {
		t.Fatal("other-goal search leaked", result.Content, err)
	}
	h.GoalID = g.ID
	if _, err = (chatRead{h}).Execute(context.Background(), raw); err != nil {
		t.Fatal("current goal chat unavailable", err)
	}
	h.CurrentConversation = c.ID
	if _, err = (chatRead{h}).Execute(context.Background(), raw); err == nil {
		t.Fatal("current transcript duplicated into history retrieval")
	}
	notes, _ := knowledge.Open(root)
	memories, _ := memory.Open(root)
	if tools := (PersonalKnowledge{Notes: notes, Memories: memories}).Tools(); len(tools) != 4 {
		t.Fatal("read-only knowledge acquired mutation tools")
	}
}
