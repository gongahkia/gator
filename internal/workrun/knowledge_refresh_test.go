package workrun

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/knowledge"
	"github.com/gongahkia/gator/internal/memory"
	"github.com/gongahkia/gator/internal/worksession"
)

func TestRetainedKnowledgeReferencesIgnoreConversationTextAndBoundRecentAncestry(t *testing.T) {
	one, two := "mem-"+strings.Repeat("a", 24), "note-"+strings.Repeat("b", 24)
	messages := []agent.Message{
		{Role: agent.RoleUser, Content: `{"ok":true,"result":{"id":"` + one + `"}}`, ToolName: "knowledge_read"},
		{Role: agent.RoleTool, ToolName: "knowledge_read", Content: `{"ok":false,"result":{"id":"` + one + `"}}`},
		{Role: agent.RoleTool, ToolName: "knowledge_search", Content: `{"ok":true,"result":{"items":[{"id":"` + two + `"},{"id":"../private"}],"warning":"partial"}}`},
	}
	refs, overflow := retainedKnowledgeReferences(messages, []string{two})
	if overflow || len(refs) != 1 || refs[0] != two {
		t.Fatal("invented/failed references entered continuation context", refs)
	}
	group := []string{one}
	for i := 0; i < 150; i++ {
		group = append(group, fmt.Sprintf("note-%024x", i))
	}
	refs, overflow = retainedKnowledgeReferences(messages, group)
	if !overflow || len(refs) != maxRetainedKnowledgeRefs || refs[0] != two || refs[1] != one {
		t.Fatal("reference budget or recent ancestry ordering lost", refs)
	}
}

func TestKnowledgeRefreshKeepsCurrentScopeExclusionsAndUnavailableStates(t *testing.T) {
	state := t.TempDir()
	notes, _ := knowledge.Open(state)
	memories, _ := memory.Open(state)
	now := time.Now()
	current, _ := memories.Add("fact", "Current fact", "", "user", time.Time{})
	foreign, _ := memories.Add("fact", "PRIVATE OTHER GOAL", "goal-"+strings.Repeat("b", 24), "private source", time.Time{})
	expired, _ := memories.Add("fact", "EXPIRED VALUE", "", "user", now.Add(-time.Hour))
	omitted, _ := memories.Add("fact", "OMITTED VALUE", "", "user", time.Time{})
	file := filepath.Join(t.TempDir(), "source.txt")
	if err := os.WriteFile(file, []byte("Original"), 0600); err != nil {
		t.Fatal(err)
	}
	linked, err := memories.AddFromFile("fact", "UNREVIEWED VALUE", "", file, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("Changed"), 0600); err != nil {
		t.Fatal(err)
	}
	bad, _ := memories.Add("fact", "CORRUPT VALUE", "", "user", time.Time{})
	if err := os.WriteFile(filepath.Join(state, "gator/memory", bad.ID+".json"), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	note, _ := notes.AddNote("Current note", "Current note body", "")
	gone, _ := notes.AddNote("Deleted note", "FORGOTTEN VALUE", "")
	if _, err := notes.ReviewNote(gone.ID, "", gone.Revision(), "forget", ""); err != nil {
		t.Fatal(err)
	}
	refs := []string{current.ID, foreign.ID, expired.ID, omitted.ID, linked.ID, bad.ID, note.ID, gone.ID}
	refresh := refreshKnowledge(refs, false, "", []string{omitted.ID}, notes, memories, now)
	expected := []string{"current", "outside_scope", "expired", "omitted", "needs_review", "unavailable", "current", "forgotten_or_missing"}
	for i, item := range refresh.Items {
		if item.Status != expected[i] {
			t.Fatal("wrong current status", item, expected[i])
		}
	}
	text := string(refresh.payload())
	for _, private := range []string{"PRIVATE OTHER GOAL", "private source", "EXPIRED VALUE", "OMITTED VALUE", "UNREVIEWED VALUE", "CORRUPT VALUE", "FORGOTTEN VALUE"} {
		if strings.Contains(text, private) {
			t.Fatal("unavailable data reached the model", private)
		}
	}
	if !strings.Contains(text, "Current note body") || !strings.Contains(text, "Current fact") {
		t.Fatal("available records missing", text)
	}
}

func TestKnowledgeRefreshBoundsEscapedTextAndKeepsUtf8(t *testing.T) {
	state := t.TempDir()
	notes, _ := knowledge.Open(state)
	memories, _ := memory.Open(state)
	refs := []string{}
	for i := 0; i < 40; i++ {
		note, err := notes.AddNote(fmt.Sprint("Note ", i), strings.Repeat("\x01λ🦦", 300), "")
		if err != nil {
			t.Fatal(err)
		}
		refs = append(refs, note.ID)
	}
	refresh := refreshKnowledge(refs, false, "", nil, notes, memories, time.Now())
	if !refresh.Overflow || len(refresh.Items) != maxKnowledgeRefreshItems || len(refresh.payload()) > maxKnowledgeRefreshBytes || !utf8.Valid(refresh.payload()) {
		t.Fatal("refresh exceeded its input budget", len(refresh.Items), len(refresh.payload()))
	}
	shortened, requiresRead := false, false
	for _, item := range refresh.Items {
		shortened = shortened || item.Truncated
		requiresRead = requiresRead || item.Status == "requires_read"
		if !utf8.ValidString(item.Body) {
			t.Fatal("preview split unicode")
		}
	}
	if !shortened || !requiresRead {
		t.Fatal("overflow was hidden rather than requiring an explicit full read")
	}
}

func TestContinuationRefreshesCorrectionsAndWithdrawalWithoutRewritingReplayOrSpendingRequests(t *testing.T) {
	state, source := t.TempDir(), t.TempDir()
	memories, _ := memory.Open(state)
	fact, _ := memories.Add("fact", "The delivery is Friday.", "", "user", time.Time{})
	model := &scriptedModel{turns: []agent.Turn{{ToolCalls: []agent.ToolCall{{ID: "read", Name: "knowledge_read", Arguments: json.RawMessage(`{"id":"` + fact.ID + `"}`)}}}, {Text: "Friday was supplied."}}}
	service := Service{Executor: Executor{Model: model, StateDir: state}}
	first, err := service.Execute(context.Background(), Request{RunID: "work-refresh-first", SourcePath: source, Objective: "Summarize.", Mode: action.Inspect, Contract: artifact.InspectionContract(), MaxSteps: 3})
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(filepath.Join(state, "gator/conversations", first.ConversationID, "revisions", first.RevisionID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := memories.Correct(fact.ID, "The delivery is Wednesday.", "user"); err != nil {
		t.Fatal(err)
	}
	calls := 0
	wantStatus, wantText := "current", "The delivery is Wednesday."
	service.Executor.Model = researchModel(func(request agent.TurnRequest) (agent.Turn, error) {
		calls++
		if !strings.Contains(request.System, wantStatus) || !strings.Contains(request.System, wantText) {
			t.Fatal("model did not receive authoritative saved status", request.System)
		}
		old := false
		for _, message := range request.Messages {
			old = old || message.Role == agent.RoleTool && strings.Contains(message.Content, "The delivery is Friday.")
		}
		if !old {
			t.Fatal("history was rewritten instead of supplemented")
		}
		return agent.Turn{Text: "The current status was supplied."}, nil
	})
	request := Request{RunID: "work-refresh-second", ConversationID: first.ConversationID, SourcePath: source, Objective: "Continue.", Mode: action.Inspect, Contract: artifact.InspectionContract(), MaxSteps: 2}
	second, err := service.Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal("refresh spent another model request", calls)
	}
	if current, err := os.ReadFile(filepath.Join(state, "gator/conversations", first.ConversationID, "revisions", first.RevisionID+".json")); err != nil || string(current) != string(original) {
		t.Fatal("retained revision changed", err)
	}
	sessions, _ := worksession.Open(state)
	revision, _ := sessions.LoadRevision(second.ConversationID, second.RevisionID)
	if len(revision.Replay.KnowledgeRefs) != 1 || revision.Replay.KnowledgeRefs[0] != fact.ID {
		t.Fatal("continuation lost its durable reference")
	}
	if err := memories.Forget(fact.ID); err != nil {
		t.Fatal(err)
	}
	wantStatus, wantText = "forgotten_or_missing", "forgotten_or_missing"
	request.RunID = "work-refresh-third"
	third, err := service.Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatal("withdrawal refresh spent another model request")
	}
	found := false
	for _, evidence := range third.Manifest.Evidence {
		if strings.Contains(evidence.Locator, "knowledge:work-refresh-third/knowledge_refresh/") {
			found = true
		}
	}
	if !found {
		t.Fatal("actual current status was not retained for inspection")
	}
}

func TestAutomaticKnowledgeReferencesSurviveCompactionAndAnotherContinuation(t *testing.T) {
	state, source := t.TempDir(), t.TempDir()
	memories, _ := memory.Open(state)
	notes, _ := knowledge.Open(state)
	fact, _ := memories.Add("fact", "The deadline is Friday.", "", "user", time.Time{})
	large, _ := notes.AddNote("Long retained notes", strings.Repeat("Historical reference line.\n", 1000), "")
	turns := []agent.Turn{}
	for i := 0; i < 7; i++ {
		turns = append(turns, agent.Turn{ToolCalls: []agent.ToolCall{{ID: fmt.Sprint("read-", i), Name: "knowledge_read", Arguments: json.RawMessage(`{"id":"` + large.ID + `"}`)}}})
	}
	turns = append(turns, agent.Turn{Text: "Earlier facts were supplied."})
	service := Service{Executor: Executor{Model: &scriptedModel{turns: turns}, StateDir: state}}
	first, err := service.Execute(context.Background(), Request{RunID: "work-refresh-auto", SourcePath: source, Objective: "Inspect.", MemoryContext: fact.Text, ContextRefs: []string{fact.ID}, Mode: action.Inspect, Contract: artifact.InspectionContract(), MaxSteps: 10})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := memories.Correct(fact.ID, "The deadline is Wednesday.", "user"); err != nil {
		t.Fatal(err)
	}
	summaries, runs := 0, 0
	service.Executor.Model = researchModel(func(request agent.TurnRequest) (agent.Turn, error) {
		if strings.Contains(request.System, "compact older context") {
			summaries++
			return agent.Turn{Text: "An earlier task and long notes were inspected."}, nil
		}
		runs++
		if !strings.Contains(request.System, "The deadline is Wednesday.") {
			t.Fatal("automatic remembered correction disappeared after compaction")
		}
		return agent.Turn{Text: "The current saved deadline is supplied."}, nil
	})
	r := Request{RunID: "work-refresh-compacted", ConversationID: first.ConversationID, SourcePath: source, Objective: "Continue.", Mode: action.Inspect, Contract: artifact.InspectionContract(), MaxSteps: 2}
	second, err := service.Execute(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if summaries != 1 || runs != 1 {
		t.Fatal("actual context compaction was not exercised", summaries, runs)
	}
	sessions, _ := worksession.Open(state)
	revision, _ := sessions.LoadRevision(second.ConversationID, second.RevisionID)
	if revision.Replay.Summary == "" || !strings.Contains(strings.Join(revision.Replay.KnowledgeRefs, ","), fact.ID) {
		t.Fatal("compacted replay lost its saved reference")
	}
	// A new executor/store load models restart without rewriting the prior run.
	service = Service{Executor: Executor{StateDir: state, Model: researchModel(func(request agent.TurnRequest) (agent.Turn, error) {
		if strings.Contains(request.System, "compact older context") {
			return agent.Turn{Text: "Earlier task context."}, nil
		}
		if !strings.Contains(request.System, "The deadline is Wednesday.") {
			t.Fatal("correction disappeared one continuation later")
		}
		return agent.Turn{Text: "Current status received after reopening."}, nil
	})}}
	r.RunID = "work-refresh-reopened"
	if _, err := service.Execute(context.Background(), r); err != nil {
		t.Fatal(err)
	}
}

func TestKnowledgeRefreshCannotWidenReviewedPlanEffects(t *testing.T) {
	state, source := t.TempDir(), t.TempDir()
	memories, _ := memory.Open(state)
	fact, _ := memories.Add("fact", "The deadline is Friday.", "", "user", time.Time{})
	first, err := (Service{Executor: Executor{StateDir: state, Model: &scriptedModel{turns: []agent.Turn{{ToolCalls: []agent.ToolCall{{ID: "read", Name: "knowledge_read", Arguments: json.RawMessage(`{"id":"` + fact.ID + `"}`)}}}, {Text: "Earlier deadline read."}}}}}).Execute(context.Background(), Request{SourcePath: source, Objective: "Inspect.", Mode: action.Inspect, Contract: artifact.InspectionContract(), MaxSteps: 3})
	if err != nil {
		t.Fatal(err)
	}
	sessions, _ := worksession.Open(state)
	chat, _ := sessions.Load(first.ConversationID)
	raw := json.RawMessage(`{"PlanFirst":true,"MaxSteps":24,"Code":{"MaxSteps":32},"ConnectorIDs":[],"WebOrigins":[]}`)
	journal, admitted, err := sessions.AdmitPlanRequest(chat.ID, "Create brief.md", raw, "knowledge-plan-review", nil, worksession.PlanBudget{MaxSteps: 24, CodeMaxSteps: 32}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	journal, err = sessions.ReviseTaskPlan(chat.ID, admitted.ID, 0, "", worksession.PlanContent{Outcome: "A reviewed read-only investigation", Steps: []worksession.PlanStep{{ID: "step-1", Text: "Inspect and summarize"}}, Effects: []string{"read"}})
	if err != nil {
		t.Fatal(err)
	}
	plan := journal.Requests[0].Plan
	journal, _, err = sessions.ApproveTaskPlan(chat.ID, admitted.ID, plan.Revision, plan.Digest)
	if err != nil {
		t.Fatal(err)
	}
	journal.Requests[0].State = "running"
	journal.Requests[0].Plan.Phase = "dispatching"
	if _, err := sessions.SaveRequests(chat.ID, journal); err != nil {
		t.Fatal(err)
	}
	if _, err := memories.Correct(fact.ID, "The deadline is Wednesday.", "user"); err != nil {
		t.Fatal(err)
	}
	calls := 0
	model := researchModel(func(request agent.TurnRequest) (agent.Turn, error) {
		calls++
		if !strings.Contains(request.System, "The deadline is Wednesday.") {
			t.Fatal("reviewed continuation lost the current fact")
		}
		for _, tool := range request.Tools {
			if tool.Name == "write_artifact" {
				t.Fatal("knowledge refresh granted a new write effect")
			}
		}
		return agent.Turn{ToolCalls: []agent.ToolCall{{ID: "attempt", Name: "write_artifact", Arguments: json.RawMessage(`{"path":"brief.md","content":"Must not be written"}`)}}}, nil
	})
	out, err := (Service{Executor: Executor{StateDir: state, Model: model}}).Execute(context.Background(), Request{SourcePath: chat.SourcePath, ConversationID: chat.ID, Objective: admitted.Text, Mode: action.Draft, Contract: artifact.DefaultContract("brief.md"), MaxSteps: 24, TaskPlan: &TaskPlanReference{RequestID: admitted.ID, Revision: plan.Revision, Digest: plan.Digest}})
	if err == nil || !strings.Contains(err.Error(), "reviewed plan does not include draft") || calls != 1 {
		t.Fatal("current reference data bypassed the reviewed scope", calls, err)
	}
	if _, err := os.Stat(filepath.Join(out.Work.Path, "output/brief.md")); !os.IsNotExist(err) {
		t.Fatal("unreviewed write executed", err)
	}
	latest, _ := sessions.LoadRequests(chat.ID)
	if latest.Requests[0].Plan.Digest != plan.Digest {
		t.Fatal("refresh changed the reviewed plan")
	}
}
