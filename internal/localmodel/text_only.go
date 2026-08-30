package localmodel

import (
	"context"
	"errors"

	"github.com/gongahkia/gator/internal/agent"
)

// TextOnlyModel rejects visual input before it reaches a catalog model that is
// documented as text-only. Text attachments remain normal user-message text.
type TextOnlyModel struct {
	Backend agent.Model
}

// SupportsVisualInput prevents a browser screenshot from reaching a locally
// documented text-only model on its next turn.
func (TextOnlyModel) SupportsVisualInput() bool { return false }

// Complete implements agent.Model.
func (m TextOnlyModel) Complete(ctx context.Context, request agent.TurnRequest) (agent.Turn, error) {
	for _, message := range request.Messages {
		if len(message.Images) != 0 {
			return agent.Turn{}, errors.New("the selected Gator local coding model accepts text only; remove --image or select a vision-capable custom provider")
		}
	}
	if m.Backend == nil {
		return agent.Turn{}, errors.New("local coding-model backend is required")
	}
	return m.Backend.Complete(ctx, request)
}
