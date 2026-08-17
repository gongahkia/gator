package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/config"
)

func TestExtensionInstallEnableDisableAndRemove(t *testing.T) {
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	t.Setenv("GATOR_DATA_DIR", t.TempDir())
	source := filepath.Join(t.TempDir(), "fixture")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"version":1,"id":"fixture","name":"Fixture","skills":["skills/guide.md"]}`
	if err := os.WriteFile(filepath.Join(source, "gator-extension.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(source, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "skills", "guide.md"), []byte("Use focused tests."), 0o644); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := extensionCommand([]string{"install", source}, &output); err != nil {
		t.Fatalf("install: %v", err)
	}
	if !strings.Contains(output.String(), "Installed and enabled extension") {
		t.Fatalf("install output = %q", output.String())
	}
	output.Reset()
	if err := extensionCommand([]string{"list"}, &output); err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(output.String(), "fixture  enabled") {
		t.Fatalf("list output = %q", output.String())
	}
	if err := extensionCommand([]string{"disable", "fixture"}, &output); err != nil {
		t.Fatalf("disable: %v", err)
	}
	store, err := config.DefaultStore()
	if err != nil {
		t.Fatalf("config store: %v", err)
	}
	settings, err := store.Load()
	if err != nil || len(settings.Extensions) != 1 || settings.Extensions[0].Enabled {
		t.Fatalf("settings = %#v, err = %v", settings, err)
	}
	if err := extensionCommand([]string{"remove", "fixture", "--yes"}, &output); err != nil {
		t.Fatalf("remove: %v", err)
	}
	settings, err = store.Load()
	if err != nil || len(settings.Extensions) != 0 {
		t.Fatalf("settings after remove = %#v, err = %v", settings, err)
	}
}
