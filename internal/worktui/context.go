package worktui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/uisnapshot"
)

func (m Model) openContext(workID string, all bool) (tea.Model, tea.Cmd) {
	if m.config.ReadContext == nil {
		m.status = "Saved context unavailable."
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
			if err := m.config.ChangeKnowledge(m.contextDetail.ID, "correct", m.contextEdit, m.contextState.GoalID); err != nil {
				m.sectionNotice = err.Error()
				return m, nil
			}
			m.contextEditing = false
			m.contextDetail.CurrentText = m.contextEdit
			if m.contextWorkID == "" {
				m.contextDetail.Text = m.contextEdit
				for i := range m.contextState.Items {
					if m.contextState.Items[i].ID == m.contextDetail.ID {
						m.contextState.Items[i].Text = m.contextEdit
						m.contextState.Items[i].CurrentText = m.contextEdit
					}
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
			if err := m.config.ChangeKnowledge(m.contextDetail.ID, "forget", "", m.contextState.GoalID); err != nil {
				m.sectionNotice = err.Error()
				return m, nil
			}
			m.contextConfirm = false
			m.contextDetail.Available = false
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
		case "e":
			if m.contextDetail.Available {
				m.contextEditing = true
				m.contextEdit = m.contextDetail.CurrentText
			}
		case "f":
			if m.contextDetail.Available {
				m.contextConfirm = true
			}
		case "o":
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
	case "a":
		return m.openContext("", true)
	case "r":
		return m.openContext(m.contextWorkID, m.contextWorkID == "")
	}
	return m, nil
}

func (m Model) contextPanel() uisnapshot.Panel {
	p := uisnapshot.Panel{Kind: "context", Title: "Context and sources", Subtitle: m.contextState.Outcome, Selected: m.selected, Notice: m.sectionNotice}
	if item := m.contextDetail; item != nil {
		p.Title = item.Title
		p.Blocks = []uisnapshot.Block{{Title: "Supplied excerpt", Lines: []string{item.Text}}, {Title: "Scope and provenance", Lines: []string{"Scope: " + knowledgeScope(item.Scope, m.contextState.GoalID), "Source labels: " + valueOrNone(item.Source), item.Updated, "This is supplied context, not proof of hidden reasoning or that a source supports a claim."}}}
		if item.SourceReviewNeeded {
			p.Blocks = append(p.Blocks, uisnapshot.Block{Title: "Source needs review", Lines: []string{"The linked file changed or is unavailable. Automatic recall skips this memory until you review the file and relink it.", "/memory correct " + item.ID + " --source-file PATH TEXT"}})
		}
		if !item.Available {
			p.Blocks = append(p.Blocks, uisnapshot.Block{Title: "Saved state", Lines: []string{"This item is no longer saved. The excerpt remains part of this earlier Work record."}})
		}
		if m.contextEditing {
			p.Kind, p.Title = "knowledge-form", "Correct saved knowledge"
			p.Rows = []uisnapshot.Row{{Label: "Current saved text", Detail: m.contextEdit, Input: bridgeInput(m.contextEdit, 4096, true)}}
			p.Hints = []uisnapshot.Hint{{Shortcut: "Ctrl+U", Action: "clear"}, {Shortcut: "Enter", Action: "save correction"}, {Shortcut: "Esc", Action: "cancel"}}
		} else if m.contextConfirm {
			p.Kind, p.Title = "confirmation", "Forget saved knowledge?"
			p.Blocks = append(p.Blocks, uisnapshot.Block{Title: "Effect", Lines: []string{"Remove this item and its retrieval vectors from future saved recall.", "Prior transcripts and retained Work evidence may contain earlier excerpts."}})
			p.Hints = []uisnapshot.Hint{{Shortcut: "Enter", Action: "forget"}, {Shortcut: "Esc", Action: "cancel"}}
		} else {
			p.Hints = []uisnapshot.Hint{{Shortcut: "E", Action: "correct"}, {Shortcut: "F", Action: "forget"}, {Shortcut: "O", Action: "omit/include next run"}, {Shortcut: "Esc", Action: "back"}}
			if !item.Available {
				p.Hints = []uisnapshot.Hint{{Shortcut: "Esc", Action: "back"}}
			}
		}
		return p
	}
	for _, item := range m.contextState.Items {
		state := knowledgeScope(item.Scope, m.contextState.GoalID)
		if sliceContains(m.options.ExcludedKnowledgeIDs, item.ID) {
			state += " · omitted next run"
		}
		p.Rows = append(p.Rows, uisnapshot.Row{Label: item.Title, Detail: item.Text, State: state})
	}
	p.Blocks = []uisnapshot.Block{{Title: "Work context", Lines: []string{"Goal: " + valueOrNone(m.contextState.GoalID), "Permitted actions: " + valueOrNone(m.contextState.Permission), "Recall: " + valueOrNone(m.contextState.Recall), m.contextState.Usage}}, {Title: "Observed source references", Lines: m.contextState.Sources}, {Title: "How to read this", Lines: []string{fmt.Sprintf("%d saved items supplied at run start or available in this goal.", len(m.contextState.Items)), "Source labels and retained references do not prove that an answer's claim is supported.", "Use Enter to inspect a saved item and correct, forget, or omit it from the next run."}}}
	if m.contextWorkID == "" {
		p.Title = "Saved knowledge"
		p.Blocks[0].Lines = []string{"Scope: global plus " + valueOrNone(m.goalID), "Showing at most 100 saved items. /memory search and /note search find specific topics."}
	}
	p.Hints = []uisnapshot.Hint{{Shortcut: "↑/↓", Action: "choose"}, {Shortcut: "Enter", Action: "expand"}, {Shortcut: "A", Action: "all saved"}, {Shortcut: "R", Action: "refresh"}, {Shortcut: "Esc", Action: "back"}}
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
