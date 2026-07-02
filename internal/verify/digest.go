package verify

import "strings"

const (
	defaultFailureDigestBytes = 4 * 1024
	defaultFailureTailLines   = 80
)

var failureMarkers = []string{
	"FAIL",
	"Error",
	"assert",
	"panic",
	"Traceback",
	"expected",
	"got",
}

func failureDigest(output []byte, maxBytes int) string {
	if len(output) == 0 {
		return ""
	}
	if maxBytes <= 0 {
		maxBytes = defaultFailureDigestBytes
	}
	lines := outputLines(output)
	selected := make([]string, 0, len(lines))
	seen := make(map[string]struct{}, len(lines))
	add := func(line string) {
		if line == "" {
			return
		}
		if _, ok := seen[line]; ok {
			return
		}
		seen[line] = struct{}{}
		selected = append(selected, line)
	}
	for _, line := range lines {
		if hasFailureMarker(line) {
			add(line)
		}
	}
	start := 0
	if len(lines) > defaultFailureTailLines {
		start = len(lines) - defaultFailureTailLines
	}
	for _, line := range lines[start:] {
		add(line)
	}
	return capLines(selected, maxBytes)
}

func outputLines(output []byte) []string {
	text := strings.ReplaceAll(string(output), "\r\n", "\n")
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

func hasFailureMarker(line string) bool {
	for _, marker := range failureMarkers {
		if strings.Contains(line, marker) {
			return true
		}
	}
	return false
}

func capLines(lines []string, maxBytes int) string {
	var b strings.Builder
	for _, line := range lines {
		if b.Len() >= maxBytes {
			break
		}
		sep := 0
		if b.Len() > 0 {
			sep = 1
		}
		remaining := maxBytes - b.Len() - sep
		if remaining < 0 {
			break
		}
		if sep == 1 {
			b.WriteByte('\n')
		}
		if len(line) <= remaining {
			b.WriteString(line)
			continue
		}
		b.WriteString(line[:remaining])
		break
	}
	return b.String()
}
