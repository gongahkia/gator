package extension

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/config"
	"github.com/gongahkia/gator/internal/workspace"
)

func TestInstallLoadsInstructionsAndExecutesSidecarTool(t *testing.T) {
	source := writeExtension(t, `{
  "version": 1,
  "id": "review-helper",
  "name": "Review helper",
  "skills": ["skills/review.md"],
	"commands": [{"name":"focused_review","description":"prepare a focused review","prompt":"prompts/review.md"}],
  "tools": [{
    "name": "review",
    "description": "return a review marker",
    "parameters": {"type":"object","additionalProperties":false},
    "command": ["bin/review"]
  }]
}`, map[string]fileSpec{
		"skills/review.md":  {contents: "Always inspect the focused diff."},
		"prompts/review.md": {contents: "Review the current diff and report only concrete findings."},
		"bin/review":        {contents: "#!/bin/sh\nprintf '%s\\n' '{\"content\":\"reviewed\"}'\n", mode: 0o755},
	})
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	installed, err := store.Install(source, false)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if installed.Manifest.ID != "review-helper" {
		t.Fatalf("installed = %#v", installed)
	}
	settings := config.Default()
	settings.Extensions = []config.Extension{{ID: "review-helper", Enabled: true}}
	resolver := NewResolver(store, settings)
	repository := t.TempDir()
	set, err := resolver.Load(repository)
	if err != nil {
		t.Fatalf("load extensions: %v", err)
	}
	instructions, err := set.Instructions()
	if err != nil || !strings.Contains(instructions, "Always inspect") || !strings.Contains(instructions, "review-helper") {
		t.Fatalf("instructions = %q, err = %v", instructions, err)
	}
	commands, err := set.Commands()
	if err != nil || len(commands) != 1 || commands[0].Name != "review-helper:focused_review" || !strings.Contains(commands[0].Prompt, "concrete findings") {
		t.Fatalf("commands = %#v, err = %v", commands, err)
	}
	root, err := workspace.Open(repository)
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	tools := set.Tools(root)
	if len(tools) != 1 || tools[0].Definition().Name != "extension_review_helper_review" {
		t.Fatalf("tools = %#v", tools)
	}
	result, err := tools[0].Execute(context.Background(), []byte(`{}`))
	if err != nil || result.Content != "reviewed" {
		t.Fatalf("execute result = %#v, err = %v", result, err)
	}
}

func TestProjectExtensionsRequireExplicitRepositoryTrust(t *testing.T) {
	repository := t.TempDir()
	writeExtensionAt(t, filepath.Join(repository, ".gator", "extensions", "project-guide"), `{
  "version": 1,
  "id": "project-guide",
  "name": "Project guide",
  "prompts": ["prompts/guide.md"]
}`, map[string]fileSpec{"prompts/guide.md": {contents: "Keep migrations reversible."}})
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	resolver := NewResolver(store, config.Default())
	set, err := resolver.Load(repository)
	if err != nil {
		t.Fatalf("load untrusted extensions: %v", err)
	}
	if !set.Empty() {
		t.Fatal("untrusted project extension was loaded")
	}
	canonical, err := CanonicalRepository(repository)
	if err != nil {
		t.Fatalf("canonical repository: %v", err)
	}
	settings := config.Default()
	settings.TrustedRepositories = []string{canonical}
	set, err = NewResolver(store, settings).Load(repository)
	if err != nil {
		t.Fatalf("load trusted extension: %v", err)
	}
	instructions, err := set.Instructions()
	if err != nil || !strings.Contains(instructions, "Keep migrations reversible") {
		t.Fatalf("trusted instructions = %q, err = %v", instructions, err)
	}
}

func TestInstallRejectsSymlinkedExtensionFiles(t *testing.T) {
	source := writeExtension(t, `{"version":1,"id":"unsafe","name":"Unsafe"}`, nil)
	if err := os.Symlink("/tmp", filepath.Join(source, "outside")); err != nil {
		t.Fatalf("create symlink: %v", err)
	}
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if _, err := store.Install(source, false); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("install error = %v", err)
	}
}

func TestValidGitSourceAcceptsOnlyExplicitRemoteForms(t *testing.T) {
	for value, want := range map[string]bool{
		"https://github.com/example/review-helper.git": true,
		"ssh://git@example.com/team/review-helper.git": true,
		"git@example.com:team/review-helper.git":       true,
		"-not-a-repository":                            false,
		"review-helper":                                false,
	} {
		if got := validGitSource(value); got != want {
			t.Fatalf("validGitSource(%q) = %v, want %v", value, got, want)
		}
	}
}

type fileSpec struct {
	contents string
	mode     os.FileMode
}

func writeExtension(t *testing.T, manifest string, files map[string]fileSpec) string {
	t.Helper()
	directory := filepath.Join(t.TempDir(), "extension")
	writeExtensionAt(t, directory, manifest, files)
	return directory
}

func writeExtensionAt(t *testing.T, directory, manifest string, files map[string]fileSpec) {
	t.Helper()
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, manifestName), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	for path, specification := range files {
		target := filepath.Join(directory, path)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		mode := specification.mode
		if mode == 0 {
			mode = 0o644
		}
		if err := os.WriteFile(target, []byte(specification.contents), mode); err != nil {
			t.Fatal(err)
		}
	}
}
