package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) drawerUsesSidePane() bool {
	return m.drawerOpen && m.width >= 96 && m.height >= 22
}

func (m Model) drawerWidth() int {
	return min(44, max(32, m.width/3))
}

func (m Model) conversationWidth() int {
	if m.drawerUsesSidePane() {
		return max(1, m.width-m.drawerWidth()-1)
	}
	return max(1, m.width)
}

func (m Model) transcriptHeight() int {
	if m.height <= 0 {
		return 8
	}
	// status rail, divider, composer label/input, notice, and keybar
	return max(1, m.height-m.task.Height()-7)
}

func (m *Model) resizeConversation() {
	m.transcript.Width = m.conversationWidth()
	m.transcript.Height = m.transcriptHeight()
}

func (m *Model) syncTranscript(follow bool) {
	m.resizeConversation()
	m.transcript.SetContent(m.transcriptContent())
	if follow {
		m.transcript.GotoBottom()
		m.followTranscript = true
		m.transcriptUnread = false
	}
}

func (m Model) transcriptContent() string {
	if len(m.chat) == 0 {
		return dimStyle.Render("No messages yet. Describe the work you want to do below.")
	}
	width := max(16, m.transcript.Width)
	entries := make([]string, 0, len(m.chat))
	for _, entry := range m.chat {
		entries = append(entries, renderChatEntry(entry, width))
	}
	return strings.Join(entries, "\n\n")
}

func renderChatEntry(entry chatEntry, width int) string {
	label, style := "Gator", labelStyle
	switch entry.author {
	case chatUser:
		label, style = "You", keyStyle
	case chatTool:
		label, style = "Tool", dimStyle
	case chatSystem:
		label, style = "System", dimStyle
	}
	if entry.isError {
		label, style = "Error", errorStyle
	}
	if entry.streaming {
		label += " · responding"
	}
	bodyStyle := lipgloss.NewStyle().Width(width).PaddingLeft(2)
	if entry.isError {
		bodyStyle = bodyStyle.Foreground(errorStyle.GetForeground())
	} else if entry.author == chatSystem || entry.author == chatTool {
		bodyStyle = bodyStyle.Foreground(dimStyle.GetForeground())
	}
	sections := []string{style.Render(label), bodyStyle.Render(entry.text)}
	if entry.detail != "" {
		sections = append(sections, dimStyle.Copy().Width(width).PaddingLeft(4).Render(entry.detail))
	}
	return strings.Join(sections, "\n")
}

func (m *Model) pageTranscript(down bool) {
	if down {
		m.transcript.PageDown()
	} else {
		m.transcript.PageUp()
	}
	m.followTranscript = m.transcript.AtBottom()
	if m.followTranscript {
		m.transcriptUnread = false
	}
}

func (m *Model) transcriptTop() {
	m.transcript.GotoTop()
	m.followTranscript = false
}

func (m *Model) transcriptBottom() {
	m.transcript.GotoBottom()
	m.followTranscript = true
	m.transcriptUnread = false
}
