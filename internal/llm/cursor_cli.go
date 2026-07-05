package llm

func NewCursorCLIClient(model string) Client {
	return newCursorCLIClientWithRunner(model, nil)
}

func newCursorCLIClientWithRunner(model string, runner cliRunner) Client {
	return &cliClient{
		command:               "cursor-agent",
		model:                 model,
		runner:                runner,
		includeSchemaInPrompt: true,
		parse:                 parseCursorOutput,
		build: func(in cliBuildInput) cliInvocation {
			prompt := "Answer only from the supplied prompt. Do not edit files.\n\n" + in.Prompt
			args := []string{
				"--print",
				"--output-format", "json",
				"--mode", "ask",
			}
			if in.Model != "" {
				args = append(args, "--model", in.Model)
			}
			args = append(args, prompt)
			return cliInvocation{Args: args}
		},
	}
}

func parseCursorOutput(result cliResult) (string, error) {
	return parseJSONStdout(result, "cursor")
}
