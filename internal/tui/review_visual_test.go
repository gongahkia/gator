package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/gongahkia/gator/internal/review"
)

func TestReviewVisualSnapshots(t *testing.T) {
	for _, size := range []struct {
		name          string
		width, height int
	}{{"desktop", 120, 48}, {"compact", 70, 30}} {
		t.Run(size.name, func(t *testing.T) {
			model := reviewVisualFixture(size.width, size.height)
			actual := normalizeReviewSnapshot(ansi.Strip(model.View()))
			path := filepath.Join("testdata", "review_"+size.name+".golden")
			if os.Getenv("UPDATE_REVIEW_GOLDEN") != "" {
				if err := os.WriteFile(path, []byte(actual+"\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			expectedBytes, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			expected := normalizeReviewSnapshot(string(expectedBytes))
			if actual != expected {
				index := firstSnapshotDifference(expected, actual)
				t.Fatalf("review visual snapshot changed for %s at byte %d: want %q got %q", size.name, index, expected[index:min(len(expected), index+24)], actual[index:min(len(actual), index+24)])
			}
		})
	}
}

func firstSnapshotDifference(first, second string) int {
	limit := min(len(first), len(second))
	for index := 0; index < limit; index++ {
		if first[index] != second[index] {
			return index
		}
	}
	return limit
}

func reviewVisualFixture(width, height int) Model {
	model := New(Config{ExtensionUI: []ExtensionUIContribution{{ID: "team:check", Slot: "review", Title: "Team checklist", Description: "Check the exported API and regression coverage."}}})
	model.width, model.height, model.screen = width, height, reviewScreen
	snapshot := review.Snapshot{All: review.ChangeSet{Files: []review.File{
		{ID: "aaaaaaaaaaaaaaaaaaaaaaaa", Path: "internal/review/example.go", Status: "modified", Stats: review.Stats{Additions: 1, Deletions: 1}, Hunks: []review.Hunk{{ID: "bbbbbbbbbbbbbbbbbbbbbbbb", Header: "@@ -10,2 +10,2 @@ func example()", Stats: review.Stats{Additions: 1, Deletions: 1}, Lines: []review.Line{{Kind: "context", Text: " func example() {", OldLine: 10, NewLine: 10}, {Kind: "deletion", Text: "-\treturn oldValue", OldLine: 11}, {Kind: "addition", Text: "+\treturn newValue", NewLine: 11}}}}},
		{ID: "cccccccccccccccccccccccc", Path: "internal/review/example_test.go", Status: "added", Stats: review.Stats{Additions: 2}, Hunks: []review.Hunk{{ID: "dddddddddddddddddddddddd", Header: "@@ -0,0 +1,2 @@", Stats: review.Stats{Additions: 2}, Lines: []review.Line{{Kind: "addition", Text: "+func TestExample(t *testing.T) {}", NewLine: 1}, {Kind: "addition", Text: "+", NewLine: 2}}}}},
	}, Stats: review.Stats{Files: 2, Additions: 3, Deletions: 1}}}
	next, _ := model.Update(reviewLoadedMsg{snapshot: snapshot})
	return next.(Model)
}

func normalizeReviewSnapshot(value string) string {
	lines := strings.Split(strings.ToValidUTF8(strings.ReplaceAll(value, "\r\n", "\n"), "�"), "\n")
	for index := range lines {
		lines[index] = strings.TrimRight(compressReplacementRunes(lines[index]), " ")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func compressReplacementRunes(value string) string {
	var result strings.Builder
	previousReplacement := false
	for _, character := range value {
		if character == '�' {
			if previousReplacement {
				continue
			}
			previousReplacement = true
		} else {
			previousReplacement = false
		}
		result.WriteRune(character)
	}
	return result.String()
}
