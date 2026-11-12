package workapp

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/uisnapshot"
)

func (m Model) openContext(workID string, all bool) (tea.Model, tea.Cmd) {
	if m.config.ReadContext == nil {
		m.status = "Saved context unavailable."
		m.bridgeNotice = m.status
		return m, nil
	}
	if !all && workID == "" {
		workID = m.lastWorkID
	}
	if !all && workID == "" && m.config.HistoryStore != nil && m.revision != "" {
		records, err := m.config.HistoryStore.List(100)
		if err == nil {
			for _, r := range records {
				if r.RevisionID == m.revision {
					workID = r.ID
					break
				}
			}
		}
	}
	state, err := m.config.ReadContext(workID, m.goalID)
	if err != nil {
		m.status = "Read context: " + err.Error()
		m.bridgeNotice = m.status
		return m, nil
	}
	m.contextState, m.contextWorkID = state, workID
	m.contextDetail, m.contextEditing, m.contextConfirm = nil, false, false
	m.section, m.home, m.launcher, m.sectionNotice, m.selected = "context", false, false, "", 0
	return m, nil
}

func (m Model) updateContextKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.contextEditing {
		switch key.String() {
		case "esc":
			m.contextEditing = false
		case "enter":
			if m.config.ChangeKnowledge == nil {
				return m, nil
			}
			revision, err := m.config.ChangeKnowledge(m.contextDetail.ID, "correct", m.contextEdit, m.contextState.GoalID, m.contextDetail.Revision)
			if err != nil {
				m.sectionNotice = err.Error()
				return m, nil
			}
			m.contextEditing = false
			m.contextEdit = strings.TrimSpace(m.contextEdit)
			m.contextDetail.CurrentText, m.contextDetail.Revision = m.contextEdit, revision
			if m.contextWorkID == "" {
				m.contextDetail.Text = m.contextEdit
			}
			for i := range m.contextState.Items {
				if m.contextState.Items[i].ID == m.contextDetail.ID {
					if m.contextWorkID == "" {
						m.contextState.Items[i].Text = m.contextEdit
					}
					m.contextState.Items[i].CurrentText = m.contextEdit
					m.contextState.Items[i].Revision = revision
				}
			}
			m.sectionNotice = "Correction saved for future recall. The retained excerpt describes the earlier run."
			if m.contextDetail.SourceReviewNeeded {
				m.sectionNotice = "Text corrected. Review and relink the changed source file with /memory correct --source-file before this memory can enter automatic recall."
			}
		default:
			editFormText(&m.contextEdit, key, 4096)
		}
		return m, nil
	}
	if m.contextConfirm {
		switch key.String() {
		case "esc":
			m.contextConfirm = false
		case "enter":
			if m.config.ChangeKnowledge == nil {
				return m, nil
			}
			if _, err := m.config.ChangeKnowledge(m.contextDetail.ID, "forget", "", m.contextState.GoalID, m.contextDetail.Revision); err != nil {
				m.sectionNotice = err.Error()
				return m, nil
			}
			m.contextConfirm = false
			m.contextDetail.Available = false
			for i := range m.contextState.Items {
				if m.contextState.Items[i].ID == m.contextDetail.ID {
					m.contextState.Items[i].Available = false
				}
			}
			m.sectionNotice = "Forgotten from future recall. Prior transcripts and retained Work evidence remain."
			if m.contextWorkID == "" {
				id := m.contextDetail.ID
				items := m.contextState.Items[:0]
				for _, item := range m.contextState.Items {
					if item.ID != id {
						items = append(items, item)
					}
				}
				m.contextState.Items = items
				m.contextDetail = nil
				m.selected = min(m.selected, max(0, len(items)-1))
			}
		}
		return m, nil
	}
	if m.contextDetail != nil {
		switch key.String() {
		case "esc":
			m.contextDetail = nil
		case "e", "ctrl+e":
			if m.contextDetail.Available && m.contextDetail.Kind != "update" {
				m.contextEditing = true
				m.contextEdit = m.contextDetail.CurrentText
			}
		case "f", "ctrl+x":
			if m.contextDetail.Available && m.contextDetail.Kind != "update" {
				m.contextConfirm = true
			}
		case "o", "ctrl+o":
			if m.contextHasSeparateOwner() {
				m.sectionNotice = "Return to the original conversation to change next-message recall. Saved corrections and forgetting here still affect future recall."
				return m, nil
			}
			if m.contextDetail.Kind == "update" {
				return m, nil
			}
			id := m.contextDetail.ID
			if sliceContains(m.options.ExcludedKnowledgeIDs, id) {
				m.options.ExcludedKnowledgeIDs = removeString(m.options.ExcludedKnowledgeIDs, id)
				m.sectionNotice = "Included again for the next run."
			} else {
				m.options.ExcludedKnowledgeIDs = append(m.options.ExcludedKnowledgeIDs, id)
				m.sectionNotice = "Omitted from saved recall and knowledge tools for the next run. Existing chat text remains."
			}
		}
		return m, nil
	}
	switch key.String() {
	case "esc":
		m.section = ""
	case "up":
		if m.selected > 0 {
			m.selected--
		}
	case "down":
		if m.selected+1 < len(m.contextState.Items) {
			m.selected++
		}
	case "enter":
		if m.selected < len(m.contextState.Items) {
			copy := m.contextState.Items[m.selected]
			m.contextDetail = &copy
		}
	case "a", "ctrl+a":
		return m.openContext("", true)
	case "r", "ctrl+r":
		return m.openContext(m.contextWorkID, m.contextWorkID == "")
	}
	return m, nil
}

func (m Model) contextPanel() uisnapshot.Panel {
	p := uisnapshot.Panel{Kind: "context", Title: "Context and sources", Subtitle: m.contextState.Outcome, Selected: m.selected, Notice: m.sectionNotice}
	p.PresentationID = "knowledge/" + m.contextState.GoalID + "/" + m.contextWorkID + "/list"
	if item := m.contextDetail; item != nil {
		p.PresentationID += "/" + item.ID + "/" + item.EvidenceID
		p.Title = item.Title
		p.Blocks = []uisnapshot.Block{{Title: "Supplied excerpt", Role: "content", Lines: []string{item.Text}}, {Title: "Scope and provenance", Collapsible: true,
			Fields: []uisnapshot.Field{{Label: "Saved item", Value: item.ID}, {Label: "Scope", Value: knowledgeScope(item.Scope, m.contextState.GoalID)}, {Label: "Source labels", Value: valueOrNone(item.Source)}, {Label: "Updated", Value: item.Updated}},
			Lines:  []string{"This is supplied context, not proof of hidden reasoning or that a source supports a claim."}}}
		if item.Available && item.CurrentText != item.Text {
			p.Blocks = append(p.Blocks, uisnapshot.Block{Title: "Current saved text", Lines: []string{item.CurrentText, "Corrections affect future recall. They do not rewrite what this earlier answer received."}})
		}
		if item.EvidenceID != "" {
			p.Blocks = append(p.Blocks, uisnapshot.Block{Title: "Retained knowledge input", Fields: []uisnapshot.Field{{Label: "Evidence", Value: item.EvidenceID}}, Lines: []string{"This excerpt came from a saved-knowledge tool response or a current-status refresh supplied to this run. It is not proof the answer used or correctly interpreted it. The complete input is retained with this answer's source evidence."}})
		}
		if item.SourceReviewNeeded {
			p.Blocks = append(p.Blocks, uisnapshot.Block{Title: "Source needs review", Lines: []string{"The linked file changed or is unavailable. Automatic recall skips this memory until you review the file and relink it.", "/memory correct " + item.ID + " --source-file PATH TEXT"}})
		}
		if !item.Available {
			message := item.UnavailableReason
			if message == "" {
				message = "This item is no longer saved. The excerpt remains part of this earlier Work record."
			}
			p.Blocks = append(p.Blocks, uisnapshot.Block{Title: "Saved state", Lines: []string{message}})
		}
		if m.contextEditing {
			p.PresentationID += "/edit"
			p.Kind, p.Title = "knowledge-form", "Correct saved knowledge"
			p.Selected = 0 // The list selection does not identify a field in this one-field form.
			p.Blocks = p.Blocks[1:]
			p.Rows = []uisnapshot.Row{{Label: "Current saved text", Detail: m.contextEdit, Input: bridgeInput(m.contextEdit, 4096, true)}}
			p.Hints = []uisnapshot.Hint{{Shortcut: "Ctrl+U", Action: "clear"}, {Shortcut: "Enter", Action: "save correction"}, {Shortcut: "Esc", Action: "cancel"}}
		} else if m.contextConfirm {
			p.PresentationID += "/forget"
			p.Kind, p.Title = "confirmation", "Forget saved knowledge?"
			p.Blocks = append(p.Blocks, uisnapshot.Block{Title: "Effect", Lines: []string{"Remove this item and its retrieval vectors from future saved recall.", "Prior transcripts and retained Work evidence may contain earlier excerpts."}})
			p.Hints = []uisnapshot.Hint{{Shortcut: "Enter", Action: "forget saved knowledge", Tone: "error"}, {Shortcut: "Esc", Action: "cancel"}}
		} else {
			p.Hints = []uisnapshot.Hint{{Shortcut: "Ctrl+E", Action: "correct"}, {Shortcut: "Ctrl+X", Action: "forget"}, {Shortcut: "Ctrl+O", Action: "omit/include next run"}, {Shortcut: "Esc", Action: "back"}}
			if m.contextHasSeparateOwner() {
				p.Hints = []uisnapshot.Hint{{Shortcut: "Ctrl+E", Action: "correct"}, {Shortcut: "Ctrl+X", Action: "forget"}, {Shortcut: "Esc", Action: "back"}}
				p.Blocks = append(p.Blocks, uisnapshot.Block{Title: "Next-message recall", Lines: []string{"Change next-message omissions in the original conversation. Saved corrections and forgetting in this view still affect future recall."}})
			}
			if !item.Available || item.Kind == "update" {
				p.Hints = []uisnapshot.Hint{{Shortcut: "Esc", Action: "back"}}
			}
			p.Hints = append(p.Hints, uisnapshot.Hint{Shortcut: "↑/↓", Action: "scroll"})
		}
		return p
	}
	for _, item := range m.contextState.Items {
		state := knowledgeScope(item.Scope, m.contextState.GoalID)
		if sliceContains(m.options.ExcludedKnowledgeIDs, item.ID) {
			state += " · omitted next run"
		}
		p.Rows = append(p.Rows, uisnapshot.Row{Label: item.Title, Detail: truncate(strings.Join(strings.Fields(item.Text), " "), 160), State: state})
		if item.Scope == "global" {
			p.Rows[len(p.Rows)-1].StateLabel = strings.Replace(state, "Global · available across goals", "Global", 1)
		}
	}
	p.Blocks = []uisnapshot.Block{{Title: "Work context", Lines: []string{"Goal: " + valueOrNone(m.contextState.GoalID), "Permitted actions: " + valueOrNone(m.contextState.Permission), "Recall: " + valueOrNone(m.contextState.Recall), m.contextState.Usage}}, {Title: "Observed source references", Lines: m.contextState.Sources}, {Title: "How to read this", Lines: []string{fmt.Sprintf("%d saved excerpts: run-start context, retained tool responses, or knowledge available in this goal. Search previews are labeled separately from complete reads.", len(m.contextState.Items)), "Source labels and retained references do not prove that an answer's claim is supported.", "Use Enter to inspect a saved item and correct, forget, or omit it from the next run."}}}
	if m.contextWorkID == "" {
		p.Title = "Saved knowledge"
		p.Subtitle = "Your saved notes and context"
		p.Blocks[0].Lines = []string{"Scope: global plus " + valueOrNone(m.goalID), "Showing at most 100 saved items. /memory search and /note search find specific topics."}
	}
	p.Hints = []uisnapshot.Hint{{Shortcut: "↑/↓", Action: "choose"}, {Shortcut: "Enter", Action: "expand"}, {Shortcut: "Ctrl+A", Action: "all saved"}, {Shortcut: "Ctrl+R", Action: "refresh"}, {Shortcut: "Esc", Action: "back"}}
	return p
}

func contextLink(workID string) *uisnapshot.Result {
	return &uisnapshot.Result{Kind: "notice", Title: "Context and sources", Summary: "Inspect supplied saved context, source references, and usage.", Footer: "/context " + strings.TrimSpace(workID) + " · /followup to keep tracking this work"}
}

func knowledgeScope(scope, goalID string) string {
	if scope == "global" {
		return "Global · available across goals"
	}
	if scope == goalID && goalID != "" {
		return "This goal"
	}
	return scope
}
