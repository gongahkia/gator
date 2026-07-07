package llm

type cliCapability struct {
	helpArgs      []string
	requiredFlags []string
}

func cliCapabilitySpec(transport string) (cliCapability, bool) {
	switch transport {
	case "codex-cli":
		return cliCapability{
			helpArgs:      []string{"exec", "--help"},
			requiredFlags: []string{"--sandbox", "--model", "--output-schema", "--cd", "--ephemeral"},
		}, true
	case "gemini-cli":
		return cliCapability{
			helpArgs:      []string{"--help"},
			requiredFlags: []string{"--prompt", "--approval-mode", "--output-format", "--skip-trust", "--model"},
		}, true
	case "claude-cli":
		return cliCapability{
			helpArgs:      []string{"--help"},
			requiredFlags: []string{"--print", "--permission-mode", "--output-format", "--model", "--json-schema"},
		}, true
	case "opencode-cli":
		return cliCapability{
			helpArgs:      []string{"run", "--help"},
			requiredFlags: []string{"--format", "--model", "--dir"},
		}, true
	case "aider-cli":
		return cliCapability{
			helpArgs: []string{"--help"},
			requiredFlags: []string{
				"--message",
				"--dry-run",
				"--no-git",
				"--no-auto-commits",
				"--no-auto-lint",
				"--no-auto-test",
				"--no-suggest-shell-commands",
				"--model",
			},
		}, true
	case "goose-cli":
		return cliCapability{
			helpArgs: []string{"run", "--help"},
			requiredFlags: []string{
				"--no-session",
				"--quiet",
				"--output-format",
				"--no-profile",
				"--max-turns",
				"--provider",
				"--model",
				"--text",
			},
		}, true
	case "qwen-cli":
		return cliCapability{
			helpArgs:      []string{"--help"},
			requiredFlags: []string{"--prompt", "--approval-mode", "--output-format", "--model"},
		}, true
	case "cursor-cli":
		return cliCapability{
			helpArgs:      []string{"--help"},
			requiredFlags: []string{"--print", "--output-format", "--mode", "--model"},
		}, true
	default:
		return cliCapability{}, false
	}
}
