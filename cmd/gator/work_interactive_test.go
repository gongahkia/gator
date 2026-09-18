package main

import (
	"reflect"
	"testing"

	"github.com/gongahkia/gator/internal/action"
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
