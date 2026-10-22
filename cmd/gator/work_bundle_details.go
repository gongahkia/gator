package main

import (
	"time"
	"unicode/utf8"

	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/workapp"
)

// Enrich already verified metadata once at the existing run/restore boundary,
// never in the streaming BridgeState path. Previews are inert snapshot text.
func projectWorkBundleDetails(bundle artifact.Bundle, summary *workapp.BundleSummary) {
	summary.RunID = bundle.Manifest.RunID
	if !bundle.Manifest.FinishedAt.IsZero() {
		summary.FinishedAt = bundle.Manifest.FinishedAt.Format(time.RFC3339)
	}
	for _, source := range bundle.Manifest.Evidence {
		if len(summary.Sources) == 16 {
			break
		}
		summary.Sources = append(summary.Sources, source.Locator)
	}
	for _, source := range bundle.Manifest.ConnectedSources {
		index := -1
		for i, connection := range summary.Connections {
			if connection.ID == source.ConnectorID {
				index = i
				break
			}
		}
		if index < 0 {
			if len(summary.Connections) == 16 {
				continue
			}
			summary.Connections = append(summary.Connections, workapp.ConnectionUse{ID: source.ConnectorID})
			index = len(summary.Connections) - 1
		}
		operations := summary.Connections[index].Operations
		if len(operations) < 32 && !containsDetail(operations, source.Operation) {
			summary.Connections[index].Operations = append(operations, source.Operation)
		}
	}
	// Keep the extra preview read/formatting work bounded on long restores.
	// Larger files still retain their metadata and validation state.
	var paths []string
	readBudget := int64(1024 * 1024)
	for _, file := range bundle.Manifest.Artifacts {
		if len(paths) < 8 && file.Bytes <= readBudget {
			paths = append(paths, file.Path)
			readBudget -= file.Bytes
		}
	}
	if len(paths) == 0 {
		return
	}
	previews, err := artifact.PreviewSelectedFiles(bundle, paths)
	if err != nil {
		// A preview failure must not turn a completed run into an execution
		// failure. The UI can still show verified metadata without content.
		return
	}
	remaining := 32 * 1024
	for _, preview := range previews {
		if remaining == 0 {
			break
		}
		var file *workapp.ArtifactSummary
		for index := range summary.Artifacts {
			if summary.Artifacts[index].Path == preview.Path {
				file = &summary.Artifacts[index]
				break
			}
		}
		if file == nil {
			continue
		}
		file.PreviewKind = preview.Kind
		limit := min(4096, remaining)
		file.Preview, file.Truncated = boundedDetailPreview(preview.Content, limit)
		file.Truncated = file.Truncated || preview.Truncated
		remaining -= len(file.Preview)
	}
}

func containsDetail(values []string, value string) bool {
	for _, current := range values {
		if current == value {
			return true
		}
	}
	return false
}

func boundedDetailPreview(value string, limit int) (string, bool) {
	if len(value) <= limit {
		return value, false
	}
	for limit > 0 && !utf8.RuneStart(value[limit]) {
		limit--
	}
	return value[:limit], true
}
