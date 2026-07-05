package llm

import (
	"encoding/json"
	"fmt"
	"strings"
)

func NewClaudeCLIClient(model string) Client {
	return newClaudeCLIClientWithRunner(model, nil)
}

func newClaudeCLIClientWithRunner(model string, runner cliRunner) Client {
	return &cliClient{
		command: "claude",
		model:   model,
		runner:  runner,
		build: func(in cliBuildInput) cliInvocation {
			args := []string{
				"-p",
				"--permission-mode", "plan",
				"--output-format", "json",
			}
			if in.Model != "" {
				args = append(args, "--model", in.Model)
			}
			if in.Request.JSONSchema != nil {
				args = append(args, "--json-schema", string(in.Request.JSONSchema))
			}
			return cliInvocation{
				Args:  args,
				Stdin: "Answer only from the supplied prompt. Do not modify files.\n\n" + in.Prompt,
			}
		},
		parse: parseClaudeOutput,
	}
}

func parseClaudeOutput(result cliResult) (string, error) {
	raw := strings.TrimSpace(result.Stdout)
	if raw == "" {
		return "", fmt.Errorf("claude response has empty stdout")
	}
	var doc any
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return raw, nil
	}
	if text, ok := jsonStringField(doc, "result", "response", "text", "content", "message"); ok {
		return text, nil
	}
	compact, err := json.Marshal(doc)
	if err != nil {
		return "", err
	}
	return string(compact), nil
}
