package llm

import "os"

func NewCodexCLIClient(model string) Client {
	return newCodexCLIClientWithRunner(model, nil)
}

func newCodexCLIClientWithRunner(model string, runner cliRunner) Client {
	return &cliClient{
		command:       "codex",
		model:         model,
		runner:        runner,
		useSchemaFile: true,
		build: func(in cliBuildInput) cliInvocation {
			args := []string{"exec", "--ephemeral", "--sandbox", "read-only"}
			if cwd, err := os.Getwd(); err == nil {
				args = append(args, "--cd", cwd)
			}
			if in.Model != "" {
				args = append(args, "--model", in.Model)
			}
			if in.SchemaPath != "" {
				args = append(args, "--output-schema", in.SchemaPath)
			}
			args = append(args, "-")
			return cliInvocation{
				Args:  args,
				Stdin: "Answer only from the supplied prompt. Do not modify files.\n\n" + in.Prompt,
			}
		},
	}
}
