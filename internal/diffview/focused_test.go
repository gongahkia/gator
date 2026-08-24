package diffview

import (
	"strconv"
	"strings"
	"testing"
)

func TestFocusCollapsesReindentedDuplicateBlock(t *testing.T) {
	diff := strings.Join([]string{
		"diff --git a/example.go b/example.go",
		"index old..new 100644",
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

	focused := Focus(diff)
	if focused.HiddenBlocks != 1 || focused.HiddenLines != 2 {
		t.Fatalf("focus result = %#v", focused)
	}
	if strings.Contains(focused.Text, "-\t\tresult := execute()") || strings.Contains(focused.Text, "+\tresult := execute()") {
		t.Fatalf("duplicated block remained visible:\n%s", focused.Text)
	}
	if !strings.Contains(focused.Text, "-\tif enabled {") || !strings.Contains(focused.Text, "2 lines duplicated on both sides") {
		t.Fatalf("focused diff did not retain the meaningful change:\n%s", focused.Text)
	}
}

func TestFocusKeepsSingleOrPunctuationOnlyMatches(t *testing.T) {
	diff := strings.Join([]string{
		"diff --git a/example.go b/example.go",
		"@@ -1 +1 @@",
		"-}",
		"+}",
	}, "\n")

	focused := Focus(diff)
	if focused.HiddenBlocks != 0 || focused.Text != diff {
		t.Fatalf("single punctuation match should remain a normal diff: %#v", focused)
	}
}

func TestFocusPreservesLargeOrNonUnifiedInput(t *testing.T) {
	lines := []string{"diff --git a/generated.go b/generated.go", "@@ -1 +1 @@"}
	for index := 0; index < maximumComparableLines+1; index++ {
		lines = append(lines, "-\tline"+strconv.Itoa(index), "+line"+strconv.Itoa(index))
	}
	diff := strings.Join(lines, "\n")
	if focused := Focus(diff); focused.HiddenBlocks != 0 || focused.Text != diff {
		t.Fatalf("large hunk should remain unchanged: %#v", focused)
	}
}
