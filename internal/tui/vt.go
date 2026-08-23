package tui

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

const maxAttachedTerminalScreenRows = 1024

// terminalDisplay is a deliberately bounded display interpreter for an
// attached PTY. It is not an escape hatch into the host terminal: it consumes
// output already owned by terminal.Manager and handles the common cursor and
// erase operations needed by progress bars, test dashboards, and simple TUI
// programs. Unknown control sequences are ignored rather than rendered.
type terminalDisplay struct {
	width           int
	lines           [][]rune
	row             int
	column          int
	savedRow        int
	savedCol        int
	primaryLines    [][]rune
	primaryRow      int
	primaryColumn   int
	primarySavedRow int
	primarySavedCol int
	alternate       bool
	pending         []byte
}

func (s *terminalDisplay) resize(width int) {
	if width < 1 {
		width = 1
	}
	s.width = width
}

func (s *terminalDisplay) reset() {
	width := s.width
	*s = terminalDisplay{width: width}
}

func (s *terminalDisplay) feed(value string) {
	if value == "" {
		return
	}
	data := append(append([]byte(nil), s.pending...), []byte(value)...)
	s.pending = nil
	for index := 0; index < len(data); {
		current := data[index]
		switch current {
		case 0x1b:
			consumed, complete := s.escape(data[index:])
			if !complete {
				s.pending = append(s.pending, data[index:]...)
				return
			}
			index += consumed
		case '\r':
			s.column = 0
			index++
		case '\n':
			s.row++
			s.ensureRow(s.row)
			index++
		case '\b':
			if s.column > 0 {
				s.column--
			}
			index++
		case '\t':
			next := ((s.column / 8) + 1) * 8
			for s.column < next {
				s.put(' ')
			}
			index++
		default:
			if current < 0x20 || current == 0x7f {
				index++
				continue
			}
			runeValue, size := utf8.DecodeRune(data[index:])
			if runeValue == utf8.RuneError && size == 1 && !utf8.FullRune(data[index:]) {
				s.pending = append(s.pending, data[index:]...)
				return
			}
			s.put(runeValue)
			index += size
		}
	}
}

func (s *terminalDisplay) escape(value []byte) (int, bool) {
	if len(value) < 2 {
		return 0, false
	}
	switch value[1] {
	case '[':
		for index := 2; index < len(value); index++ {
			if value[index] >= 0x40 && value[index] <= 0x7e {
				s.csi(string(value[2:index]), value[index])
				return index + 1, true
			}
		}
		return 0, false
	case ']':
		for index := 2; index < len(value); index++ {
			if value[index] == '\a' {
				return index + 1, true
			}
			if value[index] == 0x1b {
				if index+1 >= len(value) {
					return 0, false
				}
				if value[index+1] == '\\' {
					return index + 2, true
				}
			}
		}
		return 0, false
	case 'P', '^', '_':
		for index := 2; index < len(value); index++ {
			if value[index] != 0x1b {
				continue
			}
			if index+1 >= len(value) {
				return 0, false
			}
			if value[index+1] == '\\' {
				return index + 2, true
			}
		}
		return 0, false
	case '7':
		s.savedRow, s.savedCol = s.row, s.column
	case '8':
		s.row, s.column = s.savedRow, s.savedCol
		s.ensureRow(s.row)
	case 'D':
		s.row++
		s.ensureRow(s.row)
	case 'M':
		if s.row > 0 {
			s.row--
		}
	case 'c':
		s.reset()
	}
	return 2, true
}

func (s *terminalDisplay) csi(parameters string, command byte) {
	values := terminalParameters(parameters)
	amount := terminalParameter(values, 0, 1)
	switch command {
	case 'A':
		s.row = max(0, s.row-amount)
	case 'B':
		s.row += amount
		s.ensureRow(s.row)
	case 'C':
		s.column = min(s.width-1, s.column+amount)
	case 'D':
		s.column = max(0, s.column-amount)
	case 'E':
		s.row += amount
		s.column = 0
		s.ensureRow(s.row)
	case 'F':
		s.row = max(0, s.row-amount)
		s.column = 0
	case 'G', '`':
		s.column = max(0, terminalParameter(values, 0, 1)-1)
	case 'H', 'f':
		s.row = max(0, terminalParameter(values, 0, 1)-1)
		s.column = max(0, terminalParameter(values, 1, 1)-1)
		s.ensureRow(s.row)
	case 'J':
		s.eraseDisplay(terminalParameter(values, 0, 0))
	case 'K':
		s.eraseLine(terminalParameter(values, 0, 0))
	case 'P':
		s.deleteCells(amount)
	case 'X':
		s.eraseCells(amount)
	case 's':
		s.savedRow, s.savedCol = s.row, s.column
	case 'u':
		s.row, s.column = s.savedRow, s.savedCol
		s.ensureRow(s.row)
	case 'h':
		if strings.HasPrefix(parameters, "?") {
			s.setPrivateModes(values, true)
		}
	case 'l':
		if strings.HasPrefix(parameters, "?") {
			s.setPrivateModes(values, false)
		}
	case 'm', 'q', 'n', 'r':
		// Styles, cursor shape/status, and scroll-region controls do not carry
		// text. The viewport intentionally remains display-only.
	}
}

func (s *terminalDisplay) setPrivateModes(values []int, enabled bool) {
	for _, value := range values {
		switch value {
		case 47, 1047, 1049:
			if enabled {
				s.enterAlternateScreen()
			} else {
				s.leaveAlternateScreen()
			}
		case 1048:
			if enabled {
				s.savedRow, s.savedCol = s.row, s.column
			} else {
				s.row, s.column = s.savedRow, s.savedCol
				s.ensureRow(s.row)
			}
		}
	}
}

func (s *terminalDisplay) enterAlternateScreen() {
	if s.alternate {
		return
	}
	s.primaryLines = s.lines
	s.primaryRow = s.row
	s.primaryColumn = s.column
	s.primarySavedRow = s.savedRow
	s.primarySavedCol = s.savedCol
	s.lines = nil
	s.row, s.column = 0, 0
	s.savedRow, s.savedCol = 0, 0
	s.alternate = true
}

func (s *terminalDisplay) leaveAlternateScreen() {
	if !s.alternate {
		return
	}
	s.lines = s.primaryLines
	s.row = s.primaryRow
	s.column = s.primaryColumn
	s.savedRow = s.primarySavedRow
	s.savedCol = s.primarySavedCol
	s.primaryLines = nil
	s.primaryRow, s.primaryColumn = 0, 0
	s.primarySavedRow, s.primarySavedCol = 0, 0
	s.alternate = false
}

func terminalParameters(value string) []int {
	value = strings.TrimLeft(value, "?<=>")
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ";")
	values := make([]int, len(parts))
	for index, part := range parts {
		parsed, err := strconv.Atoi(part)
		if err == nil && parsed >= 0 {
			values[index] = parsed
		}
	}
	return values
}

func terminalParameter(values []int, index, fallback int) int {
	if index >= len(values) || values[index] == 0 {
		return fallback
	}
	return values[index]
}

func (s *terminalDisplay) put(value rune) {
	if s.width < 1 {
		s.width = 80
	}
	if s.column >= s.width {
		s.row++
		s.column = 0
	}
	s.ensureRow(s.row)
	line := s.lines[s.row]
	for len(line) <= s.column {
		line = append(line, ' ')
	}
	line[s.column] = value
	s.lines[s.row] = line
	s.column++
}

func (s *terminalDisplay) ensureRow(row int) {
	if row < 0 {
		return
	}
	for len(s.lines) <= row {
		s.lines = append(s.lines, nil)
	}
	if len(s.lines) <= maxAttachedTerminalScreenRows {
		return
	}
	drop := len(s.lines) - maxAttachedTerminalScreenRows
	s.lines = append([][]rune(nil), s.lines[drop:]...)
	s.row = max(0, s.row-drop)
	s.savedRow = max(0, s.savedRow-drop)
}

func (s *terminalDisplay) eraseDisplay(mode int) {
	s.ensureRow(s.row)
	switch mode {
	case 2, 3:
		s.lines = nil
		s.row, s.column = 0, 0
	case 1:
		for row := 0; row < s.row; row++ {
			s.lines[row] = nil
		}
		s.eraseLine(1)
	default:
		s.eraseLine(0)
		if s.row+1 < len(s.lines) {
			s.lines = s.lines[:s.row+1]
		}
	}
}

func (s *terminalDisplay) eraseLine(mode int) {
	s.ensureRow(s.row)
	line := s.lines[s.row]
	switch mode {
	case 1:
		for index := 0; index < min(len(line), s.column+1); index++ {
			line[index] = ' '
		}
	case 2:
		line = nil
	default:
		if s.column < len(line) {
			line = line[:s.column]
		}
	}
	s.lines[s.row] = line
}

func (s *terminalDisplay) eraseCells(count int) {
	s.ensureRow(s.row)
	line := s.lines[s.row]
	for index := s.column; index < min(len(line), s.column+count); index++ {
		line[index] = ' '
	}
	s.lines[s.row] = line
}

func (s *terminalDisplay) deleteCells(count int) {
	s.ensureRow(s.row)
	line := s.lines[s.row]
	if s.column >= len(line) || count <= 0 {
		return
	}
	end := min(len(line), s.column+count)
	line = append(line[:s.column], line[end:]...)
	s.lines[s.row] = line
}

func (s terminalDisplay) String() string {
	if len(s.lines) == 0 {
		return ""
	}
	lines := make([]string, len(s.lines))
	for index, line := range s.lines {
		lines[index] = strings.TrimRight(string(line), " ")
	}
	return strings.Join(lines, "\n")
}
