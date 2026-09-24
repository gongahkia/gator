package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gongahkia/gator/internal/learning"
	"github.com/gongahkia/gator/internal/workrun"
)

func TestLearningCLIProvidesUserControlOverSharedStore(t *testing.T) {
	state := t.TempDir()
	t.Setenv("GATOR_STATE_DIR", state)
	var output bytes.Buffer
	if err := run([]string{"learnings", "add", "--type", "preference", "--key", "output-format", "--scope", "project=" + t.TempDir(), "Use", "Markdown."}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Added active learning learning-") {
		t.Fatalf("add output = %q", output.String())
	}
	store, err := learning.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	records, err := store.List()
	if err != nil || len(records) != 1 || records[0].Origin != learning.UserAuthored || records[0].Status != learning.Active {
		t.Fatalf("records = %#v, %v", records, err)
	}
	id := records[0].ID
	output.Reset()
	if err := run([]string{"learnings", "edit", id, "--key", "research-format", "Use", "plain", "Markdown."}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Updated learning "+id) {
		t.Fatalf("edit output = %q", output.String())
	}
	output.Reset()
	if err := run([]string{"learnings", "disable", id}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "disabled") {
		t.Fatalf("disable output = %q", output.String())
	}
	output.Reset()
	if err := run([]string{"learnings", "enable", id}, &output); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := run([]string{"learnings", "show", id}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "research-format") || !strings.Contains(output.String(), "Use plain Markdown.") {
		t.Fatalf("show output = %q", output.String())
	}
}

func TestLearningTUIActionUsesSameInProcessCommandAdapter(t *testing.T) {
	state := t.TempDir()
	project := t.TempDir()
	text, err := learningTUIAction(state, []string{"add", "--type", "environment_fact", "--key", "toolchain", "--scope", "project=" + project, "Use", "Go", "tools."})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "Added active learning") {
		t.Fatalf("TUI action = %q", text)
	}
	store, err := learning.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	records, err := store.List()
	if err != nil || len(records) != 1 || records[0].Scope.Value != filepath.Clean(project) {
		t.Fatalf("shared TUI store records = %#v, %v", records, err)
	}
}

func TestApplyLearningContextProjectsOnlyActiveApplicableRecords(t *testing.T) {
	state := t.TempDir()
	project, other := t.TempDir(), t.TempDir()
	store, err := learning.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(learning.Create{ID: "learning-active", Type: learning.Preference, Key: "format", Content: "Use Markdown.", Scope: learning.Scope{Kind: learning.Project, Value: project}, Origin: learning.UserAuthored}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(learning.Create{ID: "learning-candidate", Type: learning.Preference, Key: "format-other", Content: "Never project this.", Scope: learning.Scope{Kind: learning.Project, Value: project}, Origin: learning.Inferred, Confidence: 55}); err != nil {
		t.Fatal(err)
	}
	request := workrun.Request{SourcePath: project}
	if err := applyLearningContext(state, &request); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(request.LearningContext, "Use Markdown.") || strings.Contains(request.LearningContext, "Never project this.") {
		t.Fatalf("projected learning context = %q", request.LearningContext)
	}
	request = workrun.Request{SourcePath: other}
	if err := applyLearningContext(state, &request); err != nil {
		t.Fatal(err)
	}
	if request.LearningContext != "" {
		t.Fatalf("unrelated project context = %q", request.LearningContext)
	}
}
