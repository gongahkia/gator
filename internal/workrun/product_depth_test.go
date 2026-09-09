package workrun

import (
	"context"
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
