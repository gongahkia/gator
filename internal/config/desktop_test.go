package config

import (
	"strings"
	"testing"
)

func TestJumpLatestShortcutUsesAuthoritativeBindingsAndResolvesExistingOverrides(t *testing.T) {
	settings := DesktopSettings{}
	if got := settings.ResolvedShortcuts()["jump_latest"]; got != "Ctrl+L" {
		t.Fatalf("default latest shortcut = %q", got)
	}
	settings.Shortcuts = map[string]string{"command_search": "Ctrl+L"}
	if err := settings.Validate(); err != nil {
		t.Fatal(err)
	}
	if got := settings.ResolvedShortcuts()["jump_latest"]; got != "Ctrl+Shift+L" {
		t.Fatalf("existing binding was not preserved: latest=%q", got)
	}
	settings.Shortcuts["jump_latest"] = "Ctrl+K"
	if err := settings.Validate(); err != nil || settings.ResolvedShortcuts()["jump_latest"] != "Ctrl+K" {
		t.Fatalf("custom latest binding failed: %v", err)
	}
}

func TestDesktopShortcutValidationPreservesTypingAndConflicts(t *testing.T) {
	for _, binding := range []string{"K", "Mod+Q", "Tab", "Escape", "Mod+K+K", "Ctrl+Ctrl+K", "Mod+" + physicalShortcut("Mod") + "+K"} {
		if _, err := NormalizeShortcut(binding); err == nil {
			t.Errorf("accepted %q", binding)
		}
	}
	if got, err := NormalizeShortcut("cmd + shift + p"); err != nil || got != "Ctrl+Shift+P" {
		t.Fatalf("normalized=%q %v", got, err)
	}
	d := DesktopSettings{Shortcuts: map[string]string{"command_search": "Mod+F", "conversation_search": "Ctrl+F"}}
	if err := d.Validate(); err == nil {
		t.Fatal("conflicting shortcut accepted")
	}
	d.Shortcuts["command_search"] = "Mod+P"
	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestDesktopPreferencesSurviveConfigRoundTrip(t *testing.T) {
	store, _ := New(t.TempDir())
	settings := Default()
	settings.Desktop = DesktopSettings{Shortcuts: map[string]string{"voice_input": "Mod+Shift+D"}, Voice: VoiceSettings{Provider: "gemini", SpokenReplies: true, Language: "en-SG", Rate: 1.1}}
	if err := store.Save(settings); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load()
	if err != nil || loaded.Desktop.ResolvedShortcuts()["voice_input"] != "Ctrl+Shift+D" || !loaded.Desktop.Voice.SpokenReplies {
		t.Fatalf("preferences lost: %#v %v", loaded.Desktop, err)
	}
}

func TestChatLearningDefaultsOnAndPersistsOff(t *testing.T) {
	if !(DesktopSettings{}).LearnsFromChats() {
		t.Fatal("chat learning should be enabled by default")
	}
	store, _ := New(t.TempDir())
	settings := Default()
	off := false
	settings.Desktop.LearnFromChats = &off
	if err := store.Save(settings); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load()
	if err != nil || loaded.Desktop.LearnsFromChats() {
		t.Fatal("disabled preference lost", err)
	}
}

func TestNewChatShortcutPreservesExistingCustomBinding(t *testing.T) {
	d := DesktopSettings{Shortcuts: map[string]string{"command_search": "Mod+N"}}
	if err := d.Validate(); err != nil {
		t.Fatal("adding new chat invalidated an older profile", err)
	}
	r := d.ResolvedShortcuts()
	if r["command_search"] != "Ctrl+N" || r["new_chat"] != "Ctrl+Shift+N" {
		t.Fatal(r)
	}
	d.Shortcuts["new_chat"] = "Mod+N"
	if d.Validate() == nil {
		t.Fatal("explicit conflicting shortcuts accepted")
	}
}

func TestContextActionsPreserveExistingShortcutAndConfiguredBinding(t *testing.T) {
	d := DesktopSettings{Shortcuts: map[string]string{"command_search": "Ctrl+E"}}
	if err := d.Validate(); err != nil {
		t.Fatal("new item action invalidated an older profile", err)
	}
	if got := d.ResolvedShortcuts(); got["command_search"] != "Ctrl+E" || got["context_actions"] != "Ctrl+Shift+E" {
		t.Fatal("existing shortcut was stolen", got)
	}
	d.Shortcuts["context_actions"] = "Ctrl+K"
	if err := d.Validate(); err != nil || d.ResolvedShortcuts()["context_actions"] != "Ctrl+K" {
		t.Fatal("configured item shortcut was lost", err)
	}
}

func TestTerminalShortcutsMigrateFunctionKeysAndPrimaryModifiers(t *testing.T) {
	d := DesktopSettings{Shortcuts: map[string]string{"help": "F1", "new_chat": "Ctrl+F3", "command_search": "Cmd+O"}}
	if err := d.Validate(); err != nil {
		t.Fatal("legacy settings no longer load", err)
	}
	resolved := d.ResolvedShortcuts()
	if resolved["command_search"] != "Ctrl+O" || resolved["help"] != "Ctrl+Shift+O" || resolved["new_chat"] != "Ctrl+N" {
		t.Fatal("migration lost a custom binding or introduced a conflict", resolved)
	}
	for action, binding := range resolved {
		if binding != "Enter" && !(action == "queue_message" && binding == "Tab") && !strings.HasPrefix(binding, "Ctrl+") {
			t.Errorf("%s resolved to %q", action, binding)
		}
	}
	for _, binding := range []string{"F1", "F12", "Ctrl+F3", "Alt+F4", "Shift+F7"} {
		if _, err := NormalizeShortcut(binding); err == nil {
			t.Errorf("accepted retired function shortcut %q", binding)
		}
	}
	// Migration must not conceal invalid action names in the settings file.
	d.Shortcuts["unknown"] = "F1"
	if err := d.Validate(); err == nil {
		t.Fatal("unknown shortcut action accepted")
	}
}

func TestQueueShortcutUsesTabOnlyForItsContextualAction(t *testing.T) {
	settings := DesktopSettings{}
	if err := settings.Validate(); err != nil || settings.ResolvedShortcuts()["queue_message"] != "Tab" {
		t.Fatal("queue default did not resolve", err)
	}
	settings.Shortcuts = map[string]string{"queue_message": "Ctrl+Shift+K"}
	if err := settings.Validate(); err != nil || settings.ResolvedShortcuts()["queue_message"] != "Ctrl+Shift+K" {
		t.Fatal("custom queue binding did not resolve", err)
	}
	settings.Shortcuts["queue_message"] = "tab"
	if err := settings.Validate(); err != nil || settings.ResolvedShortcuts()["queue_message"] != "Tab" {
		t.Fatal(err)
	}
	settings.Shortcuts["help"] = "Tab"
	if err := settings.Validate(); err == nil {
		t.Fatal("Tab became a global action")
	}
}
