package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/knowledge"
	"github.com/gongahkia/gator/internal/suggestion"
)

func TestReviewSuggestionsActivatesOnlyAcceptedChanges(t *testing.T) {
	state := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", state)
	queue, err := suggestion.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	note, err := queue.Create(suggestion.Record{Kind: "note", Title: "Venue", Body: "Capacity is 90", WorkID: "work-1"})
	if err != nil {
		t.Fatal(err)
	}
	knowledgeStore, err := knowledge.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	if notes, err := knowledgeStore.ListNotes(""); err != nil || len(notes) != 0 {
		t.Fatalf("pending note became active: %#v, %v", notes, err)
	}
	var output bytes.Buffer
	if err := suggestionCommand([]string{"list"}, &output); err != nil || !strings.Contains(output.String(), note.ID) {
		t.Fatalf("pending list = %q, %v", output.String(), err)
	}
	if err := suggestionCommand([]string{"accept", note.ID}, &output); err != nil {
		t.Fatal(err)
	}
	accepted, err := queue.Load(note.ID)
	if err != nil || accepted.Status != "accepted" {
		t.Fatalf("accepted = %#v, %v", accepted, err)
	}
	if saved, err := knowledgeStore.LoadNote(accepted.ResultID); err != nil || saved.Body != note.Body || len(saved.SourceRefs) != 1 || saved.SourceRefs[0] != "work:work-1" {
		t.Fatalf("saved note = %#v, %v", saved, err)
	}
	if err := suggestionCommand([]string{"accept", note.ID}, &output); err == nil {
		t.Fatal("accepted suggestion applied twice")
	}
	skill, err := queue.Create(suggestion.Record{Kind: "skill", SkillID: "daily-brief", Description: "Use for a daily briefing", Body: "Read calendar, then email.", WorkID: "work-2"})
	if err != nil {
		t.Fatal(err)
	}
	if err := suggestionCommand([]string{"reject", skill.ID}, &output); err != nil {
		t.Fatal(err)
	}
	if _, err := knowledgeStore.LoadSkill("daily-brief"); err == nil {
		t.Fatal("rejected skill became active")
	}
}

func TestAcceptedPlanSuggestionChecksCurrentRevision(t *testing.T) {
	state := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", state)
	goals, err := goal.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	record, err := goals.Add("Plan offsite", "", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	queue, err := suggestion.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := queue.Create(suggestion.Record{Kind: "plan", GoalID: record.ID, WorkID: "work-1", PlanChange: goal.PlanChange{Kind: "add_step", ExpectedRevision: 0, Title: "Confirm capacity"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := goals.ApplyPlanChange(record.ID, goal.PlanChange{Kind: "set_outcome", ExpectedRevision: 0, Outcome: "Choose a venue"}); err != nil {
		t.Fatal(err)
	}
	if err := suggestionCommand([]string{"accept", proposal.ID}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "expected revision") {
		t.Fatalf("stale plan suggestion applied: %v", err)
	}
	loaded, err := queue.Load(proposal.ID)
	if err != nil || loaded.Status != "pending" {
		t.Fatalf("stale proposal status = %#v, %v", loaded, err)
	}
}
