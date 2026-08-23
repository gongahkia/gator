package tui

import (
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/gongahkia/gator/internal/terminal"
)

const (
	maxAttachedTerminalBytes  = 128 * 1024
	maxTerminalRefreshChunks  = 8
	terminalScrollMarker      = "… earlier attached terminal output omitted …\n"
	terminalDroppedScrollback = "… terminal scrollback dropped while this view was inactive …\n"
)

// attachedTerminalView is a private, display-only cursor for one task. The
// terminal manager remains the source of truth and owns the process lifetime.
type attachedTerminalView struct {
	cursor  int64
	screen  terminalDisplay
	dropped bool
}

func (m *Model) setTerminalAttachment(attachment terminal.Attachment) {
	m.terminalAttachment = attachment
	m.terminalTasks = nil
	m.terminalIndex = 0
	m.terminalScroll = 0
	m.terminalErr = nil
	m.terminalViews = make(map[string]attachedTerminalView)
	if attachment == nil {
		if m.screen == terminalScreen {
			m.screen = m.terminalReturn
		}
		return
	}
	m.refreshAttachedTerminal()
}

func (m *Model) openAttachedTerminal() {
	if m.terminalAttachment == nil {
		m.notice = notice{text: "No model-started terminal task is available in this run.", kind: noticeInfo}
		return
	}
	m.terminalReturn = m.screen
	m.screen = terminalScreen
	m.terminalScroll = 0
	m.refreshAttachedTerminal()
	_ = m.terminalInput.Focus()
}

func (m *Model) closeAttachedTerminal() {
	m.terminalInput.Blur()
	m.screen = m.terminalReturn
	if m.screen == composeScreen || m.screen == runningScreen {
		m.focus = taskField
		_ = m.focusField()
	}
}

func (m *Model) refreshAttachedTerminal() {
	if m.terminalAttachment == nil {
		m.terminalTasks = nil
		return
	}
	m.terminalTasks = m.terminalAttachment.List()
	if len(m.terminalTasks) == 0 {
		m.terminalIndex = 0
		return
	}
	if m.terminalIndex >= len(m.terminalTasks) {
		m.terminalIndex = len(m.terminalTasks) - 1
	}
	if m.terminalIndex < 0 {
		m.terminalIndex = 0
	}
	task := m.terminalTasks[m.terminalIndex]
	if m.screen == terminalScreen && task.Status == "running" {
		rows, columns := m.attachedTerminalSize()
		if task.Rows != rows || task.Columns != columns {
			resized, err := m.terminalAttachment.Resize(task.ID, rows, columns)
			if err != nil {
				m.terminalErr = err
				return
			}
			task = resized
			m.terminalTasks[m.terminalIndex] = resized
		}
	}
	view := m.terminalViews[task.ID]
	view.screen.resize(task.Columns)
	for count := 0; count < maxTerminalRefreshChunks; count++ {
		read, err := m.terminalAttachment.Read(task.ID, view.cursor)
		if err != nil {
			m.terminalErr = err
			return
		}
		if read.Dropped {
			view.screen.reset()
			view.dropped = true
		}
		if read.Output != "" {
			view.screen.feed(read.Output)
		}
		if read.Next == view.cursor {
			break
		}
		view.cursor = read.Next
		if len(read.Output) == 0 {
			break
		}
	}
	m.terminalViews[task.ID] = view
}

func (m *Model) moveAttachedTerminal(delta int) {
	if len(m.terminalTasks) == 0 {
		return
	}
	m.terminalIndex = (m.terminalIndex + delta + len(m.terminalTasks)) % len(m.terminalTasks)
	m.terminalScroll = 0
	m.refreshAttachedTerminal()
}

func (m Model) selectedTerminalTask() (terminal.Task, bool) {
	if m.terminalIndex < 0 || m.terminalIndex >= len(m.terminalTasks) {
		return terminal.Task{}, false
	}
	return m.terminalTasks[m.terminalIndex], true
}

func (m Model) updateAttachedTerminal(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.terminalAttachment == nil {
		m.closeAttachedTerminal()
		m.notice = notice{text: "The attached terminal run has ended.", kind: noticeInfo}
		return m, nil
	}
	switch message.String() {
	case "esc", "ctrl+t":
		m.closeAttachedTerminal()
		return m, waitForExecution(m.execution)
	case "tab", "down", "ctrl+n":
		m.moveAttachedTerminal(1)
		return m, nil
	case "shift+tab", "up", "ctrl+p":
		m.moveAttachedTerminal(-1)
		return m, nil
	case "pgup":
		m.terminalScroll += max(1, m.height-18)
		return m, nil
	case "pgdown", "end":
		m.terminalScroll = max(0, m.terminalScroll-max(1, m.height-18))
		return m, nil
	case "home":
		m.terminalScroll = len(strings.Split(m.currentAttachedTerminalOutput(), "\n"))
		return m, nil
	case "ctrl+x":
		task, found := m.selectedTerminalTask()
		if !found {
			m.notice = notice{text: "No attached terminal task is selected.", kind: noticeError}
			return m, nil
		}
		if _, err := m.terminalAttachment.Stop(task.ID); err != nil {
			m.notice = notice{text: "Stop terminal task: " + err.Error(), kind: noticeError}
		} else {
			m.notice = notice{text: "Stopping " + task.ID + ".", kind: noticeInfo}
		}
		m.refreshAttachedTerminal()
		return m, nil
	case "ctrl+c":
		return m.writeAttachedTerminalInput([]byte{3}, "Interrupt sent to")
	case "enter":
		input := m.terminalInput.Value()
		m.terminalInput.Reset()
		return m.writeAttachedTerminalInput(append([]byte(input), '\r'), "Input sent to")
	}
	var command tea.Cmd
	m.terminalInput, command = m.terminalInput.Update(message)
	return m, command
}

func (m Model) writeAttachedTerminalInput(input []byte, action string) (tea.Model, tea.Cmd) {
	task, found := m.selectedTerminalTask()
	if !found {
		m.notice = notice{text: "No attached terminal task is selected.", kind: noticeError}
		return m, nil
	}
	if task.Status != "running" {
		m.notice = notice{text: "Terminal task " + task.ID + " has already exited.", kind: noticeError}
		return m, nil
	}
	if _, err := m.terminalAttachment.WriteDeveloper(task.ID, input); err != nil {
		m.notice = notice{text: "Write terminal task: " + err.Error(), kind: noticeError}
		return m, nil
	}
	m.notice = notice{text: action + " " + task.ID + " in its existing sandbox.", kind: noticeInfo}
	m.refreshAttachedTerminal()
	return m, waitForExecution(m.execution)
}

func (m Model) currentAttachedTerminalOutput() string {
	task, found := m.selectedTerminalTask()
	if !found {
		return ""
	}
	view := m.terminalViews[task.ID]
	output := view.screen.String()
	if view.dropped {
		return terminalDroppedScrollback + output
	}
	return output
}

func displayTerminalOutput(value string) string {
	value = ansi.Strip(value)
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	return strings.Map(func(character rune) rune {
		if character == '\n' || character == '\t' || character >= ' ' {
			return character
		}
		return -1
	}, value)
}

func (m Model) attachedTerminalSize() (int, int) {
	rows := min(120, max(6, m.height-13))
	columns := min(240, max(20, m.panelTextWidth()))
	return rows, columns
}

func (m Model) attachedTerminalView() string {
	if m.terminalAttachment == nil {
		return m.header("attached terminal") + "\n" + m.panel(dimStyle.Render("No active terminal manager is available.")) + "\n" + m.footer("esc return")
	}
	if len(m.terminalTasks) == 0 {
		return strings.Join([]string{
			m.header("attached terminal"),
			m.panel(dimStyle.Render("The agent has not started a terminal task in this run.")),
			m.noticeView(),
			m.footer("esc/ctrl+t return", "f1 shortcuts"),
		}, "\n")
	}
	task, _ := m.selectedTerminalTask()
	view := m.terminalViews[task.ID]
	tasks := make([]string, 0, len(m.terminalTasks))
	for index, candidate := range m.terminalTasks {
		prefix := "  "
		if index == m.terminalIndex {
			prefix = "> "
		}
		status := candidate.Status
		if candidate.ExitCode != nil {
			status += " · exit " + strconv.Itoa(*candidate.ExitCode)
		}
		command := displayTerminalOutput(strings.Join(candidate.Argv, " "))
		tasks = append(tasks, prefix+keyStyle.Render(candidate.ID)+" "+dimStyle.Render(compact(status+" · "+command, max(16, m.panelTextWidth()-14))))
	}
	output := m.visibleAttachedTerminalOutput(m.currentAttachedTerminalOutput())
	if output == "" {
		output = dimStyle.Render("Waiting for terminal output…")
	}
	if m.terminalErr != nil {
		output = errorStyle.Render("Terminal read error: " + m.terminalErr.Error())
	}
	sections := []string{
		m.header("attached terminal · " + task.ID),
		labelStyle.Render("Tasks") + "\n" + m.panel(strings.Join(tasks, "\n")),
		labelStyle.Render("Output") + "\n" + m.panel(output),
		labelStyle.Render("Input") + "\n" + m.terminalInput.View(),
		m.noticeView(),
		m.footer("enter send line", "ctrl+c interrupt task", "ctrl+x stop task", "tab switch task", "pgup/pgdn scroll", "esc/ctrl+t return", "f1 shortcuts"),
	}
	return strings.Join(sections, "\n")
}

func (m Model) visibleAttachedTerminalOutput(output string) string {
	lines := strings.Split(output, "\n")
	visible := max(3, m.height-18)
	end := len(lines) - m.terminalScroll
	if end < 0 {
		end = 0
	}
	start := max(0, end-visible)
	selected := append([]string(nil), lines[start:end]...)
	if start > 0 {
		selected = append([]string{"… scroll up for earlier attached output …"}, selected...)
	}
	if end < len(lines) {
		selected = append(selected, "… scroll down for latest attached output …")
	}
	return strings.Join(selected, "\n")
}
