package run

import (
	"context"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/agent"
)

func TestCompactMessagesSummarizesOnlyOlderHistory(t *testing.T) {
	messages := make([]agent.Message, 0, recentMessagesToKeep+2)
	for index := 0; index < recentMessagesToKeep+2; index++ {
		messages = append(messages, agent.Message{Role: agent.RoleUser, Content: strings.Repeat(string(rune('a'+index%26)), 8*1024)})
	}
	model := &scriptedModel{turns: []agent.Turn{{Text: "Objective: finish the parser. Changed files: parser.go. Verification: pending."}}}
	compacted, summary, didCompact, err := compactMessages(context.Background(), model, messages, false)
	if err != nil {
		t.Fatalf("compact messages: %v", err)
	}
	if !didCompact || !strings.Contains(summary, "parser.go") || len(compacted) != recentMessagesToKeep+1 {
		t.Fatalf("compaction result = compacted:%t summary:%q messages:%d", didCompact, summary, len(compacted))
	}
	if !strings.HasPrefix(compacted[0].Content, compactionPrefix) || compacted[1].Content != messages[2].Content {
		t.Fatalf("compacted history = %#v", compacted[:2])
	}
	if len(model.requests) != 1 || len(model.requests[0].Messages) != 2 {
		t.Fatalf("compaction request = %#v", model.requests)
	}
}

func TestCompactMessagesLeavesBoundedHistoryUntouched(t *testing.T) {
	messages := []agent.Message{{Role: agent.RoleUser, Content: "small history"}}
	compacted, summary, didCompact, err := compactMessages(context.Background(), &scriptedModel{}, messages, false)
	if err != nil || didCompact || summary != "" || len(compacted) != 1 || compacted[0].Content != "small history" {
		t.Fatalf("compaction result = %#v, %q, %t, %v", compacted, summary, didCompact, err)
	}
}

func TestCompactMessagesAllowsExplicitCompactionOfShortHistory(t *testing.T) {
	messages := make([]agent.Message, 20)
	for index := range messages {
		messages[index] = agent.Message{Role: agent.RoleUser, Content: "short"}
	}
	model := &scriptedModel{turns: []agent.Turn{{Text: "preserved decisions"}}}
	compacted, summary, didCompact, err := compactMessages(context.Background(), model, messages, true)
	if err != nil {
		t.Fatalf("compact messages: %v", err)
	}
	if !didCompact || summary != "preserved decisions" {
		t.Fatalf("expected explicit compaction, got compacted=%v summary=%q", didCompact, summary)
	}
	if len(compacted) != recentMessagesToKeep+1 || compacted[0].Content != compactionPrefix+"preserved decisions" {
		t.Fatalf("unexpected compacted history: %#v", compacted)
	}
}

func TestCompactMessagesRejectsToolCallingSummaries(t *testing.T) {
	messages := make([]agent.Message, 20)
	for index := range messages {
		messages[index] = agent.Message{Role: agent.RoleUser, Content: "short"}
	}
	model := &scriptedModel{turns: []agent.Turn{{Text: "ignored", ToolCalls: []agent.ToolCall{{ID: "call-1", Name: "read_file", Arguments: []byte(`{}`)}}}}}
	compacted, summary, didCompact, err := compactMessages(context.Background(), model, messages, true)
	if err == nil || didCompact || summary != "" || compacted != nil {
		t.Fatalf("tool-calling summary = %#v %q %t %v", compacted, summary, didCompact, err)
	}
}

func TestCompactMessagesDoesNotReturnPartialHistoryOnFailure(t *testing.T) {
	messages := make([]agent.Message, 20)
	for index := range messages {
		messages[index] = agent.Message{Role: agent.RoleUser, Content: "keep-original"}
	}
	model := &scriptedModel{}
	compacted, summary, didCompact, err := compactMessages(context.Background(), model, messages, true)
	if err == nil || didCompact || summary != "" || compacted != nil {
		t.Fatalf("failed compaction = %#v %q %t %v", compacted, summary, didCompact, err)
	}
}
