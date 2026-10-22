package main

import (
	"encoding/json"
	"errors"

	"github.com/gongahkia/gator/internal/inbox"
	"github.com/gongahkia/gator/internal/workhistory"
	"github.com/gongahkia/gator/internal/workrun"
)

// Reload authoritative records at execution, including queued/recovered replies.
// Renderer text never supplies the job, goal, summary or execution reference.
func applyUpdateContext(stateDir string, request *workrun.Request) error {
	request.UpdateContext = ""
	if !request.ForegroundApp || request.ConversationID == "" {
		return nil
	}
	store, err := inbox.Open(stateDir)
	if err != nil {
		return err
	}
	var entries []inbox.Entry
	if request.ReplyToUpdateID != "" {
		entry, err := store.Load(request.ReplyToUpdateID)
		if err != nil {
			return err
		}
		if entry.OriginConversationID != request.ConversationID {
			return errors.New("the selected update belongs to another conversation")
		}
		entries = []inbox.Entry{entry}
	} else {
		entries, err = store.ForConversation(request.ConversationID, 50)
		if err != nil {
			return err
		}
	}
	type reference struct {
		NextStep                string   `json:"next_step,omitempty"`
		Evidence                []string `json:"evidence,omitempty"`
		ID                      string   `json:"update_id"`
		JobID                   string   `json:"job_id,omitempty"`
		GoalID                  string   `json:"goal_id,omitempty"`
		ExecutionConversationID string   `json:"execution_conversation_id,omitempty"`
		Title                   string   `json:"title"`
		Summary                 string   `json:"summary"`
		Status                  string   `json:"status"`
	}
	refs := make([]reference, 0, 4)
	for _, entry := range entries {
		if request.ReplyToUpdateID == "" && !entry.ResolvedAt.IsZero() {
			continue
		}
		refs = append(refs, reference{entry.NextStep, entry.Evidence, entry.ID, entry.JobID, entry.GoalID, entry.ConversationID, entry.Title, entry.Summary, entry.Status})
		request.ContextRefs = append(request.ContextRefs, entry.ID)
		request.ContextItems = append(request.ContextItems, workhistory.ContextItem{ID: entry.ID, Kind: "update", Title: entry.Title, Excerpt: boundedContextExcerpt(entry.Summary), Scope: "conversation", Source: entry.JobID, UpdatedAt: entry.CreatedAt})
		if len(refs) == 4 {
			break
		}
	}
	if len(refs) == 0 {
		return nil
	}
	payload, err := json.Marshal(struct {
		SelectedID string      `json:"selected_update_id,omitempty"`
		Updates    []reference `json:"recent_updates"`
	}{request.ReplyToUpdateID, refs})
	if err != nil {
		return err
	}
	request.UpdateContext = string(payload)
	return nil
}
