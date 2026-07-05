package llm

func NewAiderCLIClient(model string) Client {
	return newAiderCLIClientWithRunner(model, nil)
}

func newAiderCLIClientWithRunner(model string, runner cliRunner) Client {
	return &cliClient{
		command:               "aider",
		model:                 model,
		runner:                runner,
		includeSchemaInPrompt: true,
		parse:                 parseAiderOutput,
		build: func(in cliBuildInput) cliInvocation {
			prompt := "Answer only from the supplied prompt. Do not inspect files. Do not edit files.\n\n" + in.Prompt
			args := []string{
				"--message", prompt,
				"--dry-run",
				"--no-git",
				"--no-gitignore",
				"--no-auto-commits",
				"--no-dirty-commits",
				"--no-auto-lint",
				"--no-auto-test",
				"--no-suggest-shell-commands",
			}
			if in.Model != "" {
				args = append(args, "--model", in.Model)
			}
			return cliInvocation{Args: args}
		},
	}
}

func parseAiderOutput(result cliResult) (string, error) {
	return parseJSONStdout(result, "aider")
}
