package workapp

import (
	"errors"
	"testing"

	"github.com/gongahkia/gator/internal/config"
)

func TestTerminalPreferencesBindChangesToCurrentReviewAndPreserveLiveTaskState(t *testing.T) {
	store, _ := config.New(t.TempDir())
	read := func() (config.TerminalPreferences, bool, error) {
		s, err := store.Load()
		return s.TerminalPreferences(), s.TUI.Preferences != nil, err
	}
	m := New(Config{ReadTerminal: read, SaveTerminal: store.UpdateTerminalPreferences, ImportTerminal: store.ImportTerminalPreferences})
	legacy := config.DefaultTerminalPreferences()
	legacy.Theme = "mono"
	legacy.SidebarCollapsed = false
	next, _ := m.ApplyTerminalPreferences(TerminalPreferencesAction{ID: m.terminal.ID, RequestID: "import", Action: "migrate", Legacy: &legacy})
	m = next.(Model)
	if m.terminalResult.Error != "" || !m.terminal.Initialized || m.terminal.Values != legacy {
		t.Fatal("migration was not acknowledged", m.terminalResult)
	}
	m.running, m.activeRequest, m.conversation, m.input = true, "original-request", "original-chat", "Keep the draft"
	before := m.terminal.ID
	light := "light"
	patch := &config.TerminalPreferencesPatch{Theme: &light}
	next, _ = m.ApplyTerminalPreferences(TerminalPreferencesAction{ID: before, RequestID: "change", Action: "configure", Patch: patch})
	m = next.(Model)
	if m.terminalResult.Error != "" || m.terminal.Values.Theme != light || m.terminalResult.Values == nil {
		t.Fatal("live preference change failed")
	}
	if !m.running || m.activeRequest != "original-request" || m.conversation != "original-chat" || m.input != "Keep the draft" {
		t.Fatal("display change altered work ownership or draft")
	}
	next, _ = m.ApplyTerminalPreferences(TerminalPreferencesAction{ID: before, RequestID: "stale", Action: "configure", Patch: patch})
	m = next.(Model)
	if m.terminalResult.Error == "" {
		t.Fatal("stale preference identity was accepted")
	}
	m.config.SaveTerminal = func(config.TerminalPreferencesPatch, *config.TerminalPreferences) (config.TerminalPreferences, error) {
		return config.TerminalPreferences{}, errors.New("write refused")
	}
	next, _ = m.ApplyTerminalPreferences(TerminalPreferencesAction{ID: m.terminal.ID, RequestID: "failure", Action: "configure", Patch: patch})
	m = next.(Model)
	if m.terminalResult.Error == "" || m.terminalResult.Values != nil || m.terminal.Values.Theme != light {
		t.Fatal("failed persistence reported success or changed the projection")
	}
}
