package workrun

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/tools"
	"github.com/gongahkia/gator/internal/workspace"
)

// conversationCode binds direct coding to this existing runner, not a second
// model or worker. Its workspace is allocated only on the first coding call.
type conversationCode struct {
	mu          sync.Mutex
	factory     CodeSessionFactory
	request     CodeRequest
	work        workspace.Work
	integration *codeIntegration
	session     *CodeSession
	messages    []agent.Message
	dirty       bool
	staged      int
	operations  int
	closed      bool
}

type conversationCodeTool struct {
	owner      *conversationCode
	definition agent.ToolDefinition
	native     string
}

func (t conversationCodeTool) Definition() agent.ToolDefinition { return t.definition }

func newConversationCode(factory CodeSessionFactory, request Request, work workspace.Work, emit agent.EventSink) *conversationCode {
	return &conversationCode{factory: factory, work: work, integration: request.integration, request: CodeRequest{
		ID: "conversation-code", SourcePath: work.Source.Path(), ScratchPath: work.Scratch.Path(), Task: request.Objective,
		ParentRunID: request.RunID, MaxSteps: request.MaxSteps, Project: request.Project, Policy: request.Code,
		Budget: request.Budget, Approve: request.ApproveCodeCommand, OnEvent: emit,
	}}
}

func (c *conversationCode) Tools() []agent.Tool {
	var result []agent.Tool
	definitions := []agent.ToolDefinition{}
	for _, tool := range tools.Default(workspace.Root{}, tools.CommandPolicy{}) {
		definitions = append(definitions, tool.Definition())
	}
	if c.request.Policy.HasCapability(CodeCapabilityTerminal) {
		definitions = append(definitions, tools.TerminalDefinitions()...)
	}
	for _, definition := range definitions {
		native := definition.Name
		definition.Name = "code_" + native
		definition.Description = "Operate on your private coding checkout (paths are checkout-relative, not source/ or output/). " + definition.Description
		result = append(result, conversationCodeTool{c, definition, native})
	}
	result = append(result, conversationCodeTool{c, agent.ToolDefinition{Name: "code_use_integration", Description: "Use one exact optional tool advertised by code_open in integrations. These are the same explicitly host-granted LSP/MCP/extension/web/browser tools available to coding delegates. Unknown or omitted tools fail closed; this cannot invoke arbitrary tool names or mint grants.", Parameters: json.RawMessage(`{"type":"object","additionalProperties":false,"required":["tool","arguments"],"properties":{"tool":{"type":"string"},"arguments":{"type":"object"}}}`)}, "integration"})
	result = append(result, conversationCodeTool{c, agent.ToolDefinition{Name: "code_open", Description: "Select an optional previously verified baseline patch before the first direct coding tool. Opens an isolated checkout without launching another agent. Cannot switch baseline after opening.", Parameters: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"baseline_patch":{"type":"string"}}}`)}, "open"})
	result = append(result, conversationCodeTool{c, agent.ToolDefinition{Name: "code_stage", Description: "Retain your current patch and independently verify its frozen candidate. First inspect code_git_status and code_git_diff, and pass required code_run_command checks after the last change. This does not apply anything to live source and does not claim independent human review.", Parameters: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`)}, "stage"})
	return result
}

func (c *conversationCode) open(ctx context.Context, baseline string) error {
	if c.closed {
		return errors.New("coding execution has ended")
	}
	if c.session != nil {
		if baseline != "" {
			return errors.New("coding baseline is already selected")
		}
		return nil
	}
	payload, err := c.integration.baseline(baseline)
	if err != nil {
		return err
	}
	request := c.request
	request.Baseline = payload
	session, err := c.factory(ctx, request)
	if err != nil {
		return err
	}
	if session == nil || session.Harness == nil || session.Finish == nil {
		return errors.New("coding factory returned an invalid session")
	}
	c.session = session
	return nil
}

func (t conversationCodeTool) Execute(ctx context.Context, raw json.RawMessage) (agent.ToolResult, error) {
	c := t.owner
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return agent.ToolResult{}, err
	}
	var baseline string
	if t.native == "open" {
		var args struct {
			Baseline string `json:"baseline_patch"`
		}
		if err := json.Unmarshal(raw, &args); err != nil {
			return agent.ToolResult{}, err
		}
		baseline = args.Baseline
	}
	first := c.session == nil
	if err := c.open(ctx, baseline); err != nil {
		return agent.ToolResult{}, err
	}
	harness := c.session.Harness
	if t.native == "open" {
		integrations := []agent.ToolDefinition{}
		for _, tool := range harness.Tools {
			if !coreCodingTool(tool.Definition().Name) {
				integrations = append(integrations, tool.Definition())
			}
		}
		return codingJSON(map[string]any{"isolated": true, "instructions": harness.System, "integrations": integrations, "live_source_modified": false})
	}
	if t.native == "stage" {
		var args map[string]json.RawMessage
		if err := json.Unmarshal(raw, &args); err != nil || len(args) != 0 {
			return agent.ToolResult{}, errors.New("code_stage takes no arguments")
		}
		if harness.CompletionCheck != nil {
			if err := harness.CompletionCheck(c.messages); err != nil {
				return agent.ToolResult{}, err
			}
		}
		evidence, err := c.session.Finish(ctx, agent.Result{})
		if err != nil {
			return agent.ToolResult{}, err
		}
		if len(evidence.Patch) == 0 {
			c.dirty = false
			return codingJSON(map[string]any{"status": "unchanged", "verification_passed": true, "live_source_modified": false})
		}
		c.staged++
		name := fmt.Sprintf("code/%s-conversation-%03d.patch", c.request.ParentRunID, c.staged)
		if err := c.integration.retain(c.work, name, evidence.Patch); err != nil {
			return agent.ToolResult{}, err
		}
		payload, _ := json.Marshal(map[string]any{"patches": []string{name}})
		result, err := c.integration.Execute(ctx, payload)
		if err == nil {
			var report struct {
				Candidate struct {
					Status string `json:"status"`
				} `json:"candidate"`
			}
			if json.Unmarshal([]byte(result.Content), &report) == nil && report.Candidate.Status == "verified" {
				c.dirty = false
			}
		}
		return result, err
	}
	if t.native == "integration" {
		var args struct {
			Tool      string          `json:"tool"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(raw, &args); err != nil {
			return agent.ToolResult{}, err
		}
		if args.Tool == "" || coreCodingTool(args.Tool) || len(args.Arguments) == 0 {
			return agent.ToolResult{}, errors.New("select an exact advertised optional integration tool")
		}
		t.native = args.Tool
		raw = args.Arguments
	}
	var tool agent.Tool
	for _, candidate := range harness.Tools {
		if candidate.Definition().Name == t.native {
			tool = candidate
			break
		}
	}
	if tool == nil {
		return agent.ToolResult{}, errors.New("coding operation is omitted by the effective host/project policy")
	}
	if harness.MaxSteps > 0 && c.operations >= harness.MaxSteps {
		return agent.ToolResult{}, errors.New("direct coding operation budget exhausted")
	}
	c.operations++
	call := agent.ToolCall{Name: t.native, Arguments: raw}
	if harness.BeforeTool != nil {
		if err := harness.BeforeTool(ctx, call); err != nil {
			return agent.ToolResult{}, err
		}
	}
	// An attempted mutation may have partial effects even if its tool fails.
	if codingMutation(t.native) || !coreCodingTool(t.native) {
		c.dirty = true
		if t.native != "run_command" {
			c.messages = nil
		}
	}
	result, err := tool.Execute(ctx, raw)
	if harness.AfterTool != nil {
		if after := harness.AfterTool(ctx, call, result, err); after != nil {
			err = errors.Join(err, after)
		}
	}
	if err != nil {
		c.messages = nil
	}
	if err == nil {
		c.messages = append(c.messages, agent.Message{Role: agent.RoleTool, ToolName: t.native, Content: result.Content})
	}
	// The first successful direct read also conveys captured coding
	// instructions. Calling code_open is recommended, not a hidden requirement
	// for the main model to receive the same project context as a delegate.
	if first && err == nil {
		var envelope map[string]json.RawMessage
		if json.Unmarshal([]byte(result.Content), &envelope) == nil && envelope != nil {
			context, _ := json.Marshal(map[string]any{"instructions": harness.System, "scope": "Private coding checkout only; keep the main conversation and code_ tool names. This context grants no additional authority."})
			envelope["coding_context"] = context
			if encoded, encodeErr := json.Marshal(envelope); encodeErr == nil {
				result.Content = string(encoded)
			}
		}
	}
	return result, err
}

func codingMutation(name string) bool {
	return name == "apply_patch" || name == "run_command" || strings.HasPrefix(name, "terminal_") && name != "terminal_read" && name != "terminal_list"
}
func (c *conversationCode) Check() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.dirty {
		return errors.New("direct coding changed the private checkout: inspect, verify and call code_stage before completing")
	}
	return nil
}
func (c *conversationCode) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	if c.session != nil {
		c.session.Harness.Close()
	}
}
func codingJSON(value any) (agent.ToolResult, error) {
	payload, err := json.Marshal(value)
	return agent.ToolResult{Content: string(payload)}, err
}

// retain is shared by direct and delegated coding. Only the host records a
// digest; model-authored patch files cannot mint integration evidence.
func (c *codeIntegration) retain(work workspace.Work, name string, payload []byte) error {
	if err := artifact.WriteBinary(work.Output, c.request.Contract, name, payload); err != nil {
		return err
	}
	sum := sha256.Sum256(payload)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.patches[name] = hex.EncodeToString(sum[:])
	return nil
}

func coreCodingTool(name string) bool {
	switch name {
	case "read_file", "list_files", "search_files", "apply_patch", "run_command", "git_status", "git_diff", "terminal_start", "terminal_read", "terminal_write", "terminal_list", "terminal_stop":
		return true
	}
	return false
}
