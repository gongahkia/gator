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
			WriteJSONArtifact{Root: output, Contract: contract},
			WriteTableArtifact{Root: output, Contract: contract},
			WriteDocumentArtifact{Root: output, Contract: contract},
			WriteWorkbookArtifact{Root: output, Contract: contract},
			ArtifactStatus{Root: output, Contract: contract},
		)
	}
	return result, nil
}

// WriteJSONArtifact stages one syntactically valid, consistently formatted
// JSON deliverable without asking the model to escape an entire file string.
type WriteJSONArtifact struct {
	Root     workspace.Root
	Contract artifact.Contract
}

func (t WriteJSONArtifact) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{
		Name:        "write_json_artifact",
		Description: "Create or replace a formatted JSON deliverable in isolated output. The value is validated as exactly one JSON value before the file changes.",
		Parameters:  schema(`{"type":"object","additionalProperties":false,"required":["path","value"],"properties":{"path":{"type":"string","minLength":1,"description":"Portable output-relative .json artifact path"},"value":{}}}`),
	}
}

func (t WriteJSONArtifact) Execute(_ context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	var arguments struct {
		Path  string          `json:"path"`
		Value json.RawMessage `json:"value"`
	}
	if err := decodeArguments(raw, &arguments); err != nil {
		return agent.ToolResult{}, err
	}
	if len(arguments.Value) == 0 {
		return agent.ToolResult{}, fmt.Errorf("JSON artifact value is required")
	}
	if err := artifact.WriteJSON(t.Root, t.Contract, arguments.Path, arguments.Value); err != nil {
		return agent.ToolResult{}, err
	}
	content, err := success(struct {
		Path string `json:"path"`
	}{Path: arguments.Path})
	if err != nil {
		return agent.ToolResult{}, err
	}
	return agent.ToolResult{Content: content}, nil
}

// WriteTableArtifact turns bounded rectangular values into valid CSV.
type WriteTableArtifact struct {
	Root     workspace.Root
	Contract artifact.Contract
}

func (t WriteTableArtifact) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{
		Name:        "write_table_artifact",
		Description: "Create or replace a rectangular CSV deliverable in isolated output. Gator handles quoting and rejects duplicate headers or ragged rows.",
		Parameters: schema(`{"type":"object","additionalProperties":false,"required":["path","headers","rows"],"properties":{
"path":{"type":"string","minLength":1,"description":"Portable output-relative .csv artifact path"},
"headers":{"type":"array","minItems":1,"maxItems":256,"items":{"type":"string","maxLength":65536}},
"rows":{"type":"array","maxItems":10000,"items":{"type":"array","maxItems":256,"items":{"type":"string","maxLength":65536}}}
}}`),
	}
}

func (t WriteTableArtifact) Execute(_ context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	var arguments struct {
		Path    string     `json:"path"`
		Headers []string   `json:"headers"`
		Rows    [][]string `json:"rows"`
	}
	if err := decodeArguments(raw, &arguments); err != nil {
		return agent.ToolResult{}, err
	}
	if err := artifact.WriteTable(t.Root, t.Contract, arguments.Path, arguments.Headers, arguments.Rows); err != nil {
		return agent.ToolResult{}, err
	}
	content, err := success(struct {
		Path    string `json:"path"`
		Rows    int    `json:"rows"`
		Columns int    `json:"columns"`
	}{Path: arguments.Path, Rows: len(arguments.Rows), Columns: len(arguments.Headers)})
	if err != nil {
		return agent.ToolResult{}, err
	}
	return agent.ToolResult{Content: content}, nil
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
