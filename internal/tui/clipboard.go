package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type clipboardWriteMsg struct {
	label string
	err   error
}

// copyLatestAgentOutput writes the complete newest Gator response, including
// text that is still streaming, without copying intervening tool activity.
func (m Model) copyLatestAgentOutput() (tea.Model, tea.Cmd) {
	for index := len(m.chat) - 1; index >= 0; index-- {
		entry := m.chat[index]
		if entry.author != chatAgent || strings.TrimSpace(entry.text) == "" {
			continue
		}
		return m.copyTextToClipboard("Latest Gator response", entry.text)
	}
	m.notice = notice{text: "No Gator response is available to copy yet.", kind: noticeInfo}
	return m, nil
}

// copyConversationTranscript writes the local, readable conversation log.
// It preserves the entry roles and tool details but omits terminal styling.
func (m Model) copyConversationTranscript() (tea.Model, tea.Cmd) {
	if len(m.chat) == 0 {
		m.notice = notice{text: "No conversation transcript is available to copy yet.", kind: noticeInfo}
		return m, nil
	}
	return m.copyTextToClipboard("Conversation transcript", m.plainTranscript())
}

func (m Model) copyTextToClipboard(label, text string) (tea.Model, tea.Cmd) {
	if strings.TrimSpace(text) == "" {
		m.notice = notice{text: "Nothing is available to copy.", kind: noticeInfo}
		return m, nil
	}
	write := m.copyToClipboard
	if write == nil {
		m.notice = notice{text: "Copy is unavailable in this Gator session.", kind: noticeError}
		return m, nil
	}
	return m, func() tea.Msg {
		return clipboardWriteMsg{label: label, err: write(text)}
	}
}

func (m Model) plainTranscript() string {
	entries := make([]string, 0, len(m.chat))
	for _, entry := range m.chat {
		label := "Gator"
		switch entry.author {
		case chatUser:
			label = "You"
		case chatTool:
			label = "Tool"
		case chatSystem:
			label = "System"
		}
		if entry.isError {
			label = "Error"
		}
		content := label + "\n" + entry.text
		if entry.detail != "" {
			content += "\nDetails\n" + entry.detail
		}
		entries = append(entries, content)
	}
	return strings.Join(entries, "\n\n")
}

func (m Model) applyClipboardWrite(message clipboardWriteMsg) Model {
	if message.err != nil {
		m.notice = notice{text: fmt.Sprintf("Copy %s: %v", strings.ToLower(message.label), message.err), kind: noticeError}
		return m
	}
	m.notice = notice{text: message.label + " copied to the clipboard.", kind: noticeSuccess}
	return m
}
