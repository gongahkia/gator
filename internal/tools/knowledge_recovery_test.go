package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/knowledge"
	"github.com/gongahkia/gator/internal/memory"
)

func TestKnowledgeSearchRetainsHealthyResultsAndReportsPartialMemory(t *testing.T) {
	state := t.TempDir()
	memories, err := memory.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	notes, err := knowledge.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	healthy, err := memories.Add("fact", "Harbor project starts Monday", "", "fixture", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	bad, err := memories.Add("fact", "Synthetic bad fact", "", "fixture", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state, "gator", "memory", bad.ID+".json"), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	tool := knowledgeSearch{PersonalKnowledge{Notes: notes, Memories: memories}}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"query":"harbor"}`))
	if err != nil || !strings.Contains(result.Content, healthy.ID) || !strings.Contains(result.Content, `"warning"`) || !strings.Contains(result.Content, "unreadable") || strings.Contains(result.Content, bad.ID) {
		t.Fatalf("partial tool result: %s %v", result.Content, err)
	}
	tool.ExcludedIDs = []string{healthy.ID}
	result, err = tool.Execute(context.Background(), json.RawMessage(`{"query":"harbor"}`))
	if err != nil || strings.Contains(result.Content, healthy.ID) || !strings.Contains(result.Content, "unreadable") {
		t.Fatalf("exclusions changed during recovery: %s %v", result.Content, err)
	}
}
