package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestTerminalDisplayRendersCursorAndEraseSequencesAcrossChunks(t *testing.T) {
	var screen terminalDisplay
	screen.resize(8, 40)
	screen.feed("building 12%\r\x1b[2Kbuilding 100%\r\n")
	if got, want := screen.String(), "building 100%"; got != want {
		t.Fatalf("progress display = %q, want %q", got, want)
	}
	screen.feed("first\r\nsecond")
	screen.feed("\x1b[")
	screen.feed("2;1H\x1b[2Kdone")
	if got, want := screen.String(), "building 100%\ndone\nsecond"; got != want {
		t.Fatalf("cursor display = %q, want %q", got, want)
	}
}

func TestTerminalDisplayPreservesStylesAndWideUnicode(t *testing.T) {
	var screen terminalDisplay
	screen.resize(4, 20)
	screen.feed("\x1b[1;31mred")
	screen.feed("\x1b[0m plain \x1b[38;2;1;2;3m界")
	viewport := screen.viewport()
	if got := ansi.Strip(viewport); !strings.Contains(got, "red plain 界") {
		t.Fatalf("styled viewport = %q", got)
	}
	if !strings.Contains(viewport, "\x1b[1;38;5;1mred") {
		t.Fatalf("viewport did not render bold ANSI color: %q", viewport)
	}
	if !strings.Contains(viewport, "\x1b[38;2;1;2;3m界") {
		t.Fatalf("viewport did not render RGB color: %q", viewport)
	}
}

func TestTerminalDisplayMaintainsBoundedScrollbackAndViewport(t *testing.T) {
	var screen terminalDisplay
	screen.resize(2, 16)
	for index := 0; index < maxAttachedTerminalScreenRows+16; index++ {
		screen.feed(fmt.Sprintf("row-%04d\r\n", index))
	}
	if got, limit := screen.terminal.Buffer().Lines.Length(), maxAttachedTerminalScreenRows+screen.terminal.Rows(); got > limit {
		t.Fatalf("terminal retained %d lines, limit %d", got, limit)
	}
	if got := screen.String(); strings.Contains(got, "row-0000") || !strings.Contains(got, "row-1039") {
		t.Fatalf("bounded scrollback = %q", got)
	}
	screen.scrollToTop()
	if got := ansi.Strip(screen.viewport()); !strings.Contains(got, "row-0016") {
		t.Fatalf("top viewport = %q", got)
	}
	screen.scrollToBottom()
	if got := ansi.Strip(screen.viewport()); !strings.Contains(got, "row-1039") {
		t.Fatalf("bottom viewport = %q", got)
	}
}

func TestTerminalDisplayHandlesAlternateScreenAndProtocolResponses(t *testing.T) {
	var screen terminalDisplay
	screen.resize(4, 40)
	screen.feed("primary prompt\r\n")
	screen.feed("\x1b[?10")
	screen.feed("49h\x1b[2Jalternate editor")
	if got, want := screen.String(), "\nalternate editor"; got != want {
		t.Fatalf("alternate screen = %q, want %q", got, want)
	}
	screen.feed("\x1b[5n")
	if got, want := string(screen.takeProtocol()), "\x1b[0n"; got != want {
		t.Fatalf("device status response = %q, want %q", got, want)
	}
	screen.feed("\x1b[?1049l")
	if got, want := screen.String(), "primary prompt"; got != want {
		t.Fatalf("restored primary screen = %q, want %q", got, want)
	}
}

func TestTerminalDisplayDropsNonRenderingControlPayloads(t *testing.T) {
	var screen terminalDisplay
	screen.resize(4, 20)
	screen.feed("\x1b]0;untrusted title\a\x1b]52;c;clipboard\a\x1bPpayload\x1b\\safe")
	if got, want := screen.String(), "safe"; got != want {
		t.Fatalf("control payload display = %q, want %q", got, want)
	}
}
