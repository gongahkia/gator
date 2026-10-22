package workapp

// A host navigation request is emitted only after the application validates
// the selected command/list entry. Sources have passed the workspace resolver;
// the renderer cannot construct this host-only request or select executables.
// Each process retains its own run, approval tokens, queue and conversation.
type BridgeSessionRequest struct {
	Sequence       uint64 `json:"sequence"`
	ConversationID string `json:"conversation_id,omitempty"`
	Goal           bool   `json:"goal,omitempty"`
	New            bool   `json:"new,omitempty"`
	View           string `json:"view,omitempty"`
	Source         string `json:"source,omitempty"`
}

func (m *Model) requestSession(request BridgeSessionRequest) {
	m.sessionSequence++
	request.Sequence = m.sessionSequence
	m.sessionRequest = &request
}

// Focus is host-owned. A task completing in a background chat must not replace
// the user's last-opened chat or receive decisions meant for the visible chat.
func (m Model) FocusBridgeSession(foreground bool) Model {
	if !m.config.ManagedSessions {
		return m
	}
	m.background = !foreground
	m.sessionRequest = nil
	if foreground {
		if !m.running && len(m.queue) == 0 {
			m.refreshModelStatus()
		}
		m.refreshBridgeConversations()
		m.rememberChat()
	}
	return m
}

func (m Model) submissionOptions() RunOptions {
	options := cloneRunOptions(m.options)
	options.ModelProvider, options.ModelName = m.modelStatus.Provider, m.modelStatus.Model
	return options
}
