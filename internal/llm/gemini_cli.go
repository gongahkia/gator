package llm

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
	return parseJSONStdout(result, "gemini")
}
