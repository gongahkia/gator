package tui

import (
	"fmt"
	"strings"

	xterm "github.com/gitpod-io/xterm-go"
)

const (
	maxAttachedTerminalScreenRows    = 1024
	maxTerminalProtocolResponseBytes = 4 * 1024
)

// terminalDisplay is the bounded, headless terminal emulator behind an
// attached PTY. It consumes task output into a VT500/xterm screen buffer; it
// never forwards that output to Gator's host terminal.
type terminalDisplay struct {
	terminal *xterm.Terminal
	rows     int
	columns  int
	protocol []byte
}

func (s *terminalDisplay) resize(rows, columns int) {
	if rows < 1 {
		rows = 1
	}
	if columns < 1 {
		columns = 1
	}
	if s.terminal == nil {
		s.newTerminal(rows, columns)
		return
	}
	s.rows, s.columns = rows, columns
	s.terminal.Resize(columns, rows)
}

func (s *terminalDisplay) newTerminal(rows, columns int) {
	colorSchemeQuery := false
	s.rows, s.columns = rows, columns
	s.terminal = xterm.New(
		xterm.WithCols(columns),
		xterm.WithRows(rows),
		xterm.WithScrollback(maxAttachedTerminalScreenRows),
		xterm.WithVtExtensions(xterm.VtExtensions{ColorSchemeQuery: &colorSchemeQuery}),
	)
	s.terminal.OnData(func(value string) {
		remaining := maxTerminalProtocolResponseBytes - len(s.protocol)
		if remaining <= 0 {
			return
		}
		if len(value) > remaining {
			value = value[:remaining]
		}
		s.protocol = append(s.protocol, value...)
	})
}

func (s *terminalDisplay) reset() {
	rows, columns := s.rows, s.columns
	s.dispose()
	*s = terminalDisplay{}
	s.resize(rows, columns)
}

func (s *terminalDisplay) dispose() {
	if s.terminal != nil {
		s.terminal.Dispose()
	}
	s.terminal = nil
	s.protocol = nil
}

func (s *terminalDisplay) feed(value string) {
	if value == "" {
		return
	}
	if s.terminal == nil {
		s.resize(24, 80)
	}
	s.terminal.WriteString(value)
}

func (s *terminalDisplay) takeProtocol() []byte {
	input := append([]byte(nil), s.protocol...)
	s.protocol = nil
	return input
}

func (s terminalDisplay) String() string {
	if s.terminal == nil {
		return ""
	}
	buffer := s.terminal.Buffer()
	lines := make([]string, buffer.Lines.Length())
	for index := range lines {
		lines[index] = buffer.TranslateBufferLineToString(index, true, 0, -1)
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

func (s terminalDisplay) viewport() string {
	if s.terminal == nil {
		return ""
	}
	buffer := s.terminal.Buffer()
	lines := make([]string, s.terminal.Rows())
	for row := range lines {
		lines[row] = s.renderLine(buffer.Lines.Get(buffer.YDisp+row), row)
	}
	return strings.Join(lines, "\n")
}

func (s terminalDisplay) renderLine(line *xterm.BufferLine, row int) string {
	if s.terminal == nil || line == nil {
		return strings.Repeat(" ", s.columns)
	}
	columns := s.terminal.Cols()
	var output strings.Builder
	var run strings.Builder
	var previous xterm.CellData
	hasPrevious := false
	previousCursor := false
	buffer := s.terminal.Buffer()
	cursorVisible := !s.terminal.IsCursorHidden() && buffer.YDisp == buffer.YBase && row == s.terminal.CursorY()

	flush := func() {
		if !hasPrevious || run.Len() == 0 {
			return
		}
		output.WriteString(renderTerminalRun(previous, previousCursor, run.String()))
		run.Reset()
	}
	for column := 0; column < columns; column++ {
		cell := line.LoadCell(column, &xterm.CellData{})
		if cell.GetWidth() == 0 {
			continue
		}
		cursor := cursorVisible && column == s.terminal.CursorX()
		if hasPrevious && (cursor != previousCursor || !cell.AttributesEqual(&previous)) {
			flush()
			hasPrevious = false
		}
		if !hasPrevious {
			previous = *cell
			previousCursor = cursor
			hasPrevious = true
		}
		content := cell.GetChars()
		if content == "" {
			content = " "
		}
		run.WriteString(content)
	}
	flush()
	return output.String()
}

func (s *terminalDisplay) scrollPages(pages int) {
	if s.terminal != nil {
		s.terminal.ScrollPages(pages)
	}
}

func (s *terminalDisplay) scrollToTop() {
	if s.terminal != nil {
		s.terminal.ScrollToTop()
	}
}

func (s *terminalDisplay) scrollToBottom() {
	if s.terminal != nil {
		s.terminal.ScrollToBottom()
	}
}

func renderTerminalRun(cell xterm.CellData, cursor bool, value string) string {
	parameters := make([]string, 0, 12)
	if cell.IsBold() != 0 {
		parameters = append(parameters, "1")
	}
	if cell.IsDim() != 0 {
		parameters = append(parameters, "2")
	}
	if cell.IsItalic() != 0 {
		parameters = append(parameters, "3")
	}
	if cell.IsUnderline() != 0 {
		switch cell.GetUnderlineStyle() {
		case xterm.UnderlineStyleDouble:
			parameters = append(parameters, "21")
		default:
			parameters = append(parameters, "4")
		}
	}
	if cell.IsBlink() != 0 {
		parameters = append(parameters, "5")
	}
	if cell.IsInverse() != 0 || cursor {
		parameters = append(parameters, "7")
	}
	if cell.IsInvisible() != 0 {
		parameters = append(parameters, "8")
	}
	if cell.IsStrikethrough() != 0 {
		parameters = append(parameters, "9")
	}
	if cell.IsOverline() != 0 {
		parameters = append(parameters, "53")
	}
	parameters = append(parameters, terminalColorParameters(cell, true)...)
	parameters = append(parameters, terminalColorParameters(cell, false)...)
	if len(parameters) == 0 {
		return value
	}
	return "\x1b[" + strings.Join(parameters, ";") + "m" + value + "\x1b[0m"
}

func terminalColorParameters(cell xterm.CellData, foreground bool) []string {
	mode := cell.GetFgColorMode()
	color := cell.GetFgColor()
	prefix := "38"
	if !foreground {
		mode = cell.GetBgColorMode()
		color = cell.GetBgColor()
		prefix = "48"
	}
	switch mode {
	case xterm.AttrCMP16, xterm.AttrCMP256:
		return []string{prefix, "5", fmt.Sprintf("%d", color)}
	case xterm.AttrCMRGB:
		rgb := xterm.ToColorRGB(uint32(color))
		return []string{prefix, "2", fmt.Sprintf("%d", rgb[0]), fmt.Sprintf("%d", rgb[1]), fmt.Sprintf("%d", rgb[2])}
	default:
		return nil
	}
}
