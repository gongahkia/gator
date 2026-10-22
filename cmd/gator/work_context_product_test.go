package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/knowledge"
	"github.com/gongahkia/gator/internal/memory"
	"github.com/gongahkia/gator/internal/workhistory"
	"github.com/gongahkia/gator/internal/workrun"
)

func TestOmittedSavedKnowledgeIsAbsentFromAutomaticContext(t *testing.T) {
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	state := t.TempDir()
	memories, _ := memory.Open(state)
	notes, _ := knowledge.Open(state)
	m, _ := memories.Add("preference", "Planning replies should be short.", "", "user", time.Time{})
	n, _ := notes.AddNote("Planning replies", "Use three paragraphs.", "")
	request := workrun.Request{Objective: "Planning replies", ExcludedKnowledgeIDs: []string{m.ID, n.ID}}
	if err := applyMemoryContext(state, &request); err != nil {
		t.Fatal(err)
	}
	if err := applyNoteContext(state, &request); err != nil {
		t.Fatal(err)
	}
	if request.MemoryContext != "" || request.NoteContext != "" || len(request.ContextItems) > 0 {
		t.Fatalf("excluded knowledge supplied to model: %#v", request.ContextItems)
	}
}

func TestContextRetainsSuppliedExcerptAfterCorrectionAndForgetting(t *testing.T) {
	state := t.TempDir()
	memories, _ := memory.Open(state)
	record, err := memories.Add("preference", "Use the short reply.", "", "user", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	history, _ := workhistory.Open(state)
	now := time.Now().UTC()
	_, err = history.Start(workhistory.Start{ID: "work-context-test", Objective: "Prepare a reply", Mode: action.Inspect, ExternalActions: action.Forbid, StartedAt: now, Decision: &workhistory.Decision{Version: 1, Mode: action.Inspect, ContextRefs: []string{record.ID}, ContextItems: []workhistory.ContextItem{{ID: record.ID, Kind: "memory", Title: "preference", Excerpt: record.Text, Scope: "global", UpdatedAt: record.UpdatedAt}}}})
	if err != nil {
		t.Fatal(err)
	}
	revision, err := changeWorkKnowledge(state, record.ID, "correct", "Use the detailed reply.", "", record.UpdatedAt.Format(time.RFC3339Nano))
	if err != nil {
		t.Fatal(err)
	}
	stateView, err := readWorkContext(state, "work-context-test", "")
	if err != nil {
		t.Fatal(err)
	}
	if stateView.Items[0].Text != "Use the short reply." || stateView.Items[0].CurrentText != "Use the detailed reply." {
		t.Fatalf("earlier context was rewritten: %#v", stateView.Items)
	}
	if stateView.Items[0].Revision != revision {
		t.Fatal("earlier excerpt replaced the current saved revision")
	}
	if _, err := changeWorkKnowledge(state, record.ID, "forget", "", "", revision); err != nil {
		t.Fatal(err)
	}
	stateView, err = readWorkContext(state, "work-context-test", "")
	if err != nil {
		t.Fatal(err)
	}
	if stateView.Items[0].Available || stateView.Items[0].Text != "Use the short reply." {
		t.Fatal("forgetting misrepresented retained Work evidence")
	}
}

func TestMemorySetupExplainsStorageRetrievalAndChatSeparately(t *testing.T) {
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	status, err := workRecallStatus(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(status.Storage, "local") || !strings.Contains(status.Retrieval, "Ollama") || !strings.Contains(status.Chat, "remotely") {
		t.Fatalf("missing processing distinctions: %#v", status)
	}
}

func TestSavedKnowledgeMutationsUseDisplayedRevision(t *testing.T) {
	for _, kind := range []string{"memory", "note"} {
		t.Run(kind, func(t *testing.T) {
			state := t.TempDir()
			memories, _ := memory.Open(state)
			notes, _ := knowledge.Open(state)
			var id string
			if kind == "memory" {
				r, err := memories.Add("preference", "Original", "", "user", time.Time{})
				if err != nil {
					t.Fatal(err)
				}
				id = r.ID
			} else {
				r, err := notes.AddNote("Plan", "Original", "")
				if err != nil {
					t.Fatal(err)
				}
				id = r.ID
			}
			view, err := readWorkContext(state, "", "")
			if err != nil || len(view.Items) != 1 || view.Items[0].Revision == "" {
				t.Fatalf("missing displayed revision: %#v %v", view, err)
			}
			shown := view.Items[0].Revision
			if kind == "memory" {
				_, err = memories.Correct(id, "Changed elsewhere", "user")
			} else {
				_, err = notes.UpdateNote(id, "Plan", "Changed elsewhere")
			}
			if err != nil {
				t.Fatal(err)
			}
			for _, action := range []string{"correct", "forget"} {
				if _, err := changeWorkKnowledge(state, id, action, "Stale edit", "", shown); err == nil || !strings.Contains(err.Error(), "changed") {
					t.Fatalf("%s accepted stale display: %v", action, err)
				}
			}
			view, _ = readWorkContext(state, "", "")
			if view.Items[0].Text != "Changed elsewhere" {
				t.Fatal("stale UI mutation overwrote newer data")
			}
		})
	}
}
