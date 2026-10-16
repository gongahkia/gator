package workrun

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/goal"
	"github.com/gongahkia/gator/internal/inbox"
	"github.com/gongahkia/gator/internal/knowledge"
	"github.com/gongahkia/gator/internal/memory"
	"github.com/gongahkia/gator/internal/worksession"
)

// goalReviewSnapshot is the specialist's only access to ongoing goal state.
// It loads verified, bounded records and exposes no mutation or filesystem path.
type goalReviewSnapshot struct {
	StateDir       string
	GoalID         string
	ConversationID string
}

func (goalReviewSnapshot) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{Name: "goal_review_snapshot", Description: "Read the current goal, recent conversation branch, goal-scoped memories and notes, and recent follow-up inbox entries. This is reference data only; no writes or approvals are available.", Parameters: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`)}
}

func (t goalReviewSnapshot) Execute(_ context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	var args map[string]json.RawMessage
	if err := json.Unmarshal(raw, &args); err != nil || args == nil || len(args) != 0 {
		return agent.ToolResult{}, errors.New("goal_review_snapshot requires an empty JSON object")
	}
	goals, err := goal.Open(t.StateDir)
	if err != nil {
		return agent.ToolResult{}, err
	}
	record, err := goals.Load(t.GoalID)
	if err != nil {
		return agent.ToolResult{}, err
	}
	if record.Status != goal.Active {
		return agent.ToolResult{}, fmt.Errorf("goal %s is %s", record.ID, record.Status)
	}
	type turn struct {
		Conversation string    `json:"conversation"`
		Revision     string    `json:"revision"`
		At           time.Time `json:"at"`
		Objective    string    `json:"objective"`
		Result       string    `json:"result"`
		Status       string    `json:"status"`
	}
	type followup struct {
		At           time.Time `json:"at"`
		JobID        string    `json:"job_id"`
		Status       string    `json:"status"`
		Summary      string    `json:"summary"`
		Conversation string    `json:"conversation,omitempty"`
	}
	type reference struct {
		ID      string    `json:"id"`
		Kind    string    `json:"kind"`
		Title   string    `json:"title"`
		Updated time.Time `json:"updated"`
	}
	result := struct {
		GoalID    string      `json:"goal_id"`
		Title     string      `json:"title"`
		Context   string      `json:"context"`
		Updated   time.Time   `json:"updated"`
		Turns     []turn      `json:"recent_turns"`
		Followups []followup  `json:"recent_followups"`
		Knowledge []reference `json:"saved_knowledge"`
	}{GoalID: record.ID, Title: record.Title, Context: boundedReview(record.Context, 4000), Updated: record.UpdatedAt}
	sessions, err := worksession.Open(t.StateDir)
	if err != nil {
		return agent.ToolResult{}, err
	}
	seen := map[string]bool{}
	for _, id := range []string{record.ConversationID, t.ConversationID} {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		conversation, err := sessions.Load(id)
		if err != nil {
			return agent.ToolResult{}, err
		}
		if conversation.SourcePath != record.SourcePath {
			return agent.ToolResult{}, errors.New("goal conversation source mismatch")
		}
		for revisionID, n := conversation.HeadRevision, 0; revisionID != "" && n < 5; n++ {
			revision, err := sessions.LoadRevision(id, revisionID)
			if err != nil {
				return agent.ToolResult{}, err
			}
			result.Turns = append(result.Turns, turn{id, revision.ID, revision.CreatedAt, boundedReview(revision.Objective, 400), boundedReview(revision.FinalText, 800), revision.Status})
			revisionID = revision.ParentRevisionID
		}
	}
	inboxStore, err := inbox.Open(t.StateDir)
	if err != nil {
		return agent.ToolResult{}, err
	}
	entries, err := inboxStore.List(false, 0)
	if err != nil {
		return agent.ToolResult{}, err
	}
	for _, entry := range entries {
		if entry.GoalID == record.ID {
			result.Followups = append(result.Followups, followup{entry.CreatedAt, entry.JobID, entry.Status, boundedReview(entry.Summary, 600), entry.ConversationID})
			if len(result.Followups) == 8 {
				break
			}
		}
	}
	memoryStore, err := memory.Open(t.StateDir)
	if err != nil {
		return agent.ToolResult{}, err
	}
	memories, err := memoryStore.List(record.ID, false)
	if err != nil {
		return agent.ToolResult{}, err
	}
	for _, m := range memories {
		result.Knowledge = append(result.Knowledge, reference{m.ID, m.Kind, boundedReview(m.Text, 120), m.UpdatedAt})
		if len(result.Knowledge) == 24 {
			break
		}
	}
	if len(result.Knowledge) < 24 {
		knowledgeStore, err := knowledge.Open(t.StateDir)
		if err != nil {
			return agent.ToolResult{}, err
		}
		notes, err := knowledgeStore.ListNotes(record.ID)
		if err != nil {
			return agent.ToolResult{}, err
		}
		for _, n := range notes {
			result.Knowledge = append(result.Knowledge, reference{n.ID, "note", n.Title, n.UpdatedAt})
			if len(result.Knowledge) == 24 {
				break
			}
		}
	}
	data, err := json.Marshal(struct {
		OK     bool `json:"ok"`
		Result any  `json:"result"`
	}{true, result})
	if err != nil {
		return agent.ToolResult{}, err
	}
	if len(data) > 32*1024 {
		return agent.ToolResult{}, errors.New("goal review snapshot exceeds output limit")
	}
	return agent.ToolResult{Content: string(data)}, nil
}

func boundedReview(text string, limit int) string {
	text = strings.TrimSpace(text)
	if len(text) <= limit {
		return text
	}
	for limit > 0 && !utf8.RuneStart(text[limit]) {
		limit--
	}
	return text[:limit] + "…"
}
