package localmodel

import (
	"context"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/agent"
)

type scriptedModel struct {
	calls int
}

func (m *scriptedModel) Complete(context.Context, agent.TurnRequest) (agent.Turn, error) {
	m.calls++
	return agent.Turn{Text: "ok"}, nil
}

func TestTextOnlyModelRejectsImagesBeforeCallingBackend(t *testing.T) {
	backend := &scriptedModel{}
	model := TextOnlyModel{Backend: backend}
	_, err := model.Complete(context.Background(), agent.TurnRequest{Messages: []agent.Message{{Role: agent.RoleUser, Images: []agent.Image{{Name: "design.png"}}}}})
	if err == nil || !strings.Contains(err.Error(), "text only") || backend.calls != 0 {
		t.Fatalf("image error = %v, backend calls = %d", err, backend.calls)
	}
	turn, err := model.Complete(context.Background(), agent.TurnRequest{Messages: []agent.Message{{Role: agent.RoleUser, Content: "inspect this"}}})
	if err != nil || turn.Text != "ok" || backend.calls != 1 {
		t.Fatalf("text turn = %#v, err = %v, backend calls = %d", turn, err, backend.calls)
	}
}
