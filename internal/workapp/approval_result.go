package workapp

import (
	"encoding/json"
	"fmt"

	"github.com/gongahkia/gator/internal/uisnapshot"
	"github.com/gongahkia/gator/internal/workrun"
)

func approvalMessage(interaction workrun.Interaction) message {
	preview, err := json.Marshal(interaction.Preview)
	if err != nil {
		preview = json.RawMessage(`{"error":"Preview could not be encoded"}`)
	}
	return message{
		role: "Approval",
		text: fmt.Sprintf("%s #%d\n%s\nUse /approve or /deny.", interaction.Kind, interaction.ID, preview),
		result: &uisnapshot.Result{
			Kind: "approval", Title: "Review requested action", Tone: "warning",
			Fields:  []uisnapshot.Field{{Label: "Request", Value: fmt.Sprint(interaction.ID)}, {Label: "Action", Value: interaction.Kind}},
			Preview: preview, Footer: "/approve to allow · /deny to refuse",
		},
	}
}
