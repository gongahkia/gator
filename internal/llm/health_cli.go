package llm

import (
	"context"
	"os/exec"
	"strings"
)

func (c EndpointHealthChecker) checkCLI(ctx context.Context, endpoint EndpointConfig, report HealthReport) HealthReport {
	bin := cliBinary(endpoint.Transport)
	lookPath := c.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	path, err := lookPath(bin)
	if err != nil {
		report.add(HealthCheck{Name: "installed", Status: HealthFail, Detail: err.Error(), Action: "install " + bin})
		report.add(HealthCheck{Name: "auth", Status: HealthUnknown, Detail: "not checked because binary is missing"})
		report.add(HealthCheck{Name: "model", Status: HealthUnknown, Detail: "not checked because binary is missing"})
		report.add(cliSchemaCheck(endpoint.Transport))
		return report
	}
	report.add(HealthCheck{Name: "installed", Status: HealthOK, Detail: path})
	version, err := c.runCLI(ctx, bin, []string{"--version"})
	if err != nil {
		report.add(HealthCheck{Name: "running", Status: HealthFail, Detail: err.Error()})
	} else {
		report.add(HealthCheck{Name: "running", Status: HealthOK, Detail: strings.TrimSpace(version)})
	}
	report.add(c.cliCapabilityHealth(ctx, endpoint.Transport, bin))
	report.add(HealthCheck{Name: "auth", Status: HealthUnknown, Detail: "not checked without a model call"})
	if endpoint.Model == "" {
		report.add(HealthCheck{Name: "model", Status: HealthUnknown, Detail: "no model configured"})
	} else {
		report.add(HealthCheck{Name: "model", Status: HealthUnknown, Detail: "model availability is not checked by version probe"})
	}
	report.add(cliSchemaCheck(endpoint.Transport))
	return report
}

func (c EndpointHealthChecker) runCLI(ctx context.Context, command string, args []string) (string, error) {
	runner := c.CLIRunner
	if runner == nil {
		runner = execCLIRunner{}
	}
	result, err := runner.Run(ctx, cliInvocation{Command: command, Args: args})
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(result.Stdout) != "" {
		return result.Stdout, nil
	}
	return result.Stderr, nil
}

func cliSchemaCheck(transport string) HealthCheck {
	switch transport {
	case "codex-cli", "claude-cli":
		return HealthCheck{Name: "schema", Status: HealthOK, Detail: "native schema flag configured"}
	case "gemini-cli", "opencode-cli":
		return HealthCheck{Name: "schema", Status: HealthUnknown, Detail: "schema is prompt-enforced, not CLI-enforced"}
	default:
		return HealthCheck{Name: "schema", Status: HealthUnknown}
	}
}

func (c EndpointHealthChecker) cliCapabilityHealth(ctx context.Context, transport, bin string) HealthCheck {
	spec, ok := cliCapabilitySpec(transport)
	if !ok {
		return HealthCheck{Name: "capabilities", Status: HealthUnknown, Detail: "no capability spec"}
	}
	help, err := c.runCLI(ctx, bin, spec.helpArgs)
	if err != nil {
		return HealthCheck{Name: "capabilities", Status: HealthFail, Detail: err.Error(), Action: "upgrade or reinstall " + bin}
	}
	var missing []string
	for _, flag := range spec.requiredFlags {
		if !strings.Contains(help, flag) {
			missing = append(missing, flag)
		}
	}
	if len(missing) > 0 {
		return HealthCheck{
			Name:   "capabilities",
			Status: HealthFail,
			Detail: "missing flags: " + strings.Join(missing, ", "),
			Action: "upgrade " + bin + " or choose another brain transport",
		}
	}
	return HealthCheck{Name: "capabilities", Status: HealthOK, Detail: "required flags present"}
}

func cliBinary(transport string) string {
	switch transport {
	case "codex-cli":
		return "codex"
	case "gemini-cli":
		return "gemini"
	case "claude-cli":
		return "claude"
	case "opencode-cli":
		return "opencode"
	case "aider-cli":
		return "aider"
	case "goose-cli":
		return "goose"
	case "qwen-cli":
		return "qwen"
	case "cursor-cli":
		return "cursor-agent"
	default:
		return transport
	}
}
