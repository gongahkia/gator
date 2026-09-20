package worktui

import "strings"

// Prompt history belongs to one active Work conversation. It tracks only
// prompts sent to Work, not slash commands that change local TUI settings.
func (m *Model) recordPrompt(prompt string) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" || strings.HasPrefix(prompt, "/") {
		return
	}
	m.promptHistory = append(m.promptHistory, prompt)
	m.historyIndex = len(m.promptHistory)
	m.historyDraft = ""
}

func (m *Model) rebuildPromptHistory() {
	m.promptHistory = nil
	for _, item := range m.messages {
		if item.role == "You" {
			m.recordPrompt(item.text)
		}
	}
	m.historyIndex = len(m.promptHistory)
	m.historyDraft = ""
}

func (m *Model) clearPromptHistory() {
	m.promptHistory = nil
	m.historyIndex = 0
	m.historyDraft = ""
}

func (m *Model) recallPreviousPrompt() {
	if len(m.promptHistory) == 0 || m.historyIndex == 0 {
		return
	}
	if m.historyIndex == len(m.promptHistory) {
		m.historyDraft = m.input
	}
	m.historyIndex--
	m.input = m.promptHistory[m.historyIndex]
}

func (m *Model) recallNextPrompt() {
	if m.historyIndex >= len(m.promptHistory) {
		return
	}
	m.historyIndex++
	if m.historyIndex == len(m.promptHistory) {
		m.input = m.historyDraft
		m.historyDraft = ""
		return
	}
	m.input = m.promptHistory[m.historyIndex]
}

func (m *Model) resetPromptHistoryNavigation() {
	if m.historyIndex == len(m.promptHistory) {
		return
	}
	m.historyIndex = len(m.promptHistory)
	m.historyDraft = ""
}
