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

func TestLSPCommandTrustsAndRevokesProjectBundle(t *testing.T) {
	t.Setenv("GATOR_CONFIG_DIR", t.TempDir())
	repository := t.TempDir()
	command := exec.Command("git", "init", "--quiet")
	command.Dir = repository
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("initialize repository: %v: %s", err, output)
	}
	if err := os.MkdirAll(filepath.Join(repository, ".gator"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, ".gator", "fixture-lsp"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"version":1,"servers":[{"name":"fixture","command":[".gator/fixture-lsp"],"language":"go"}]}`
	if err := os.WriteFile(filepath.Join(repository, ".gator", "lsp.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	original, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(repository); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(original) })

	var output bytes.Buffer
	if err := lspCommand([]string{"status"}, &output); err != nil {
		t.Fatalf("status before trust: %v", err)
	}
	if !strings.Contains(output.String(), "disabled") {
		t.Fatalf("status before trust = %q", output.String())
	}
	output.Reset()
	if err := lspCommand([]string{"trust"}, &output); err != nil {
		t.Fatalf("trust bundle: %v", err)
	}
	if !strings.Contains(output.String(), "Trusted project LSP bundle") {
		t.Fatalf("trust output = %q", output.String())
	}
	store, err := config.DefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	settings, err := store.Load()
	if err != nil || len(settings.LSPTrusts) != 1 {
		t.Fatalf("stored LSP trust = %#v, %v", settings.LSPTrusts, err)
	}
	output.Reset()
	if err := lspCommand([]string{"status"}, &output); err != nil {
		t.Fatalf("status after trust: %v", err)
	}
	if !strings.Contains(output.String(), "state: active") {
		t.Fatalf("status after trust = %q", output.String())
	}
	if err := lspCommand([]string{"untrust"}, &output); err != nil {
		t.Fatalf("untrust bundle: %v", err)
	}
	settings, err = store.Load()
	if err != nil || len(settings.LSPTrusts) != 0 {
		t.Fatalf("stored LSP trust after revoke = %#v, %v", settings.LSPTrusts, err)
	}
}
