package main

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/delivery"
	"github.com/gongahkia/gator/internal/learning"
	"github.com/gongahkia/gator/internal/workhistory"
)

// workHistoryText is the shared presentation used by the Work CLI and the
// active Work TUI. Querying remains in the UI-independent workhistory store.
func workHistoryText(stateDir string, store workhistory.Store, conversationID string) (string, error) {
	records, err := store.ListConversation(conversationID, 0)
	if err != nil {
		return "", err
	}
	return formatWorkHistory(stateDir, records)
}

func writeWorkHistory(out io.Writer, stateDir string, records []workhistory.Record) error {
	text, err := formatWorkHistory(stateDir, records)
	if err != nil {
		return err
	}
	_, err = io.WriteString(out, text)
	return err
}

// formatWorkHistory turns retained technical evidence into a compact Work
// timeline. Full IDs and raw evidence remain available through `work show`,
// `work review`, and `learnings show` rather than occupying the normal path.
func formatWorkHistory(stateDir string, records []workhistory.Record) (string, error) {
	if len(records) == 0 {
		return "No Work executions are recorded here yet.\n", nil
	}
	deliveryStore, err := delivery.Open(stateDir)
	if err != nil {
		return "", err
	}
	learningStore, err := learning.Open(stateDir)
	if err != nil {
		return "", err
	}
	learnings, err := learningStore.List()
	if err != nil {
		return "", err
	}
	sections := make([]string, 0, len(records))
	for _, record := range records {
		deliveries, err := deliveryStore.ListWork(record.ID)
		if err != nil {
			return "", err
		}
		observations, err := learningStore.ListObservations(record.ID)
		if err != nil {
			return "", err
		}
		sections = append(sections, formatWorkHistoryRecord(record, deliveries, observations, learnings))
	}
	return strings.Join(sections, "\n\n") + "\n", nil
}

func formatWorkHistoryRecord(record workhistory.Record, deliveries []delivery.Record, observations []learning.Observation, learnings []learning.Record) string {
	lines := []string{record.ObjectiveSummary, fmt.Sprintf("  Status: %s · verification %s · %s", record.Status, valueOrDash(string(record.VerificationStatus)), valueOrDash(record.ArtifactStatus))}
	lines = append(lines, "  Delivery: "+historyDeliveryState(deliveries))
	lines = append(lines, "  Artifacts: "+historyArtifacts(record))
	lines = append(lines, "  Feedback: "+historyFeedback(observations))
	if derived := historyLearnings(record.ID, learnings); derived != "" {
		lines = append(lines, "  Learnings: "+derived)
	}
	return strings.Join(lines, "\n")
}

func historyDeliveryState(records []delivery.Record) string {
	if len(records) == 0 {
		return "not delivered"
	}
	counts := map[delivery.Status]int{}
	for _, record := range records {
		for _, effect := range record.Effects {
			counts[effect.Status]++
		}
	}
	if counts[delivery.Failed]+counts[delivery.Unknown]+counts[delivery.Pending] > 0 {
		return "needs attention"
	}
	if counts[delivery.Applied] > 0 {
		return "applied"
	}
	return "recorded"
}

func historyArtifacts(record workhistory.Record) string {
	if record.Evidence.ArtifactManifestPath == "" {
		return "none"
	}
	bundle, err := artifact.OpenBundle(filepath.Dir(record.Evidence.ArtifactManifestPath))
	if err != nil || len(bundle.Manifest.Artifacts) == 0 {
		return "none"
	}
	paths := make([]string, 0, len(bundle.Manifest.Artifacts))
	for _, file := range bundle.Manifest.Artifacts {
		paths = append(paths, file.Path)
	}
	return compactHistoryItems(paths, 3)
}

func historyFeedback(observations []learning.Observation) string {
	if len(observations) == 0 {
		return "none"
	}
	items := make([]string, 0, len(observations))
	for _, observation := range observations {
		switch observation.Signal {
		case learning.UserAccepted:
			items = append(items, "accepted")
		case learning.UserRejected:
			items = append(items, "rejected")
		case learning.UserCorrected:
			items = append(items, "corrected")
		case learning.UserRemembered:
			items = append(items, "remembered")
		case learning.UserDeclinedLearning:
			items = append(items, "not learned")
		}
	}
	if len(items) == 0 {
		return "none"
	}
	return compactHistoryItems(items, 3)
}

func historyLearnings(workID string, records []learning.Record) string {
	items := make([]string, 0)
	for _, record := range records {
		if !sliceContains(record.Provenance.WorkIDs, workID) {
			continue
		}
		if record.Status != learning.Active && record.Status != learning.Candidate {
			continue
		}
		items = append(items, string(record.Status)+" “"+firstLine(record.Content)+"”")
	}
	return compactHistoryItems(items, 2)
}

func compactHistoryItems(items []string, limit int) string {
	if len(items) == 0 {
		return ""
	}
	if len(items) <= limit {
		return strings.Join(items, ", ")
	}
	return strings.Join(items[:limit], ", ") + fmt.Sprintf(" · +%d more", len(items)-limit)
}
