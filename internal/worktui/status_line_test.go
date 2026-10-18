package worktui

import (
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestConfiguredEmptyStatusLineHidesComposerFooter(t *testing.T) {
	empty := []string{}
	model := New(Config{CurrentFolder: "/work", StatusLine: &empty, StatusLineEnabled: true})
	model.width, model.height = 30, 20
	view := ansi.Strip(model.snapshotText())
	if strings.Contains(view, "ctrl+p commands") || strings.Contains(view, "enter send") {
		t.Fatalf("explicit empty status line was rendered: %q", view)
	}
}

func TestConfiguredStatusLineShowsSelectedLocalModel(t *testing.T) {
	items := []string{"model"}
	model := New(Config{
		CurrentFolder:     "/work",
		StatusLine:        &items,
		StatusLineEnabled: true,
		ModelStatus: func() (ModelStatus, error) {
			return ModelStatus{Provider: "gator-local", Model: "qwen3:8b", Access: "local model configured"}, nil
		},
	})
	model.home = false
	model.width, model.height = 100, 24
	view := ansi.Strip(model.snapshotText())
	if !strings.Contains(view, "model gator-local/qwen3:8b") {
		t.Fatalf("configured status line omitted selected local model: %q", view)
	}
}

func TestConfiguredStatusLineAdvertisesEditorShortcut(t *testing.T) {
	items := []string{"editor"}
	model := New(Config{CurrentFolder: "/work", StatusLine: &items, StatusLineEnabled: true})
	model.home = false
	model.width, model.height = 100, 24
	if view := ansi.Strip(model.snapshotText()); !strings.Contains(view, "ctrl+g editor") {
		t.Fatalf("configured status line omitted composer editor shortcut: %q", view)
	}
}

func TestConfiguredStatusLineAdvertisesWorkspaceShortcut(t *testing.T) {
	items := []string{"source"}
	model := New(Config{CurrentFolder: "/work", StatusLine: &items, StatusLineEnabled: true})
	model.home = false
	model.width, model.height = 100, 24
	if view := ansi.Strip(model.snapshotText()); !strings.Contains(view, "ctrl+i workspace") {
		t.Fatalf("configured status line omitted workspace shortcut: %q", view)
	}
}

func TestStatusLineBridgePreservesConfiguredItems(t *testing.T) {
	items := append([]string(nil), defaultStatusLine...)
	model := New(Config{CurrentFolder: "/work", StatusLine: &items, StatusLineEnabled: true})
	state := model.BridgeState()
	if !state.StatusLineEnabled || len(state.StatusLine) < 7 {
		t.Fatalf("configured status line = %#v", state.StatusLine)
	}
}

func TestStatusLineEditorTogglesReordersAndPersists(t *testing.T) {
	configured := []string{"send", "model"}
	var saved *[]string
	var savedEnabled bool
	model := New(Config{
		CurrentFolder:     "/work",
		StatusLine:        &configured,
		StatusLineEnabled: true,
		SetStatusLine: func(enabled bool, items *[]string) error {
			savedEnabled = enabled
			if items != nil {
				copyOfItems := append([]string(nil), (*items)...)
				saved = &copyOfItems
			}
			return nil
		},
	})
	model.openStatusLineEditor()
	if panel := model.BridgeState().Panel; panel == nil || panel.Title != "Configure status line" || len(panel.Rows) == 0 {
		t.Fatalf("status line editor = %#v", panel)
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
	if saved == nil || !savedEnabled || !reflect.DeepEqual(*saved, want) || !reflect.DeepEqual(model.statusLine, want) || model.launcher {
		t.Fatalf("saved = %#v enabled=%t, model status line = %#v, launcher = %t", saved, savedEnabled, model.statusLine, model.launcher)
	}
}

func TestStatusLineEditorCanRestoreDefaults(t *testing.T) {
	configured := []string{"model"}
	called := false
	model := New(Config{CurrentFolder: "/work", StatusLine: &configured, StatusLineEnabled: true, SetStatusLine: func(enabled bool, items *[]string) error {
		called = true
		if !enabled {
			t.Fatal("reset unexpectedly disabled the status line")
		}
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

func TestStatusLineRequiresExplicitEnable(t *testing.T) {
	items := []string{"model", "mode"}
	model := New(Config{CurrentFolder: "/work", StatusLine: &items})
	model.home = false
	model.width, model.height = 100, 24
	if view := ansi.Strip(model.snapshotText()); strings.Contains(view, "model ") || strings.Contains(view, "mode auto") {
		t.Fatalf("status line was shown without explicit enable: %q", view)
	}
	model.openStatusLineEditor()
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if !model.statusLineEnabled || !strings.Contains(ansi.Strip(model.snapshotText()), "mode auto") {
		t.Fatalf("status line did not become visible after explicit enable: %#v\n%s", model, model.snapshotText())
	}
}

func TestSettingsStatusLineOpensTheVisibilityControl(t *testing.T) {
	model := New(Config{CurrentFolder: "/work"})
	next, command := model.runLocalCommand("/settings statusline")
	model = next.(Model)
	if command != nil || !model.launcher || model.launcherMode != "status-line" {
		t.Fatalf("settings status line did not open visibility control: %#v", model)
	}
}
