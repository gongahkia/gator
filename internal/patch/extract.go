package patch

import (
	"fmt"
	"strings"
)

func Extract(text string) (string, error) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	lines := strings.Split(text, "\n")
	start := -1
	for i, line := range lines {
		if strings.HasPrefix(line, "--- ") {
			start = i
			break
		}
	}
	if start == -1 {
		return "", fmt.Errorf("no unified diff found")
	}
	end := start
	for i := start; i < len(lines); i++ {
		if isDiffLine(lines[i]) {
			end = i
		}
	}
	return strings.Join(lines[start:end+1], "\n") + "\n", nil
}

func isDiffLine(line string) bool {
	if line == "" {
		return false
	}
	for _, prefix := range []string{"--- ", "+++ ", "@@ ", " ", "+", "-", `\ No newline`} {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}
