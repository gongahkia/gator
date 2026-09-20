package worktui

import "strings"

// restoreConversation replaces all revision-dependent UI state with the
// selected durable head. It is used for an initial resume, picker selection,
// and history navigation, ensuring each path has identical transcript and
// artifact-card semantics.
func (m *Model) restoreConversation(conversationID, announcement string) {
	m.input = ""
	m.clearPromptHistory()
	m.queue = nil
	m.pendingBundleAction = nil
	m.lastOutput = ""
	m.lastBundle = BundleSummary{}
	m.messages = nil
	m.revision = ""
	m.snapshot = ""

	if m.config.LoadConversation != nil {
		state, err := m.config.LoadConversation(conversationID)
		if err != nil {
			m.status = "Restore conversation: " + err.Error()
			return
		}
		m.applyConversationState(conversationID, state)
	}
	if m.config.LoadConversationOptions != nil {
		options, err := m.config.LoadConversationOptions(conversationID)
		if err != nil {
			m.status = "Load conversation settings: " + err.Error()
			return
		}
		m.options = retainedRunOptions(options)
	}
	m.status = joinConversationStatus(announcement, m.revisionStatus())
}

func (m *Model) applyConversationState(conversationID string, state ConversationState) {
	m.conversation = conversationID
	if strings.TrimSpace(state.Title) != "" {
		m.title = state.Title
	}
	if strings.TrimSpace(state.SourcePath) != "" {
		m.source = state.SourcePath
	}
	m.revision = state.RevisionID
	m.snapshot = state.SnapshotID
	m.lastOutput = state.OutputPath
	m.lastBundle = cloneBundleSummary(state.LastBundle)
	m.messages = make([]message, 0, len(state.Messages))
	for _, item := range state.Messages {
		m.messages = append(m.messages, message{
			role: item.Role, text: item.Text, bundle: cloneBundlePointer(item.Bundle),
		})
	}
	m.rebuildPromptHistory()
}

func retainedRunOptions(options RunOptions) RunOptions {
	options = cloneRunOptions(options)
	// These callbacks are owned by a live run. Retained conversation settings
	// must never resurrect an old context, event sink, or steering channel.
	options.Context = nil
	options.OnEvent = nil
	options.OnOperation = nil
	options.Steering = nil
	return options
}

func (m Model) revisionStatus() string {
	if m.revision == "" {
		return ""
	}
	if m.snapshot == "" {
		return "Revision " + m.revision
	}
	return "Revision " + m.revision + " · snapshot " + m.snapshot
}

func joinConversationStatus(values ...string) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			parts = append(parts, value)
		}
	}
	return strings.Join(parts, " · ")
}

func cloneBundlePointer(value *BundleSummary) *BundleSummary {
	if value == nil {
		return nil
	}
	copy := cloneBundleSummary(*value)
	return &copy
}

func cloneBundleSummary(value BundleSummary) BundleSummary {
	copy := value
	copy.Artifacts = append([]ArtifactSummary(nil), value.Artifacts...)
	copy.Candidates = make([]CandidateSummary, len(value.Candidates))
	for index, candidate := range value.Candidates {
		copy.Candidates[index] = candidate
		copy.Candidates[index].ChangedPaths = append([]string(nil), candidate.ChangedPaths...)
	}
	return copy
}
