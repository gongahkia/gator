package llm

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

func NewOpenCodeCLIClient(model string) Client {
	return newOpenCodeCLIClientWithRunner(model, nil)
}

func newOpenCodeCLIClientWithRunner(model string, runner cliRunner) Client {
	return &cliClient{
		command:               "opencode",
		model:                 model,
		runner:                runner,
		includeSchemaInPrompt: true,
		build: func(in cliBuildInput) cliInvocation {
			args := []string{"run", "--format", "json"}
			if cwd, err := os.Getwd(); err == nil {
				args = append(args, "--dir", cwd)
			}
			if in.Model != "" {
				args = append(args, "--model", in.Model)
			}
			args = append(args, "Answer only from the supplied prompt. Do not modify files.\n\n"+in.Prompt)
			return cliInvocation{Args: args}
		},
		parse: parseOpenCodeOutput,
	}
}

func parseOpenCodeOutput(result cliResult) (string, error) {
	raw := strings.TrimSpace(result.Stdout)
	if raw == "" {
		return "", fmt.Errorf("opencode response has empty stdout")
	}
	var text string
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if found, ok := parseOpenCodeJSONText(line); ok {
			text = found
		}
	}
	if text != "" {
		return text, nil
	}
	if found, ok := parseOpenCodeJSONText(raw); ok {
		return found, nil
	}
	return raw, nil
}

func parseOpenCodeJSONText(raw string) (string, bool) {
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

func lastJSONText(doc any) (string, bool) {
	switch v := doc.(type) {
	case string:
		return v, true
	case []any:
		for i := len(v) - 1; i >= 0; i-- {
			if text, ok := lastJSONText(v[i]); ok {
				return text, true
			}
		}
	case map[string]any:
		for _, key := range []string{"content", "message", "response", "output", "result", "text"} {
			if text, ok := v[key].(string); ok && text != "" {
				return text, true
			}
			if value, ok := v[key]; ok {
				if text, ok := lastJSONText(value); ok {
					return text, true
				}
			}
		}
		for key, value := range v {
			switch key {
			case "type", "id", "role", "status":
				continue
			}
			if text, ok := lastJSONText(value); ok {
				return text, true
			}
		}
	}
	return "", false
}
