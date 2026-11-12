package workapp

import (
	"regexp"
	"strings"
)

var contextWorkID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,95}$`)

// Parameterized context navigation is limited to the selected conversation's
// retained ancestry. The management worker revalidates that ancestry using its
// host-supplied origin; knowing a global history ID never grants this route.
func (m Model) ownsContextRun(id string) bool {
	if !contextWorkID.MatchString(id) {
		return false
	}
	if m.conversation != "" {
		for _, message := range m.messages {
			if message.runID == id || message.bundle != nil && message.bundle.RunID == id {
				return true
			}
		}
		return m.lastBundle.RunID == id
	}
	if m.config.ViewOriginConversationID == "" || m.config.LoadConversation == nil {
		return false
	}
	state, err := m.config.LoadConversation(m.config.ViewOriginConversationID)
	if err != nil {
		return false
	}
	for _, message := range state.Messages {
		if message.RunID == id || message.Bundle != nil && message.Bundle.RunID == id {
			return true
		}
	}
	return state.LastBundle.RunID == id
}

func (m Model) contextNavigation(command string) (string, bool) {
	fields := strings.Fields(command)
	if len(fields) != 2 || fields[0] != "/context" || !m.ownsContextRun(fields[1]) {
		return command, false
	}
	return "/context " + fields[1], true
}

func (m Model) contextHasSeparateOwner() bool {
	return m.config.ManagedSessions && m.conversation == "" && m.config.ViewOriginConversationID != ""
}
