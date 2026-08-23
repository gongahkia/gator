package tui

import (
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/gongahkia/gator/internal/terminal"
)

const (
	maxTerminalRefreshChunks  = 8
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
	if m.terminalRawInput && m.terminalAttachment != nil {
		m.flushAttachedTerminalRawInput()
	}
	for _, view := range m.terminalViews {
		view.screen.dispose()
	}
	m.terminalAttachment = attachment
	m.terminalRawInput = false
	m.terminalTasks = nil
	m.terminalIndex = 0
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
	m.terminalRawInput = false
	m.refreshAttachedTerminal()
	_ = m.terminalInput.Focus()
}

func (m *Model) closeAttachedTerminal() {
	m.flushAttachedTerminalRawInput()
	m.terminalRawInput = false
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
	view.screen.resize(task.Rows, task.Columns)
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
			if response := view.screen.takeProtocol(); len(response) > 0 {
				if _, err := m.terminalAttachment.WriteProtocol(task.ID, response); err != nil {
					m.terminalViews[task.ID] = view
					m.terminalErr = err
					return
				}
			}
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
	m.refreshAttachedTerminal()
}

func (m Model) selectedTerminalTask() (terminal.Task, bool) {
	if m.terminalIndex < 0 || m.terminalIndex >= len(m.terminalTasks) {
		return terminal.Task{}, false
	}
	return m.terminalTasks[m.terminalIndex], true
}

func (m Model) hasBackgroundTerminalTask() bool {
	if m.terminalAttachment == nil {
		return false
	}
	for _, task := range m.terminalAttachment.List() {
		if task.Background && task.Status == "running" {
			return true
		}
	}
	return false
}

func (m Model) updateAttachedTerminal(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.terminalAttachment == nil {
		m.closeAttachedTerminal()
		m.notice = notice{text: "The attached terminal run has ended.", kind: noticeInfo}
		return m, nil
	}
	if m.terminalRawInput {
		return m.updateAttachedTerminalRawInput(message)
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
		m.scrollAttachedTerminal(-1)
		return m, nil
	case "pgdown", "end":
		m.scrollAttachedTerminal(1)
		return m, nil
	case "home":
		m.scrollAttachedTerminalToTop()
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
	case "ctrl+o":
		task, found := m.selectedTerminalTask()
		if !found || task.Status != "running" {
			m.notice = notice{text: "Select a running terminal task before enabling raw keyboard input.", kind: noticeError}
			return m, nil
		}
		m.terminalRawInput = true
		m.terminalInput.Blur()
		m.notice = notice{text: "Raw terminal keyboard enabled. Ctrl+] returns to Gator controls.", kind: noticeInfo}
		return m, nil
	case "enter":
		input := m.terminalInput.Value()
		m.terminalInput.Reset()
		return m.writeAttachedTerminalInput(append([]byte(input), '\r'), "Input sent to")
	}
	var command tea.Cmd
	m.terminalInput, command = m.terminalInput.Update(message)
	return m, command
}

func (m Model) updateAttachedTerminalRawInput(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	if message.Type == tea.KeyCtrlCloseBracket && !message.Alt {
		m.flushAttachedTerminalRawInput()
		m.terminalRawInput = false
		_ = m.terminalInput.Focus()
		if m.notice.kind != noticeError {
			m.notice = notice{text: "Raw terminal keyboard disabled. Line input is ready.", kind: noticeInfo}
		}
		return m, nil
	}
	input, supported := attachedTerminalKeyInput(message)
	if !supported {
		m.notice = notice{text: "That key is not available in raw terminal input.", kind: noticeInfo}
		return m, nil
	}
	return m.writeAttachedTerminalRawInput(input)
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

func (m Model) writeAttachedTerminalRawInput(input []byte) (tea.Model, tea.Cmd) {
	task, found := m.selectedTerminalTask()
	if !found {
		m.notice = notice{text: "No attached terminal task is selected.", kind: noticeError}
		return m, nil
	}
	if task.Status != "running" {
		m.notice = notice{text: "Terminal task " + task.ID + " has already exited.", kind: noticeError}
		return m, nil
	}
	if _, err := m.terminalAttachment.WriteDeveloperRaw(task.ID, input); err != nil {
		m.notice = notice{text: "Write terminal task: " + err.Error(), kind: noticeError}
		return m, nil
	}
	m.refreshAttachedTerminal()
	return m, nil
}

func (m *Model) flushAttachedTerminalRawInput() {
	if !m.terminalRawInput || m.terminalAttachment == nil {
		return
	}
	task, found := m.selectedTerminalTask()
	if !found {
		return
	}
	if _, err := m.terminalAttachment.FlushDeveloperInput(task.ID); err != nil {
		m.notice = notice{text: "Record raw terminal input: " + err.Error(), kind: noticeError}
	}
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
	rows := min(120, max(3, m.height-18))
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
		if candidate.Background {
			status += " · background"
		}
		command := displayTerminalOutput(strings.Join(candidate.Argv, " "))
		tasks = append(tasks, prefix+keyStyle.Render(candidate.ID)+" "+dimStyle.Render(compact(status+" · "+command, max(16, m.panelTextWidth()-14))))
	}
	view := m.terminalViews[task.ID]
	output := view.screen.viewport()
	if view.dropped {
		output = terminalDroppedScrollback + output
	}
	if output == "" {
		output = dimStyle.Render("Waiting for terminal output…")
	}
	if m.terminalErr != nil {
		output = errorStyle.Render("Terminal read error: " + m.terminalErr.Error())
	}
	input := m.terminalInput.View()
	footer := m.footer("enter send line", "ctrl+o raw keyboard", "ctrl+c interrupt task", "ctrl+x stop task", "tab switch task", "pgup/pgdn scroll", "esc/ctrl+t return", "f1 shortcuts")
	if m.terminalRawInput {
		input = dimStyle.Render("Raw keyboard is active; keys go directly to the task. Ctrl+] returns to Gator controls.")
		footer = m.footer("ctrl+] controls", "keys send to task", "raw input stays sandboxed", "f1 sends F1 to task")
	}
	sections := []string{
		m.header("attached terminal · " + task.ID),
		labelStyle.Render("Tasks") + "\n" + m.panel(strings.Join(tasks, "\n")),
		labelStyle.Render("Output") + "\n" + m.panel(output),
		labelStyle.Render("Input") + "\n" + input,
		m.noticeView(),
		footer,
	}
	return strings.Join(sections, "\n")
}

func (m *Model) scrollAttachedTerminal(pages int) {
	task, found := m.selectedTerminalTask()
	if !found {
		return
	}
	view := m.terminalViews[task.ID]
	view.screen.scrollPages(pages)
	m.terminalViews[task.ID] = view
}

func (m *Model) scrollAttachedTerminalToTop() {
	task, found := m.selectedTerminalTask()
	if !found {
		return
	}
	view := m.terminalViews[task.ID]
	view.screen.scrollToTop()
	m.terminalViews[task.ID] = view
}
