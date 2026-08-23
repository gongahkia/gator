package tui

import (
	"strings"
	"testing"
)

func TestTerminalDisplayRendersCursorAndEraseSequencesAcrossChunks(t *testing.T) {
	var screen terminalDisplay
	screen.resize(40)
	screen.feed("building 12%\r\x1b[2Kbuilding 100%\r\n")
	if got, want := screen.String(), "building 100%\n"; got != want {
		t.Fatalf("progress display = %q, want %q", got, want)
	}
	screen.feed("first\r\nsecond")
	screen.feed("\x1b[")
	screen.feed("2;1H\x1b[2Kdone")
	if got, want := screen.String(), "building 100%\ndone\nsecond"; got != want {
		t.Fatalf("cursor display = %q, want %q", got, want)
	}
}

func TestTerminalDisplayBoundsRowsAndDropsUnknownControlPayloads(t *testing.T) {
	var screen terminalDisplay
	screen.resize(8)
	screen.feed("\x1b]0;untrusted title\aone\r\n")
	for index := 0; index < maxAttachedTerminalScreenRows+8; index++ {
		screen.feed("row\r\n")
	}
	if len(screen.lines) > maxAttachedTerminalScreenRows {
		t.Fatalf("terminal screen retained %d rows", len(screen.lines))
	}
	if got := screen.String(); got == "" || strings.Contains(got, "untrusted title") {
		t.Fatalf("terminal screen output = %q", got)
	}
}

func TestTerminalDisplaySwapsPrivateAlternateScreenAcrossChunks(t *testing.T) {
	var screen terminalDisplay
	screen.resize(40)
	screen.feed("primary prompt\r\n")
	screen.feed("\x1b[?10")
	screen.feed("49h\x1b[2Jalternate editor")
	if got, want := screen.String(), "alternate editor"; got != want {
		t.Fatalf("alternate screen = %q, want %q", got, want)
	}
	screen.feed("\x1b[?1049l")
	if got, want := screen.String(), "primary prompt\n"; got != want {
		t.Fatalf("restored primary screen = %q, want %q", got, want)
	}
}
