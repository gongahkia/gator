package worktui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestRenderViewportCoversEveryTerminalCell(t *testing.T) {
	view := New(Config{}).renderViewport("Gator", 12, 4)
	if strings.Contains(view, "48;5;") {
		t.Fatalf("viewport must not force an opaque background: %q", view)
	}
	rendered := ansi.Strip(view)
	lines := strings.Split(rendered, "\n")
	if len(lines) != 4 {
		t.Fatalf("viewport rows = %d, want 4: %q", len(lines), rendered)
	}
	for index, line := range lines {
		if width := ansi.StringWidth(line); width != 12 {
			t.Fatalf("viewport row %d width = %d, want 12: %q", index, width, line)
		}
	}
}

func TestInitRequestsAStartupScreenClear(t *testing.T) {
	if New(Config{}).Init() == nil {
		t.Fatal("Work TUI did not request a startup redraw")
	}
}
