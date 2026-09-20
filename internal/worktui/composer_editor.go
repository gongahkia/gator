package worktui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// composerEditor returns the conventional editor selection for terminal
// programs. VISUAL takes precedence because it is commonly set to a full-screen
// editor while EDITOR may be a line editor.
func composerEditor() string {
	for _, name := range []string{"VISUAL", "EDITOR"} {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}
	return "nvim"
}

func composerEditorCommand(editor, path string) *exec.Cmd {
	// EDITOR and VISUAL conventionally permit flags (for example, "code
	// --wait"). The inherited value is user-controlled; the temporary path is
	// passed separately as $1 so it remains one safely quoted argument.
	return exec.Command("sh", "-c", "exec "+editor+" \"$1\"", "gator-composer", path)
}

func (m Model) openComposerEditor() (tea.Model, tea.Cmd) {
	if m.composerEditorPath != "" {
		return m, nil
	}

	file, err := os.CreateTemp("", "gator-composer-*.md")
	if err != nil {
		m.status = "Open composer editor: " + err.Error()
		return m, nil
	}
	path := file.Name()
	cleanup := func() {
		_ = file.Close()
		_ = os.Remove(path)
	}
	if _, err := file.WriteString(m.input); err != nil {
		cleanup()
		m.status = "Write composer draft: " + err.Error()
		return m, nil
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		m.status = "Prepare composer editor: " + err.Error()
		return m, nil
	}

	editor := composerEditor()
	command := composerEditorCommand(editor, path)
	m.composerEditorPath = path
	m.status = "Editing composer in " + singleLine(editor) + "… Save and exit to return."
	return m, tea.ExecProcess(command, func(err error) tea.Msg {
		return composerEditorDone{path: path, editor: editor, err: err}
	})
}

func (m Model) updateComposerEditorDone(value composerEditorDone) (tea.Model, tea.Cmd) {
	if value.path == "" || value.path != m.composerEditorPath {
		return m, nil
	}
	m.composerEditorPath = ""
	contents, readErr := os.ReadFile(value.path)
	removeErr := os.Remove(value.path)
	if readErr != nil {
		m.status = "Read edited composer: " + readErr.Error()
		if removeErr != nil {
			m.status += "; remove temporary draft: " + removeErr.Error()
		}
		return m, nil
	}

	m.input = string(contents)
	m.resetPromptHistoryNavigation()
	if value.err != nil {
		m.status = fmt.Sprintf("Composer restored after %s exited: %v", singleLine(value.editor), value.err)
	} else {
		m.status = "Composer restored from " + singleLine(value.editor) + "."
	}
	if removeErr != nil {
		m.status += " Remove temporary draft: " + removeErr.Error()
	}
	return m, nil
}
