package config

import (
	"os"
	"strings"
	"testing"
)

func TestTerminalPreferencesImportPreservesExistingLayoutAndRecentTheme(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	light := "light"
	if _, err := store.UpdateTerminalPreferences(TerminalPreferencesPatch{Theme: &light}, nil); err != nil {
		t.Fatal(err)
	}
	legacy := DefaultTerminalPreferences()
	legacy.Theme = "contrast"
	legacy.SidebarCollapsed = false
	legacy.GroupChatsByFolder = true
	legacy.WorkDetailsSections.Context = true
	got, err := store.ImportTerminalPreferences(legacy)
	if err != nil || got.Theme != light || got.SidebarCollapsed || !got.GroupChatsByFolder || !got.WorkDetailsSections.Context {
		t.Fatalf("import lost explicit theme or layout: %#v, %v", got, err)
	}
	other := DefaultTerminalPreferences()
	if got2, err := store.ImportTerminalPreferences(other); err != nil || got2 != got {
		t.Fatalf("second import overwrote canonical preferences: %#v, %v", got2, err)
	}
	settings, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if settings.TUI.Preferences == nil || settings.Theme != light {
		t.Fatal("import did not persist in Go settings")
	}
	info, err := os.Stat(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0077 != 0 {
		t.Fatal("preferences lost private file permissions")
	}
}

func TestTerminalPreferencePatchesPreserveUnrelatedAcknowledgedChangesAndRejectChangedTargets(t *testing.T) {
	store, _ := New(t.TempDir())
	before, err := store.ImportTerminalPreferences(DefaultTerminalPreferences())
	if err != nil {
		t.Fatal(err)
	}
	show := false
	if _, err := store.UpdateTerminalPreferences(TerminalPreferencesPatch{SidebarCollapsed: &show}, &before); err != nil {
		t.Fatal(err)
	}
	light := "light"
	after, err := store.UpdateTerminalPreferences(TerminalPreferencesPatch{Theme: &light}, &before)
	if err != nil || after.SidebarCollapsed || after.Theme != light {
		t.Fatalf("unrelated patch clobbered acknowledged settings: %#v, %v", after, err)
	}
	contrast := "contrast"
	if _, err := store.UpdateTerminalPreferences(TerminalPreferencesPatch{Theme: &contrast}, &before); err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatal("stale theme replaced a newer choice", err)
	}
	grouped, context := true, true
	if _, err := store.UpdateTerminalPreferences(TerminalPreferencesPatch{GroupChatsByFolder: &grouped, WorkDetailsSections: &TerminalSectionsPatch{Context: &context}}, &after); err != nil {
		t.Fatal(err)
	}
	settings, _ := store.Load()
	got := settings.TerminalPreferences()
	if got.Theme != light || got.SidebarCollapsed || !got.GroupChatsByFolder || !got.WorkDetailsSections.Context || !got.WorkDetailsSections.Outputs {
		t.Fatalf("partial section update lost data: %#v", got)
	}
}

func TestTerminalPreferenceImportUsesLegacyThemeOnlyOnceAndDoesNotAcceptInvalidChanges(t *testing.T) {
	store, _ := New(t.TempDir())
	legacy := DefaultTerminalPreferences()
	legacy.Theme = "mono"
	got, err := store.ImportTerminalPreferences(legacy)
	if err != nil || got != legacy {
		t.Fatalf("legacy values = %#v, %v", got, err)
	}
	before, _ := os.ReadFile(store.Path())
	bad := "rainbow"
	if _, err := store.UpdateTerminalPreferences(TerminalPreferencesPatch{Theme: &bad}, nil); err == nil {
		t.Fatal("invalid theme saved")
	}
	if _, err := store.UpdateTerminalPreferences(TerminalPreferencesPatch{}, nil); err == nil {
		t.Fatal("empty change saved")
	}
	after, _ := os.ReadFile(store.Path())
	if string(after) != string(before) {
		t.Fatal("invalid changes modified the profile")
	}
}
