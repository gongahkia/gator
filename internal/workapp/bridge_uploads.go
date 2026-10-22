package workapp

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
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
	if request.RequestID == "" || len(request.RequestID) > 64 || len(request.Files) > attachment.MaxInputs || len(request.Text) > 128*1024 || len(request.Files) == 0 && strings.TrimSpace(request.Text) == "" {
		return fail("Invalid attachment request.")
	}
	if m.models != nil || m.launcher || m.section != "" || m.pendingBundleAction != nil || m.bridgeExternal != nil || m.firstRun || m.config.Run == nil {
		return fail("Return to the conversation and choose a model before sending attachments.")
	}
	text := strings.TrimSpace(request.Text)
	if strings.HasPrefix(text, "/") || text == "exit" {
		return fail("Send attachments with a message, rather than a command.")
	}
	if text == "" {
		text = "Please take a look at the attached files."
	}
	// A transport retry acknowledges the original admission rather than creating
	// a second task. The client retains this identity until it sees acceptance.
	for _, retained := range m.requests.Requests {
		if retained.ClientID != request.RequestID {
			continue
		}
		var options RunOptions
		if json.Unmarshal(retained.Options, &options) != nil || retained.Text != text || !slices.Equal(options.Uploads, request.Files) {
			return fail("This message identity was already used. Review the retained request before sending a different message.")
		}
		return m, nil
	}
	if m.running && len(m.queue) >= 16 {
		return fail("The queue is full. Remove a message before adding another.")
	}
	seen := map[string]bool{}
	for _, id := range request.Files {
		if !attachment.ValidUploadID(id) || seen[id] {
			return fail("Invalid attachment selection.")
		}
		seen[id] = true
	}
	if len(request.Files) > 0 && m.config.ValidateUploads == nil {
		return fail("Attachments are unavailable in this build.")
	}
	if len(request.Files) > 0 {
		if err := m.config.ValidateUploads(m.source, m.options.Attachments, request.Files); err != nil {
			return fail(fmt.Sprintf("Couldn’t attach these files: %v", err))
		}
	}
	m.options.Uploads = append([]string(nil), request.Files...)
	m.submissionID = request.RequestID
	next, command := m.SubmitText(text)
	m = next.(Model)
	m.submissionID = ""
	return m, command
}

func uploadNames(ids []string) []string {
	names := make([]string, 0, len(ids))
	for _, id := range ids {
		names = append(names, filepath.Base(id))
	}
	return names
}
