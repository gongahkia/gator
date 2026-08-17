package tui

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"
	"github.com/rivo/uniseg"
)

const maxVimUndoEntries = 100

type vimSnapshot struct {
	value  string
	row    int
	column int
}

type vimRegister struct {
	text     string
	linewise bool
}

type vimDisplayRow struct {
	line  int
	first bool
}

type vimLineNumberState struct {
	abs     bool
	rel     bool
	cursor  int
	digits  int
	rows    []vimDisplayRow
	enabled bool
}

func (m Model) composerInputWidth() int {
	width := max(1, m.conversationWidth()-4)
	if m.drawerUsesSidePane() && m.drawerSection == drawerRuntime && m.focus != taskField {
		width = max(1, m.drawerWidth()-4)
	}
	return width
}

func (m *Model) syncVimLineNumbers() {
	if m.vimNumberState == nil {
		m.vimNumberState = &vimLineNumberState{}
	}
	state := m.vimNumberState
	state.enabled = m.vim != vimOff && (m.vimAbsoluteNumbers || m.vimRelativeNumbers)
	state.abs = m.vimAbsoluteNumbers
	state.rel = m.vimRelativeNumbers
	m.task.ShowLineNumbers = false
	if !state.enabled {
		m.task.SetPromptFunc(0, func(int) string { return "" })
		m.task.SetWidth(m.composerInputWidth())
		return
	}

	state.digits = len(strconv.Itoa(max(1, m.task.LineCount())))
	m.task.SetPromptFunc(state.digits+1, func(displayLine int) string {
		return state.prompt(displayLine)
	})
	m.task.SetWidth(m.composerInputWidth())
	state.cursor = m.task.Line()
	state.rows = vimDisplayRows(m.task.Value(), m.task.Width())
}

func (s vimLineNumberState) prompt(displayLine int) string {
	padding := strings.Repeat(" ", s.digits+1)
	if !s.enabled || displayLine < 0 || displayLine >= len(s.rows) || !s.rows[displayLine].first {
		return padding
	}
	line := s.rows[displayLine].line
	value := 0
	switch {
	case s.rel && line != s.cursor:
		value = vimAbs(line - s.cursor)
	case s.abs:
		value = line + 1
	case s.rel:
		value = 0
	default:
		return padding
	}
	return fmt.Sprintf("%*d ", s.digits, value)
}

func (s vimLineNumberState) emptyPrompt() string {
	if !s.enabled {
		return ""
	}
	value := 0
	if s.abs {
		value = 1
	}
	return fmt.Sprintf("%*d ", max(1, s.digits), value)
}

func vimDisplayRows(value string, width int) []vimDisplayRow {
	lines := strings.Split(value, "\n")
	rows := make([]vimDisplayRow, 0, len(lines))
	for lineIndex, line := range lines {
		for wrappedIndex := range vimWrap([]rune(line), width) {
			rows = append(rows, vimDisplayRow{line: lineIndex, first: wrappedIndex == 0})
		}
	}
	return rows
}

// vimWrap mirrors bubbles/textarea's wrapping so the custom prompt gutter
// stays aligned with the editor's physical rows, including soft wraps.
func vimWrap(runes []rune, width int) [][]rune {
	if width < 1 {
		return [][]rune{{}}
	}
	lines := [][]rune{{}}
	word := []rune{}
	row := 0
	spaces := 0
	for _, character := range runes {
		if unicode.IsSpace(character) {
			spaces++
		} else {
			word = append(word, character)
		}
		if spaces > 0 {
			if uniseg.StringWidth(string(lines[row]))+uniseg.StringWidth(string(word))+spaces > width {
				row++
				lines = append(lines, []rune{})
				lines[row] = append(lines[row], word...)
				lines[row] = append(lines[row], []rune(strings.Repeat(" ", spaces))...)
				spaces = 0
				word = nil
			} else {
				lines[row] = append(lines[row], word...)
				lines[row] = append(lines[row], []rune(strings.Repeat(" ", spaces))...)
				spaces = 0
				word = nil
			}
		} else if len(word) > 0 {
			lastWidth := runewidth.RuneWidth(word[len(word)-1])
			if uniseg.StringWidth(string(word))+lastWidth > width {
				if len(lines[row]) > 0 {
					row++
					lines = append(lines, []rune{})
				}
				lines[row] = append(lines[row], word...)
				word = nil
			}
		}
	}
	if uniseg.StringWidth(string(lines[row]))+uniseg.StringWidth(string(word))+spaces >= width {
		lines = append(lines, []rune{})
		lines[row+1] = append(lines[row+1], word...)
		lines[row+1] = append(lines[row+1], ' ')
	} else {
		lines[row] = append(lines[row], word...)
		lines[row] = append(lines[row], []rune(strings.Repeat(" ", spaces+1))...)
	}
	return lines
}

func (m Model) vimSnapshot() vimSnapshot {
	row, column := m.vimCursor()
	return vimSnapshot{value: m.task.Value(), row: row, column: column}
}

func (m Model) vimCursor() (int, int) {
	lines := strings.Split(m.task.Value(), "\n")
	row := min(max(0, m.task.Line()), len(lines)-1)
	info := m.task.LineInfo()
	column := min(len([]rune(lines[row])), max(0, info.StartColumn+info.ColumnOffset))
	return row, column
}

func (m *Model) rememberVimEdit(before vimSnapshot) {
	after := m.vimSnapshot()
	if before.value == after.value {
		return
	}
	m.vimUndo = append(m.vimUndo, before)
	if len(m.vimUndo) > maxVimUndoEntries {
		m.vimUndo = m.vimUndo[len(m.vimUndo)-maxVimUndoEntries:]
	}
	m.vimRedo = nil
}

func (m *Model) restoreVimSnapshot(snapshot vimSnapshot) {
	m.task.SetValue(snapshot.value)
	m.setVimCursor(snapshot.row, snapshot.column)
	m.persistDraft()
	m.refreshPreflight()
	m.syncVimLineNumbers()
}

func (m *Model) undoVimEdit() {
	if len(m.vimUndo) == 0 {
		m.notice = notice{text: "Already at the oldest Vim change.", kind: noticeInfo}
		return
	}
	current := m.vimSnapshot()
	last := len(m.vimUndo) - 1
	m.vimRedo = append(m.vimRedo, current)
	snapshot := m.vimUndo[last]
	m.vimUndo = m.vimUndo[:last]
	m.restoreVimSnapshot(snapshot)
	m.notice = notice{text: "Vim change undone.", kind: noticeInfo}
}

func (m *Model) redoVimEdit() {
	if len(m.vimRedo) == 0 {
		m.notice = notice{text: "Already at the newest Vim change.", kind: noticeInfo}
		return
	}
	current := m.vimSnapshot()
	last := len(m.vimRedo) - 1
	m.vimUndo = append(m.vimUndo, current)
	snapshot := m.vimRedo[last]
	m.vimRedo = m.vimRedo[:last]
	m.restoreVimSnapshot(snapshot)
	m.notice = notice{text: "Vim change redone.", kind: noticeInfo}
}

func (m *Model) replaceVimBuffer(value string, row, column int) {
	before := m.vimSnapshot()
	m.task.SetValue(value)
	m.setVimCursor(row, column)
	m.rememberVimEdit(before)
	m.persistDraft()
	m.refreshPreflight()
	m.syncVimLineNumbers()
}

func (m *Model) setVimCursor(row, column int) {
	lines := strings.Split(m.task.Value(), "\n")
	row = min(max(0, row), len(lines)-1)
	limit := max(1, m.task.LineCount()*(m.task.Width()+2))
	for steps := 0; m.task.Line() > row && steps < limit; steps++ {
		m.task.CursorUp()
	}
	for steps := 0; m.task.Line() < row && steps < limit; steps++ {
		m.task.CursorDown()
	}
	m.task.SetCursor(min(max(0, column), len([]rune(lines[row]))))
}

func (m *Model) moveVimCursor(row, column int) {
	m.setVimCursor(row, column)
	m.syncVimLineNumbers()
}

func (m *Model) resetVimPending() {
	m.vimCount = 0
	m.vimPending = ""
	m.vimPendingCount = 0
}

func (m *Model) vimCountOrOne() int {
	count := m.vimCount
	m.vimCount = 0
	if count < 1 {
		return 1
	}
	return count
}

func vimLines(value string) []string {
	return strings.Split(value, "\n")
}

func vimOffset(lines []string, row, column int) int {
	row = min(max(0, row), len(lines)-1)
	offset := 0
	for index := 0; index < row; index++ {
		offset += len([]rune(lines[index])) + 1
	}
	return offset + min(max(0, column), len([]rune(lines[row])))
}

func vimPosition(lines []string, offset int) (int, int) {
	offset = max(0, offset)
	for row, line := range lines {
		length := len([]rune(line))
		if offset <= length {
			return row, offset
		}
		offset -= length + 1
	}
	last := len(lines) - 1
	return last, len([]rune(lines[last]))
}

func isVimWord(character rune) bool {
	return unicode.IsLetter(character) || unicode.IsDigit(character) || character == '_'
}

func vimAbs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func vimNextWordStart(text []rune, offset, count int) int {
	position := min(max(0, offset), len(text))
	for range count {
		for position < len(text) && isVimWord(text[position]) {
			position++
		}
		for position < len(text) && !isVimWord(text[position]) {
			position++
		}
	}
	return position
}

func vimPreviousWordStart(text []rune, offset, count int) int {
	position := min(max(0, offset), len(text))
	for range count {
		if position > 0 {
			position--
		}
		for position > 0 && !isVimWord(text[position]) {
			position--
		}
		for position > 0 && isVimWord(text[position-1]) {
			position--
		}
	}
	return position
}

func vimWordEnd(text []rune, offset, count int) int {
	position := min(max(0, offset), len(text))
	for step := 0; step < count; step++ {
		if step > 0 && position < len(text) {
			position++
		}
		for position < len(text) && !isVimWord(text[position]) {
			position++
		}
		for position+1 < len(text) && isVimWord(text[position+1]) {
			position++
		}
	}
	return min(position, max(0, len(text)-1))
}

func (m Model) updateVimNormalMode(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	defer m.syncVimLineNumbers()
	key := message.String()
	if key == "esc" {
		m.resetVimPending()
		m.notice = notice{text: "Vim Normal mode. Enter sends; : opens Ex commands.", kind: noticeInfo}
		return m, nil
	}
	if m.vimPending != "" {
		return m.resolveVimPending(message)
	}
	if len(message.Runes) == 1 && unicode.IsDigit(message.Runes[0]) && (message.Runes[0] != '0' || m.vimCount > 0) {
		m.vimCount = min(9999, m.vimCount*10+int(message.Runes[0]-'0'))
		return m, nil
	}
	count := m.vimCountOrOne()
	switch key {
	case "enter":
		return m.startRun()
	case ":":
		m.vimCommand = ":"
		m.notice = notice{text: "Vim Ex command. :w sends · :set configures line numbers · :help lists supported commands.", kind: noticeInfo}
	case "i":
		m.vim = vimInsert
		m.notice = notice{text: "Vim Insert mode. Esc returns to Normal mode; Enter adds a line.", kind: noticeInfo}
	case "a":
		row, column := m.vimCursor()
		line := []rune(vimLines(m.task.Value())[row])
		m.moveVimCursor(row, min(len(line), column+1))
		m.vim = vimInsert
	case "I":
		row, _ := m.vimCursor()
		m.moveVimCursor(row, vimFirstNonBlank(vimLines(m.task.Value())[row]))
		m.vim = vimInsert
	case "A":
		row, _ := m.vimCursor()
		m.moveVimCursor(row, len([]rune(vimLines(m.task.Value())[row])))
		m.vim = vimInsert
	case "o", "O":
		row, _ := m.vimCursor()
		if key == "o" {
			m.moveVimCursor(row, len([]rune(vimLines(m.task.Value())[row])))
		} else {
			m.moveVimCursor(row, 0)
		}
		command := m.updateTask(tea.KeyMsg{Type: tea.KeyEnter})
		if key == "O" {
			m.task.CursorUp()
			m.task.CursorStart()
		}
		m.vim = vimInsert
		m.notice = notice{text: "Vim Insert mode. Esc returns to Normal mode; Enter adds a line.", kind: noticeInfo}
		return m, command
	case "h", "j", "k", "l", "left", "down", "up", "right", "home", "end", "0", "$", "^", "w", "b", "e", "G":
		m.applyVimMotion(key, count)
	case "g":
		m.vimPending = "g"
		m.vimPendingCount = count
	case "d", "c", "y":
		m.vimPending = key
		m.vimPendingCount = count
	case "D":
		m.applyVimOperator("d", "$", count)
	case "C":
		m.applyVimOperator("c", "$", count)
	case "Y":
		m.applyVimOperator("y", "y", count)
	case "x", "delete":
		m.deleteVimCharacters(count, false)
	case "X", "backspace":
		m.deleteVimCharacters(count, true)
	case "r":
		m.vimPending = "r"
		m.vimPendingCount = count
	case "p", "P":
		m.putVimRegister(key == "p")
	case "u":
		m.undoVimEdit()
	case "ctrl+r":
		m.redoVimEdit()
	default:
		m.notice = notice{text: "Unsupported Vim Normal command " + key + ". Use :help for the supported editing set.", kind: noticeError}
	}
	return m, nil
}

func (m Model) resolveVimPending(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := message.String()
	if key == "esc" {
		m.resetVimPending()
		return m, nil
	}
	if len(message.Runes) == 1 && unicode.IsDigit(message.Runes[0]) && (message.Runes[0] != '0' || m.vimCount > 0) {
		m.vimCount = min(9999, m.vimCount*10+int(message.Runes[0]-'0'))
		return m, nil
	}
	pending := m.vimPending
	count := m.vimPendingCount * m.vimCountOrOne()
	m.vimPending = ""
	m.vimPendingCount = 0
	switch pending {
	case "g":
		if key != "g" {
			m.notice = notice{text: "Unsupported g command. Use gg to move to the first line.", kind: noticeError}
			return m, nil
		}
		m.applyVimMotion("gg", count)
		return m, nil
	case "r":
		if len(message.Runes) != 1 {
			m.notice = notice{text: "r expects one replacement character.", kind: noticeError}
			return m, nil
		}
		m.replaceVimCharacters(message.Runes[0], count)
		return m, nil
	case "d", "c", "y":
		if key == "g" {
			m.vimPending = pending + "g"
			m.vimPendingCount = count
			return m, nil
		}
		m.applyVimOperator(pending, key, count)
		return m, nil
	case "dg", "cg", "yg":
		if key == "g" {
			m.applyVimOperator(string(pending[0]), "gg", count)
			return m, nil
		}
	}
	m.notice = notice{text: "Unsupported Vim command sequence.", kind: noticeError}
	return m, nil
}

func vimFirstNonBlank(line string) int {
	for index, character := range []rune(line) {
		if !unicode.IsSpace(character) {
			return index
		}
	}
	return len([]rune(line))
}

func (m *Model) applyVimMotion(motion string, count int) {
	lines := vimLines(m.task.Value())
	row, column := m.vimCursor()
	switch motion {
	case "h", "left":
		column = max(0, column-count)
	case "l", "right":
		column = min(len([]rune(lines[row])), column+count)
	case "j", "down":
		row = min(len(lines)-1, row+count)
		column = min(len([]rune(lines[row])), column)
	case "k", "up":
		row = max(0, row-count)
		column = min(len([]rune(lines[row])), column)
	case "0", "home":
		column = 0
	case "$", "end":
		row = min(len(lines)-1, row+count-1)
		column = len([]rune(lines[row]))
	case "^":
		column = vimFirstNonBlank(lines[row])
	case "w":
		row, column = vimPosition(lines, vimNextWordStart([]rune(m.task.Value()), vimOffset(lines, row, column), count))
	case "b":
		row, column = vimPosition(lines, vimPreviousWordStart([]rune(m.task.Value()), vimOffset(lines, row, column), count))
	case "e":
		row, column = vimPosition(lines, vimWordEnd([]rune(m.task.Value()), vimOffset(lines, row, column), count))
	case "gg":
		row = min(len(lines)-1, max(0, count-1))
		column = 0
	case "G":
		if count > 1 {
			row = min(len(lines)-1, count-1)
		} else {
			row = len(lines) - 1
		}
		column = 0
	}
	m.moveVimCursor(row, column)
}

func (m *Model) deleteVimCharacters(count int, backward bool) {
	lines := vimLines(m.task.Value())
	row, column := m.vimCursor()
	start := vimOffset(lines, row, column)
	end := min(len([]rune(m.task.Value())), start+count)
	if backward {
		start, end = max(0, start-count), start
	}
	m.applyVimRange("d", start, end, false, row, column)
}

func (m *Model) replaceVimCharacters(character rune, count int) {
	lines := vimLines(m.task.Value())
	row, column := m.vimCursor()
	start := vimOffset(lines, row, column)
	input := []rune(m.task.Value())
	end := min(len(input), start+count)
	if start == end || strings.ContainsRune(string(input[start:end]), '\n') {
		m.notice = notice{text: "r cannot replace past the end of this line.", kind: noticeError}
		return
	}
	replacement := strings.Repeat(string(character), end-start)
	m.replaceVimBuffer(string(input[:start])+replacement+string(input[end:]), row, column)
}

func (m *Model) applyVimOperator(operator, motion string, count int) {
	lines := vimLines(m.task.Value())
	row, column := m.vimCursor()
	start := vimOffset(lines, row, column)
	end := start
	linewise := false
	switch motion {
	case "d", "c", "y":
		linewise = true
		start = vimOffset(lines, row, 0)
		endRow := min(len(lines), row+count)
		if endRow < len(lines) {
			end = vimOffset(lines, endRow, 0)
		} else {
			end = len([]rune(m.task.Value()))
		}
	case "$":
		targetRow := min(len(lines)-1, row+count-1)
		end = vimOffset(lines, targetRow, len([]rune(lines[targetRow])))
	case "0", "^":
		start = vimOffset(lines, row, 0)
		if motion == "^" {
			start = vimOffset(lines, row, vimFirstNonBlank(lines[row]))
		}
	case "w":
		end = vimNextWordStart([]rune(m.task.Value()), start, count)
		if end == start {
			end = min(len([]rune(m.task.Value())), vimWordEnd([]rune(m.task.Value()), start, 1)+1)
		}
	case "e":
		end = min(len([]rune(m.task.Value())), vimWordEnd([]rune(m.task.Value()), start, count)+1)
	case "b":
		start = vimPreviousWordStart([]rune(m.task.Value()), start, count)
	case "j":
		targetRow := min(len(lines)-1, row+count)
		end = vimOffset(lines, targetRow, len([]rune(lines[targetRow])))
	case "k":
		targetRow := max(0, row-count)
		start = vimOffset(lines, targetRow, 0)
		end = vimOffset(lines, row, len([]rune(lines[row])))
	case "G":
		end = len([]rune(m.task.Value()))
	case "gg":
		start = 0
		end = vimOffset(lines, row, len([]rune(lines[row])))
	default:
		m.notice = notice{text: "Unsupported Vim operator motion " + motion + ".", kind: noticeError}
		return
	}
	if start > end {
		start, end = end, start
	}
	m.applyVimRange(operator, start, end, linewise, row, column)
}

func (m *Model) applyVimRange(operator string, start, end int, linewise bool, row, column int) {
	input := []rune(m.task.Value())
	start = min(max(0, start), len(input))
	end = min(max(start, end), len(input))
	if start == end {
		m.notice = notice{text: "Vim motion reached the edge of the composer.", kind: noticeInfo}
		return
	}
	m.vimRegister = vimRegister{text: string(input[start:end]), linewise: linewise}
	if operator == "y" {
		m.notice = notice{text: "Yanked to Gator's local Vim register.", kind: noticeInfo}
		return
	}
	if operator == "c" && linewise {
		lines := vimLines(m.task.Value())
		count := max(1, strings.Count(m.vimRegister.text, "\n"))
		endRow := min(len(lines), row+count)
		lines = append(lines[:row], append([]string{""}, lines[endRow:]...)...)
		m.replaceVimBuffer(strings.Join(lines, "\n"), row, 0)
		m.vim = vimInsert
		return
	}
	value := string(input[:start]) + string(input[end:])
	lines := vimLines(value)
	targetRow, targetColumn := vimPosition(lines, start)
	m.replaceVimBuffer(value, targetRow, targetColumn)
	if operator == "c" {
		m.vim = vimInsert
	}
}

func (m *Model) putVimRegister(after bool) {
	if m.vimRegister.text == "" {
		m.notice = notice{text: "Gator's local Vim register is empty.", kind: noticeInfo}
		return
	}
	lines := vimLines(m.task.Value())
	row, column := m.vimCursor()
	if m.vimRegister.linewise {
		registerLines := strings.Split(strings.TrimSuffix(m.vimRegister.text, "\n"), "\n")
		at := row
		if after {
			at++
		}
		updated := append([]string{}, lines[:at]...)
		updated = append(updated, registerLines...)
		updated = append(updated, lines[at:]...)
		m.replaceVimBuffer(strings.Join(updated, "\n"), at, 0)
		return
	}
	input := []rune(m.task.Value())
	position := vimOffset(lines, row, column)
	if after && position < len(input) && input[position] != '\n' {
		position++
	}
	insert := []rune(m.vimRegister.text)
	value := string(input[:position]) + string(insert) + string(input[position:])
	targetRow, targetColumn := vimPosition(vimLines(value), position+len(insert))
	m.replaceVimBuffer(value, targetRow, targetColumn)
}

func (m Model) updateVimExCommand(message tea.KeyMsg, running bool) (tea.Model, tea.Cmd) {
	switch message.String() {
	case "esc":
		m.vimCommand = ""
		m.notice = notice{text: "Vim command cancelled.", kind: noticeInfo}
		return m, nil
	case "backspace", "delete":
		command := []rune(m.vimCommand)
		if len(command) > 1 {
			m.vimCommand = string(command[:len(command)-1])
		}
		return m, nil
	case "enter":
		command := strings.TrimSpace(m.vimCommand)
		m.vimCommand = ""
		return m.executeVimExCommand(command, running)
	}
	if len(message.Runes) > 0 {
		m.vimCommand += string(message.Runes)
	}
	return m, nil
}

func (m Model) executeVimExCommand(command string, running bool) (tea.Model, tea.Cmd) {
	switch command {
	case ":w":
		if running {
			return m.steerCurrentInput()
		}
		return m.startRun()
	case ":wq", ":x":
		var next tea.Model
		var runCommand tea.Cmd
		if running {
			next, runCommand = m.steerCurrentInput()
		} else {
			next, runCommand = m.startRun()
		}
		updated := next.(Model)
		if (!running && (updated.screen == runningScreen || updated.screen == attachmentConfirmScreen)) || (running && updated.task.Value() == "") {
			updated.quitAfterRun = true
			updated.notice = notice{text: "Submission accepted. Gator will exit after active and queued work completes successfully.", kind: noticeInfo}
		}
		return updated, runCommand
	case ":q", ":qa":
		if running {
			m.notice = notice{text: "An active run owns this terminal. Use Ctrl+C to stop it, then :q to exit.", kind: noticeError}
			return m, nil
		}
		if strings.TrimSpace(m.task.Value()) != "" {
			m.notice = notice{text: "Unsent composer text remains. Use :q! to discard it and exit.", kind: noticeError}
			return m, nil
		}
		return m, tea.Quit
	case ":q!", ":qa!":
		if running {
			m.notice = notice{text: "An active run owns this terminal. Use Ctrl+C to stop it before exiting.", kind: noticeError}
			return m, nil
		}
		return m, tea.Quit
	case ":help":
		m.notice = notice{text: "Normal: motions h/j/k/l, 0/^/$, w/b/e, gg/G; edits i/I/a/A/o/O, x/X/r, d/c/y + motions, p/P, u/Ctrl+R. Ex: :w, :wq, :x, :q!, :set.", kind: noticeInfo}
		return m, nil
	case ":set":
		m.notice = notice{text: "Vim numbers: " + m.vimNumberSettings() + ". Use :set number relativenumber (or nonumber/norelativenumber).", kind: noticeInfo}
		return m, nil
	}
	if strings.HasPrefix(command, ":set ") {
		options := strings.Fields(strings.TrimPrefix(command, ":set "))
		if m.applyVimSetOptions(options) {
			m.syncVimLineNumbers()
			m.notice = notice{text: "Vim numbers: " + m.vimNumberSettings() + ".", kind: noticeSuccess}
			return m, nil
		}
	}
	if line, err := strconv.Atoi(strings.TrimPrefix(command, ":")); err == nil && line > 0 {
		m.applyVimMotion("G", line)
		return m, nil
	}
	m.notice = notice{text: "Unsupported Vim command " + command + ". Use :help.", kind: noticeError}
	return m, nil
}

func (m *Model) applyVimSetOptions(options []string) bool {
	if len(options) == 0 {
		return false
	}
	for _, option := range options {
		switch option {
		case "number", "nu":
			m.vimAbsoluteNumbers = true
		case "nonumber", "nonu":
			m.vimAbsoluteNumbers = false
		case "relativenumber", "rnu":
			m.vimRelativeNumbers = true
		case "norelativenumber", "nornu":
			m.vimRelativeNumbers = false
		case "number!", "nu!":
			m.vimAbsoluteNumbers = !m.vimAbsoluteNumbers
		case "relativenumber!", "rnu!":
			m.vimRelativeNumbers = !m.vimRelativeNumbers
		default:
			m.notice = notice{text: "Unsupported :set option " + option + ". Use number, relativenumber, nonumber, or norelativenumber.", kind: noticeError}
			return false
		}
	}
	return true
}

func (m Model) vimNumberSettings() string {
	settings := make([]string, 0, 2)
	if m.vimAbsoluteNumbers {
		settings = append(settings, "number")
	} else {
		settings = append(settings, "nonumber")
	}
	if m.vimRelativeNumbers {
		settings = append(settings, "relativenumber")
	} else {
		settings = append(settings, "norelativenumber")
	}
	return strings.Join(settings, " ")
}
