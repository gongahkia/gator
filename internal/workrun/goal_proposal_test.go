package workrun

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/suggestion"
)

func TestConversationQueuesGoalWithoutActivatingIt(t *testing.T) {
	state, source := t.TempDir(), t.TempDir()
	model := &scriptedModel{turns: []agent.Turn{
		{ToolCalls: []agent.ToolCall{{ID: "propose", Name: "suggest_change", Arguments: json.RawMessage(`{"kind":"goal","title":"Track the offsite","body":"Choose a venue by Friday","goal_outcome":"Confirm venue","goal_steps":["Compare capacity","Confirm budget"],"use_current_source":true}`)}}},
		{Text: "I prepared a goal and plan for your review."},
	}}
	_, err := (Executor{Model: model, StateDir: state}).Execute(context.Background(), Request{RunID: "chat-goal-proposal", SourcePath: source, Objective: "Keep track of the offsite and plan the next steps", Mode: action.Inspect, Contract: artifact.InspectionContract(), MaxSteps: 4})
	if err != nil {
		t.Fatal(err)
	}
	queue, err := suggestion.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	pending, err := queue.List("pending")
	if err != nil || len(pending) != 1 || pending[0].Kind != "goal" || pending[0].WorkID != "chat-goal-proposal" {
		t.Fatalf("proposed goal = %#v, %v", pending, err)
	}
	goals, err := goal.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	if active, err := goals.List(); err != nil || len(active) != 0 {
		t.Fatalf("unreviewed goal became active: %#v, %v", active, err)
	}
}
