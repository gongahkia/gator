package main

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/worksession"
	"github.com/gongahkia/gator/internal/worktui"
)

func TestInferInteractiveWorkKeepsConversationNatural(t *testing.T) {
	tests := []struct {
		name   string
		prompt string
		opts   worktui.RunOptions
		mode   action.Mode
		paths  []string
	}{
		{name: "question", prompt: "What conflicts are in these notes?", mode: action.Inspect},
		{name: "brief", prompt: "Write a cited brief", mode: action.Draft, paths: []string{"brief.md"}},
		{name: "named output", prompt: "Create summary.json", mode: action.Draft, paths: []string{"summary.json"}},
		{name: "table workflow", prompt: "Reconcile these invoices", mode: action.Draft, paths: []string{"checked.xlsx", "memo.md"}},
		{name: "code without fake report", prompt: "Implement the missing parser", mode: action.Draft},
		{name: "continued artifact", prompt: "Tighten it", opts: worktui.RunOptions{PreviousArtifacts: []string{"memo.md"}}, mode: action.Draft, paths: []string{"memo.md"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mode, paths, err := inferInteractiveWork(test.prompt, test.opts)
			if err != nil || mode != test.mode || !reflect.DeepEqual(paths, test.paths) {
				t.Fatalf("infer = %q %#v, %v; want %q %#v", mode, paths, err, test.mode, test.paths)
			}
		})
	}
}

func TestInferInteractiveWorkHonorsExplicitInspection(t *testing.T) {
	_, _, err := inferInteractiveWork("write it", worktui.RunOptions{Mode: "inspect", Artifacts: []string{"report.md"}})
	if err == nil {
		t.Fatal("inspection accepted an artifact contract")
	}
}

func TestLoadWorkTUIConversationRestoresOnlySelectedRevisionLineage(t *testing.T) {
	stateDir := t.TempDir()
	store, err := worksession.Open(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 18, 9, 0, 0, 0, time.UTC)
	conversation, err := store.Create("Report", "/source", "snap-one", now)
	if err != nil {
		t.Fatal(err)
	}
	rootBundle := createCLIWorkBundle(t, stateDir, "rev-root")
	selectedBundle := createCLIWorkBundle(t, stateDir, "rev-selected")
	siblingBundle := createCLIWorkBundle(t, stateDir, "rev-sibling")
	for _, revision := range []worksession.Revision{
		{ID: "rev-root", SnapshotID: "snap-one", Objective: "Draft the report", BundlePath: rootBundle.Path, Status: "completed", FinalText: "Initial report ready.", CreatedAt: now},
		{ID: "rev-selected", ParentRevisionID: "rev-root", SnapshotID: "snap-one", Objective: "Revise the report", BundlePath: selectedBundle.Path, Status: "completed", FinalText: "Revised report ready.", CreatedAt: now.Add(time.Minute)},
		{ID: "rev-sibling", ParentRevisionID: "rev-root", SnapshotID: "snap-one", Objective: "Take another direction", BundlePath: siblingBundle.Path, Status: "completed", FinalText: "Sibling report ready.", CreatedAt: now.Add(2 * time.Minute)},
	} {
		if _, err := store.AddRevision(conversation.ID, revision); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.MoveHead(conversation.ID, "rev-selected"); err != nil {
		t.Fatal(err)
	}
	state, err := loadWorkTUIConversation(store, conversation.ID)
	if err != nil {
		t.Fatal(err)
	}
	var transcript string
	cardCount := 0
	for _, item := range state.Messages {
		transcript += item.Text + "\n"
		if item.Bundle != nil {
			cardCount++
		}
	}
	if state.RevisionID != "rev-selected" || state.LastBundle.Path != selectedBundle.Path || state.OutputPath != selectedBundle.Output.Path() ||
		cardCount != 2 || !containsAll(transcript, "Draft the report", "Initial report ready.", "Revise the report", "Revised report ready.") ||
		containsAll(transcript, "Take another direction") {
		t.Fatalf("restored state = %#v\ntranscript=%q", state, transcript)
	}
}

func containsAll(value string, needles ...string) bool {
	for _, needle := range needles {
		if !strings.Contains(value, needle) {
			return false
		}
	}
	return true
}
