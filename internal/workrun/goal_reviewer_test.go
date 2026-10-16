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

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/inbox"
	"github.com/gongahkia/gator/internal/knowledge"
	"github.com/gongahkia/gator/internal/memory"
	"github.com/gongahkia/gator/internal/worksession"
)

func TestGoalReviewSnapshotIncludesCurrentBranchAndScopedFollowups(t *testing.T) {
	state, source := t.TempDir(), t.TempDir()
	goals, err := goal.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	record, err := goals.Add("Prepare the offsite", "Venue decision due Monday", source)
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := worksession.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := sessions.Create("Offsite", record.SourcePath, "snap-first", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, revision := range []worksession.Revision{
		{ID: "rev-first", SnapshotID: "snap-first", Objective: "Compare venues", BundlePath: "/bundle/first", Status: "completed", FinalText: "Venue A is available"},
		{ID: "rev-current", ParentRevisionID: "rev-first", SnapshotID: "snap-first", Objective: "Check availability", BundlePath: "/bundle/current", Status: "completed", FinalText: "Venue A is unavailable"},
	} {
		if _, err := sessions.AddRevision(conversation.ID, revision); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := goals.AttachConversation(record.ID, conversation.ID); err != nil {
		t.Fatal(err)
	}
	notes, err := knowledge.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := notes.AddNote("Venue checklist", "Confirm capacity and accessibility", record.ID); err != nil {
		t.Fatal(err)
	}
	memories, err := memory.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := memories.Add("decision", "Venue B is preferred", record.ID, "user", time.Time{}); err != nil {
		t.Fatal(err)
	}
	inboxStore, err := inbox.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := inboxStore.Add(inbox.Entry{Kind: "job", Title: "Offsite check", Summary: "Venue A changed availability", GoalID: record.ID, Status: "completed"}); err != nil {
		t.Fatal(err)
	}
	if _, err := inboxStore.Add(inbox.Entry{Kind: "job", Title: "Unrelated", Summary: "Secret unrelated result", GoalID: "goal-aaaaaaaaaaaaaaaaaaaaaaaa", Status: "completed"}); err != nil {
		t.Fatal(err)
	}
	result, err := (goalReviewSnapshot{StateDir: state, GoalID: record.ID}).Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"Prepare the offsite", "Venue A is unavailable", "Venue decision due Monday", "Venue checklist", "Venue B is preferred", "Venue A changed availability"} {
		if !strings.Contains(result.Content, expected) {
			t.Fatalf("snapshot omitted %q: %s", expected, result.Content)
		}
	}
	if strings.Contains(result.Content, "Secret unrelated result") {
		t.Fatalf("cross-goal follow-up leaked: %s", result.Content)
	}
	if _, err := (goalReviewSnapshot{StateDir: state, GoalID: record.ID}).Execute(context.Background(), json.RawMessage(`{"write":true}`)); err == nil {
		t.Fatal("unexpected mutation argument accepted")
	}
}

func TestGoalReviewerDelegationHasOnlyReadTools(t *testing.T) {
	state, source := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "status.txt"), []byte("Deadline moved to Tuesday"), 0600); err != nil {
		t.Fatal(err)
	}
	goals, err := goal.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	record, err := goals.Add("Track launch", "Check the current deadline", source)
	if err != nil {
		t.Fatal(err)
	}
	manager := &scriptedModel{turns: []agent.Turn{{ToolCalls: []agent.ToolCall{{ID: "delegate", Name: "delegate_agents", Arguments: json.RawMessage(`{"tasks":[{"agent":"goal_reviewer","task":"Find the current deadline and next step"}]}`)}}}, {Text: "The deadline moved to Tuesday."}}}
	child := &scriptedModel{turns: []agent.Turn{{ToolCalls: []agent.ToolCall{{ID: "snapshot", Name: "goal_review_snapshot", Arguments: json.RawMessage(`{}`)}}}, {Text: "Next step: revise the launch plan for Tuesday."}}}
	outcome, err := (Executor{Model: manager, RoleModels: map[string]agent.Model{"goal_reviewer": child}, StateDir: state}).Execute(context.Background(), Request{SourcePath: record.SourcePath, GoalID: record.ID, GoalContext: record.Context, Objective: "What changed in my launch plan?", RunID: "goal-review-integration", Mode: action.Inspect, Contract: artifact.InspectionContract(), MaxSteps: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(outcome.Manifest.Subagents) != 1 || outcome.Manifest.Subagents[0].Agent != "goal_reviewer" || outcome.Manifest.Subagents[0].Status != "completed" {
		t.Fatalf("subagent evidence = %#v", outcome.Manifest.Subagents)
	}
	if len(child.requests) < 2 {
		t.Fatalf("child calls = %d", len(child.requests))
	}
	names := map[string]bool{}
	for _, tool := range child.requests[0].Tools {
		names[tool.Name] = true
	}
	for _, required := range []string{"goal_review_snapshot", "read_file", "list_files", "search_files", "knowledge_search", "knowledge_read"} {
		if !names[required] {
			t.Fatalf("goal reviewer missing %s: %#v", required, names)
		}
	}
	for _, forbidden := range []string{"write_artifact", "run_command", "process_execute", "delegate_agents", "connector_send"} {
		if names[forbidden] {
			t.Fatalf("goal reviewer received %s", forbidden)
		}
	}
	if !strings.Contains(child.requests[1].Messages[len(child.requests[1].Messages)-1].Content, "Track launch") {
		t.Fatalf("goal snapshot not delivered to specialist: %#v", child.requests[1].Messages)
	}
}

func TestGoalReviewSnapshotBoundsLongHistory(t *testing.T) {
	state, source := t.TempDir(), t.TempDir()
	goals, err := goal.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	record, err := goals.Add("Long project", strings.Repeat("context ", 1000), source)
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := worksession.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, 2)
	for branch := 0; branch < 2; branch++ {
		conversation, err := sessions.Create("Long project", record.SourcePath, "snap-long", time.Now())
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, conversation.ID)
		parent := ""
		for n := 0; n < 5; n++ {
			id := fmt.Sprintf("rev-long-%d-%d", branch, n)
			if _, err := sessions.AddRevision(conversation.ID, worksession.Revision{ID: id, ParentRevisionID: parent, SnapshotID: "snap-long", Objective: strings.Repeat("objective ", 100), BundlePath: "/bundle/long", Status: "completed", FinalText: strings.Repeat("result ", 400)}); err != nil {
				t.Fatal(err)
			}
			parent = id
		}
	}
	if _, err := goals.AttachConversation(record.ID, ids[0]); err != nil {
		t.Fatal(err)
	}
	inboxStore, err := inbox.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 8; n++ {
		if _, err := inboxStore.Add(inbox.Entry{Kind: "job", Title: "Long update", Summary: strings.Repeat("followup ", 400), GoalID: record.ID, Status: "completed"}); err != nil {
			t.Fatal(err)
		}
	}
	result, err := (goalReviewSnapshot{StateDir: state, GoalID: record.ID, ConversationID: ids[1]}).Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Content) > 32*1024 || strings.Count(result.Content, "rev-long-") != 10 || strings.Count(result.Content, "followup ") < 8 {
		t.Fatalf("bounded snapshot omitted history or exceeded limit: %d bytes", len(result.Content))
	}
	if got := boundedReview("hello 🌿 world", 8); got != "hello …" {
		t.Fatalf("UTF-8 preview = %q", got)
	}
}
