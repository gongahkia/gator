package tui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gongahkia/gator/internal/agent"
)

func TestVimNormalEditingSupportsMotionsOperatorsAndUndo(t *testing.T) {
	model := New(Config{})
	model.vim = vimNormal
	model.vimAbsoluteNumbers = true
	model.vimRelativeNumbers = true
	model.task.SetValue("alpha beta\ngamma")
	model.setVimCursor(0, 0)
	model.syncVimLineNumbers()

	model = drive(t, model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("w")})
	if row, column := model.vimCursor(); row != 0 || column != 6 {
		t.Fatalf("w cursor = %d:%d, want 0:6", row, column)
	}
	model = drive(t, model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	if row, column := model.vimCursor(); row != 0 || column != 0 {
		t.Fatalf("b cursor = %d:%d, want 0:0", row, column)
	}

	model = drive(t, model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	model = drive(t, model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("w")})
	if got := model.task.Value(); got != "beta\ngamma" {
		t.Fatalf("dw value = %q, want beta\\ngamma", got)
	}
	model = drive(t, model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("u")})
	if got := model.task.Value(); got != "alpha beta\ngamma" {
		t.Fatalf("u value = %q", got)
	}
	model = drive(t, model, tea.KeyMsg{Type: tea.KeyCtrlR})
	if got := model.task.Value(); got != "beta\ngamma" {
		t.Fatalf("ctrl+r value = %q", got)
	}

	model = drive(t, model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("0")})
	model = drive(t, model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	model = drive(t, model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("X")})
	if got := model.task.Value(); got != "bXeta\ngamma" {
		t.Fatalf("a inserted at %q, want bXeta\\ngamma", got)
	}

	model.vim = vimNormal
	model.task.SetValue("alpha")
	model.setVimCursor(0, len([]rune("alpha")))
	model = drive(t, model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	model = drive(t, model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("0")})
	if got := model.task.Value(); got != "" {
		t.Fatalf("d0 value = %q, want empty", got)
	}
}

func TestVimLineNumbersAndSetCommands(t *testing.T) {
	model := New(Config{})
	model.width = 100
	model.height = 40
	model.vim = vimNormal
	model.vimAbsoluteNumbers = true
	model.vimRelativeNumbers = true
	model.task.SetValue("one\ntwo\nthree")
	model.setVimCursor(1, 0)
	model.resizeInputs()

	view := model.composerInputView()
	for _, expected := range []string{"1 one", "2 two", "1 three"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("hybrid line number view omitted %q:\n%s", expected, view)
		}
	}

	next, command := model.executeVimExCommand(":set nonumber", false)
	if command != nil {
		t.Fatal(":set unexpectedly returned a command")
	}
	model = next.(Model)
	if model.vimAbsoluteNumbers || !model.vimRelativeNumbers {
		t.Fatalf(":set nonumber settings = %s", model.vimNumberSettings())
	}
	next, command = model.executeVimExCommand(":set norelativenumber", false)
	if command != nil {
		t.Fatal(":set unexpectedly returned a command")
	}
	model = next.(Model)
	if model.vimAbsoluteNumbers || model.vimRelativeNumbers {
		t.Fatalf(":set norelativenumber settings = %s", model.vimNumberSettings())
	}
}

func TestChatAggregatesStreamingTextAndKeepsToolActivityInOrder(t *testing.T) {
	model := New(Config{})
	model.width = 100
	model.height = 40
	model.resizeInputs()
	model.appendChat(chatEntry{author: chatUser, text: "Implement the feature."})
	model.appendEvent(agent.Event{Kind: agent.EventTextDelta, Step: 1, Text: "I will "})
	model.appendEvent(agent.Event{Kind: agent.EventTextDelta, Step: 1, Text: "inspect the repository."})
	model.appendEvent(agent.Event{Kind: agent.EventToolCalled, Step: 1, ToolCall: &agent.ToolCall{Name: "git_status", Arguments: json.RawMessage(`{}`)}})

	if len(model.chat) != 4 {
		t.Fatalf("chat entries = %#v", model.chat)
	}
	if got := model.chat[2].text; got != "I will inspect the repository." {
		t.Fatalf("streamed chat text = %q", got)
	}
	view := model.View()
	for _, expected := range []string{"Implement the feature.", "I will inspect the repository.", "tool -> git_status", "Message Gator"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("chat view omitted %q: %s", expected, view)
		}
	}
}

func TestEmptyComposerUsesDedicatedPromptInsteadOfTextareaPlaceholderCursor(t *testing.T) {
	model := New(Config{})
	model.task.SetHeight(3)
	view := model.composerInputView()
	if !strings.Contains(view, "Message Gator...") {
		t.Fatalf("empty composer omitted prompt: %q", view)
	}
	if !strings.Contains(view, "›") {
		t.Fatalf("focused empty composer omitted caret: %q", view)
	}
	if lines := strings.Count(view, "\n") + 1; lines != 3 {
		t.Fatalf("empty composer lines = %d, want 3: %q", lines, view)
	}

	model.task.SetValue("Check the repository status.")
	view = model.composerInputView()
	if !strings.Contains(view, "Check the repository status.") {
		t.Fatalf("composer omitted entered text: %q", view)
	}
}

func TestChatPageKeysBrowseConversation(t *testing.T) {
	model := New(Config{})
	model.width = 100
	model.height = 40
	model.resizeInputs()
	for index := 0; index < 12; index++ {
		model.appendChat(chatEntry{author: chatAgent, text: fmt.Sprintf("message %d", index)})
	}
	model.transcriptBottom()
	last := model.transcript.YOffset
	up, _ := model.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	browsed := up.(Model)
	if browsed.transcript.YOffset >= last || browsed.followTranscript {
		t.Fatalf("page up did not leave transcript history: offset=%d follow=%t", browsed.transcript.YOffset, browsed.followTranscript)
	}
	beforeAppend := browsed.transcript.YOffset
	browsed.appendChat(chatEntry{author: chatAgent, text: "new activity while browsing"})
	if !browsed.transcriptUnread || browsed.transcript.YOffset != beforeAppend {
		t.Fatalf("new activity changed a browsed transcript: unread=%t offset=%d want=%d", browsed.transcriptUnread, browsed.transcript.YOffset, beforeAppend)
	}
	down, _ := browsed.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	latest, _ := down.(Model).Update(tea.KeyMsg{Type: tea.KeyEnd})
	if !latest.(Model).followTranscript || latest.(Model).transcriptUnread || !latest.(Model).transcript.AtBottom() {
		t.Fatalf("end did not restore transcript follow state: %#v", latest.(Model).transcript)
	}
}

func TestConversationScrollbarTracksTranscriptPosition(t *testing.T) {
	model := New(Config{})
	model.width = 100
	model.height = 40
	model.resizeInputs()
	for index := 0; index < 20; index++ {
		model.appendChat(chatEntry{author: chatAgent, text: fmt.Sprintf("message %d", index)})
	}

	model.transcriptTop()
	if !model.showsTranscriptScrollbar(model.transcript) {
		t.Fatalf("scrollable transcript did not enable its scrollbar: lines=%d height=%d", model.transcript.TotalLineCount(), model.transcript.Height)
	}
	topThumb := scrollbarThumbRow(model.transcriptScrollbar(model.transcript))
	if topThumb != 0 {
		t.Fatalf("top scrollbar thumb row = %d, want 0", topThumb)
	}

	model.transcriptBottom()
	bottomThumb := scrollbarThumbRow(model.transcriptScrollbar(model.transcript))
	if bottomThumb <= topThumb {
		t.Fatalf("bottom scrollbar thumb row = %d, want after %d", bottomThumb, topThumb)
	}
	if !strings.Contains(model.View(), "█") {
		t.Fatalf("conversation view omitted the scrollbar thumb: %s", model.View())
	}

	short := New(Config{})
	short.width = 100
	short.height = 40
	short.resizeInputs()
	if short.showsTranscriptScrollbar(short.transcript) || short.transcriptScrollbar(short.transcript) != "" {
		t.Fatal("short transcript unexpectedly rendered a scrollbar")
	}
}

func scrollbarThumbRow(scrollbar string) int {
	for row, line := range strings.Split(scrollbar, "\n") {
		if strings.Contains(line, "█") {
			return row
		}
	}
	return -1
}

func TestChatMouseWheelBrowsesConversation(t *testing.T) {
	model := New(Config{})
	model.width = 100
	model.height = 40
	model.resizeInputs()
	for index := 0; index < 12; index++ {
		model.appendChat(chatEntry{author: chatAgent, text: fmt.Sprintf("message %d", index)})
	}
	model.transcriptBottom()
	bottom := model.transcript.YOffset

	next, command := model.Update(tea.MouseMsg{Button: tea.MouseButtonWheelUp})
	if command != nil {
		t.Fatal("mouse-wheel transcript scroll returned a command")
	}
	browsed := next.(Model)
	if browsed.transcript.YOffset >= bottom || browsed.followTranscript {
		t.Fatalf("mouse wheel did not browse transcript history: offset=%d bottom=%d follow=%t", browsed.transcript.YOffset, bottom, browsed.followTranscript)
	}

	next, _ = browsed.Update(tea.MouseMsg{Button: tea.MouseButtonWheelDown})
	if next.(Model).transcript.YOffset <= browsed.transcript.YOffset {
		t.Fatalf("mouse wheel did not move transcript toward the latest entry: offset=%d", next.(Model).transcript.YOffset)
	}
}

func TestCopyCommandsCopyAgentOutputAndPlainTranscript(t *testing.T) {
	var copied []string
	model := New(Config{CopyToClipboard: func(text string) error {
		copied = append(copied, text)
		return nil
	}})
	model.chat = []chatEntry{
		{author: chatUser, text: "Add a focused feature."},
		{author: chatAgent, text: "I added the feature.\n\nTests pass."},
		{author: chatTool, text: "tool ok go_test", detail: "go test ./..."},
	}

	model.task.SetValue("/copy")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if command == nil {
		t.Fatal("/copy did not request a clipboard write")
	}
	updated, command := next.(Model).Update(command())
	if command != nil {
		t.Fatal("clipboard completion returned an unexpected command")
	}
	model = updated.(Model)
	if got, want := copied, []string{"I added the feature.\n\nTests pass."}; !reflect.DeepEqual(got, want) {
		t.Fatalf("/copy text = %#v, want %#v", got, want)
	}
	if model.notice.kind != noticeSuccess || !strings.Contains(model.notice.text, "Latest Gator response copied") {
		t.Fatalf("/copy notice = %#v", model.notice)
	}

	model.task.SetValue("/copyall")
	next, command = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if command == nil {
		t.Fatal("/copyall did not request a clipboard write")
	}
	updated, _ = next.(Model).Update(command())
	model = updated.(Model)
	if len(copied) != 2 {
		t.Fatalf("clipboard writes = %#v", copied)
	}
	for _, expected := range []string{"You\nAdd a focused feature.", "Gator\nI added the feature.", "Tool\ntool ok go_test\nDetails\ngo test ./..."} {
		if !strings.Contains(copied[1], expected) {
			t.Fatalf("/copyall omitted %q from %q", expected, copied[1])
		}
	}
	if model.notice.kind != noticeSuccess || !strings.Contains(model.notice.text, "Conversation transcript copied") {
		t.Fatalf("/copyall notice = %#v", model.notice)
	}
}

func TestCopyCommandsReportUnavailableContentAndClipboardFailures(t *testing.T) {
	model := New(Config{})
	model.chat = []chatEntry{{author: chatSystem, text: "System event"}}
	model.task.SetValue("/copy")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if command != nil {
		t.Fatal("/copy requested a clipboard write without agent output")
	}
	if !strings.Contains(next.(Model).notice.text, "No Gator response") {
		t.Fatalf("empty /copy notice = %#v", next.(Model).notice)
	}

	model = New(Config{CopyToClipboard: func(string) error { return errors.New("clipboard unavailable") }})
	model.chat = []chatEntry{{author: chatAgent, text: "Response"}}
	model.task.SetValue("/copy")
	next, command = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if command == nil {
		t.Fatal("/copy did not request the injected clipboard write")
	}
	updated, _ := next.(Model).Update(command())
	if notice := updated.(Model).notice; notice.kind != noticeError || !strings.Contains(notice.text, "clipboard unavailable") {
		t.Fatalf("clipboard failure notice = %#v", notice)
	}
}

func TestControlCenterDrawerOpensNavigatesAndFallsBackOnNarrowTerminals(t *testing.T) {
	model := New(Config{RepositoryPath: testRepository(t)})
	model.width = 120
	model.height = 48
	model.resizeInputs()
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
	if command != nil {
		t.Fatal("opening the control center returned an unexpected command")
	}
	opened := next.(Model)
	if !opened.drawerOpen || !opened.drawerUsesSidePane() || !strings.Contains(opened.View(), "Control center") {
		t.Fatalf("drawer did not open as a side pane:\n%s", opened.View())
	}
	if !strings.Contains(opened.View(), gatorWordmark) {
		t.Fatalf("TUI header is missing the Gator wordmark:\n%s", opened.View())
	}
	dividerWidth, dividerHeight := lipgloss.Size(opened.controlCenterDivider())
	if dividerWidth != 1 || dividerHeight != opened.height {
		t.Fatalf("control-center divider = %dx%d, want 1x%d", dividerWidth, dividerHeight, opened.height)
	}
	assertViewFits(t, opened, 120, 48)
	next, _ = opened.Update(tea.KeyMsg{Type: tea.KeyTab})
	runtime := next.(Model)
	if runtime.drawerSection != drawerRuntime || !strings.Contains(runtime.View(), "Runtime") {
		t.Fatalf("drawer did not select runtime: %#v", runtime.drawerSection)
	}
	next, command = runtime.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	if command == nil || next.(Model).focus != providerField {
		t.Fatalf("drawer provider action = focus:%v command:%t", next.(Model).focus, command != nil)
	}
	next, _ = next.(Model).Update(tea.KeyMsg{Type: tea.KeyEscape})
	closed := next.(Model)
	if closed.drawerOpen || closed.focus != taskField {
		t.Fatalf("drawer did not close to the composer: %#v", closed)
	}

	closed.width = 44
	closed.height = 18
	closed.resizeInputs()
	next, _ = closed.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
	narrow := next.(Model)
	if !narrow.drawerOpen || narrow.drawerUsesSidePane() || !strings.Contains(narrow.View(), "Control center") {
		t.Fatalf("drawer did not use narrow-terminal fallback:\n%s", narrow.View())
	}
	assertViewFits(t, narrow, 44, 18)
}

func agentEvent(step int, text string) agent.Event {
	return agent.Event{Kind: agent.EventText, Step: step, Text: text}
}

func drive(t *testing.T, model Model, message tea.Msg) Model {
	t.Helper()
	current, command := model.Update(message)
	updated := current.(Model)
	for command != nil {
		current, command = updated.Update(command())
		updated = current.(Model)
	}
	return updated
}

func runTeaCommand(t *testing.T, model Model, command tea.Cmd) Model {
	t.Helper()
	for command != nil {
		current, nextCommand := model.Update(command())
		model = current.(Model)
		command = nextCommand
	}
	return model
}

func assertViewFits(t *testing.T, model Model, width, height int) {
	t.Helper()
	gotWidth, gotHeight := lipgloss.Size(model.View())
	if gotWidth > width || gotHeight > height {
		t.Fatalf("view is %dx%d, terminal is %dx%d:\n%s", gotWidth, gotHeight, width, height, model.View())
	}
}

type testAgentModel struct {
	turns    []agent.Turn
	requests []agent.TurnRequest
}

func (m *testAgentModel) Complete(_ context.Context, request agent.TurnRequest) (agent.Turn, error) {
	m.requests = append(m.requests, request)
	if len(m.turns) == 0 {
		return agent.Turn{}, errors.New("unexpected model call")
	}
	turn := m.turns[0]
	m.turns = m.turns[1:]
	return turn, nil
}

func testRepository(t *testing.T) string {
	t.Helper()
	repository := filepath.Join(t.TempDir(), "repository")
	if err := os.MkdirAll(repository, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, contents string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(repository, name), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.com/tui-test\n\ngo 1.25.0\n")
	write("hello.go", "package tuitest\n\nfunc Hello() string { return \"hello\" }\n")
	runGit(t, repository, "init", "--quiet")
	runGit(t, repository, "add", ".")
	runGit(t, repository, "-c", "user.name=Gator Test", "-c", "user.email=gator@example.invalid", "commit", "--quiet", "-m", "fixture")
	return repository
}

func runGit(t *testing.T, directory string, arguments ...string) {
	t.Helper()
	command := exec.Command("git", arguments...)
	command.Dir = directory
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(arguments, " "), err, output)
	}
}
