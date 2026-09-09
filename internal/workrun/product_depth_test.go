package workrun

import (
	"context"
	"encoding/json"
	"github.com/gongahkia/gator/internal/worksession"
	"os"
	"path/filepath"
	"testing"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
)

type auditRecordingModel struct{ requests []agent.TurnRequest }

func (m *auditRecordingModel) Complete(_ context.Context, r agent.TurnRequest) (agent.Turn, error) {
	m.requests = append(m.requests, r)
	return agent.Turn{Text: "Inspection done."}, nil
}

func TestContinuationReplaysEarlierConstraint(t *testing.T) {
	m := &auditRecordingModel{}
	e := Executor{Model: m, StateDir: t.TempDir()}
	r := Request{SourcePath: t.TempDir(), Objective: "Remember the codename amber.", Mode: action.Inspect, Contract: artifact.InspectionContract()}
	a, err := e.Execute(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	r.ConversationID = a.ConversationID
	r.Objective = "What was the codename?"
	_, err = e.Execute(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	last := m.requests[len(m.requests)-1]
	if len(last.Messages) != 3 || last.Messages[0].Content != "Remember the codename amber." || last.Messages[2].Content != r.Objective {
		t.Fatalf("unexpected history: %#v", last.Messages)
	}
}

func TestExplicitParentSelectsItsOwnSnapshot(t *testing.T) {
	source := t.TempDir()
	p := filepath.Join(source, "notes.txt")
	if err := os.WriteFile(p, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	e := Executor{Model: &auditRecordingModel{}, StateDir: t.TempDir()}
	r := Request{SourcePath: source, Objective: "Inspect", Mode: action.Inspect, Contract: artifact.InspectionContract()}
	a, err := e.Execute(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	r.ConversationID = a.ConversationID
	r.RefreshSource = true
	b, err := e.Execute(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	r.ParentRevisionID = a.RevisionID
	r.RefreshSource = false
	c, err := e.Execute(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if c.SnapshotID == b.SnapshotID || c.SnapshotID != a.SnapshotID {
		t.Fatal("unexpected snapshot selection")
	}
}

func TestLegacyReplayProviderChangeAndHeadNavigation(t *testing.T) {
	state, source := t.TempDir(), t.TempDir()
	model := &auditRecordingModel{}
	executor := Executor{Model: model, StateDir: state}
	request := Request{SourcePath: source, Objective: "Remember amber", Provider: "old/model", Mode: action.Inspect, Contract: artifact.InspectionContract()}
	first, err := executor.Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := worksession.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	parent, err := sessions.LoadRevision(first.ConversationID, first.RevisionID)
	if err != nil {
		t.Fatal(err)
	}
	parent.Version = 1
	parent.Replay = nil
	payload, _ := json.Marshal(parent)
	path := filepath.Join(state, "gator", "conversations", first.ConversationID, "revisions", first.RevisionID+".json")
	if err := os.WriteFile(path, payload, 0600); err != nil {
		t.Fatal(err)
	}
	request.ConversationID = first.ConversationID
	request.Objective = "Legacy followup"
	request.Provider = "new/model"
	second, err := executor.Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	messages := model.requests[len(model.requests)-1].Messages
	if len(messages) != 3 || messages[0].Content != "Remember amber" || messages[1].Content != "Inspection done." {
		t.Fatalf("legacy replay: %+v", messages)
	}
	if _, err := sessions.MoveHead(first.ConversationID, first.RevisionID); err != nil {
		t.Fatal(err)
	}
	request.Objective = "Back branch"
	branch, err := executor.Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	revision, _ := sessions.LoadRevision(first.ConversationID, branch.RevisionID)
	if revision.ParentRevisionID != first.RevisionID {
		t.Fatal("back selected wrong parent")
	}
	if _, err := sessions.MoveHead(first.ConversationID, second.RevisionID); err != nil {
		t.Fatal(err)
	}
	request.Objective = "Forward continuation"
	_, err = executor.Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	// opaque provider state is cleared when changing adapters, with normalized calls retained.
	retained := worksession.Revision{Replay: &worksession.ReplayState{Version: 1, Provider: "old/model", Messages: []agent.Message{{Role: agent.RoleAgent, ProviderData: json.RawMessage(`{"opaque":"private"}`), ToolCalls: []agent.ToolCall{{ID: "logical", ProviderID: "provider-specific", Name: "read_file", Arguments: json.RawMessage(`{}`)}}}}}}
	replay, err := replayMessages(sessions, retained, "new/model")
	if err != nil {
		t.Fatal(err)
	}
	if len(replay[0].ProviderData) != 0 || replay[0].ToolCalls[0].ProviderID != "" || replay[0].ToolCalls[0].ID != "logical" {
		t.Fatal("provider state was not normalized")
	}
}
