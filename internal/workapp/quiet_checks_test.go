package workapp

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/jobs"
	"github.com/gongahkia/gator/internal/workhome"
)

func TestQuietReviewRejectsStaleIdenticalLookingTargets(t *testing.T) {
	for _, screen := range []string{"list", "detail", "form"} {
		for _, change := range []string{"job", "attempt", "assessment"} {
			t.Run(screen+"/"+change, func(t *testing.T) {
				calls := 0
				m := New(Config{ReviewQuietCheck: func(_, _, _, _, _ string) error { calls++; return nil }})
				m.section = "followup"
				m.quietChecks = &quietChecksView{JobID: "job-one", Title: "Same title", Detail: 0, Editing: screen == "form", Audit: jobs.QuietAudit{Total: 1, Entries: []jobs.QuietAuditEntry{{Attempt: jobs.Attempt{ID: "attempt-one", JobID: "job-one", StartedAt: time.Now(), Summary: "No material change"}, Review: jobs.QuietReview{Version: 2, Sequence: 1}}}}}
				if screen == "list" {
					m.quietChecks.Detail = -1
				}
				before := m.quietChecksPanel()
				beforeDisplay, _ := json.Marshal(before)
				switch change {
				case "job":
					m.quietChecks.JobID = "job-two"
					m.quietChecks.Audit.Entries[0].Attempt.JobID = "job-two"
				case "attempt":
					m.quietChecks.Audit.Entries[0].Attempt.ID = "attempt-two"
				case "assessment":
					m.quietChecks.Audit.Entries[0].Review.Sequence++
				}
				after := m.quietChecksPanel()
				afterDisplay, _ := json.Marshal(after)
				if string(beforeDisplay) != string(afterDisplay) {
					t.Fatal("fixture does not represent identical-looking views")
				}
				if bridgePanelID(before) == bridgePanelID(after) {
					t.Fatal("action identity did not bind the exact target and revision")
				}
				event := BridgePanelAction{ID: bridgePanelID(before), Action: "key"}
				key := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")}
				if screen != "detail" {
					key = tea.KeyMsg{Type: tea.KeyEnter}
				}
				if screen == "form" {
					event.Fields = map[int]string{0: "Feedback intended for the older view"}
				}
				state, _ := m.ApplyBridgePanel(event, key)
				m = state.(Model)
				if calls != 0 || m.quietChecks.Note != "" || !strings.Contains(m.bridgeNotice, "view changed") || screen == "list" && m.quietChecks.Detail != -1 {
					t.Fatal("stale action mutated or opened the current check")
				}
			})
		}
	}
}

func TestQuietCheckReviewPreservesMissedDraftAndUsesDisplayedAssessment(t *testing.T) {
	now := time.Now()
	entry := jobs.QuietAuditEntry{Attempt: jobs.Attempt{ID: "attempt-one", JobID: "job-one", StartedAt: now, Summary: "No material change", WatchDecision: &jobs.WatchDecision{Status: "quiet", Summary: "No material change", Rationale: "Deadline remains Friday", Evidence: []string{"source/project.md"}}}}
	calls := 0
	var note, revision string
	m := New(Config{
		ReadQuietChecks: func(id string) (jobs.QuietAudit, error) {
			return jobs.QuietAudit{Total: 1, Unreviewed: 1, Entries: []jobs.QuietAuditEntry{entry}}, nil
		},
		ReviewQuietCheck: func(job, attempt, verdict, text, version string) error {
			calls++
			note, revision = text, version
			if job != "job-one" || attempt != "attempt-one" || verdict != "missed" {
				t.Fatal("wrong review target")
			}
			return errors.New("feedback changed; refresh this check")
		},
	})
	m.section, m.checkDetail = "followup", &workhome.Check{ID: "job-one", Title: "Project deadline"}
	if !m.openQuietChecks() {
		t.Fatal("could not open quiet checks")
	}
	m, _ = productKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	p := m.quietChecksPanel()
	if p.Title != "Review a quiet check" || len(p.Blocks) != 2 {
		t.Fatal("decision evidence not available", p)
	}
	m, _ = productKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	m, _ = productKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if calls != 0 || !m.quietChecks.Editing {
		t.Fatal("blank missed-change review saved")
	}
	p = *m.bridgePanel()
	state, _ := m.ApplyBridgePanel(BridgePanelAction{ID: bridgePanelID(p), Action: "key", Fields: map[int]string{0: "The deadline moved to Thursday."}}, tea.KeyMsg{Type: tea.KeyEnter})
	m = state.(Model)
	if calls != 1 || note != "The deadline moved to Thursday." || revision != entry.Review.Revision() {
		t.Fatal("review did not bind displayed assessment")
	}
	if !m.quietChecks.Editing || m.quietChecks.Note != note || !strings.Contains(m.sectionNotice, "feedback changed") {
		t.Fatal("conflicting assessment lost draft or claimed success")
	}
	m, _ = productKey(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.quietChecks.Editing || m.quietChecks.Detail != 0 {
		t.Fatal("cancel left the check detail")
	}
}

func TestQuietReviewReportsSavedButRefreshFailure(t *testing.T) {
	saved := false
	entry := jobs.QuietAuditEntry{Attempt: jobs.Attempt{ID: "attempt-one", JobID: "job-one", StartedAt: time.Now()}}
	m := New(Config{ReadQuietChecks: func(string) (jobs.QuietAudit, error) {
		if saved {
			return jobs.QuietAudit{}, errors.New("unavailable")
		}
		return jobs.QuietAudit{Entries: []jobs.QuietAuditEntry{entry}}, nil
	}, ReviewQuietCheck: func(_, _, _, _, _ string) error { saved = true; return nil }})
	m.section, m.checkDetail = "followup", &workhome.Check{ID: "job-one"}
	m.openQuietChecks()
	m.quietChecks.Detail = 0
	m.saveQuietReview("correct", "")
	if m.quietChecks != nil || !strings.Contains(m.sectionNotice, "Feedback saved, but") {
		t.Fatal("partial refresh misreported", m.sectionNotice)
	}
}
