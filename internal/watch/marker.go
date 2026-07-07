package watch

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

const DefaultContextLines = 3

type MarkerOptions struct {
	Markers      []string
	ContextLines int
	DisplayPath  string
}

type Marker struct {
	Path         string
	DisplayPath  string
	Line         int
	Marker       string
	Text         string
	Instruction  string
	Context      string
	ContextStart int
	ContextEnd   int
	lineIndex    int
	commentStart int
	commentEnd   int
	fullLine     bool
}

type commentSegment struct {
	body  string
	start int
	end   int
}

func DefaultMarkers() []string {
	return []string{"ai:"}
}

func ParseMarkers(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return DefaultMarkers()
	}
	var markers []string
	for _, marker := range strings.Split(raw, ",") {
		marker = strings.TrimSpace(marker)
		if marker != "" {
			markers = append(markers, marker)
		}
	}
	if len(markers) == 0 {
		return DefaultMarkers()
	}
	return markers
}

func FindMarkers(path string, data []byte, opts MarkerOptions) ([]Marker, error) {
	markers := opts.Markers
	if len(markers) == 0 {
		markers = DefaultMarkers()
	}
	contextLines := opts.ContextLines
	if contextLines <= 0 {
		contextLines = DefaultContextLines
	}
	displayPath := opts.DisplayPath
	if displayPath == "" {
		displayPath = path
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	var out []Marker
	for i, line := range lines {
		segment, marker, text, ok := findMarker(line, markers)
		if !ok {
			continue
		}
		start := i - contextLines
		if start < 0 {
			start = 0
		}
		end := i + contextLines
		if end >= len(lines) {
			end = len(lines) - 1
		}
		m := Marker{
			Path:         path,
			DisplayPath:  displayPath,
			Line:         i + 1,
			Marker:       marker,
			Text:         text,
			Context:      formatContext(lines, start, end),
			ContextStart: start + 1,
			ContextEnd:   end + 1,
			lineIndex:    i,
			commentStart: segment.start,
			commentEnd:   segment.end,
			fullLine:     markerOwnsLine(line, segment.start, segment.end),
		}
		m.Instruction = buildInstruction(m)
		out = append(out, m)
	}
	return out, nil
}

func RemoveMarkers(path string, markers []Marker) error {
	if len(markers) == 0 {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	byLine := map[int]Marker{}
	for _, marker := range markers {
		byLine[marker.lineIndex] = marker
	}
	lines := splitLinesKeepEndings(string(data))
	var out strings.Builder
	for i, line := range lines {
		marker, ok := byLine[i]
		if !ok {
			out.WriteString(line)
			continue
		}
		body, newline := splitLineEnding(line)
		if marker.fullLine {
			continue
		}
		start, end := marker.commentStart, marker.commentEnd
		if start < 0 || end > len(body) || start > end {
			out.WriteString(line)
			continue
		}
		trimmed := strings.TrimRight(body[:start]+body[end:], " \t")
		out.WriteString(trimmed)
		out.WriteString(newline)
	}
	return os.WriteFile(path, []byte(out.String()), info.Mode().Perm())
}

func findMarker(line string, markers []string) (commentSegment, string, string, bool) {
	for _, segment := range commentSegments(line) {
		lowerBody := strings.ToLower(segment.body)
		for _, marker := range markers {
			idx := strings.Index(lowerBody, strings.ToLower(marker))
			if idx < 0 {
				continue
			}
			text := strings.TrimSpace(segment.body[idx+len(marker):])
			if text == "" {
				continue
			}
			return segment, marker, text, true
		}
	}
	return commentSegment{}, "", "", false
}

func commentSegments(line string) []commentSegment {
	var segments []commentSegment
	if idx := strings.Index(line, "//"); idx >= 0 {
		segments = append(segments, commentSegment{body: line[idx+2:], start: idx, end: len(line)})
	}
	if idx := strings.Index(line, "#"); idx >= 0 {
		segments = append(segments, commentSegment{body: line[idx+1:], start: idx, end: len(line)})
	}
	if idx := strings.Index(line, "/*"); idx >= 0 {
		bodyStart := idx + 2
		if close := strings.Index(line[bodyStart:], "*/"); close >= 0 {
			end := bodyStart + close + 2
			segments = append(segments, commentSegment{body: line[bodyStart : bodyStart+close], start: idx, end: end})
		}
	}
	if idx := strings.Index(line, "<!--"); idx >= 0 {
		bodyStart := idx + 4
		if close := strings.Index(line[bodyStart:], "-->"); close >= 0 {
			end := bodyStart + close + 3
			segments = append(segments, commentSegment{body: line[bodyStart : bodyStart+close], start: idx, end: end})
		}
	}
	sort.Slice(segments, func(i, j int) bool {
		return segments[i].start < segments[j].start
	})
	return segments
}

func markerOwnsLine(line string, start, end int) bool {
	return strings.TrimSpace(line[:start]) == "" && strings.TrimSpace(line[end:]) == ""
}

func formatContext(lines []string, start, end int) string {
	var out strings.Builder
	for i := start; i <= end; i++ {
		_, _ = fmt.Fprintf(&out, "%d: %s\n", i+1, lines[i])
	}
	return strings.TrimRight(out.String(), "\n")
}

func buildInstruction(marker Marker) string {
	return fmt.Sprintf("Apply this AI marker in %s:%d: %s\n\nContext:\n%s",
		marker.DisplayPath,
		marker.Line,
		marker.Text,
		marker.Context,
	)
}

func splitLinesKeepEndings(s string) []string {
	if s == "" {
		return nil
	}
	var lines []string
	start := 0
	for i, r := range s {
		if r == '\n' {
			lines = append(lines, s[start:i+1])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func splitLineEnding(line string) (string, string) {
	if strings.HasSuffix(line, "\r\n") {
		return strings.TrimSuffix(line, "\r\n"), "\r\n"
	}
	if strings.HasSuffix(line, "\n") {
		return strings.TrimSuffix(line, "\n"), "\n"
	}
	return line, ""
}
