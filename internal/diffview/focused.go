// Package diffview produces a human-focused view of a unified Git diff.
// It deliberately does not produce an applyable patch: callers retain the
// original diff for tools, export, and every machine-readable boundary.
package diffview

import (
	"strconv"
	"strings"
)

const (
	minimumDuplicateBlockLines = 2
	maximumComparableLines     = 512
)

// Focused is a review-only rendering of a unified diff. HiddenLines counts
// source lines that appeared on both sides of a hunk after indentation-only
// normalization; it never changes the underlying patch.
type Focused struct {
	Text         string
	HiddenBlocks int
	HiddenLines  int
}

// Focus collapses duplicated deletion/addition blocks inside each unified-diff
// hunk. This makes an enclosing-control-flow removal easier to review when Git
// otherwise displays its reindented body as a large rewrite. Matching is
// bounded and requires at least two content-bearing lines, so lone braces and
// large generated hunks stay in the complete diff unchanged.
func Focus(diff string) Focused {
	if diff == "" {
		return Focused{}
	}
	lines := strings.Split(diff, "\n")
	rendered := make([]string, 0, len(lines))
	result := Focused{}
	for index := 0; index < len(lines); {
		if !isHunkHeader(lines[index]) {
			rendered = append(rendered, lines[index])
			index++
			continue
		}
		rendered = append(rendered, lines[index])
		end := index + 1
		for end < len(lines) && !isHunkHeader(lines[end]) && !strings.HasPrefix(lines[end], "diff --") {
			end++
		}
		focused, blocks, hidden := focusHunk(lines[index+1 : end])
		rendered = append(rendered, focused...)
		result.HiddenBlocks += blocks
		result.HiddenLines += hidden
		index = end
	}
	result.Text = strings.Join(rendered, "\n")
	return result
}

func isHunkHeader(line string) bool {
	return strings.HasPrefix(line, "@@ ")
}

type changedLine struct {
	rawIndex   int
	normalized string
}

type matchedBlock struct {
	removedStart int
	addedStart   int
	length       int
}

func focusHunk(lines []string) ([]string, int, int) {
	removed, added := changedLines(lines)
	blocks := duplicateBlocks(removed, added)
	if len(blocks) == 0 {
		return lines, 0, 0
	}

	hiddenRemoved := make(map[int]bool)
	hiddenAdded := make(map[int]bool)
	collapseAt := make(map[int]int)
	hiddenLines := 0
	for _, block := range blocks {
		start := min(removed[block.removedStart].rawIndex, added[block.addedStart].rawIndex)
		collapseAt[start] = block.length
		for offset := 0; offset < block.length; offset++ {
			hiddenRemoved[removed[block.removedStart+offset].rawIndex] = true
			hiddenAdded[added[block.addedStart+offset].rawIndex] = true
		}
		hiddenLines += block.length
	}

	focused := make([]string, 0, len(lines)-hiddenLines*2+len(blocks))
	for index, line := range lines {
		if length, found := collapseAt[index]; found {
			focused = append(focused, duplicateBlockMarker(length))
		}
		if hiddenRemoved[index] || hiddenAdded[index] {
			continue
		}
		focused = append(focused, line)
	}
	return focused, len(blocks), hiddenLines
}

func changedLines(lines []string) (removed, added []changedLine) {
	for index, line := range lines {
		switch {
		case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
			if normalized := normalizeContent(line[1:]); normalized != "" {
				removed = append(removed, changedLine{rawIndex: index, normalized: normalized})
			}
		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			if normalized := normalizeContent(line[1:]); normalized != "" {
				added = append(added, changedLine{rawIndex: index, normalized: normalized})
			}
		}
	}
	return removed, added
}

func normalizeContent(line string) string {
	return strings.TrimSpace(line)
}

func duplicateBlocks(removed, added []changedLine) []matchedBlock {
	if len(removed) < minimumDuplicateBlockLines || len(added) < minimumDuplicateBlockLines || len(removed) > maximumComparableLines || len(added) > maximumComparableLines {
		return nil
	}

	// A bounded LCS gives stable, order-preserving matching even when a hunk
	// contains repeated statements. The cap above limits this table to 263,169
	// cells for a malicious or generated patch.
	table := make([][]uint16, len(removed)+1)
	for index := range table {
		table[index] = make([]uint16, len(added)+1)
	}
	for removedIndex := len(removed) - 1; removedIndex >= 0; removedIndex-- {
		for addedIndex := len(added) - 1; addedIndex >= 0; addedIndex-- {
			if removed[removedIndex].normalized == added[addedIndex].normalized {
				table[removedIndex][addedIndex] = table[removedIndex+1][addedIndex+1] + 1
				continue
			}
			table[removedIndex][addedIndex] = max(table[removedIndex+1][addedIndex], table[removedIndex][addedIndex+1])
		}
	}

	type pair struct{ removed, added int }
	pairs := make([]pair, 0, table[0][0])
	for removedIndex, addedIndex := 0, 0; removedIndex < len(removed) && addedIndex < len(added); {
		switch {
		case removed[removedIndex].normalized == added[addedIndex].normalized:
			pairs = append(pairs, pair{removed: removedIndex, added: addedIndex})
			removedIndex++
			addedIndex++
		case table[removedIndex+1][addedIndex] >= table[removedIndex][addedIndex+1]:
			removedIndex++
		default:
			addedIndex++
		}
	}

	blocks := make([]matchedBlock, 0)
	for start := 0; start < len(pairs); {
		end := start + 1
		for end < len(pairs) && pairs[end].removed == pairs[end-1].removed+1 && pairs[end].added == pairs[end-1].added+1 {
			end++
		}
		length := end - start
		if length >= minimumDuplicateBlockLines && contentBearing(removed, pairs[start].removed, length) {
			blocks = append(blocks, matchedBlock{removedStart: pairs[start].removed, addedStart: pairs[start].added, length: length})
		}
		start = end
	}
	return blocks
}

func contentBearing(lines []changedLine, start, length int) bool {
	for index := start; index < start+length; index++ {
		for _, character := range lines[index].normalized {
			if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') {
				return true
			}
		}
	}
	return false
}

func duplicateBlockMarker(length int) string {
	return "  … " + pluralLines(length) + " duplicated on both sides; collapsed in focused review"
}

func pluralLines(length int) string {
	if length == 1 {
		return "1 line"
	}
	return strconv.Itoa(length) + " lines"
}
