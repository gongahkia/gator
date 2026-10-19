package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/knowledge"
	"github.com/gongahkia/gator/internal/memory"
)

func TestUserExcludedKnowledgeCannotBeSearchedOrRead(t *testing.T) {
	state := t.TempDir()
	memories, _ := memory.Open(state)
	notes, _ := knowledge.Open(state)
	record, err := memories.Add("preference", "Project planning replies should be concise.", "", "user", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	note, err := notes.AddNote("Project planning", "Reply to Mina.", "")
	if err != nil {
		t.Fatal(err)
	}
	p := PersonalKnowledge{Notes: notes, Memories: memories, ExcludedIDs: []string{record.ID, note.ID}}
	result, err := (knowledgeSearch{p}).Execute(context.Background(), json.RawMessage(`{"query":"project planning"}`))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result.Content, record.ID) || strings.Contains(result.Content, note.ID) {
		t.Fatal("excluded item was searchable")
	}
	for _, id := range p.ExcludedIDs {
		raw, _ := json.Marshal(map[string]string{"id": id})
		if _, err := (knowledgeRead{p}).Execute(context.Background(), raw); err == nil {
			t.Fatal("excluded item could be read")
		}
	}
}
