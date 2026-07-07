package cmd

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/gongahkia/paw/internal/config"
)

func TestInitNonInteractiveWritesConfigAndRunsDoctor(t *testing.T) {
	isolateEnv(t)
	checker := &fakeEndpointHealth{}
	withDoctorHealth(t, checker)
	path := filepath.Join(t.TempDir(), "config.toml")

	stdout, stderr, err := executeRootErr(t, []string{
		"--config", path,
		"init",
		"--non-interactive",
		"--brain-transport", "ollama",
		"--drone-transport", "ollama",
	}, "")
	if err != nil {
		t.Fatalf("init: %v stderr=%s", err, stderr)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stdout, "wrote "+path) || !strings.Contains(stdout, "brain: ollama") || !strings.Contains(stdout, "next: paw run") {
		t.Fatalf("stdout = %q", stdout)
	}
	if len(checker.endpoints) != 2 {
		t.Fatalf("doctor endpoints = %#v", checker.endpoints)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("load written config: %v", err)
	}
	if cfg.Brain.Transport != "ollama" || cfg.Brain.Model != "gpt-oss:20b" || cfg.Drone.Transport != "ollama" || cfg.Drone.Model != "qwen3:8b" {
		t.Fatalf("config = %#v", cfg)
	}
}

func TestInitInteractiveWritesRepoConfig(t *testing.T) {
	isolateEnv(t)
	withDoctorHealth(t, &fakeEndpointHealth{})
	dir := t.TempDir()
	chdir(t, dir)

	stdout, stderr, err := executeRootErr(t, []string{"init"}, "\nbrain-custom\n\ndrone-custom\nrepo\n")
	if err != nil {
		t.Fatalf("init interactive: %v stderr=%s", err, stderr)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q", stderr)
	}
	for _, want := range []string{"brain transport", "brain model", "drone transport", "drone model", "config location"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout missing %q:\n%s", want, stdout)
		}
	}
	path := filepath.Join(dir, ".paw", "config.toml")
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("load written config: %v", err)
	}
	if cfg.Brain.Model != "brain-custom" || cfg.Drone.Model != "drone-custom" {
		t.Fatalf("config = %#v", cfg)
	}
}
