package workapp

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/attachment"
)

type BridgeUploadSubmission struct {
	RequestID string   `json:"request_id"`
	Context   string   `json:"context"`
	Text      string   `json:"text"`
	Files     []string `json:"files"`
}
type BridgeUploadResult struct {
	RequestID string `json:"request_id"`
	Error     string `json:"error,omitempty"`
}

func (m Model) SubmitBridgeUploads(request BridgeUploadSubmission) (tea.Model, tea.Cmd) {
	m.bridgeUploadResult = &BridgeUploadResult{RequestID: request.RequestID}
	fail := func(text string) (tea.Model, tea.Cmd) { m.bridgeUploadResult.Error = text; return m, nil }
	context := "workspace:"
	if !m.primary {
		context += "topic:"
	}
	context += m.source
	if m.conversation != "" {
		context = "conversation:" + m.conversation
	}
	if request.Context != context {
		return fail("The conversation changed. Review your attachments before sending.")
	}
	if request.RequestID == "" || len(request.RequestID) > 64 || len(request.Files) == 0 || len(request.Files) > attachment.MaxInputs || len(request.Text) > 128*1024 {
		return fail("Invalid attachment request.")
	}
	if m.models != nil || m.launcher || m.section != "" || m.pendingBundleAction != nil || m.bridgeExternal != nil || m.firstRun || m.config.Run == nil {
		return fail("Return to the conversation and choose a model before sending attachments.")
	}
	if m.running && len(m.queue) >= 16 {
		return fail("The queue is full. Remove a message before adding another.")
	}
	text := strings.TrimSpace(request.Text)
	if strings.HasPrefix(text, "/") || text == "exit" {
		return fail("Send attachments with a message, rather than a command.")
	}
	if text == "" {
		text = "Please take a look at the attached files."
	}
	seen := map[string]bool{}
	for _, id := range request.Files {
		if !attachment.ValidUploadID(id) || seen[id] {
			return fail("Invalid attachment selection.")
		}
		seen[id] = true
	}
	if m.config.ValidateUploads == nil {
		return fail("Attachments are unavailable in this build.")
	}
	if err := m.config.ValidateUploads(m.source, m.options.Attachments, request.Files); err != nil {
		return fail(fmt.Sprintf("Couldn’t attach these files: %v", err))
	}
	m.options.Uploads = append([]string(nil), request.Files...)
	return m.SubmitText(text)
}

func uploadNames(ids []string) []string {
	names := make([]string, 0, len(ids))
	for _, id := range ids {
		names = append(names, filepath.Base(id))
	}
	return names
}
