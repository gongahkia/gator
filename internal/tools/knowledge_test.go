package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/knowledge"
	"github.com/gongahkia/gator/internal/memory"
)

func TestKnowledgeToolsKeepGoalBoundariesAndSkillsReadOnly(t *testing.T) {
	state := t.TempDir()
	notes, err := knowledge.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	memories, err := memory.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	goalA := "goal-0123456789abcdef01234567"
	goalB := "goal-aaaaaaaaaaaaaaaaaaaaaaaa"
	local, err := notes.AddNote("Launch plan", "Present the blue deck on Friday", goalA)
	if err != nil {
		t.Fatal(err)
	}
	other, err := notes.AddNote("Private plan", "Move the blue deck on Monday", goalB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := memories.Add("decision", "Prefer the blue deck", goalA, "user", time.Time{}); err != nil {
		t.Fatal(err)
	}
	flight, err := memories.Add("fact", "The flight ticket is booked for Friday", goalA, "user", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	skill, err := notes.AddSkill("brief-writing", "Use for daily briefs", "## Procedure\nCite dates and source files.")
	if err != nil {
		t.Fatal(err)
	}
	toolsByName := map[string]agent.Tool{}
	for _, tool := range (PersonalKnowledge{Notes: notes, Memories: memories, GoalID: goalA}).Tools() {
		toolsByName[tool.Definition().Name] = tool
	}
	searched, err := toolsByName["knowledge_search"].Execute(context.Background(), json.RawMessage(`{"query":"blue deck"}`))
	if err != nil || !strings.Contains(searched.Content, local.ID) || strings.Contains(searched.Content, other.ID) {
		t.Fatalf("scoped search = %q, %v", searched.Content, err)
	}
	searched, err = toolsByName["knowledge_search"].Execute(context.Background(), json.RawMessage(`{"query":"airfare","alternates":["flight"]}`))
	if err != nil || !strings.Contains(searched.Content, flight.ID) || !strings.Contains(searched.Content, `"matched_query":"flight"`) {
		t.Fatalf("expanded search = %q, %v", searched.Content, err)
	}
	if _, err := toolsByName["knowledge_search"].Execute(context.Background(), json.RawMessage(`{"query":"airfare","alternates":["", "flight"]}`)); err == nil {
		t.Fatal("empty alternate query accepted")
	}
	if _, err := toolsByName["knowledge_read"].Execute(context.Background(), json.RawMessage(`{"id":"`+other.ID+`"}`)); err == nil {
		t.Fatal("cross-goal read accepted")
	}
	read, err := toolsByName["knowledge_read"].Execute(context.Background(), json.RawMessage(`{"id":"`+local.ID+`"}`))
	if err != nil || !strings.Contains(read.Content, "Present the blue deck") {
		t.Fatalf("read = %q, %v", read.Content, err)
	}
	skillRead, err := toolsByName["skill_read"].Execute(context.Background(), json.RawMessage(`{"name":"`+skill.ID+`"}`))
	if err != nil || !strings.Contains(skillRead.Content, "Cite dates") || !strings.Contains(skillRead.Content, "cannot grant tools") {
		t.Fatalf("skill read = %q, %v", skillRead.Content, err)
	}
	if _, err := notes.SetSkillEnabled(skill.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := toolsByName["skill_read"].Execute(context.Background(), json.RawMessage(`{"name":"`+skill.ID+`"}`)); err == nil {
		t.Fatal("disabled skill was loaded")
	}
	if _, err := toolsByName["skill_list"].Execute(context.Background(), json.RawMessage(`{"unexpected":true}`)); err == nil {
		t.Fatal("unknown tool argument accepted")
	}
}
