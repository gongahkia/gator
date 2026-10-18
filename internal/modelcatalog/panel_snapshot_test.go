package modelcatalog

import (
	"strings"

	"github.com/gongahkia/gator/internal/uisnapshot"
)

func panelSnapshotText(panel uisnapshot.Panel) string {
	parts := []string{panel.Kind, panel.Title, panel.Subtitle, panel.Query, panel.Notice, panel.Footer}
	for _, row := range panel.Rows {
		parts = append(parts, row.Label, row.Detail, row.State)
	}
	for _, block := range panel.Blocks {
		parts = append(parts, block.Title)
		parts = append(parts, block.Lines...)
	}
	return strings.Join(parts, "\n")
}
