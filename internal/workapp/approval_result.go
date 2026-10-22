package workapp

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gongahkia/gator/internal/uisnapshot"
	"github.com/gongahkia/gator/internal/workrun"
)

func approvalMessage(interaction workrun.Interaction) message {
	preview, err := json.Marshal(interaction.Preview)
	if err != nil {
		preview = json.RawMessage(`{"error":"Preview could not be encoded"}`)
	}
	result := message{
		role: "Approval",
		text: fmt.Sprintf("%s #%d\n%s\nUse /approve or /deny.", interaction.Kind, interaction.ID, preview),
		result: &uisnapshot.Result{
			Kind: "approval", Title: "Review requested action", Tone: "warning",
			Fields:  []uisnapshot.Field{{Label: "Request", Value: fmt.Sprint(interaction.ID)}, {Label: "Action", Value: interaction.Kind}},
			Preview: preview, Footer: "/approve to allow · /deny to refuse",
		},
	}
	if agreement, ok := interaction.Preview.(workrun.AppAgreement); ok && interaction.Kind == "app_change" && agreement.Action != "" {
		result.result.Title = "Review follow-up"
		result.result.Summary = strings.ToUpper(agreement.Action[:1]) + agreement.Action[1:] + ": " + agreement.Title
		result.result.Preview = nil
		result.result.Fields = []uisnapshot.Field{{Label: "Request", Value: fmt.Sprint(interaction.ID)}}
		details := uisnapshot.ResultSection{Title: "Limits and delivery details", Collapsible: true}
		for _, line := range agreement.Lines {
			if label, value, ok := strings.Cut(line, ": "); ok {
				if label == "Limits" {
					details.Fields = append(details.Fields, uisnapshot.Field{Label: label, Value: value})
					continue
				}
				result.result.Fields = append(result.result.Fields, uisnapshot.Field{Label: label, Value: value})
			} else {
				details.Lines = append(details.Lines, line)
			}
		}
		result.result.Sections = []uisnapshot.ResultSection{details}
		result.result.Footer = "Sending or publishing still needs separate approval."
	}
	return result
}
