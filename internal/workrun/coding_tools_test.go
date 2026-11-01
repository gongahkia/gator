package workrun

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/codeexec"
	"github.com/gongahkia/gator/internal/workspace"
)

type codingTestTool struct{ calls *int }

func (codingTestTool) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{Name: "read_file", Parameters: json.RawMessage(`{"type":"object"}`)}
}
func (t codingTestTool) Execute(context.Context, json.RawMessage) (agent.ToolResult, error) {
	*t.calls++
	return agent.ToolResult{Content: `{"ok":true}`}, nil
}

func TestDirectCodingIsLazyBoundedAndClosedWithItsOwner(t *testing.T) {
	work, err := workspace.CreateWork(t.TempDir(), t.TempDir(), "direct-lazy", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	request := Request{RunID: "direct-lazy", Objective: "Inspect code", Contract: artifact.InspectionContract()}
	request.integration = &codeIntegration{request: request, work: work, patches: map[string]string{}, accepted: map[string]string{}}
	opens, calls := 0, 0
	c := newConversationCode(func(context.Context, CodeRequest) (*CodeSession, error) {
		opens++
		return &CodeSession{Harness: &codeexec.Prepared{Tools: []agent.Tool{codingTestTool{&calls}}, MaxSteps: 1}, Finish: func(context.Context, agent.Result) (CodeResult, error) { return CodeResult{}, nil }}, nil
	}, request, work, nil)
	surface := c.Tools()
	if opens != 0 {
		t.Fatal("ordinary chat eagerly allocated coding tools")
	}
	var read, patch, integration agent.Tool
	for _, tool := range surface {
		switch tool.Definition().Name {
		case "code_read_file":
			read = tool
		case "code_apply_patch":
			patch = tool
		case "code_use_integration":
			integration = tool
		}
	}
	if _, err := patch.Execute(context.Background(), json.RawMessage(`{}`)); err == nil {
		t.Fatal("omitted patch operation was admitted")
	}
	if opens != 1 || calls != 0 {
		t.Fatalf("open/dispatch=%d/%d", opens, calls)
	}
	if _, err := integration.Execute(context.Background(), json.RawMessage(`{"tool":"run_shell","arguments":{}}`)); err == nil {
		t.Fatal("unknown integration was admitted")
	}
	if _, err := read.Execute(context.Background(), json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := read.Execute(context.Background(), json.RawMessage(`{}`)); err == nil {
		t.Fatal("profile operation limit was bypassed")
	}
	if calls != 1 {
		t.Fatalf("dispatch count=%d", calls)
	}
	c.Close()
	c.Close()
	if _, err := read.Execute(context.Background(), json.RawMessage(`{}`)); err == nil {
		t.Fatal("closed owner accepted a tool call")
	}
}

func TestDirectCodingRechecksTrustedToolHook(t *testing.T) {
	calls := 0
	c := &conversationCode{session: &CodeSession{Harness: &codeexec.Prepared{Tools: []agent.Tool{codingTestTool{&calls}}, BeforeTool: func(context.Context, agent.ToolCall) error { return errors.New("host policy denied") }}}}
	tool := conversationCodeTool{owner: c, native: "read_file"}
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{}`)); err == nil || calls != 0 {
		t.Fatalf("policy denial bypassed: %v count=%d", err, calls)
	}
}
