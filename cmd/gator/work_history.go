package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/gongahkia/gator/internal/workhistory"
)

// workHistoryText is the shared presentation used by the Work CLI and the
// active Work TUI. Querying remains in the UI-independent workhistory store.
func workHistoryText(store workhistory.Store, conversationID string) (string, error) {
	records, err := store.ListConversation(conversationID, 0)
	if err != nil {
		return "", err
	}
	return formatWorkHistory(records), nil
}

func writeWorkHistory(out io.Writer, records []workhistory.Record) error {
	_, err := io.WriteString(out, formatWorkHistory(records))
	return err
}

func formatWorkHistory(records []workhistory.Record) string {
	if len(records) == 0 {
		return "No Work executions are recorded here yet.\n"
	}
	var lines []string
	for _, record := range records {
		line := fmt.Sprintf("%s\t%s\t%s\t%s\trevision=%s\tsnapshot=%s\t%s", record.ID, record.StartedAt.Format("2006-01-02 15:04"), record.Status, record.Mode, valueOrDash(record.RevisionID), valueOrDash(record.SnapshotID), record.ObjectiveSummary)
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n") + "\n"
}
