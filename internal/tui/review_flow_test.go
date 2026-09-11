package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gongahkia/gator/internal/review"
	gatorrun "github.com/gongahkia/gator/internal/run"
)

func TestReviewShowsExplicitPatchHandoffCommands(t *testing.T) {
	model := New(Config{})
	model.screen = reviewScreen
	model.outcome = &gatorrun.Outcome{StatePath: "/state/run-001"}
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	if command != nil {
		t.Fatal("patch handoff returned an unexpected command")
	}
	updated := next.(Model)
	if !strings.Contains(updated.commandOutput, "gator export /state/run-001") || !strings.Contains(updated.commandOutput, "gator apply --check /state/run-001") {
		t.Fatalf("patch handoff = %q", updated.commandOutput)
	}
}

func TestReviewUsesFocusedDiffByDefaultAndCanShowTheFullPatch(t *testing.T) {
	rawDiff := strings.Join([]string{
		"diff --git a/example.go b/example.go",
		"--- a/example.go",
		"+++ b/example.go",
		"@@ -10,7 +10,3 @@ func run() {",
		"-\tif enabled {",
		"-\t\tresult := execute()",
		"-\t\tpersist(result)",
		"-\t}",
		"+\tresult := execute()",
		"+\tpersist(result)",
		" }",
	}, "\n")
	model := New(Config{})
	model.width, model.height = 120, 48
	model.screen = reviewScreen
	next, _ := model.Update(diffLoadedMsg{diff: rawDiff})
	model = next.(Model)
	view := model.View()
	if !strings.Contains(view, "Focused review: collapsed 2 duplicate lines") || strings.Contains(view, "-\t\tresult := execute()") {
		t.Fatalf("focused review view = %q", view)
	}
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	model = next.(Model)
	if model.diffMode != fullDiffDisplay || !strings.Contains(model.View(), "result := execute()") {
		t.Fatalf("full patch view = %q", model.View())
	}
}

func TestReviewDiffNavigationIsBounded(t *testing.T) {
	lines := []string{"diff --git a/example.go b/example.go", "@@ -1 +1 @@"}
	for index := 0; index < 40; index++ {
		lines = append(lines, "+line "+fmt.Sprint(index))
	}
	model := New(Config{})
	model.width, model.height = 100, 18
	model.screen = reviewScreen
	next, _ := model.Update(diffLoadedMsg{diff: strings.Join(lines, "\n")})
	model = next.(Model)
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = next.(Model)
	if model.diffOffset != 1 {
		t.Fatalf("diff offset after down = %d", model.diffOffset)
	}
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnd})
	model = next.(Model)
	if model.diffOffset == 0 || !strings.Contains(model.View(), "diff lines above") {
		t.Fatalf("diff end state = offset:%d view:%q", model.diffOffset, model.View())
	}
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyHome})
	if next.(Model).diffOffset != 0 {
		t.Fatalf("diff offset after home = %d", next.(Model).diffOffset)
	}
}

func TestStructuredReviewNavigatesFilesHunksScopesAndFocusedRawModes(t *testing.T) {
	model := New(Config{})
	model.width, model.height = 120, 48
	model.screen = reviewScreen
	snapshot := review.Snapshot{
		All: review.ChangeSet{Files: []review.File{{
			ID: "aaaaaaaaaaaaaaaaaaaaaaaa", Path: "internal/example.go", Status: "modified", Patch: "diff --git a/internal/example.go b/internal/example.go\n@@ -10 +10 @@\n-old\n+new\n", Stats: review.Stats{Additions: 1, Deletions: 1},
			Hunks: []review.Hunk{{
				ID: "bbbbbbbbbbbbbbbbbbbbbbbb", Header: "@@ -10 +10 @@", Stats: review.Stats{Additions: 1, Deletions: 1}, Lines: []review.Line{{Kind: "deletion", Text: "-old", OldLine: 10}, {Kind: "addition", Text: "+new", NewLine: 10}},
			}},
		}}, Stats: review.Stats{Files: 1, Additions: 1, Deletions: 1}},
		Unstaged: review.ChangeSet{Files: []review.File{{
			ID: "aaaaaaaaaaaaaaaaaaaaaaaa", Path: "internal/example.go", Status: "modified", Patch: "diff --git a/internal/example.go b/internal/example.go\n@@ -10 +10 @@\n-old\n+new\n", Stats: review.Stats{Additions: 1, Deletions: 1},
			Hunks: []review.Hunk{{
				ID: "bbbbbbbbbbbbbbbbbbbbbbbb", Header: "@@ -10 +10 @@", Stats: review.Stats{Additions: 1, Deletions: 1}, Lines: []review.Line{{Kind: "deletion", Text: "-old", OldLine: 10}, {Kind: "addition", Text: "+new", NewLine: 10}},
			}},
		}}, Stats: review.Stats{Files: 1, Additions: 1, Deletions: 1}},
	}
	next, _ := model.Update(reviewLoadedMsg{snapshot: snapshot})
	model = next.(Model)
	if !model.reviewLoaded || !strings.Contains(model.View(), "all changes") || !strings.Contains(model.View(), "Focused selected hunk") {
		t.Fatalf("structured review view = %q", model.View())
	}
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	model = next.(Model)
	if model.reviewScope != review.Unstaged || !strings.Contains(model.View(), "unstaged") {
		t.Fatalf("review scope = %q view=%q", model.reviewScope, model.View())
	}
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	model = next.(Model)
	if !model.reviewRawFiles["aaaaaaaaaaaaaaaaaaaaaaaa"] || !strings.Contains(model.View(), "Raw patch for this file") {
		t.Fatalf("raw review view = %q", model.View())
	}
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	model = next.(Model)
	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	model = next.(Model)
	if model.reviewRangeFrom != 0 || model.reviewPane != reviewLinesPane {
		t.Fatalf("range selection = %d pane=%d", model.reviewRangeFrom, model.reviewPane)
	}
}

func TestStructuredReviewMouseSelectsFileAndHunk(t *testing.T) {
	model := New(Config{})
	model.width, model.height = 120, 48
	model.screen = reviewScreen
	snapshot := review.Snapshot{All: review.ChangeSet{Files: []review.File{
		{ID: "aaaaaaaaaaaaaaaaaaaaaaaa", Path: "one.go", Hunks: []review.Hunk{{ID: "bbbbbbbbbbbbbbbbbbbbbbbb", Header: "@@ -1 +1 @@", Lines: []review.Line{{Kind: "addition", Text: "+one", NewLine: 1}}}}},
		{ID: "cccccccccccccccccccccccc", Path: "two.go", Hunks: []review.Hunk{{ID: "dddddddddddddddddddddddd", Header: "@@ -2 +2 @@", Lines: []review.Line{{Kind: "addition", Text: "+two", NewLine: 2}}}}},
	}}}
	next, _ := model.Update(reviewLoadedMsg{snapshot: snapshot})
	model = next.(Model)
	next, _ = model.Update(tea.MouseMsg{X: 2, Y: 5, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	model = next.(Model)
	if model.reviewFileIndex != 1 || model.reviewPane != reviewFilesPane {
		t.Fatalf("mouse file selection = index:%d pane:%d", model.reviewFileIndex, model.reviewPane)
	}
	next, _ = model.Update(tea.MouseMsg{X: 70, Y: 4, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	model = next.(Model)
	if model.reviewPane != reviewHunksPane || model.reviewHunkIndex != 0 {
		t.Fatalf("mouse hunk selection = index:%d pane:%d", model.reviewHunkIndex, model.reviewPane)
	}
}

func TestStructuredReviewMouseActionBarMatchesKeyboardScopeAndStageActions(t *testing.T) {
	model := New(Config{})
	model.width, model.height, model.screen = 120, 48, reviewScreen
	snapshot := review.Snapshot{Unstaged: review.ChangeSet{Files: []review.File{{
		ID: "aaaaaaaaaaaaaaaaaaaaaaaa", Path: "one.go", Hunks: []review.Hunk{{ID: "bbbbbbbbbbbbbbbbbbbbbbbb", Header: "@@ -1 +1 @@", Lines: []review.Line{{Kind: "addition", Text: "+one", NewLine: 1}}}},
	}}}}
	next, _ := model.Update(reviewLoadedMsg{snapshot: snapshot})
	model = next.(Model)
	next, _ = model.Update(tea.MouseMsg{X: 20, Y: 2, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	model = next.(Model)
	if model.reviewScope != review.Unstaged {
		t.Fatalf("mouse scope = %q", model.reviewScope)
	}
	next, _ = model.Update(tea.MouseMsg{X: 52, Y: 2, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	model = next.(Model)
	if model.reviewMutation == nil || model.reviewMutation.whole {
		t.Fatalf("mouse stage action did not open hunk confirmation: %#v", model.reviewMutation)
	}
	next, _ = model.Update(tea.MouseMsg{X: 12, Y: 2, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if next.(Model).reviewMutation == nil {
		t.Fatal("mouse confirmation did not preserve the explicit mutation command")
	}
}

func TestReviewRequestUsesTerminalPortableCtrlRSendShortcut(t *testing.T) {
	model := New(Config{})
	model.width, model.height, model.screen = 100, 40, reviewScreen
	model.reviewLoaded = true
	model.reviewRequestOn = true
	model.reviewRequest.SetValue("Please update this line.")
	next, command := model.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	if command != nil {
		t.Fatal("request send without a retained run unexpectedly started a command")
	}
	updated := next.(Model)
	if !strings.Contains(updated.notice.text, "no retained thread") {
		t.Fatalf("Ctrl+R did not reach review request submission: %#v", updated.notice)
	}
}
