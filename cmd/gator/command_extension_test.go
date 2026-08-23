package main

import (
	"bytes"
	"os"
	"os/exec"
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

func TestProjectExtensionTrustPinsBundleHash(t *testing.T) {
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	t.Setenv("GATOR_DATA_DIR", t.TempDir())
	repository := t.TempDir()
	if output, err := exec.Command("git", "init", "--quiet", repository).CombinedOutput(); err != nil {
		t.Fatalf("initialize Git repository: %v: %s", err, output)
	}
	directory := filepath.Join(repository, ".gator", "extensions", "project-guide")
	if err := os.MkdirAll(filepath.Join(directory, "prompts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "gator-extension.json"), []byte(`{"version":1,"id":"project-guide","name":"Project guide","prompts":["prompts/guide.md"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	prompt := filepath.Join(directory, "prompts", "guide.md")
	if err := os.WriteFile(prompt, []byte("Review all migrations."), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(repository)
	var output bytes.Buffer
	if err := extensionCommand([]string{"trust"}, &output); err != nil {
		t.Fatalf("trust project extension: %v", err)
	}
	if !strings.Contains(output.String(), "Trusted project extension bundle") {
		t.Fatalf("trust output = %q", output.String())
	}
	output.Reset()
	if err := extensionCommand([]string{"status"}, &output); err != nil {
		t.Fatalf("status trusted project extension: %v", err)
	}
	if !strings.Contains(output.String(), "state: active") {
		t.Fatalf("status output = %q", output.String())
	}
	if err := os.WriteFile(prompt, []byte("Changed after trust."), 0o644); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := extensionCommand([]string{"status"}, &output); err != nil {
		t.Fatalf("status changed project extension: %v", err)
	}
	if !strings.Contains(output.String(), "disabled (hash is not trusted)") {
		t.Fatalf("changed status output = %q", output.String())
	}
}
