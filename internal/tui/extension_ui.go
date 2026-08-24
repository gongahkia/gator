package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) extensionUIForSlot(slot string) []ExtensionUIContribution {
	items := make([]ExtensionUIContribution, 0, len(m.config.ExtensionUI))
	for _, item := range m.config.ExtensionUI {
		if item.Slot != slot || strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.Title) == "" || strings.TrimSpace(item.Description) == "" {
			continue
		}
		items = append(items, item)
	}
	return items
}

func extensionUISlot(screen screen) string {
	if screen == reviewScreen {
		return "review"
	}
	return "composer"
}

func (m Model) openExtensionUI(returnTo screen) (tea.Model, tea.Cmd) {
	slot := extensionUISlot(returnTo)
	if len(m.extensionUIForSlot(slot)) == 0 {
		m.notice = notice{text: "No trusted extension cards contribute to this surface.", kind: noticeInfo}
		return m, nil
	}
	m.extensionUIReturn = returnTo
	m.extensionUIIndex = 0
	m.screen = extensionUIScreen
	m.notice = notice{text: "Extension cards are host-rendered. Their optional action only fills the composer; sending remains explicit.", kind: noticeInfo}
	return m, nil
}

func (m Model) updateExtensionUI(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	items := m.extensionUIForSlot(extensionUISlot(m.extensionUIReturn))
	if len(items) == 0 {
		m.screen = m.extensionUIReturn
		return m, nil
	}
	switch message.String() {
	case "esc", "q":
		m.screen = m.extensionUIReturn
		if m.screen == composeScreen {
			return m, m.focusField()
		}
		return m, nil
	case "up", "k", "ctrl+p":
		m.extensionUIIndex = (m.extensionUIIndex - 1 + len(items)) % len(items)
	case "down", "j", "ctrl+n":
		m.extensionUIIndex = (m.extensionUIIndex + 1) % len(items)
	case "enter":
		selected := items[m.extensionUIIndex]
		if strings.TrimSpace(selected.Prompt) == "" {
			m.notice = notice{text: selected.Title + " is an informational extension card.", kind: noticeInfo}
			return m, nil
		}
		m.screen = composeScreen
		m.task.SetValue(selected.Prompt)
		m.notice = notice{text: "Loaded trusted extension prompt. Review or edit it before sending.", kind: noticeInfo}
		return m, m.focusField()
	}
	return m, nil
}

func (m Model) extensionUISummary(slot string) string {
	items := m.extensionUIForSlot(slot)
	if len(items) == 0 {
		return ""
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		parts = append(parts, item.Title+": "+item.Description)
	}
	return m.panel(labelStyle.Render("Trusted extension cards") + "\n" + dimStyle.Render(compact(strings.Join(parts, "\n"), m.panelTextWidth())) + "\n" + dimStyle.Render("Use /extensions to inspect; card actions only fill the composer."))
}

func (m Model) extensionUIView() string {
	slot := extensionUISlot(m.extensionUIReturn)
	items := m.extensionUIForSlot(slot)
	if len(items) == 0 {
		return m.header("extension cards") + "\n" + m.panel(dimStyle.Render("No trusted extension cards are available.")) + "\n" + m.footer("esc return")
	}
	lines := make([]string, 0, len(items)*2)
	for index, item := range items {
		prefix := "  "
		if index == m.extensionUIIndex {
			prefix = "> "
		}
		line := prefix + item.Title + " · " + item.Description
		if item.Prompt != "" {
			line += " · fills composer"
		} else {
			line += " · information only"
		}
		if index == m.extensionUIIndex {
			line = keyStyle.Render(compact(line, m.panelTextWidth()))
		} else {
			line = compact(line, m.panelTextWidth())
		}
		lines = append(lines, line)
	}
	return m.header("extension cards · "+slot) + "\n" + m.panel(strings.Join(lines, "\n\n")) + "\n" + m.noticeView() + "\n" + m.footer("up/down choose", "enter fill composer", "esc return")
}
