package config

import "testing"

func TestDesktopShortcutValidationPreservesTypingAndConflicts(t *testing.T) {
	for _, binding := range []string{"K", "Mod+Q", "Tab", "Escape", "Mod+K+K", "Ctrl+Ctrl+K", "Mod+" + physicalShortcut("Mod") + "+K"} {
		if _, err := NormalizeShortcut(binding); err == nil {
			t.Errorf("accepted %q", binding)
		}
	}
	if got, err := NormalizeShortcut("cmd + shift + p"); err != nil || got != "Meta+Shift+P" {
		t.Fatalf("normalized=%q %v", got, err)
	}
	d := DesktopSettings{Shortcuts: map[string]string{"command_search": "Mod+F"}}
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
	if err != nil || loaded.Desktop.ResolvedShortcuts()["voice_input"] != "Mod+Shift+D" || !loaded.Desktop.Voice.SpokenReplies {
		t.Fatalf("preferences lost: %#v %v", loaded.Desktop, err)
	}
}
