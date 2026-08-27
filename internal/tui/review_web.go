package tui

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/reviewweb"
)

type reviewWebState struct {
	listen      textinput.Model
	openBrowser bool
	url         string
	err         string
	server      *http.Server
}

type reviewWebStartedMsg struct {
	url    string
	err    error
	server *http.Server
}

func newReviewWebState() reviewWebState {
	listen := textinput.New()
	listen.Prompt = ""
	listen.Placeholder = "127.0.0.1:0"
	listen.CharLimit = 64
	listen.Width = 40
	listen.SetValue("127.0.0.1:0")
	listen.Blur()
	return reviewWebState{listen: listen}
}

func (m Model) openReviewWeb() (tea.Model, tea.Cmd) {
	statePath := m.reviewStatePath()
	if statePath == "" {
		m.notice = notice{text: "Choose a retained run before starting browser review. Use /review PATH or finish a run first.", kind: noticeError}
		return m, nil
	}
	m.screen = reviewWebScreen
	_ = m.reviewWeb.listen.Focus()
	m.notice = notice{text: "Loopback-only browser review. The one-use URL is shown after start; this is not gator serve and has no RPC access.", kind: noticeInfo}
	return m, nil
}

func (m Model) reviewStatePath() string {
	if m.outcome != nil && strings.TrimSpace(m.outcome.StatePath) != "" {
		return m.outcome.StatePath
	}
	if strings.TrimSpace(m.resumeStatePath) != "" {
		return m.resumeStatePath
	}
	return m.forkStatePath
}

func (m Model) updateReviewWeb(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch message.String() {
	case "esc", "q":
		m.reviewWeb.listen.Blur()
		if m.outcome != nil {
			m.screen = reviewScreen
			return m, nil
		}
		m.screen = composeScreen
		return m, m.focusField()
	case "o":
		m.reviewWeb.openBrowser = !m.reviewWeb.openBrowser
		return m, nil
	case "s":
		m.stopReviewWeb()
		m.notice = notice{text: "Browser review listener stopped.", kind: noticeInfo}
		return m, nil
	case "enter":
		return m.startReviewWeb()
	}
	var command tea.Cmd
	m.reviewWeb.listen, command = m.reviewWeb.listen.Update(message)
	return m, command
}

func (m Model) startReviewWeb() (tea.Model, tea.Cmd) {
	if m.reviewWeb.server != nil {
		m.notice = notice{text: "A browser review listener is already running. Press s to stop it first.", kind: noticeInfo}
		return m, nil
	}
	statePath := m.reviewStatePath()
	if statePath == "" {
		m.notice = notice{text: "No retained run record is available for browser review.", kind: noticeError}
		return m, nil
	}
	listen := strings.TrimSpace(m.reviewWeb.listen.Value())
	if listen == "" {
		listen = "127.0.0.1:0"
	}
	if err := reviewweb.ValidateListenAddress(listen); err != nil {
		m.notice = notice{text: err.Error(), kind: noticeError}
		return m, nil
	}
	open := m.reviewWeb.openBrowser
	openBrowser := m.config.OpenBrowser
	if openBrowser == nil {
		openBrowser = reviewweb.OpenInBrowser
	}
	return m, func() tea.Msg {
		server, err := reviewweb.New(reviewweb.Config{StatePath: statePath})
		if err != nil {
			return reviewWebStartedMsg{err: err}
		}
		listener, err := net.Listen("tcp", listen)
		if err != nil {
			return reviewWebStartedMsg{err: err}
		}
		url, err := server.BootstrapURL(listener.Addr().String())
		if err != nil {
			_ = listener.Close()
			return reviewWebStartedMsg{err: err}
		}
		httpServer := &http.Server{Handler: server.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second}
		go func() {
			err := httpServer.Serve(listener)
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				_ = err
			}
		}()
		if open {
			if err := openBrowser(url); err != nil {
				return reviewWebStartedMsg{url: url, err: err, server: httpServer}
			}
		}
		return reviewWebStartedMsg{url: url, server: httpServer}
	}
}

func (m *Model) applyReviewWebStarted(msg reviewWebStartedMsg) {
	if msg.server != nil {
		m.reviewWeb.server = msg.server
	}
	if msg.url != "" {
		m.reviewWeb.url = msg.url
		m.commandOutput = "Gator browser review (loopback-only)\n  URL: " + msg.url + "\n  Scope: retained worktree only; the active checkout is untouched\n  Security: one-use URL, HttpOnly same-site session, no RPC access\n  Stop: s on this screen, or quit Gator"
	}
	if msg.err != nil && msg.url == "" {
		m.notice = notice{text: "Start browser review: " + msg.err.Error(), kind: noticeError}
		return
	}
	if msg.err != nil {
		m.notice = notice{text: "Browser review is listening, but opening a browser failed: " + msg.err.Error(), kind: noticeInfo}
		return
	}
	m.notice = notice{text: "Browser review is listening on loopback. Open the one-use URL shown below.", kind: noticeSuccess}
}

func (m *Model) stopReviewWeb() {
	if m.reviewWeb.server == nil {
		m.reviewWeb.url = ""
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = m.reviewWeb.server.Shutdown(ctx)
	m.reviewWeb.server = nil
	m.reviewWeb.url = ""
}

func (m Model) reviewWebView() string {
	open := "will not open a browser"
	if m.reviewWeb.openBrowser {
		open = "will open the one-use URL after start"
	}
	status := "not listening"
	if m.reviewWeb.url != "" {
		status = "listening · one-use URL shown below (not copied to drafts or transcripts)"
	}
	lines := []string{
		"Listen: " + m.reviewWeb.listen.View(),
		"Open:   " + open,
		"Status: " + status,
	}
	if m.reviewWeb.url != "" {
		lines = append(lines, "URL:    "+m.reviewWeb.url)
	}
	return strings.Join([]string{
		m.header("browser review"),
		m.panel(strings.Join(lines, "\n")),
		m.noticeView(),
		m.footer("enter start", "o toggle open", "s stop", "esc return"),
	}, "\n")
}
