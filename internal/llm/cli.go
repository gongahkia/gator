package llm

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type cliRunner interface {
	Run(ctx context.Context, inv cliInvocation) (cliResult, error)
}

type cliInvocation struct {
	Command string
	Args    []string
	Stdin   string
}

type cliResult struct {
	Stdout string
	Stderr string
}

type cliClient struct {
	command               string
	model                 string
	runner                cliRunner
	includeSchemaInPrompt bool
	useSchemaFile         bool
	build                 func(cliBuildInput) cliInvocation
	parse                 func(cliResult) (string, error)
}

type cliBuildInput struct {
	Request    ChatRequest
	Prompt     string
	SchemaPath string
	Model      string
}

type execCLIRunner struct{}

func (r execCLIRunner) Run(ctx context.Context, inv cliInvocation) (cliResult, error) {
	cmd := exec.CommandContext(ctx, inv.Command, inv.Args...)
	cmd.Stdin = strings.NewReader(inv.Stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	result := cliResult{Stdout: stdout.String(), Stderr: stderr.String()}
	if err != nil {
		return result, fmt.Errorf("%s failed: %w: %s", inv.Command, err, strings.TrimSpace(result.Stderr))
	}
	return result, nil
}

func (c *cliClient) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	runner := c.runner
	if runner == nil {
		runner = execCLIRunner{}
	}
	parse := c.parse
	if parse == nil {
		parse = parseTextStdout
	}
	model := req.Model
	if model == "" {
		model = c.model
	}
	prompt := cliPrompt(req, c.includeSchemaInPrompt)
	run := func(schemaPath string) (*ChatResponse, error) {
		inv := c.build(cliBuildInput{
			Request:    req,
			Prompt:     prompt,
			SchemaPath: schemaPath,
			Model:      model,
		})
		if inv.Command == "" {
			inv.Command = c.command
		}
		result, err := runner.Run(ctx, inv)
		if err != nil {
			return nil, err
		}
		content, err := parse(result)
		if err != nil {
			return nil, err
		}
		return &ChatResponse{
			Content: content,
			Usage:   usageWithEstimate(Usage{}, prompt+strings.Join(inv.Args, " ")+inv.Stdin, content),
		}, nil
	}
	if c.useSchemaFile && req.JSONSchema != nil {
		return withSchemaFile(req.JSONSchema, run)
	}
	return run("")
}

func cliPrompt(req ChatRequest, includeSchema bool) string {
	var b strings.Builder
	for _, msg := range req.Messages {
		b.WriteString(strings.ToUpper(msg.Role))
		b.WriteString(":\n")
		b.WriteString(msg.Content)
		b.WriteString("\n\n")
	}
	if includeSchema && req.JSONSchema != nil {
		b.WriteString("Return only JSON matching this schema:\n")
		b.Write(req.JSONSchema)
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func withSchemaFile(schema []byte, run func(string) (*ChatResponse, error)) (*ChatResponse, error) {
	dir, err := os.MkdirTemp("", "paw-schema-*")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	path := filepath.Join(dir, "schema.json")
	if err := os.WriteFile(path, schema, 0o600); err != nil {
		return nil, err
	}
	return run(path)
}

func parseTextStdout(result cliResult) (string, error) {
	content := strings.TrimSpace(result.Stdout)
	if content == "" {
		return "", fmt.Errorf("cli response has empty stdout")
	}
	return content, nil
}
