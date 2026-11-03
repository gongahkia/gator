package tools

import (
	"context"
	"encoding/json"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/attachment"
)

// ListInputFiles exposes metadata for explicitly selected originals, never the
// private upload store or raw binary content. References are not file mounts.
type ListInputFiles struct{ Files []attachment.OriginalMetadata }

func (t ListInputFiles) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{Name: "list_input_files", Description: "List this task's selected original files with exact references, byte sizes and hashes. Use input/... references for reviewed connector attachments, including original Office documents. These references expose no other upload and are not read_file paths. Original binary bytes are never returned by this tool.", Parameters: schema(`{"type":"object","properties":{},"additionalProperties":false}`)}
}

func (t ListInputFiles) Execute(_ context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	if err := decodeArguments(raw, &struct{}{}); err != nil {
		return agent.ToolResult{}, err
	}
	content, err := success(struct {
		Files []attachment.OriginalMetadata `json:"files"`
	}{Files: t.Files})
	return agent.ToolResult{Content: content}, err
}
