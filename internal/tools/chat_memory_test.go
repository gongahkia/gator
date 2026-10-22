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
	forgotten, _ := json.Marshal(map[string]string{"id": records[0].ID, "quote": "Please forget my reply preference.", "expected_updated": records[0].UpdatedAt.Format(time.RFC3339Nano)})
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

func TestMultilingualMemoryRequestsNeedExactHostReview(t *testing.T) {
	for _, quote := range []string{"请记住，我喜欢简短的回复。", "Recuerda que prefiero respuestas breves.", "Tolong ingat saya suka jawapan ringkas."} {
		t.Run(quote, func(t *testing.T) {
			store, _ := memory.Open(t.TempDir())
			m := ChatMemory{Store: store, UserMessage: quote, ConversationID: "work-one", RunID: "run-one", Automatic: false}
			raw, _ := json.Marshal(map[string]any{"subject": "preference.response_length", "kind": "preference", "quote": quote, "scope": "personal", "explicit": true})
			if _, err := (rememberChat{m}).Execute(context.Background(), raw); err == nil {
				t.Fatal("unrecognized intent authorized without review")
			}
			for _, approve := range []bool{false, true} {
				calls := 0
				m.Review = func(_ context.Context, preview any) (bool, error) {
					calls++
					if preview.(map[string]any)["saved_text"] != quote {
						t.Fatal("review changed user text")
					}
					return approve, nil
				}
				_, err := (rememberChat{m}).Execute(context.Background(), raw)
				if calls != 1 || approve != (err == nil) {
					t.Fatal("review ignored", calls, err)
				}
				records, _ := store.List("", false)
				if !approve && len(records) != 0 || approve && len(records) != 1 {
					t.Fatal("wrong saved state", records)
				}
			}
			records, _ := store.List("", false)
			m.UserMessage = "请忘记我的回复偏好。"
			forget, _ := json.Marshal(map[string]string{"id": records[0].ID, "quote": m.UserMessage, "expected_updated": records[0].UpdatedAt.Format(time.RFC3339Nano)})
			m.Review = func(_ context.Context, preview any) (bool, error) {
				if preview.(map[string]any)["saved_text"] != quote {
					t.Fatal("forget omitted exact saved text")
				}
				return false, nil
			}
			if _, err := (forgetChat{m}).Execute(context.Background(), forget); err == nil {
				t.Fatal("declined forget executed")
			}
			m.Review = func(context.Context, any) (bool, error) { return true, nil }
			if _, err := (forgetChat{m}).Execute(context.Background(), forget); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestMemoryForgetReviewCannotDeleteConcurrentCorrection(t *testing.T) {
	store, _ := memory.Open(t.TempDir())
	quote := "I like short replies."
	record, err := store.RememberChat(memory.ChatInput{Subject: "preference.response_length", Kind: "preference", Quote: quote, ConversationID: "work-one", RunID: "run-one"})
	if err != nil {
		t.Fatal(err)
	}
	m := ChatMemory{Store: store, UserMessage: "请忘记这条记忆。", ConversationID: "work-two", RunID: "run-two", Review: func(context.Context, any) (bool, error) {
		_, err := store.Correct(record.ID, "I now want detailed replies.", record.Source)
		return true, err
	}}
	raw, _ := json.Marshal(map[string]string{"id": record.ID, "quote": m.UserMessage, "expected_updated": record.UpdatedAt.Format(time.RFC3339Nano)})
	if _, err := (forgetChat{m}).Execute(context.Background(), raw); err == nil {
		t.Fatal("stale review deleted newer correction")
	}
	retained, err := store.Load(record.ID)
	if err != nil || retained.Text != "I now want detailed replies." {
		t.Fatal(retained, err)
	}
}

func TestMemoryForgetUsesKnowledgeReadRevision(t *testing.T) {
	store, err := memory.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.RememberChat(memory.ChatInput{Subject: "preference.response_length", Kind: "preference", Quote: "I like short replies.", ConversationID: "work-one", RunID: "run-one"})
	if err != nil {
		t.Fatal(err)
	}
	read := func() string {
		t.Helper()
		raw, _ := json.Marshal(map[string]string{"id": record.ID})
		result, err := (knowledgeRead{PersonalKnowledge{Memories: store}}).Execute(context.Background(), raw)
		if err != nil {
			t.Fatal(err)
		}
		var inspected struct{ Result struct{ Updated string } }
		if err := json.Unmarshal([]byte(result.Content), &inspected); err != nil {
			t.Fatal(err)
		}
		if inspected.Result.Updated == "" {
			t.Fatal("knowledge_read omitted the exact memory revision")
		}
		return inspected.Result.Updated
	}
	inspected := read()
	m := ChatMemory{Store: store, UserMessage: "Please forget my reply preference.", ConversationID: "work-two", RunID: "run-two"}
	forget := func(version string) error {
		raw, _ := json.Marshal(map[string]string{"id": record.ID, "quote": m.UserMessage, "expected_updated": version})
		_, err := (forgetChat{m}).Execute(context.Background(), raw)
		return err
	}
	for _, version := range []string{"", "yesterday", "2026-10-23"} {
		if err := forget(version); err == nil {
			t.Fatal("forgot without an inspected revision", version)
		}
	}
	if _, err := store.Correct(record.ID, "I now want detailed replies.", record.Source); err != nil {
		t.Fatal(err)
	}
	if err := forget(inspected); err == nil {
		t.Fatal("old knowledge inspection deleted a newer correction")
	}
	retained, err := store.Load(record.ID)
	if err != nil || retained.Text != "I now want detailed replies." {
		t.Fatal(retained, err)
	}
	if err := forget(read()); err != nil {
		t.Fatal("fresh inspection could not forget memory", err)
	}
	if _, err := store.Load(record.ID); err == nil {
		t.Fatal("successful forget retained the memory")
	}
}

func TestMemoryForgetRejectsNegativeIntentEvenWithCroppedQuote(t *testing.T) {
	store, err := memory.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.RememberChat(memory.ChatInput{Subject: "preference.response_length", Kind: "preference", Quote: "I like short replies.", ConversationID: "work-one", RunID: "run-one"})
	if err != nil {
		t.Fatal(err)
	}
	for _, user := range []string{
		"Please do not forget my reply preference.",
		"Don't forget my reply preference.",
		"Don’t forget my reply preference.",
		"Never ever forget my reply preference.",
		"Remember not to forget my reply preference.",
		"I don't want you to forget my reply preference.",
		"You should not delete my reply preference.",
		"Please do not remove my reply preference.",
		"Don't stop remembering my reply preference.",
	} {
		t.Run(user, func(t *testing.T) {
			reviews := 0
			m := ChatMemory{Store: store, UserMessage: user, ConversationID: "work-two", RunID: "run-two", Review: func(context.Context, any) (bool, error) { reviews++; return true, nil }}
			for _, quote := range []string{user, "my reply preference."} {
				raw, _ := json.Marshal(map[string]string{"id": record.ID, "quote": quote, "expected_updated": record.UpdatedAt.Format(time.RFC3339Nano)})
				if _, err := (forgetChat{m}).Execute(context.Background(), raw); err == nil || !strings.Contains(err.Error(), "not to forget") {
					t.Fatal("negative intent did not reject deletion", quote, err)
				}
			}
			if reviews != 0 {
				t.Fatal("negative request should not ask to authorize deletion")
			}
			if _, err := store.Load(record.ID); err != nil {
				t.Fatal("negative forget removed the memory", err)
			}
		})
	}
}

func TestHistorySearchAcceptsShortAndCombiningScriptTerms(t *testing.T) {
	root := t.TempDir()
	sessions, _ := worksession.Open(root)
	chat, _ := sessions.CreateChat(t.TempDir(), time.Now())
	if _, err := sessions.AddRevision(chat.ID, worksession.Revision{ID: "rev-language", Objective: "東京の旅行。猫。यह मेरी दिल्ली यात्रा है।AI research with Bo.", SnapshotID: "snap-test", BundlePath: "/bundle/root", Status: "completed"}); err != nil {
		t.Fatal(err)
	}
	h := ChatHistory{StateDir: root, CurrentConversation: "work-current"}
	for _, query := range []string{"東京", "猫", "दिल्ली", "Bo", "AI"} {
		raw, _ := json.Marshal(map[string]string{"query": query})
		result, err := (chatSearch{h}).Execute(context.Background(), raw)
		if err != nil || !strings.Contains(result.Content, chat.ID) {
			t.Fatal("query did not recover retained source", query, result.Content, err)
		}
	}
}
