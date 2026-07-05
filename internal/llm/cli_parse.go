package llm

import (
	"encoding/json"
	"fmt"
	"strings"
)

func parseJSONStdout(result cliResult, label string) (string, error) {
	raw := strings.TrimSpace(result.Stdout)
	if raw == "" {
		return "", fmt.Errorf("%s response has empty stdout", label)
	}
	var text string
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if found, ok := parseJSONText(line); ok {
			text = found
		}
	}
	if text != "" {
		return text, nil
	}
	if found, ok := parseJSONText(raw); ok {
		return found, nil
	}
	return raw, nil
}

func parseJSONText(raw string) (string, bool) {
	var doc any
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return "", false
	}
	if text, ok := lastJSONText(doc); ok {
		return text, true
	}
	compact, err := json.Marshal(doc)
	if err != nil {
		return "", false
	}
	return string(compact), true
}
