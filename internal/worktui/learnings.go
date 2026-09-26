package worktui

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/learning"
)

type learningForm struct {
	EditingID string
	Field     int
	Type      learning.Type
	Scope     learning.Scope
	Key       string
	Content   string
}

func (m *Model) openLearnings() {
	m.section, m.launcher, m.home = "learnings", false, false
	m.learningDetail, m.learningForm, m.sectionNotice = nil, nil, ""
	m.selected = 0
	m.refreshLearnings("")
}

func (m *Model) refreshLearnings(selectedID string) {
	m.learningItems = nil
	if m.config.LearningStore == nil {
		m.sectionNotice = "Learnings are unavailable in this build."
		return
	}
	records, err := m.config.LearningStore.List()
	if err != nil {
		m.sectionNotice = "Read learnings: " + boundedSectionText(err.Error())
		return
	}
	for _, record := range records {
		if m.learningFilter != "" && record.Status != m.learningFilter {
			continue
		}
		m.learningItems = append(m.learningItems, record)
	}
	for index, record := range m.learningItems {
		if record.ID == selectedID {
			m.selected = index
			break
		}
	}
}

func (m *Model) selectLearningItem() {
	if m.selected < 0 || m.selected >= len(m.learningItems) {
		return
	}
	if m.config.LearningStore == nil {
		m.sectionNotice = "Learnings are unavailable in this build."
		return
	}
	record, err := m.config.LearningStore.Load(m.learningItems[m.selected].ID)
	if err != nil {
		m.sectionNotice = "Load learning: " + boundedSectionText(err.Error())
		return
	}
	m.learningDetail, m.sectionNotice = &record, ""
}

func (m *Model) cycleLearningFilter() {
	filters := []learning.Status{"", learning.Active, learning.Candidate, learning.Disabled, learning.Rejected}
	for index, value := range filters {
		if value == m.learningFilter {
			m.learningFilter = filters[(index+1)%len(filters)]
			break
		}
	}
	m.selected = 0
	m.refreshLearnings("")
}

func (m *Model) setLearningFilter(value string) error {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "all":
		m.learningFilter = ""
	case string(learning.Active):
		m.learningFilter = learning.Active
	case string(learning.Candidate):
		m.learningFilter = learning.Candidate
	case string(learning.Disabled):
		m.learningFilter = learning.Disabled
	case string(learning.Rejected):
		m.learningFilter = learning.Rejected
	default:
		return fmt.Errorf("learning status must be all, active, candidate, disabled, or rejected")
	}
	return nil
}

func (m *Model) openNewLearning() {
	m.learningDetail = nil
	m.learningForm = &learningForm{Type: learning.Preference, Scope: learning.Scope{Kind: learning.Global}}
	m.sectionNotice = ""
}

func (m *Model) openEditLearning() {
	if m.learningDetail == nil {
		return
	}
	if m.learningDetail.Origin != learning.UserAuthored {
		m.sectionNotice = "Inferred learnings preserve their wording and provenance; approve, reject, or disable them instead."
		return
	}
	record := *m.learningDetail
	m.learningDetail = nil
	m.learningForm = &learningForm{EditingID: record.ID, Field: 2, Type: record.Type, Scope: record.Scope, Key: record.Key, Content: record.Content}
	m.sectionNotice = ""
}

func (m *Model) saveLearningForm() {
	if m.config.LearningStore == nil || m.learningForm == nil {
		m.sectionNotice = "Learnings are unavailable in this build."
		return
	}
	form := *m.learningForm
	var (
		record learning.Record
		err    error
	)
	if form.EditingID != "" {
		record, err = m.config.LearningStore.Edit(form.EditingID, strings.TrimSpace(form.Key), strings.TrimSpace(form.Content))
	} else {
		record, err = m.config.LearningStore.Create(learning.Create{
			Type: form.Type, Key: strings.TrimSpace(form.Key), Content: strings.TrimSpace(form.Content), Scope: form.Scope, Origin: learning.UserAuthored,
		})
	}
	if err != nil {
		m.sectionNotice = "Save learning: " + boundedSectionText(err.Error())
		return
	}
	m.learningForm, m.learningDetail, m.learningFilter = nil, &record, ""
	m.refreshLearnings(record.ID)
	m.sectionNotice = "Saved " + string(record.Status) + " learning."
}

func (m *Model) mutateLearning(action string) {
	if m.config.LearningStore == nil || m.learningDetail == nil {
		return
	}
	before := *m.learningDetail
	var (
		record learning.Record
		err    error
	)
	switch action {
	case "approve":
		if before.Status != learning.Candidate {
			err = fmt.Errorf("only a candidate learning can be approved")
		} else {
			record, err = m.config.LearningStore.Enable(before.ID)
		}
	case "enable":
		if before.Status != learning.Disabled {
			err = fmt.Errorf("only a disabled learning can be enabled")
		} else {
			record, err = m.config.LearningStore.Enable(before.ID)
		}
	case "disable":
		if before.Status != learning.Active {
			err = fmt.Errorf("only an active learning can be disabled")
		} else {
			record, err = m.config.LearningStore.Disable(before.ID)
		}
	case "reject":
		if before.Status != learning.Candidate {
			err = fmt.Errorf("only a candidate learning can be rejected")
		} else {
			record, err = m.config.LearningStore.Reject(before.ID)
		}
	default:
		err = fmt.Errorf("unknown learning action %q", action)
	}
	if err != nil {
		m.sectionNotice = boundedSectionText(err.Error())
		return
	}
	m.learningDetail = &record
	m.refreshLearnings(record.ID)
	m.sectionNotice = "Learning is now " + string(record.Status) + "."
}

func (m Model) updateLearningFormKey(value tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.learningForm == nil {
		return m, nil
	}
	form := m.learningForm
	switch value.String() {
	case "esc":
		m.learningForm, m.sectionNotice = nil, ""
		return m, nil
	case "tab":
		if form.EditingID != "" {
			form.Field = 2 + (form.Field-1)%2
			return m, nil
		}
		form.Field = (form.Field + 1) % 4
		return m, nil
	case "shift+tab":
		if form.EditingID != "" {
			form.Field = 2 + (form.Field+1)%2
			return m, nil
		}
		form.Field = (form.Field + 3) % 4
		return m, nil
	case "enter":
		if form.EditingID != "" {
			if form.Field == 3 {
				m.saveLearningForm()
			} else {
				form.Field = 3
			}
			return m, nil
		}
		if form.Field == 3 {
			m.saveLearningForm()
			return m, nil
		}
		form.Field++
		return m, nil
	case "left", "right", " ":
		if form.EditingID != "" {
			return m, nil
		}
		if form.Field == 0 {
			form.Type = nextLearningType(form.Type)
			return m, nil
		}
		if form.Field == 1 {
			if form.Scope.Kind == learning.Global {
				form.Scope = learning.Scope{Kind: learning.Project, Value: m.source}
			} else {
				form.Scope = learning.Scope{Kind: learning.Global}
			}
			return m, nil
		}
	case "backspace":
		if form.Field == 2 {
			form.Key = removeLastRune(form.Key)
		} else if form.Field == 3 {
			form.Content = removeLastRune(form.Content)
		}
		return m, nil
	}
	if value.Type == tea.KeyRunes {
		if form.Field == 2 {
			form.Key += value.String()
		} else if form.Field == 3 {
			form.Content += value.String()
		}
	}
	return m, nil
}

func nextLearningType(current learning.Type) learning.Type {
	values := []learning.Type{learning.Preference, learning.EnvironmentFact, learning.Procedure, learning.FailurePrevention}
	for index, value := range values {
		if value == current {
			return values[(index+1)%len(values)]
		}
	}
	return learning.Preference
}

func removeLastRune(value string) string {
	runes := []rune(value)
	if len(runes) == 0 {
		return ""
	}
	return string(runes[:len(runes)-1])
}

func learningFilterName(value learning.Status) string {
	if value == "" {
		return "all"
	}
	return string(value)
}

func learningScopeText(scope learning.Scope) string {
	if scope.Kind == learning.Project {
		return "project:" + scope.Value
	}
	return string(learning.Global)
}

func learningScopeShort(scope learning.Scope) string {
	if scope.Kind == learning.Project {
		return "project:" + filepath.Base(scope.Value)
	}
	return string(learning.Global)
}
