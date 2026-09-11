package tui

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/sandbox"
	"github.com/gongahkia/gator/internal/terminal"
	"github.com/gongahkia/gator/internal/workspace"
)

func TestSubagentProgressAppearsInScrollableConversation(t *testing.T) {
	model := New(Config{})
	model.width = 72
	model.height = 24
	model.resizeInputs()
	model.appendEvent(agent.Event{Kind: agent.EventSubagent, Step: 3, At: time.Now(), Text: "starting 2 read-only scout(s)"})
	if len(model.chat) == 0 || model.chat[len(model.chat)-1].author != chatSystem || !strings.Contains(model.chat[len(model.chat)-1].text, "subagents starting 2") {
		t.Fatalf("subagent transcript entry = %#v", model.chat)
	}
	if model.activity.phase != activityInspecting || !strings.Contains(model.activity.detail, "subagent") {
		t.Fatalf("subagent activity = %#v", model.activity)
	}
	if transcript := model.transcriptContent(); !strings.Contains(transcript, "subagents starting 2") {
		t.Fatalf("subagent transcript = %q", transcript)
	}
}

func TestTerminalLifecycleAppearsInScrollableConversation(t *testing.T) {
	model := New(Config{})
	model.width = 72
	model.height = 24
	model.resizeInputs()
	model.appendEvent(agent.Event{Kind: agent.EventTerminal, Step: 4, At: time.Now(), Text: "term-001 exited with code 0"})
	if len(model.chat) == 0 || model.chat[len(model.chat)-1].author != chatSystem || !strings.Contains(model.chat[len(model.chat)-1].text, "terminal term-001 exited") {
		t.Fatalf("terminal transcript entry = %#v", model.chat)
	}
	if model.activity.phase != activityTool || !strings.Contains(model.activity.detail, "terminal term-001") {
		t.Fatalf("terminal activity = %#v", model.activity)
	}
}

func TestAttachedTerminalShowsBoundedOutputAndSendsDeveloperInput(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh is unavailable")
	}
	root, err := workspace.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := terminal.New(terminal.Config{Root: root, Policy: sandbox.Policy{Mode: sandbox.Off}})
	defer manager.Close()
	if _, err := manager.Start(context.Background(), []string{sh, "-lc", "printf 'phase 1\\r\\033[2Kphase complete\\n'; printf '\\033[31mready\\033[0m\\n'; IFS= read line; printf 'received:%s\\n' \"$line\""}); err != nil {
		t.Fatalf("start terminal task: %v", err)
	}
	model := New(Config{})
	model.width, model.height = 100, 40
	model.resizeInputs()
	next, _ := model.Update(terminalManagerMsg{attachment: manager.Attachment()})
	attached := next.(Model)
	next, _ = attached.Update(tea.KeyMsg{Type: tea.KeyCtrlT})
	attached = next.(Model)
	if attached.screen != terminalScreen || !strings.Contains(attached.View(), "attached terminal") {
		t.Fatalf("attached terminal view = %s", attached.View())
	}
	if len(attached.terminalTasks) != 1 || attached.terminalTasks[0].Rows == 24 || attached.terminalTasks[0].Columns == 80 {
		t.Fatalf("attached terminal did not resize its PTY viewport: %#v", attached.terminalTasks)
	}
	attached.terminalInput.SetValue("Ada")
	next, _ = attached.Update(tea.KeyMsg{Type: tea.KeyEnter})
	attached = next.(Model)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		attached.refreshAttachedTerminal()
		if strings.Contains(attached.currentAttachedTerminalOutput(), "received:Ada") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	output := attached.currentAttachedTerminalOutput()
	if !strings.Contains(output, "phase complete") || strings.Contains(output, "phase 1") || !strings.Contains(output, "ready") || !strings.Contains(output, "received:Ada") || strings.Contains(output, "\x1b[31m") {
		t.Fatalf("attached terminal output = %q", output)
	}
	if !strings.Contains(attached.notice.text, "existing sandbox") {
		t.Fatalf("developer terminal write notice = %#v", attached.notice)
	}
	next, _ = attached.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if returned := next.(Model); returned.screen != composeScreen {
		t.Fatalf("terminal close returned to screen %v", returned.screen)
	}
}

func TestAttachedTerminalRawKeyboardInputFlushesOnReturnToControls(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh is unavailable")
	}
	root, err := workspace.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var inputs []terminal.DeveloperInput
	manager := terminal.New(terminal.Config{
		Root:   root,
		Policy: sandbox.Policy{Mode: sandbox.Off},
		OnDeveloperInput: func(_ terminal.Task, input terminal.DeveloperInput) {
			inputs = append(inputs, input)
		},
	})
	defer manager.Close()
	if _, err := manager.Start(context.Background(), []string{sh, "-lc", "IFS= read line; printf 'received:%s\\n' \"$line\""}); err != nil {
		t.Fatalf("start terminal task: %v", err)
	}
	model := New(Config{})
	model.width, model.height = 100, 40
	model.resizeInputs()
	next, _ := model.Update(terminalManagerMsg{attachment: manager.Attachment()})
	attached := next.(Model)
	next, _ = attached.Update(tea.KeyMsg{Type: tea.KeyCtrlT})
	attached = next.(Model)
	next, _ = attached.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	attached = next.(Model)
	if !attached.terminalRawInput || !strings.Contains(attached.View(), "Raw keyboard is active") {
		t.Fatalf("raw keyboard mode did not open: %s", attached.View())
	}
	next, _ = attached.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Ada")})
	attached = next.(Model)
	next, _ = attached.Update(tea.KeyMsg{Type: tea.KeyEnter})
	attached = next.(Model)
	next, _ = attached.Update(tea.KeyMsg{Type: tea.KeyCtrlCloseBracket})
	attached = next.(Model)
	if attached.terminalRawInput || !strings.Contains(attached.notice.text, "Line input is ready") {
		t.Fatalf("raw keyboard mode did not return to controls: %#v", attached.notice)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		attached.refreshAttachedTerminal()
		if strings.Contains(attached.currentAttachedTerminalOutput(), "received:Ada") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if output := attached.currentAttachedTerminalOutput(); !strings.Contains(output, "received:Ada") {
		t.Fatalf("raw keyboard terminal output = %q", output)
	}
	if len(inputs) != 1 || !inputs[0].Raw || inputs[0].Bytes != len("Ada\r") {
		t.Fatalf("raw keyboard journal metadata = %#v", inputs)
	}
}

func TestCompletedRunKeepsDetachedTerminalAttachedToInteractiveSession(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh is unavailable")
	}
	root, err := workspace.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	registry := terminal.NewRegistry()
	defer registry.Close()
	manager := terminal.New(terminal.Config{Root: root, Policy: sandbox.Policy{Mode: sandbox.Off}, Registry: registry})
	task, err := manager.Start(context.Background(), []string{sh, "-lc", "sleep 30"})
	if err != nil {
		t.Fatalf("start terminal task: %v", err)
	}
	if _, err := manager.Detach(task.ID); err != nil {
		t.Fatalf("detach terminal task: %v", err)
	}
	manager.Close()

	model := New(Config{TerminalRegistry: registry})
	model.width, model.height = 100, 40
	model.resizeInputs()
	model.setTerminalAttachment(registry.Attachment())
	next, _ := model.Update(executionDoneMsg{})
	completed := next.(Model)
	if completed.terminalAttachment == nil || !completed.hasBackgroundTerminalTask() {
		t.Fatalf("completed TUI dropped detached terminal attachment: %#v", completed.terminalTasks)
	}
	next, _ = completed.Update(tea.KeyMsg{Type: tea.KeyCtrlT})
	attached := next.(Model)
	if attached.screen != terminalScreen || !strings.Contains(attached.View(), task.ID) || !strings.Contains(attached.View(), "background") {
		t.Fatalf("detached terminal attachment view = %s", attached.View())
	}
}
