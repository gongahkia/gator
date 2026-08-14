package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/agent"
)

func TestNewUsesConfiguredRunDefaults(t *testing.T) {
	model := New(Config{
		RepositoryPath: "/tmp/example-repository",
		Model:          "test-model",
		Verification:   [][]string{{"go", "test", "./..."}, {"go", "vet", "./..."}},
	})
	if got, want := model.model.Value(), "test-model"; got != want {
		t.Fatalf("model = %q, want %q", got, want)
	}
	if got, want := model.verification.Value(), "go test ./...\ngo vet ./..."; got != want {
		t.Fatalf("verification = %q, want %q", got, want)
	}
	if model.config.MaxSteps != defaultMaxSteps {
		t.Fatalf("max steps = %d, want %d", model.config.MaxSteps, defaultMaxSteps)
	}
}

func TestComposerRejectsEmptyTaskBeforeRun(t *testing.T) {
	model := New(Config{APIKey: "test-key"})
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	if command != nil {
		t.Fatal("empty task started an execution")
	}
	updated := next.(Model)
	if updated.notice.kind != noticeError || !strings.Contains(updated.notice.text, "Describe a task") {
		t.Fatalf("notice = %#v", updated.notice)
	}
}

func TestComposerReportsMissingAPIKeyBeforeRun(t *testing.T) {
	model := New(Config{Verification: [][]string{{"go", "test", "./..."}}})
	model.task.SetValue("Add a greeting")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	if command != nil {
		t.Fatal("missing API key started an execution")
	}
	updated := next.(Model)
	if updated.notice.kind != noticeError || !strings.Contains(updated.notice.text, "OPENAI_API_KEY") {
		t.Fatalf("notice = %#v", updated.notice)
	}
}

func TestParseVerificationSplitsOneCommandPerLine(t *testing.T) {
	commands, err := parseVerification("go test ./...\n\nnpm test")
	if err != nil {
		t.Fatalf("parse verification: %v", err)
	}
	if got, want := formatVerification(commands), "go test ./...\nnpm test"; got != want {
		t.Fatalf("commands = %q, want %q", got, want)
	}
}

func TestReviewContinuationLoadsRetainedSession(t *testing.T) {
	model := New(Config{})
	model.screen = reviewScreen
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	if command != nil {
		t.Fatal("missing run record returned a command")
	}
	updated := next.(Model)
	if updated.notice.kind != noticeError || !strings.Contains(updated.notice.text, "no resume record") {
		t.Fatalf("notice = %#v", updated.notice)
	}
}

func TestRenderEventDoesNotLeakMultilineOutputIntoTimeline(t *testing.T) {
	entry := renderEvent(agentEvent(3, "one\ntwo"))
	if entry.text != "[03] agent: one two" {
		t.Fatalf("timeline entry = %q", entry.text)
	}
}

func agentEvent(step int, text string) agent.Event {
	return agent.Event{Kind: agent.EventText, Step: step, Text: text}
}
