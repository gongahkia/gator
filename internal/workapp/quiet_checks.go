package workapp

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/jobs"
	"github.com/gongahkia/gator/internal/uisnapshot"
)

type quietChecksView struct {
	JobID, Title string
	Audit        jobs.QuietAudit
	Detail       int
	Editing      bool
	Note         string
}

func (m *Model) openQuietChecks() bool {
	if m.checkDetail == nil || m.config.ReadQuietChecks == nil {
		return false
	}
	audit, err := m.config.ReadQuietChecks(m.checkDetail.ID)
	if err != nil {
		m.sectionNotice = "Quiet checks could not load: " + err.Error()
		return false
	}
	m.quietChecks = &quietChecksView{JobID: m.checkDetail.ID, Title: m.checkDetail.Title, Audit: audit, Detail: -1}
	m.selected, m.sectionNotice = 0, ""
	return true
}

func (m Model) updateQuietChecks(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	v := m.quietChecks
	if v.Editing {
		switch key.String() {
		case "esc":
			v.Editing = false
		case "enter":
			m.saveQuietReview("missed", strings.TrimSpace(v.Note))
		default:
			editFormText(&v.Note, key, 512)
		}
		return m, nil
	}
	switch key.String() {
	case "esc":
		if v.Detail >= 0 {
			v.Detail = -1
		} else {
			m.quietChecks = nil
		}
		m.sectionNotice = ""
	case "r":
		m.openQuietChecks()
	case "up":
		m.selected = max(0, m.selected-1)
	case "down":
		m.selected = min(max(0, len(v.Audit.Entries)-1), m.selected+1)
	case "enter":
		if v.Detail < 0 && m.selected < len(v.Audit.Entries) {
			v.Detail = m.selected
		}
	case "c":
		m.saveQuietReview("correct", "")
	case "x":
		m.saveQuietReview("clear", "")
	case "m":
		if v.Detail >= 0 && v.Detail < len(v.Audit.Entries) {
			v.Editing, v.Note = true, v.Audit.Entries[v.Detail].Review.Note
		}
	}
	return m, nil
}

func (m *Model) saveQuietReview(verdict, note string) {
	v := m.quietChecks
	if v == nil || v.Detail < 0 || v.Detail >= len(v.Audit.Entries) || m.config.ReviewQuietCheck == nil {
		return
	}
	entry := v.Audit.Entries[v.Detail]
	if verdict == "missed" && note == "" {
		m.sectionNotice = "Describe the change Gator should have reported."
		return
	}
	if err := m.config.ReviewQuietCheck(v.JobID, entry.Attempt.ID, verdict, note, entry.Review.Revision()); err != nil {
		m.sectionNotice = err.Error()
		return
	}
	if !m.openQuietChecks() {
		m.quietChecks = nil
		m.sectionNotice = "Feedback saved, but quiet checks could not refresh. Open them again to retry."
		return
	}
	if m.quietChecks != nil {
		for i, current := range m.quietChecks.Audit.Entries {
			if current.Attempt.ID == entry.Attempt.ID {
				m.quietChecks.Detail, m.selected = i, i
				break
			}
		}
	}
	m.sectionNotice = "Feedback saved. It does not change the agreement or send an alert."
}

func (m Model) quietChecksPanel() uisnapshot.Panel {
	v := m.quietChecks
	p := uisnapshot.Panel{Kind: "quiet-checks", Title: "Quiet checks", Subtitle: v.Title, Notice: m.sectionNotice, Selected: m.selected}
	if v.Detail >= 0 && v.Detail < len(v.Audit.Entries) {
		entry := v.Audit.Entries[v.Detail]
		p.ActionIdentity = strings.Join([]string{v.JobID, entry.Attempt.ID, entry.Review.Revision()}, "\x00")
		p.Title = "Review a quiet check"
		p.Subtitle = entry.Attempt.StartedAt.Local().Format("2 Jan 2006, 15:04")
		p.Blocks = []uisnapshot.Block{{Title: "What Gator concluded", Role: "content", Lines: []string{entry.Attempt.Summary}}}
		if decision := entry.Attempt.WatchDecision; decision != nil {
			p.Blocks[0].Lines = []string{decision.Summary, decision.Rationale}
			if len(decision.Evidence) > 0 {
				p.Blocks = append(p.Blocks, uisnapshot.Block{Title: "Retained source references", Collapsible: true, Lines: append([]string(nil), decision.Evidence...)})
			}
		}
		p.Summary = []uisnapshot.Field{{Label: "Your assessment", Value: quietReviewLabel(entry.Review.Verdict)}}
		if entry.Review.Note != "" {
			p.Blocks = append(p.Blocks, uisnapshot.Block{Title: "Your feedback", Lines: []string{entry.Review.Note}})
		}
		if v.Editing {
			p.Kind, p.Title = "quiet-review-form", "What did Gator miss?"
			p.Rows = []uisnapshot.Row{{Label: "Change that should have been reported", Input: &uisnapshot.Input{Kind: "text", Value: v.Note, Limit: 512, Multiline: true, Help: "Describe the missed change using the sources available at the time."}}}
			p.Hints = []uisnapshot.Hint{{Shortcut: "Enter", Action: "save feedback"}, {Shortcut: "Esc", Action: "cancel"}}
		} else {
			p.Hints = []uisnapshot.Hint{{Shortcut: "C", Action: "correctly stayed quiet"}, {Shortcut: "M", Action: "missed a change"}, {Shortcut: "R", Action: "refresh"}, {Shortcut: "Esc", Action: "back"}}
			if entry.Review.Verdict != "" {
				p.Hints = append(p.Hints, uisnapshot.Hint{Shortcut: "X", Action: "clear feedback", Group: "more"})
			}
		}
		return p
	}
	p.Blocks = []uisnapshot.Block{{Title: "Checks that stayed quiet", Lines: []string{"Review whether Gator should have interrupted you. Only completed, evaluated checks appear here; skipped or failed checks are not quiet decisions.", fmt.Sprintf("%d quiet checks · %d reviewed · %d marked missed. These counts do not measure recall without reviewing the relevant source changes.", v.Audit.Total, v.Audit.Correct+v.Audit.Missed, v.Audit.Missed)}}}
	identity := []string{v.JobID}
	for _, entry := range v.Audit.Entries {
		identity = append(identity, entry.Attempt.ID, entry.Review.Revision())
		summary := entry.Attempt.Summary
		if entry.Attempt.WatchDecision != nil {
			summary = entry.Attempt.WatchDecision.Summary
		}
		if strings.TrimSpace(summary) == "" {
			summary = "Quiet check"
		}
		p.Rows = append(p.Rows, uisnapshot.Row{Label: summary, State: quietReviewLabel(entry.Review.Verdict), UpdatedAt: entry.Attempt.StartedAt.Format(time.RFC3339)})
	}
	p.ActionIdentity = strings.Join(identity, "\x00")
	if len(v.Audit.Entries) < v.Audit.Total {
		p.Blocks[0].Lines = append(p.Blocks[0].Lines, "Showing the 50 most recent quiet checks.")
	}
	if len(v.Audit.Entries) == 0 {
		p.Blocks = append(p.Blocks, uisnapshot.Block{Title: "No quiet checks to review yet", Lines: []string{"A completed check that finds no reason to notify you will appear here."}})
	}
	p.Hints = []uisnapshot.Hint{{Shortcut: "Enter", Action: "review check"}, {Shortcut: "R", Action: "refresh"}, {Shortcut: "Esc", Action: "back"}}
	return p
}

func quietReviewLabel(verdict string) string {
	switch verdict {
	case "correct":
		return "Correctly quiet"
	case "missed":
		return "Missed a change"
	default:
		return "Not reviewed"
	}
}
