package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/workspace"
)

// WorkFiles returns the narrow local surface for a non-code work run. Source
// is readable through source/...; output is independently readable through
// output/.... Only the dedicated writer can change staged output.
func WorkFiles(source, output workspace.Root, contract artifact.Contract, writable bool) ([]agent.Tool, error) {
	contract = contract.Normalize()
	if err := contract.Validate(); err != nil {
		return nil, fmt.Errorf("validate artifact contract: %w", err)
	}
	named := []workspace.NamedRoot{
		{Name: "source", Root: source},
		{Name: "output", Root: output},
	}
	if _, err := workspace.NewNamedRootSet(output, nil, named); err != nil {
		return nil, err
	}
	result := []agent.Tool{
		ReadFile{Root: output, NamedRoots: named},
		ListFiles{Root: output, NamedRoots: named},
		SearchFiles{Root: output, NamedRoots: named},
	}
	if writable {
		result = append(result,
			WriteArtifact{Root: output, Contract: contract},
			ArtifactStatus{Root: output, Contract: contract},
		)
	}
	return result, nil
}

// WriteArtifact stages one text deliverable under the isolated output root.
type WriteArtifact struct {
	Root     workspace.Root
	Contract artifact.Contract
}

func (t WriteArtifact) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{
		Name:        "write_artifact",
		Description: "Create or replace one UTF-8 deliverable in the isolated output workspace. The path is output-relative; this tool cannot modify source files.",
		Parameters:  schema(`{"type":"object","additionalProperties":false,"required":["path","content"],"properties":{"path":{"type":"string","minLength":1,"description":"Portable output-relative artifact path, for example report.md or data/results.csv"},"content":{"type":"string","description":"Complete UTF-8 artifact contents"}}}`),
	}
}

func (t WriteArtifact) Execute(_ context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	var arguments struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := decodeArguments(raw, &arguments); err != nil {
		return agent.ToolResult{}, err
	}
	if err := artifact.WriteText(t.Root, t.Contract, arguments.Path, arguments.Content); err != nil {
		return agent.ToolResult{}, err
	}
	content, err := success(struct {
		Path  string `json:"path"`
		Bytes int    `json:"bytes"`
	}{Path: arguments.Path, Bytes: len(arguments.Content)})
	if err != nil {
		return agent.ToolResult{}, err
	}
	return agent.ToolResult{Content: content}, nil
}

// ArtifactStatus evaluates staged output using trusted deterministic
// validators. A model can inspect evidence but cannot set the pass result.
type ArtifactStatus struct {
	Root     workspace.Root
	Contract artifact.Contract
}

func (t ArtifactStatus) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{
		Name:        "artifact_status",
		Description: "Inspect all staged output and run the developer-owned outcome contract. Completion is allowed only when the returned passed field is true.",
		Parameters:  schema(`{"type":"object","additionalProperties":false}`),
	}
}

func (t ArtifactStatus) Execute(_ context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	if err := decodeArguments(raw, &struct{}{}); err != nil {
		return agent.ToolResult{}, err
	}
	inspection, err := artifact.Inspect(t.Root, t.Contract)
	if err != nil {
		return agent.ToolResult{}, err
	}
	content, err := success(inspection)
	if err != nil {
		return agent.ToolResult{}, err
	}
	return agent.ToolResult{Content: content}, nil
}
