// Package personalization builds an inspectable view of retained personal
// context. It stores no derived copy, makes no model calls and infers no traits.
package personalization

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/learning"
	"github.com/gongahkia/gator/internal/memory"
)

const maxEntries = 40

type Entry struct {
	ID             string    `json:"id"`
	Category       string    `json:"category"`
	Text           string    `json:"text"`
	Source         string    `json:"source"`
	ConversationID string    `json:"conversation_id,omitempty"`
	RunID          string    `json:"run_id,omitempty"`
	EvidenceRefs   []string  `json:"evidence_refs,omitempty"`
	UpdatedAt      time.Time `json:"updated_at"`
	ConfirmedAt    time.Time `json:"confirmed_at,omitempty"`
}

type Summary struct {
	Entries        []Entry  `json:"entries"`
	Omitted        int      `json:"omitted"`
	NeedsReview    int      `json:"needs_review"`
	PendingLessons int      `json:"pending_lessons"`
	Warnings       []string `json:"warnings,omitempty"`
}

// Read is always rebuilt from owning stores. Project/goal records and inactive
// guidance never become claims about the person. Pending lessons are counted,
// not presented as established preferences. Expired/changed memories remain
// inspectable through their owning store but do not enter this active summary.
func Read(stateDir string) (Summary, error) {
	result := Summary{Entries: []Entry{}}
	store, err := memory.Open(stateDir)
	if err != nil {
		return result, err
	}
	records, err := store.List("", true)
	if warning := memory.ReadWarning(err); warning != "" {
		result.Warnings = append(result.Warnings, warning)
	} else if err != nil {
		return result, err
	}
	add := func(entry Entry) {
		if len(result.Entries) >= maxEntries {
			result.Omitted++
			return
		}
		result.Entries = append(result.Entries, entry)
	}
	now := time.Now()
	for _, record := range records {
		if record.GoalID != "" {
			continue
		}
		if !record.ExpiresAt.IsZero() && !record.ExpiresAt.After(now) || record.SourceStatus == "changed" || record.SourceStatus == "unavailable" {
			result.NeedsReview++
			continue
		}
		category := "Personal facts"
		if record.Kind == "preference" {
			category = "Preferences"
		} else if record.Kind == "note" || record.Kind == "decision" {
			category = "Saved personal context"
		}
		entry := Entry{ID: record.ID, Category: category, Text: record.Text, Source: record.Source, UpdatedAt: record.UpdatedAt, ConfirmedAt: record.LastConfirmedAt}
		if record.Origin != nil {
			entry.ConversationID, entry.RunID = record.Origin.ConversationID, record.Origin.RunID
		}
		add(entry)
	}
	lessons, err := learning.Open(stateDir)
	if err != nil {
		result.Warnings = append(result.Warnings, "Saved guidance unavailable: "+err.Error())
		return result, nil
	}
	if report, statusErr := lessons.MaintenanceStatus(); statusErr == nil {
		if report.Error != "" {
			result.Warnings = append(result.Warnings, "Memory maintenance failed: "+report.Error)
		}
		if report.Truncated {
			result.Warnings = append(result.Warnings, "Memory maintenance reached its scan or proposal limit; coverage is incomplete.")
		}
		if report.Invalid > 0 {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Memory maintenance skipped %d invalid observations.", report.Invalid))
		}
	} else if !os.IsNotExist(statusErr) {
		result.Warnings = append(result.Warnings, "Memory maintenance status unavailable: "+statusErr.Error())
	}
	guidance, err := lessons.List()
	if err != nil {
		result.Warnings = append(result.Warnings, "Saved guidance unavailable: "+err.Error())
		return result, nil
	}
	for _, record := range guidance {
		if record.Scope.Kind == learning.Global && record.Status == learning.Candidate {
			result.PendingLessons++
		}
	}
	for _, record := range learning.Applicable(guidance, learning.Context{}) {
		add(Entry{ID: record.ID, Category: "Confirmed guidance", Text: record.Content, Source: string(record.Origin), UpdatedAt: record.UpdatedAt, ConfirmedAt: record.Provenance.UserConfirmedAt, EvidenceRefs: record.Provenance.EvidenceRefs})
	}
	return result, nil
}

func (s Summary) Lines() []string {
	lines := []string{"Current personal memories and active guidance. Task-specific context and old chat history are separate."}
	for _, category := range []string{"Preferences", "Personal facts", "Saved personal context", "Confirmed guidance"} {
		first := true
		for _, entry := range s.Entries {
			if entry.Category != category {
				continue
			}
			if first {
				lines = append(lines, "", category)
				first = false
			}
			lines = append(lines, "• "+entry.Text, "  "+entry.ID+" · source: "+entry.Source)
			if entry.ConversationID != "" {
				lines = append(lines, "  Chat: "+entry.ConversationID+" · run: "+entry.RunID)
			}
			if !entry.ConfirmedAt.IsZero() {
				lines = append(lines, "  Confirmed: "+entry.ConfirmedAt.Local().Format("2006-01-02 15:04"))
			}
			if len(entry.EvidenceRefs) != 0 {
				lines = append(lines, "  Evidence: "+strings.Join(entry.EvidenceRefs, ", "))
			}
		}
	}
	if len(s.Entries) == 0 {
		lines = append(lines, "No active personal memories or guidance saved.")
	}
	if s.Omitted > 0 {
		lines = append(lines, fmt.Sprintf("%d more entries: inspect memory list and learnings list.", s.Omitted))
	}
	if s.NeedsReview > 0 {
		lines = append(lines, fmt.Sprintf("%d expired or changed-source memories omitted; inspect memory list --all.", s.NeedsReview))
	}
	if s.PendingLessons > 0 {
		lines = append(lines, fmt.Sprintf("%d proposed lessons await review; they are not active guidance.", s.PendingLessons))
	}
	for _, warning := range s.Warnings {
		lines = append(lines, "Warning: "+warning)
	}
	return lines
}
