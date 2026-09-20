package worktui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestRenderViewportCoversEveryTerminalCell(t *testing.T) {
	rendered := ansi.Strip(New(Config{}).renderViewport("Gator", 12, 4))
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
