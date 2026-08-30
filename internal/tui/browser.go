package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	gatorbrowser "github.com/gongahkia/gator/internal/browser"
)

func (m Model) browserCommand(arguments []string) (tea.Model, tea.Cmd) {
	if m.config.Browser == nil {
		m.notice = notice{text: "This Gator build does not configure local browser control.", kind: noticeError}
		return m, nil
	}
	if len(arguments) == 0 || arguments[0] == "status" {
		sessions, err := m.config.Browser.Sessions()
		if err != nil {
			m.notice = notice{text: err.Error(), kind: noticeError}
			return m, nil
		}
		m.commandOutput = browserSessionList(sessions)
		m.notice = notice{text: "Local browser sessions shown. Use /browser use ID to grant one to the next native run.", kind: noticeInfo}
		return m, nil
	}
	switch arguments[0] {
	case "start":
		visual, ok := browserVisualArgument(arguments[1:])
		if !ok {
			return m.browserUsage()
		}
		session, err := m.config.Browser.Start(true, visual)
		if err != nil {
			m.notice = notice{text: err.Error(), kind: noticeError}
			return m, nil
		}
		m.browserSession = session.ID
		m.commandOutput = "Started managed browser session " + session.ID + ". It is selected for the next native run. Add approved origins before navigating."
		m.notice = notice{text: "Browser session started and selected for the next run.", kind: noticeSuccess}
	case "attach":
		if len(arguments) < 2 || len(arguments) > 3 {
			return m.browserUsage()
		}
		visual, ok := browserVisualArgument(arguments[2:])
		if !ok {
			return m.browserUsage()
		}
		session, tabs, err := m.config.Browser.Attach(arguments[1], visual)
		if err != nil {
			m.notice = notice{text: err.Error(), kind: noticeError}
			return m, nil
		}
		m.commandOutput = "Attached browser session " + session.ID + ". No tab is shared yet.\n" + browserTabList(tabs) + "\nSelect with /browser select " + session.ID + " TAB_ID [TAB_ID...]"
		m.notice = notice{text: "Review the listed local tabs, then explicitly select one or more.", kind: noticeInfo}
	case "tabs":
		if len(arguments) != 2 {
			return m.browserUsage()
		}
		tabs, err := m.config.Browser.CandidateTabs(arguments[1])
		if err != nil {
			m.notice = notice{text: err.Error(), kind: noticeError}
			return m, nil
		}
		m.commandOutput = browserTabList(tabs)
		m.notice = notice{text: "These candidates are visible only to you until selected.", kind: noticeInfo}
	case "select":
		if len(arguments) < 3 {
			return m.browserUsage()
		}
		session, err := m.config.Browser.SelectTabs(arguments[1], arguments[2:])
		if err != nil {
			m.notice = notice{text: err.Error(), kind: noticeError}
			return m, nil
		}
		m.browserSession = session.ID
		m.commandOutput = fmt.Sprintf("Selected %d tab(s) for %s and granted it to the next native run.", len(session.SelectedTabs), session.ID)
		m.notice = notice{text: "Only selected tabs may be seen by the agent.", kind: noticeSuccess}
	case "use":
		if len(arguments) != 2 {
			return m.browserUsage()
		}
		controller, err := m.config.Browser.Controller(arguments[1])
		if err != nil {
			m.notice = notice{text: err.Error(), kind: noticeError}
			return m, nil
		}
		session, err := controller.Session(context.Background(), arguments[1])
		if err != nil || len(session.SelectedTabs) == 0 {
			if err == nil {
				err = fmt.Errorf("browser session %s has no developer-selected tabs", arguments[1])
			}
			m.notice = notice{text: err.Error(), kind: noticeError}
			return m, nil
		}
		m.browserSession = session.ID
		m.commandOutput = "Browser session " + session.ID + " is selected for the next native run."
		m.notice = notice{text: "Browser session granted to the next run.", kind: noticeSuccess}
	case "none":
		if len(arguments) != 1 {
			return m.browserUsage()
		}
		m.browserSession = ""
		m.commandOutput = "No browser session is granted to the next run."
		m.notice = notice{text: "Browser access removed from the next run.", kind: noticeInfo}
	case "origins":
		return m.browserOrigins(arguments[1:])
	case "visual":
		if len(arguments) != 3 || (arguments[2] != "on" && arguments[2] != "off") {
			return m.browserUsage()
		}
		session, err := m.config.Browser.SetVisualCapture(arguments[1], arguments[2] == "on")
		if err != nil {
			m.notice = notice{text: err.Error(), kind: noticeError}
			return m, nil
		}
		m.commandOutput = fmt.Sprintf("Model-visible screenshots for %s: %t", session.ID, session.VisualCapture)
		m.notice = notice{text: "Visual capture is session-scoped and can be revoked here.", kind: noticeInfo}
	case "upload":
		if len(arguments) != 3 {
			return m.browserUsage()
		}
		upload, err := m.config.Browser.AllowUpload(arguments[1], arguments[2])
		if err != nil {
			m.notice = notice{text: err.Error(), kind: noticeError}
			return m, nil
		}
		m.commandOutput = "Registered developer-selected upload " + upload.ID + " (" + upload.Name + ")."
		m.notice = notice{text: "The agent can only request this opaque upload ID with a fresh approval.", kind: noticeInfo}
	case "artifacts":
		if len(arguments) != 2 {
			return m.browserUsage()
		}
		artifacts, err := m.config.Browser.Artifacts(arguments[1])
		if err != nil {
			m.notice = notice{text: err.Error(), kind: noticeError}
			return m, nil
		}
		m.commandOutput = browserArtifactList(artifacts)
		m.notice = notice{text: "Browser artifacts remain private local files.", kind: noticeInfo}
	case "export":
		if len(arguments) != 4 {
			return m.browserUsage()
		}
		if err := m.config.Browser.ExportArtifact(arguments[1], arguments[2], arguments[3]); err != nil {
			m.notice = notice{text: err.Error(), kind: noticeError}
			return m, nil
		}
		m.commandOutput = "Exported browser artifact to " + arguments[3]
		m.notice = notice{text: "Browser artifact exported to the developer-selected path.", kind: noticeSuccess}
	case "stop":
		if len(arguments) != 2 {
			return m.browserUsage()
		}
		session, err := m.config.Browser.Stop(arguments[1])
		if err != nil {
			m.notice = notice{text: err.Error(), kind: noticeError}
			return m, nil
		}
		if m.browserSession == session.ID {
			m.browserSession = ""
		}
		m.commandOutput = "Stopped browser session " + session.ID + "."
		m.notice = notice{text: "Browser session stopped and revoked.", kind: noticeSuccess}
	default:
		return m.browserUsage()
	}
	return m, nil
}

func (m Model) browserOrigins(arguments []string) (tea.Model, tea.Cmd) {
	if len(arguments) < 2 {
		return m.browserUsage()
	}
	sessionID := arguments[0]
	switch arguments[1] {
	case "list":
		if len(arguments) != 2 {
			return m.browserUsage()
		}
		controller, err := m.config.Browser.Controller(sessionID)
		if err == nil {
			var session gatorbrowser.Session
			session, err = controller.Session(context.Background(), sessionID)
			if err == nil {
				m.commandOutput = browserOriginList(session.Origins)
			}
		}
		if err != nil {
			m.notice = notice{text: err.Error(), kind: noticeError}
			return m, nil
		}
		m.notice = notice{text: "Approved origins shown. Page subresources and WebSockets are restricted to these exact origins.", kind: noticeInfo}
	case "add", "remove":
		if len(arguments) != 3 {
			return m.browserUsage()
		}
		var err error
		if arguments[1] == "add" {
			_, err = m.config.Browser.AddOrigin(sessionID, arguments[2])
		} else {
			_, err = m.config.Browser.RemoveOrigin(sessionID, arguments[2])
		}
		if err != nil {
			m.notice = notice{text: err.Error(), kind: noticeError}
			return m, nil
		}
		m.commandOutput = "Updated approved origins for " + sessionID + "."
		m.notice = notice{text: "Browser network policy updated locally.", kind: noticeSuccess}
	default:
		return m.browserUsage()
	}
	return m, nil
}

func (m Model) browserUsage() (tea.Model, tea.Cmd) {
	m.commandOutput = "Browser commands:\n  /browser status\n  /browser start [visual]\n  /browser attach http://127.0.0.1:PORT [visual]\n  /browser tabs SESSION\n  /browser select SESSION TAB_ID [TAB_ID...]\n  /browser use SESSION | /browser none\n  /browser origins SESSION list|add URL|remove URL\n  /browser visual SESSION on|off\n  /browser upload SESSION /absolute/file\n  /browser artifacts SESSION\n  /browser export SESSION ARTIFACT_ID /absolute/file\n  /browser stop SESSION"
	m.notice = notice{text: "Browser sessions are local and explicit; all site mutations still need per-action approval.", kind: noticeInfo}
	return m, nil
}

func browserVisualArgument(arguments []string) (bool, bool) {
	if len(arguments) == 0 {
		return false, true
	}
	if len(arguments) == 1 && arguments[0] == "visual" {
		return true, true
	}
	return false, false
}

func browserSessionList(sessions []gatorbrowser.Session) string {
	if len(sessions) == 0 {
		return "No local browser sessions. Start one with /browser start."
	}
	lines := make([]string, 0, len(sessions))
	for _, session := range sessions {
		lines = append(lines, fmt.Sprintf("%s  %s  %s  tabs=%d origins=%d visual=%t", session.ID, session.Mode, session.State, len(session.SelectedTabs), len(session.Origins), session.VisualCapture))
	}
	return strings.Join(lines, "\n")
}

func browserTabList(tabs []gatorbrowser.Tab) string {
	if len(tabs) == 0 {
		return "No browser tabs are available."
	}
	lines := make([]string, 0, len(tabs))
	for _, tab := range tabs {
		lines = append(lines, tab.ID+"\t"+tab.Title+"\t"+tab.URL)
	}
	return strings.Join(lines, "\n")
}

func browserOriginList(origins []gatorbrowser.Origin) string {
	if len(origins) == 0 {
		return "No approved browser origins."
	}
	values := make([]string, 0, len(origins))
	for _, origin := range origins {
		values = append(values, origin.URL)
	}
	return strings.Join(values, "\n")
}

func browserArtifactList(artifacts []gatorbrowser.Artifact) string {
	if len(artifacts) == 0 {
		return "No browser artifacts."
	}
	lines := make([]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		lines = append(lines, fmt.Sprintf("%s\t%s\t%d\t%s", artifact.ID, artifact.Kind, artifact.Bytes, artifact.Name))
	}
	return strings.Join(lines, "\n")
}
