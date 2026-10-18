package main

import (
	"strings"

	"github.com/gongahkia/gator/internal/worktui"
)

func bridgeStateText(state worktui.BridgeState) string {
	parts := []string{state.Title, state.Source, state.Status, state.Input}
	if state.StatusLineEnabled {
		parts = append(parts, state.StatusLine...)
	}
	for _, message := range state.Messages {
		parts = append(parts, message.Role, message.Text)
		if message.Bundle != nil {
			for _, artifact := range message.Bundle.Artifacts {
				parts = append(parts, artifact.Path)
			}
		}
	}
	if panel := state.Panel; panel != nil {
		parts = append(parts, panel.Kind, panel.Title, panel.Subtitle, panel.Notice, panel.Footer)
		for _, row := range panel.Rows {
			parts = append(parts, row.Label, row.Detail, row.State)
		}
		for _, block := range panel.Blocks {
			parts = append(parts, block.Title)
			parts = append(parts, block.Lines...)
		}
	}
	return strings.Join(parts, "\n")
}
