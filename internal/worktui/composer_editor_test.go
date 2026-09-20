package worktui

import (
	"errors"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestComposerEditorPreference(t *testing.T) {
	t.Setenv("VISUAL", "helix")
	t.Setenv("EDITOR", "nano")
	if got := composerEditor(); got != "helix" {
		t.Fatalf("editor = %q, want VISUAL", got)
	}

	t.Setenv("VISUAL", "")
	if got := composerEditor(); got != "nano" {
		t.Fatalf("editor = %q, want EDITOR", got)
	}

	t.Setenv("EDITOR", "")
	if got := composerEditor(); got != "nvim" {
		t.Fatalf("editor = %q, want nvim fallback", got)
	}
}

func TestComposerEditorCommandPassesTheDraftPathAsOneArgument(t *testing.T) {
	file, err := os.CreateTemp("", "gator composer editor test-*.md")
	if err != nil {
		t.Fatal(err)
	}
	path := file.Name()
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(path) })

	command := composerEditorCommand(`sh -c 'printf %s edited > "$1"' gator-editor-test`, path)
	if err := command.Run(); err != nil {
		t.Fatalf("run editor command: %v", err)
	}
	contents, err := os.ReadFile(path)
	if err != nil || string(contents) != "edited" {
		t.Fatalf("edited draft = %q, err = %v", contents, err)
	}
}

func TestCtrlGEditsAndRestoresComposerBuffer(t *testing.T) {
	t.Setenv("VISUAL", "helix --wait")
	model := New(Config{CurrentFolder: "/work"})
	model.input = "first draft"

	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyCtrlG})
	model = updated.(Model)
	if command == nil || model.composerEditorPath == "" {
		t.Fatalf("editor did not open: %#v", model)
	}
	path := model.composerEditorPath
	contents, err := os.ReadFile(path)
	if err != nil || string(contents) != "first draft" {
		t.Fatalf("temporary draft = %q, err = %v", contents, err)
	}
	if !strings.Contains(model.status, "helix --wait") {
		t.Fatalf("open status = %q", model.status)
	}

	if err := os.WriteFile(path, []byte("edited\nsecond line"), 0o600); err != nil {
		t.Fatal(err)
	}
	updated, _ = model.Update(composerEditorDone{path: path, editor: "helix --wait"})
	model = updated.(Model)
	if model.input != "edited\nsecond line" || model.composerEditorPath != "" {
		t.Fatalf("restored composer = %#v", model)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary draft remains: %v", err)
	}
}

func TestComposerEditorKeepsSavedTextWhenEditorExitsWithError(t *testing.T) {
	model := New(Config{CurrentFolder: "/work"})
	file, err := os.CreateTemp("", "gator-composer-test-*.md")
	if err != nil {
		t.Fatal(err)
	}
	path := file.Name()
	if _, err := file.WriteString("saved before editor error"); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	model.composerEditorPath = path

	updated, _ := model.Update(composerEditorDone{path: path, editor: "bad-editor", err: errors.New("exit status 1")})
	model = updated.(Model)
	if model.input != "saved before editor error" || !strings.Contains(model.status, "exit status 1") {
		t.Fatalf("composer = %q, status = %q", model.input, model.status)
	}
}
