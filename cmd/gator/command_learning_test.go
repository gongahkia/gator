package main

import (
	"bytes"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/learning"
	"github.com/gongahkia/gator/internal/workrun"
	"github.com/gongahkia/gator/internal/worktui"
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

func TestLearningCLIListsFiltersAndApprovesCandidates(t *testing.T) {
	state, project := t.TempDir(), t.TempDir()
	t.Setenv("GATOR_STATE_DIR", state)
	store, err := learning.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(learning.Create{ID: "learning-candidate-filter", Type: learning.Preference, Key: "format", Content: "Use concise Markdown.", Scope: learning.Scope{Kind: learning.Project, Value: project}, Origin: learning.Inferred, Confidence: 85}); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := run([]string{"learnings", "list", "--status", "candidate", "--scope", "project=" + project}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "learning-candidate-filter") || !strings.Contains(output.String(), "candidate") {
		t.Fatalf("filtered candidates = %q", output.String())
	}
	output.Reset()
	if err := run([]string{"learnings", "approve", "learning-candidate-filter"}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "active") {
		t.Fatalf("approve output = %q", output.String())
	}
	record, err := store.Load("learning-candidate-filter")
	if err != nil || record.Status != learning.Active || record.Provenance.UserConfirmedAt.IsZero() {
		t.Fatalf("approved record = %#v, err = %v", record, err)
	}
}

func TestLearningTUIAndCLIImmediatelyShareCanonicalState(t *testing.T) {
	state, project := t.TempDir(), t.TempDir()
	t.Setenv("GATOR_STATE_DIR", state)
	store, err := learning.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := store.Create(learning.Create{ID: "learning-tui-cli", Type: learning.Preference, Key: "format", Content: "Use Markdown.", Scope: learning.Scope{Kind: learning.Project, Value: project}, Origin: learning.Inferred, Confidence: 80})
	if err != nil {
		t.Fatal(err)
	}
	model := worktui.New(worktui.Config{CurrentFolder: project, LearningStore: &store})
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	model = updated.(worktui.Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("learnings")})
	model = updated.(worktui.Model)
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if command != nil {
		t.Fatal("opening Learnings scheduled external work")
	}
	model = updated.(worktui.Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(worktui.Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	model = updated.(worktui.Model)
	if !strings.Contains(model.View(), "Status: active") {
		t.Fatalf("TUI approval did not update learning detail:\n%s", model.View())
	}
	var output bytes.Buffer
	if err := run([]string{"learnings", "show", candidate.ID}, &output); err != nil || !strings.Contains(output.String(), `"status":"active"`) {
		t.Fatalf("CLI did not observe TUI approval: %q, %v", output.String(), err)
	}
	if err := run([]string{"learnings", "disable", candidate.ID}, &output); err != nil {
		t.Fatal(err)
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(worktui.Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	model = updated.(worktui.Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(worktui.Model)
	if !strings.Contains(model.View(), "Status: disabled") {
		t.Fatalf("TUI did not observe CLI mutation:\n%s", model.View())
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

func TestWorkCLIProjectsApplicableLearningsIntoTheManagerPrompt(t *testing.T) {
	state, source := t.TempDir(), t.TempDir()
	t.Setenv("GATOR_STATE_DIR", state)
	t.Setenv("GATOR_PROVIDER", "openai")
	t.Setenv("GATOR_MODEL", "test-model")
	store, err := learning.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(learning.Create{ID: "learning-cli", Type: learning.Preference, Key: "format", Content: "Use Markdown.", Scope: learning.Scope{Kind: learning.Project, Value: source}, Origin: learning.UserAuthored}); err != nil {
		t.Fatal(err)
	}
	model := &workScriptedModel{turns: []agent.Turn{{Text: "Inspected."}}}
	if err := runWorkTask([]string{"--source", source, "--mode", "inspect", "inspect"}, strings.NewReader(""), &bytes.Buffer{}, func(_, _, _ string) (agent.Model, error) { return model, nil }); err != nil {
		t.Fatal(err)
	}
	if len(model.requests) != 1 || !strings.Contains(model.requests[0].System, "Use Markdown.") {
		t.Fatalf("manager request did not receive applicable learning: %#v", model.requests)
	}
}
