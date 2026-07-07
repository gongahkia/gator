package llm

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestHealthCLIVersionProbe(t *testing.T) {
	runner := &sequenceCLIRunner{results: []cliResult{
		{Stdout: "codex-cli 1.2.3\n"},
		{Stdout: "--sandbox --model --output-schema --cd --ephemeral"},
	}}
	report := EndpointHealthChecker{
		CLIRunner: runner,
		LookPath: func(command string) (string, error) {
			if command != "codex" {
				t.Fatalf("command = %q", command)
			}
			return "/usr/local/bin/codex", nil
		},
	}.Check(context.Background(), EndpointConfig{
		Transport: "codex-cli",
		Model:     "gpt-test",
	})
	if len(runner.invocations) != 2 {
		t.Fatalf("invocations = %#v", runner.invocations)
	}
	if runner.invocations[0].Command != "codex" || !reflect.DeepEqual(runner.invocations[0].Args, []string{"--version"}) {
		t.Fatalf("version invocation = %#v", runner.invocations[0])
	}
	if !reflect.DeepEqual(runner.invocations[1].Args, []string{"exec", "--help"}) {
		t.Fatalf("help invocation = %#v", runner.invocations[1])
	}
	requireCheck(t, report, "installed", HealthOK)
	requireCheck(t, report, "running", HealthOK)
	requireCheck(t, report, "capabilities", HealthOK)
	requireCheck(t, report, "auth", HealthUnknown)
	requireCheck(t, report, "model", HealthUnknown)
	requireCheck(t, report, "schema", HealthOK)
}

func TestHealthCLIMissingCapabilityFlag(t *testing.T) {
	runner := &sequenceCLIRunner{results: []cliResult{
		{Stdout: "2.1.119\n"},
		{Stdout: "--print --permission-mode --output-format --model"},
	}}
	report := EndpointHealthChecker{
		CLIRunner: runner,
		LookPath: func(command string) (string, error) {
			return "/usr/local/bin/" + command, nil
		},
	}.Check(context.Background(), EndpointConfig{Transport: "claude-cli"})
	check := requireCheck(t, report, "capabilities", HealthFail)
	if !strings.Contains(check.Detail, "--json-schema") || !strings.Contains(check.Action, "upgrade claude") {
		t.Fatalf("check = %#v", check)
	}
}

func TestHealthNewCLICapabilitySpecs(t *testing.T) {
	tests := []struct {
		transport string
		binary    string
		helpArgs  []string
		help      string
	}{
		{"aider-cli", "aider", []string{"--help"}, "--message --dry-run --no-git --no-auto-commits --no-auto-lint --no-auto-test --no-suggest-shell-commands --model"},
		{"goose-cli", "goose", []string{"run", "--help"}, "--no-session --quiet --output-format --no-profile --max-turns --provider --model --text"},
		{"qwen-cli", "qwen", []string{"--help"}, "--prompt --approval-mode --output-format --model"},
		{"cursor-cli", "cursor-agent", []string{"--help"}, "--print --output-format --mode --model"},
	}
	for _, tc := range tests {
		t.Run(tc.transport, func(t *testing.T) {
			runner := &sequenceCLIRunner{results: []cliResult{
				{Stdout: tc.binary + " 1.0\n"},
				{Stdout: tc.help},
			}}
			report := EndpointHealthChecker{
				CLIRunner: runner,
				LookPath: func(command string) (string, error) {
					if command != tc.binary {
						t.Fatalf("command = %q; want %q", command, tc.binary)
					}
					return "/usr/local/bin/" + command, nil
				},
			}.Check(context.Background(), EndpointConfig{Transport: tc.transport, Model: "test-model"})
			if len(runner.invocations) != 2 {
				t.Fatalf("invocations = %#v", runner.invocations)
			}
			if !reflect.DeepEqual(runner.invocations[1].Args, tc.helpArgs) {
				t.Fatalf("help args = %#v; want %#v", runner.invocations[1].Args, tc.helpArgs)
			}
			requireCheck(t, report, "installed", HealthOK)
			requireCheck(t, report, "running", HealthOK)
			requireCheck(t, report, "capabilities", HealthOK)
			requireCheck(t, report, "model", HealthUnknown)
		})
	}
}

func TestHealthCLIMissingBinary(t *testing.T) {
	report := EndpointHealthChecker{
		LookPath: func(string) (string, error) {
			return "", fmt.Errorf("not found")
		},
	}.Check(context.Background(), EndpointConfig{Transport: "gemini-cli"})
	check := requireCheck(t, report, "installed", HealthFail)
	if !strings.Contains(check.Action, "install gemini") {
		t.Fatalf("action = %q", check.Action)
	}
	requireCheck(t, report, "auth", HealthUnknown)
	requireCheck(t, report, "model", HealthUnknown)
}

type sequenceCLIRunner struct {
	invocations []cliInvocation
	results     []cliResult
	errs        []error
}

func (r *sequenceCLIRunner) Run(_ context.Context, inv cliInvocation) (cliResult, error) {
	r.invocations = append(r.invocations, inv)
	var result cliResult
	if len(r.results) > 0 {
		result = r.results[0]
		r.results = r.results[1:]
	}
	var err error
	if len(r.errs) > 0 {
		err = r.errs[0]
		r.errs = r.errs[1:]
	}
	return result, err
}
