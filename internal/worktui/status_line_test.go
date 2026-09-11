package worktui

import (
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestStatusLineWrapsAtItemBoundariesWithinViewport(t *testing.T) {
	wrapped := wrapStatusLine([]string{
		"enter send", "ctrl+p commands", "ctrl+x conversations", "ctrl+b inbox", "ctrl+j jobs",
	}, 34)
	if !strings.Contains(wrapped, "\n") {
		t.Fatalf("status line did not wrap: %q", wrapped)
	}
	for _, line := range strings.Split(wrapped, "\n") {
		if width := ansi.StringWidth(line); width > 34 {
			t.Fatalf("line width = %d, want <= 34: %q", width, line)
		}
	}
	if strings.ReplaceAll(wrapped, "\n", statusLineSeparator) != "enter send"+statusLineSeparator+"ctrl+p commands"+statusLineSeparator+"ctrl+x conversations"+statusLineSeparator+"ctrl+b inbox"+statusLineSeparator+"ctrl+j jobs" {
		t.Fatalf("wrapped status line lost or reordered items: %q", wrapped)
	}
}

func TestStatusLineSplitsAnOversizedUnicodeItem(t *testing.T) {
	wrapped := wrapStatusLine([]string{"access " + strings.Repeat("🐊", 8)}, 9)
	for _, line := range strings.Split(wrapped, "\n") {
		if width := ansi.StringWidth(line); width > 9 {
			t.Fatalf("line width = %d, want <= 9: %q", width, line)
		}
	}
	if strings.Count(wrapped, "🐊") != 8 || !strings.HasPrefix(wrapped, "access\n") {
		t.Fatalf("oversized item lost content while wrapping: %q", wrapped)
	}
}

func TestConfiguredEmptyStatusLineHidesComposerFooter(t *testing.T) {
	empty := []string{}
	model := New(Config{CurrentFolder: "/work", StatusLine: &empty})
	model.width, model.height = 30, 20
	view := ansi.Strip(model.View())
	if strings.Contains(view, "ctrl+p commands") || strings.Contains(view, "enter send") {
		t.Fatalf("explicit empty status line was rendered: %q", view)
	}
}

func TestComposerFooterWrapsWithoutLosingNavigationAtNarrowWidth(t *testing.T) {
	model := New(Config{CurrentFolder: "/work"})
	model.home = false
	model.width, model.height = 32, 24
	view := ansi.Strip(model.View())
	for _, item := range []string{"enter send", "ctrl+p commands", "ctrl+x conversations", "ctrl+b inbox", "ctrl+j jobs"} {
		if !strings.Contains(view, item) {
			t.Fatalf("narrow view lost %q: %q", item, view)
		}
	}
	for _, line := range strings.Split(view, "\n") {
		if width := ansi.StringWidth(line); width > 32 {
			t.Fatalf("view line width = %d, want <= 32: %q", width, line)
		}
	}
}

func TestStatusLineEditorTogglesReordersAndPersists(t *testing.T) {
	configured := []string{"send", "model"}
	var saved *[]string
	model := New(Config{
		CurrentFolder: "/work",
		StatusLine:    &configured,
		SetStatusLine: func(items *[]string) error {
			if items != nil {
				copyOfItems := append([]string(nil), (*items)...)
				saved = &copyOfItems
			}
			return nil
		},
	})
	model.openStatusLineEditor()
	if view := ansi.Strip(model.View()); !strings.Contains(view, "Configure status line") || !strings.Contains(view, "[x]") {
		t.Fatalf("status line editor = %q", view)
	}

	model.selected = statusLineOptionIndex("send")
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeySpace})
	model = updated.(Model)
	model.selected = statusLineOptionIndex("commands")
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeySpace})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyLeft})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)

	want := []string{"commands", "model"}
	if saved == nil || !reflect.DeepEqual(*saved, want) || !reflect.DeepEqual(model.statusLine, want) || model.launcher {
		t.Fatalf("saved = %#v, model status line = %#v, launcher = %t", saved, model.statusLine, model.launcher)
	}
}

func TestStatusLineEditorCanRestoreDefaults(t *testing.T) {
	configured := []string{"model"}
	called := false
	model := New(Config{CurrentFolder: "/work", StatusLine: &configured, SetStatusLine: func(items *[]string) error {
		called = true
		if items != nil {
			t.Fatalf("reset saved explicit items: %#v", *items)
		}
		return nil
	}})
	model.openStatusLineEditor()
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if !called || model.statusLineConfigured || !reflect.DeepEqual(model.statusLine, defaultStatusLine) {
		t.Fatalf("defaults were not restored: called=%t configured=%t items=%#v", called, model.statusLineConfigured, model.statusLine)
	}
}
