package llm

func NewQwenCLIClient(model string) Client {
	return newQwenCLIClientWithRunner(model, nil)
}

func newQwenCLIClientWithRunner(model string, runner cliRunner) Client {
	return &cliClient{
		command:               "qwen",
		model:                 model,
		runner:                runner,
		includeSchemaInPrompt: true,
		parse:                 parseQwenOutput,
		build: func(in cliBuildInput) cliInvocation {
			prompt := "Answer only from the supplied prompt. Do not inspect files. Do not edit files.\n\n" + in.Prompt
			args := []string{
				"--prompt", prompt,
				"--approval-mode", "plan",
				"--output-format", "json",
			}
			if in.Model != "" {
				args = append(args, "--model", in.Model)
			}
			return cliInvocation{Args: args}
		},
	}
}

func parseQwenOutput(result cliResult) (string, error) {
	return parseJSONStdout(result, "qwen")
}
