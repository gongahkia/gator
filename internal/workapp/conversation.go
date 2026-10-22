package workapp

import (
	"github.com/gongahkia/gator/internal/worksession"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) openPrimaryConversation() (tea.Model, tea.Cmd) {
	if m.config.LastConversation != nil {
		if m.conversation == "" {
			m.createChat()
		}
		return m, nil
	}
	if m.config.PrimaryConversation == nil {
		m.bridgeNotice = "The main conversation is unavailable. Your saved topics remain available."
		return m, nil
	}
	id, err := m.config.PrimaryConversation()
	if err != nil {
		m.bridgeNotice = "Couldn't open your main conversation: " + err.Error()
		return m, nil
	}
	if id != "" && id == m.conversation {
		m.primary, m.primaryID = true, id
		return m, nil
	}
	next, _ := m.runLocalCommand("/new")
	m = next.(Model)
	m.primary, m.primaryID = true, id
	m.source, m.title = m.primarySource, "Gator"
	if id != "" {
		m.home = false
		m.restoreConversation(id, "")
	}
	return m, nil
}

func (m *Model) rememberChat() {
	if m.conversation != "" && m.config.RememberConversation != nil {
		if err := m.config.RememberConversation(m.conversation); err != nil {
			m.bridgeNotice = "Your chat is saved, but its last-opened position could not be retained: " + err.Error()
		}
	}
}
func (m *Model) createChat() { m.beginChat(m.primarySource) }

// Persist the identity before replacing visible state, so a failed creation
// leaves the user's current chat intact. Folder changes use the same reset.
func (m *Model) beginChat(source string) bool {
	var conversation worksession.Conversation
	if m.config.CreateConversation != nil {
		var err error
		conversation, err = m.config.CreateConversation(source)
		if err != nil {
			m.bridgeNotice = "Couldn't create a new chat: " + err.Error()
			return false
		}
	}
	if m.config.ClearGoal != nil {
		m.config.ClearGoal()
	}
	m.primary, m.home, m.goalID = false, true, ""
	m.conversation, m.source, m.title = conversation.ID, source, "Work in "+filepath.Base(source)
	if conversation.ID != "" {
		m.title = conversation.Title
	}
	m.section, m.paletteQuery, m.launcherMode = "", "", ""
	m.revision, m.snapshot = "", ""
	m.clearPromptHistory()
	m.messages, m.activity, m.queue = nil, nil, nil
	m.input, m.status, m.lastOutput, m.lastWorkID = "", "", "", ""
	m.lastBundle = BundleSummary{}
	m.pendingBundleAction = nil
	m.options = RunOptions{MaxSteps: 24, Mode: "auto", Code: CodeOptions{MaxSteps: 16, Sandbox: "strict", Network: "deny"}}
	m.rememberChat()
	m.refreshBridgeConversations()
	return true
}

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
	m.lastWorkID = ""
	m.lastBundle = BundleSummary{}
	m.messages = nil
	m.activity = nil
	m.revision = ""
	m.snapshot = ""

	if m.config.LoadConversation != nil {
		state, err := m.config.LoadConversation(conversationID)
		if err != nil {
			m.status = "Restore conversation: " + err.Error()
			return
		}
		m.applyConversationState(conversationID, state)
		m.goalID = m.goalForConversation(conversationID)
	}
	if m.config.LoadConversationOptions != nil {
		options, err := m.config.LoadConversationOptions(conversationID)
		if err != nil {
			m.status = "Load conversation settings: " + err.Error()
			return
		}
		m.options = retainedRunOptions(options)
	}
	if m.config.CreateConversation != nil {
		m.home = len(m.messages) == 0
	}
	m.rememberChat()
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
			inputIDs: append([]string(nil), item.InputIDs...), inputs: append([]string(nil), item.Inputs...), activity: item.Activity.Clone(), role: item.Role, text: item.Text, bundle: cloneBundlePointer(item.Bundle),
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
	copy.Sources = append([]string(nil), value.Sources...)
	copy.Connections = make([]ConnectionUse, len(value.Connections))
	for index, connection := range value.Connections {
		copy.Connections[index] = connection
		copy.Connections[index].Operations = append([]string(nil), connection.Operations...)
	}
	copy.Candidates = make([]CandidateSummary, len(value.Candidates))
	for index, candidate := range value.Candidates {
		copy.Candidates[index] = candidate
		copy.Candidates[index].ChangedPaths = append([]string(nil), candidate.ChangedPaths...)
	}
	return copy
}
