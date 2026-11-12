package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/knowledge"
	"github.com/gongahkia/gator/internal/memory"
	"github.com/gongahkia/gator/internal/workapp"
	"github.com/gongahkia/gator/internal/workrun"
	"github.com/gongahkia/gator/internal/workspace"
)

type knowledgeContextModel func(agent.TurnRequest) (agent.Turn, error)

func (m knowledgeContextModel) Complete(_ context.Context, request agent.TurnRequest) (agent.Turn, error) {
	return m(request)
}

func TestDynamicKnowledgeContextRetainsActualToolMessagesAndGuardedCorrections(t *testing.T) {
	state, source := t.TempDir(), t.TempDir()
	memories, _ := memory.Open(state)
	notes, _ := knowledge.Open(state)
	m, _ := memories.Add("fact", "The ceramic delivery arrives Friday.", "", "Planning call", time.Time{})
	n, _ := notes.AddNoteWithRefs("Ceramic dossier", "The ceramic supplier requires a confirmed appointment.", "", []string{"Source call on Monday"})
	foreign, _ := memories.Add("fact", "Private other goal", "goal-aaaaaaaaaaaaaaaaaaaaaaaa", "private", time.Time{})
	step := 0
	model := knowledgeContextModel(func(request agent.TurnRequest) (agent.Turn, error) {
		step++
		switch step {
		case 1:
			return agent.Turn{ToolCalls: []agent.ToolCall{{ID: "find", Name: "knowledge_search", Arguments: json.RawMessage(`{"query":"ceramic"}`)}}}, nil
		case 2:
			return agent.Turn{ToolCalls: []agent.ToolCall{{ID: "memory", Name: "knowledge_read", Arguments: json.RawMessage(`{"id":"` + m.ID + `"}`)}, {ID: "note", Name: "knowledge_read", Arguments: json.RawMessage(`{"id":"` + n.ID + `"}`)}, {ID: "foreign", Name: "knowledge_read", Arguments: json.RawMessage(`{"id":"` + foreign.ID + `"}`)}}}, nil
		}
		return agent.Turn{Text: "Retained dossier inspected."}, nil
	})
	out, err := (workrun.Service{Executor: workrun.Executor{Model: model, StateDir: state}}).Execute(context.Background(), workrun.Request{RunID: "work-knowledge-context", SourcePath: source, Objective: "Inspect retained dossier", Mode: action.Inspect, Contract: artifact.InspectionContract(), MaxSteps: 4})
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := artifact.OpenBundle(out.Work.Path)
	if err != nil {
		t.Fatal(err)
	}
	if err := artifact.VerifyBundle(bundle); err != nil {
		t.Fatal(err)
	}
	if len(bundle.Manifest.Evidence) != 3 {
		t.Fatalf("missing or failed tool result retained: %+v", bundle.Manifest.Evidence)
	}
	for _, evidence := range bundle.Manifest.Evidence {
		data, err := bundle.Root.ReadRegularFile(evidence.SnapshotPath, 512*1024)
		if err != nil {
			t.Fatal(err)
		}
		exact := false
		for _, event := range out.Events {
			if event.Kind == agent.EventToolFinished && event.ToolError == "" && event.ToolResult == string(data) {
				exact = true
			}
		}
		if !exact {
			t.Fatal("capture did not match the actual supplied tool message")
		}
		if strings.Contains(string(data), foreign.Text) {
			t.Fatal("failed foreign read was exposed as supplied knowledge")
		}
	}
	if _, err := memories.Correct(m.ID, "The ceramic delivery arrives Tuesday.", "Changed source label"); err != nil {
		t.Fatal(err)
	}
	view, err := readWorkContext(state, out.Manifest.RunID, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Items) != 4 {
		t.Fatalf("expected two search previews and two full reads, got %+v", view.Items)
	}
	reads := 0
	for _, item := range view.Items {
		if item.EvidenceID == "" || item.Scope != "global" {
			t.Fatal("missing retained identity/scope", item)
		}
		if item.ID == m.ID && (item.CurrentText != "The ceramic delivery arrives Tuesday." || item.Source != "Planning call") {
			t.Fatal("current mutation rewrote the supplied source or replaced current saved text", item)
		}
		if strings.Contains(item.Title, "read during run") {
			reads++
			if item.ID == n.ID && (item.Text != n.Body || item.Source != "Source call on Monday") {
				t.Fatal("note lost actual body/source", item)
			}
			if item.ID == m.ID {
				if item.Text != m.Text {
					t.Fatal("memory history rewritten")
				}
				if _, err := changeWorkKnowledge(state, item.ID, "correct", "The ceramic delivery arrives Wednesday.", "", item.Revision); err != nil {
					t.Fatal(err)
				}
				if _, err := changeWorkKnowledge(state, item.ID, "forget", "", "", item.Revision); err == nil {
					t.Fatal("old displayed revision still authorized a mutation")
				}
			}
		}
	}
	if reads != 2 {
		t.Fatal("search titles were mislabeled as complete reads")
	}
	current, _ := notes.LoadNote(n.ID)
	if _, err := notes.ReviewNote(n.ID, "", current.Revision(), "forget", ""); err != nil {
		t.Fatal(err)
	}
	view, err = readWorkContext(state, out.Manifest.RunID, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range view.Items {
		if item.ID == n.ID && (item.Available || strings.Contains(item.Title, "read during run") && item.Text != n.Body) {
			t.Fatal("forgotten note lost retained provenance", item)
		}
	}
	// A follow-up may reuse a still-retained tool message without reading it
	// again. The copied evidence is included only if its exact response bytes
	// survive in this run's model context, and is labeled as carried context.
	follow := knowledgeContextModel(func(request agent.TurnRequest) (agent.Turn, error) {
		found := false
		for _, message := range request.Messages {
			found = found || message.Role == agent.RoleTool && message.ToolName == "knowledge_read" && strings.Contains(message.Content, m.Text)
		}
		if !found {
			t.Fatal("controlled follow-up did not receive retained context")
		}
		return agent.Turn{Text: "Follow-up from retained context."}, nil
	})
	next, err := (workrun.Service{Executor: workrun.Executor{Model: follow, StateDir: state}}).Execute(context.Background(), workrun.Request{RunID: "work-knowledge-followup", ConversationID: out.ConversationID, SourcePath: source, Objective: "Continue.", Mode: action.Inspect, Contract: artifact.InspectionContract(), MaxSteps: 2})
	if err != nil {
		t.Fatal(err)
	}
	carried, err := readWorkContext(state, next.Manifest.RunID, "")
	if err != nil || len(carried.Items) != 6 {
		t.Fatal("retained context continuity lost", carried, err)
	}
	prior, refreshed := 0, 0
	for _, item := range carried.Items {
		switch {
		case strings.Contains(item.Title, "carried from work-knowledge-context"):
			prior++
		case strings.Contains(item.Title, "refreshed for this run"):
			refreshed++
			if item.ID == m.ID && item.Text != "The ceramic delivery arrives Wednesday." {
				t.Fatal("refresh did not supply the correction", item)
			}
			if item.ID == n.ID && !strings.Contains(item.Text, "No longer saved") {
				t.Fatal("refresh did not withdraw the old note", item)
			}
		default:
			t.Fatal("earlier evidence mislabeled as a fresh read", item)
		}
	}
	if prior != 4 || refreshed != 2 {
		t.Fatal("current and historical knowledge were collapsed", prior, refreshed)
	}
	// A modified response cannot replace the retained excerpt, and knowledge
	// inspection never repairs it by reading the current saved record instead.
	first := bundle.Manifest.Evidence[0]
	if err := os.WriteFile(filepath.Join(bundle.Path, first.SnapshotPath), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readWorkContext(state, out.Manifest.RunID, ""); err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatal("tampered tool response accepted", err)
	}
}

func TestRetainedKnowledgeDoesNotTurnCopiedEvidenceIntoFreshOrForeignAuthority(t *testing.T) {
	root, _ := workspace.Open(t.TempDir())
	data := []byte(`{"ok":true,"result":{"id":"mem-aaaaaaaaaaaaaaaaaaaaaaaa","kind":"fact","text":"An earlier goal's excerpt","goal_id":"goal-other"}}`)
	digest := sha256.Sum256(data)
	hash := hex.EncodeToString(digest[:])
	path := "evidence/" + hash + ".txt"
	if err := os.MkdirAll(filepath.Join(root.Path(), "evidence"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := root.WriteRegularFileAtomic(path, data, 512*1024); err != nil {
		t.Fatal(err)
	}
	bundle := artifact.Bundle{Root: root, Manifest: artifact.Manifest{RunID: "work-current", Evidence: []artifact.Evidence{{ID: "evidence-one", Locator: "knowledge:work-before/knowledge_read/" + strings.Repeat("a", 24), SHA256: hash, SnapshotPath: path}}}}
	calls := 0
	load := func(string) workapp.KnowledgeItem { calls++; return workapp.KnowledgeItem{Available: true} }
	view := workapp.ContextState{WorkID: "work-current"}
	if err := appendRetainedKnowledge(&view, bundle, load, nil); err != nil || len(view.Items) != 0 || calls != 0 {
		t.Fatal("copied/compacted evidence claimed as supplied context", view, err)
	}
	replayed := retainedKnowledgeReplay([]agent.Message{{Role: agent.RoleTool, ToolName: "knowledge_read", Content: string(data)}})
	if err := appendRetainedKnowledge(&view, bundle, load, replayed); err != nil || len(view.Items) != 1 || calls != 0 {
		t.Fatal("cross-goal excerpt reached current saved state", view, err)
	}
	if view.Items[0].Available || view.Items[0].Revision != "" || !strings.Contains(view.Items[0].UnavailableReason, "another goal") {
		t.Fatal("carried foreign excerpt received mutation authority", view.Items[0])
	}
}

func TestRetainedKnowledgeContextRejectsRunMismatchBeforeReadingSavedKnowledge(t *testing.T) {
	calls := 0
	err := appendRetainedKnowledge(&workapp.ContextState{WorkID: "work-one"}, artifact.Bundle{Manifest: artifact.Manifest{RunID: "work-other"}}, func(string) workapp.KnowledgeItem { calls++; return workapp.KnowledgeItem{} }, nil)
	if err == nil || calls != 0 {
		t.Fatal("foreign run authority reached the current knowledge store")
	}
}

func TestRetainedKnowledgeSearchWarningAndGoalBoundary(t *testing.T) {
	root := t.TempDir()
	for _, scope := range []string{"", "goal-other"} {
		data, _ := json.Marshal(map[string]any{"ok": true, "result": map[string]any{"items": []any{map[string]any{"id": "mem-" + strings.Repeat("a", 24), "kind": "fact", "title": "Actual search preview", "goal_id": scope}}, "warning": "One saved record was unreadable."}})
		digest := sha256.Sum256(data)
		hash := hex.EncodeToString(digest[:])
		path := "evidence/" + hash + ".txt"
		if err := os.MkdirAll(filepath.Join(root, "evidence"), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, path), data, 0600); err != nil {
			t.Fatal(err)
		}
		bundleRoot, err := workspace.Open(root)
		if err != nil {
			t.Fatal(err)
		}
		bundle := artifact.Bundle{Root: bundleRoot, Manifest: artifact.Manifest{RunID: "work-one", Evidence: []artifact.Evidence{{ID: "evidence-one", Locator: "knowledge:work-one/knowledge_search/" + strings.Repeat("a", 24), SnapshotPath: path, SHA256: hash}}}}
		view := workapp.ContextState{WorkID: "work-one"}
		calls := 0
		err = appendRetainedKnowledge(&view, bundle, func(string) workapp.KnowledgeItem { calls++; return workapp.KnowledgeItem{} }, nil)
		if scope == "goal-other" {
			if err == nil || calls != 0 {
				t.Fatal("foreign knowledge acquired correction authority")
			}
			continue
		}
		if err != nil || len(view.Items) != 1 || view.Items[0].Text != "Actual search preview" || !strings.Contains(view.Recall, "unreadable") {
			t.Fatal("partial search provenance lost", view, err)
		}
	}
}
