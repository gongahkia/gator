package llm

func NewGooseCLIClient(provider, model string) Client {
	return newGooseCLIClientWithRunner(provider, model, nil)
}

func newGooseCLIClientWithRunner(provider, model string, runner cliRunner) Client {
	return &cliClient{
		command:               "goose",
		model:                 model,
		runner:                runner,
		includeSchemaInPrompt: true,
		parse:                 parseGooseOutput,
		build: func(in cliBuildInput) cliInvocation {
			prompt := "Answer only from the supplied prompt. Do not inspect files. Do not edit files.\n\n" + in.Prompt
			args := []string{
				"run",
				"--no-session",
				"--quiet",
				"--output-format", "json",
				"--no-profile",
				"--max-turns", "1",
				"--text", prompt,
			}
			if provider != "" {
				args = append(args, "--provider", provider)
			}
			if in.Model != "" {
				args = append(args, "--model", in.Model)
			}
			return cliInvocation{Args: args}
		},
	}
}

func parseGooseOutput(result cliResult) (string, error) {
	return parseJSONStdout(result, "goose")
}
