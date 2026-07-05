package llm

import (
	"encoding/json"
	"fmt"
	"strings"
)

func NewGeminiCLIClient(model string) Client {
	return newGeminiCLIClientWithRunner(model, nil)
}

func newGeminiCLIClientWithRunner(model string, runner cliRunner) Client {
	return &cliClient{
		command:               "gemini",
		model:                 model,
		runner:                runner,
		includeSchemaInPrompt: true,
		build: func(in cliBuildInput) cliInvocation {
			args := []string{
				"--prompt", "Answer only from stdin. Do not modify files.",
				"--approval-mode", "plan",
				"--output-format", "json",
				"--skip-trust",
			}
			if in.Model != "" {
				args = append(args, "--model", in.Model)
			}
			return cliInvocation{Args: args, Stdin: in.Prompt}
		},
		parse: parseGeminiOutput,
	}
}

func parseGeminiOutput(result cliResult) (string, error) {
	raw := strings.TrimSpace(result.Stdout)
	if raw == "" {
		return "", fmt.Errorf("gemini response has empty stdout")
	}
	var doc any
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return raw, nil
	}
	if text, ok := jsonStringField(doc, "response", "text", "content", "output", "message"); ok {
		return text, nil
	}
	compact, err := json.Marshal(doc)
	if err != nil {
		return "", err
	}
	return string(compact), nil
}

func jsonStringField(doc any, keys ...string) (string, bool) {
	obj, ok := doc.(map[string]any)
	if !ok {
		if s, ok := doc.(string); ok {
			return s, true
		}
		return "", false
	}
	for _, key := range keys {
		if value, ok := obj[key].(string); ok {
			return value, true
		}
	}
	return "", false
}
