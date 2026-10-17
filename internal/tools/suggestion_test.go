package tools

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/gongahkia/gator/internal/suggestion"
)

func TestSuggestChangeCanProposeGoalOnlyFromMainConversation(t *testing.T) {
	store, err := suggestion.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	source := t.TempDir()
	source, err = filepath.EvalSymlinks(source)
	if err != nil {
		t.Fatal(err)
	}
	tool := SuggestChange{Store: store, WorkID: "work-chat", SourcePath: source}
	request := json.RawMessage(`{"kind":"goal","title":"Plan the offsite","body":"Decide this month","goal_outcome":"Choose a venue","goal_steps":["Check capacity"],"use_current_source":true}`)
	if _, err := tool.Execute(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	pending, err := store.List("pending")
	if err != nil || len(pending) != 1 || pending[0].GoalSourcePath != source || pending[0].GoalOutcome != "Choose a venue" {
		t.Fatalf("goal proposal = %#v, %v", pending, err)
	}
	tool.GoalID = "goal-000000000000000000000000"
	if _, err := tool.Execute(context.Background(), request); err == nil {
		t.Fatal("existing goal conversation proposed a new goal")
	}
	if _, err := store.Create(suggestion.Record{Kind: "goal", WorkID: "work-chat", Title: "Invalid", GoalOutcome: "Choose", GoalSteps: []string{"Step"}, Cron: "* * * * *"}); err == nil {
		t.Fatal("follow-up schedule without task or timezone accepted")
	}
}

func TestSuggestChangeQueuesScopedProposal(t *testing.T) {
	store, err := suggestion.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	tool := SuggestChange{Store: store, WorkID: "work-1"}
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"kind":"plan","plan_change":{"kind":"add_step","title":"Check source"}}`)); err == nil {
		t.Fatal("unbound goal plan proposal accepted")
	}
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"kind":"schedule","cron":"0 9 * * *","timezone":"UTC","task":"Check"}`)); err == nil {
		t.Fatal("unbound goal schedule proposal accepted")
	}
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"kind":"note","title":"Preference","body":"Use short briefs"}`)); err != nil {
		t.Fatal(err)
	}
	records, err := store.List("pending")
	if err != nil || len(records) != 1 || records[0].Kind != "note" || records[0].WorkID != "work-1" {
		t.Fatalf("queued = %#v, %v", records, err)
	}
}
