package main

import (
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/knowledge"
	"github.com/gongahkia/gator/internal/memory"
)

func TestWorkKnowledgeResultsReadStructuredStores(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", stateDir)
	memories, err := memory.Open(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	memoryRecord, err := memories.Add("preference", "Keep daily briefs concise", "", "user", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := workMemoryResult(stateDir, "", []string{"list"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Kind != "list" || len(result.Items) != 1 || result.Items[0].Detail != memoryRecord.ID {
		t.Fatalf("memory list result = %#v", result)
	}
	result, err = workMemoryResult(stateDir, "", []string{"show", memoryRecord.ID})
	if err != nil {
		t.Fatal(err)
	}
	if result.Kind != "details" || result.Summary != memoryRecord.Text {
		t.Fatalf("memory detail = %#v", result)
	}

	store, err := knowledge.Open(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	note, err := store.AddNote("Launch brief", "# Today\nReview the launch deck.", "")
	if err != nil {
		t.Fatal(err)
	}
	result, err = workNoteResult("", "list")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || result.Items[0].Label != note.Title {
		t.Fatalf("note list = %#v", result)
	}
	result, err = workNoteResult("", "show "+note.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Markdown != note.Body {
		t.Fatalf("note Markdown lost: %#v", result)
	}

	skill, err := store.AddSkill("brief-writer", "Use for daily briefs", "# Steps\n1. Read sources.")
	if err != nil {
		t.Fatal(err)
	}
	result, err = workSkillResult("list")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || result.Items[0].Label != skill.ID || result.Items[0].State != "enabled" {
		t.Fatalf("skill list = %#v", result)
	}
	result, err = workSkillResult("show " + skill.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Markdown != skill.Body {
		t.Fatalf("skill Markdown lost: %#v", result)
	}
}
