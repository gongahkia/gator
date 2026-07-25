package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gongahkia/norbot/internal/config"
	"github.com/gongahkia/norbot/internal/domain"
	"github.com/gongahkia/norbot/internal/provider"
	"github.com/gongahkia/norbot/internal/runtime"
)

const agentMaxTurns = 6
const agentMaxToolCalls = 8

type agentPlan struct {
	Final     string          `json:"final"`
	ToolCalls []agentToolCall `json:"tool_calls"`
}
type agentToolCall struct {
	Tool   string         `json:"tool"`
	Params map[string]any `json:"params"`
}

func (s *Service) invokeCentralAgent(ctx context.Context, run domain.Run, input runtime.AgentInvocation) (runtime.AgentResponse, error) {
	providerID := run.Providers[domain.StageBuilder]
	if input.Provider.Kind != "" {
		providerID = ""
	}
	if input.IdempotencyKey == "" || input.SessionID == "" || input.ExternalID == "" {
		return runtime.AgentResponse{}, fmt.Errorf("session_id, external_id, and idempotency_key are required")
	}
	turn, created, err := s.store.CreateAgentTurn(ctx, domain.AgentTurn{ID: mustID(), RunID: run.ID, SessionID: input.SessionID, ExternalID: input.ExternalID, Role: input.Role, IdempotencyKey: input.IdempotencyKey, Prompt: input.Prompt, History: []map[string]any{{"kind": "user", "content": input.Prompt}}, State: "running", ProviderID: providerID})
	if err != nil {
		return runtime.AgentResponse{}, err
	}
	if !created {
		return turnResponse(turn), nil
	}
	return s.advanceAgentTurn(ctx, run, turn)
}

func (s *Service) PendingAgentActions(ctx context.Context, limit int) ([]domain.AgentAction, error) {
	return s.store.PendingAgentActions(ctx, limit)
}
func (s *Service) AgentAction(ctx context.Context, id string) (domain.AgentAction, error) {
	return s.store.AgentAction(ctx, id)
}

func (s *Service) DecideAgentAction(ctx context.Context, id, decision, operator string) (runtime.AgentResponse, domain.AgentAction, error) {
	action, err := s.store.DecideAgentAction(ctx, id, decision, operator)
	if err != nil {
		return runtime.AgentResponse{}, domain.AgentAction{}, err
	}
	turn, err := s.store.AgentTurn(ctx, action.TurnID)
	if err != nil {
		return runtime.AgentResponse{}, domain.AgentAction{}, err
	}
	run, err := s.store.GetRun(ctx, action.RunID)
	if err != nil {
		return runtime.AgentResponse{}, domain.AgentAction{}, err
	}
	if action.State == "rejected" {
		action, err = s.store.CompleteAgentAction(ctx, action.ID, "rejected", map[string]any{"error": "operator rejected action"}, "")
		if err != nil {
			return runtime.AgentResponse{}, domain.AgentAction{}, err
		}
		turn.History = append(turn.History, map[string]any{"kind": "tool", "tool": action.Tool, "action_id": action.ID, "outcome": action.Result})
		turn.State = "running"
		if err := s.store.UpdateAgentTurn(ctx, turn); err != nil {
			return runtime.AgentResponse{}, domain.AgentAction{}, err
		}
		response, err := s.advanceAgentTurn(ctx, run, turn)
		return response, action, err
	}
	action, err = s.executeAgentAction(ctx, run, action)
	if err != nil {
		return runtime.AgentResponse{}, action, err
	}
	turn.History = append(turn.History, map[string]any{"kind": "tool", "tool": action.Tool, "action_id": action.ID, "outcome": action.Result})
	turn.State = "running"
	if err := s.store.UpdateAgentTurn(ctx, turn); err != nil {
		return runtime.AgentResponse{}, domain.AgentAction{}, err
	}
	response, err := s.advanceAgentTurn(ctx, run, turn)
	return response, action, err
}

func (s *Service) advanceAgentTurn(ctx context.Context, run domain.Run, turn domain.AgentTurn) (runtime.AgentResponse, error) {
	providerConfig, ok := s.config.Manifest.Provider(turn.ProviderID, domain.StageBuilder)
	if !ok {
		return runtime.AgentResponse{}, fmt.Errorf("agent provider %q is unavailable", turn.ProviderID)
	}
	workspace, _, err := s.backendForRun(ctx, run)
	if err != nil {
		return runtime.AgentResponse{}, err
	}
	invoker := provider.Invoker{Workspace: workspace, Extensions: s.extensions}
	for count := 0; count < agentMaxTurns; count++ {
		prompt := agentPrompt(turn.History)
		result, err := invoker.Invoke(ctx, providerConfig, provider.Request{RunID: run.ID, Stage: domain.StageBuilder, Prompt: prompt})
		if err != nil {
			turn.State = "failed"
			_ = s.store.UpdateAgentTurn(ctx, turn)
			return runtime.AgentResponse{}, err
		}
		s.recordUsage(ctx, run.ID, domain.StageBuilder, nil, providerConfig.ID, result, "agent")
		plan, err := parseAgentPlan(result.Text)
		if err != nil {
			turn.State = "failed"
			_ = s.store.UpdateAgentTurn(ctx, turn)
			return runtime.AgentResponse{}, err
		}
		if len(plan.ToolCalls) == 0 {
			turn.State = "completed"
			turn.Final = strings.TrimSpace(plan.Final)
			if err := s.store.UpdateAgentTurn(ctx, turn); err != nil {
				return runtime.AgentResponse{}, err
			}
			return turnResponse(turn), nil
		}
		if len(plan.ToolCalls) > agentMaxToolCalls {
			return runtime.AgentResponse{}, fmt.Errorf("agent tool call limit exceeded")
		}
		for _, call := range plan.ToolCalls {
			policy, err := s.validateAgentTool(turn.Role, call)
			if err != nil {
				return runtime.AgentResponse{}, err
			}
			params, err := json.Marshal(call.Params)
			if err != nil {
				return runtime.AgentResponse{}, err
			}
			digest := sha256.Sum256(append([]byte(call.Tool+"\n"+turn.Role+"\n"), params...))
			action := domain.AgentAction{ID: mustID(), TurnID: turn.ID, RunID: run.ID, Tool: call.Tool, Role: turn.Role, Params: call.Params, Digest: "sha256:" + hex.EncodeToString(digest[:]), State: "approved"}
			if policy.ApprovalRequired {
				action.State = "pending"
			}
			action, err = s.store.CreateAgentAction(ctx, action)
			if err != nil {
				return runtime.AgentResponse{}, err
			}
			if action.State == "pending" {
				turn.State = "awaiting_approval"
				turn.History = append(turn.History, map[string]any{"kind": "tool_request", "tool": action.Tool, "action_id": action.ID, "digest": action.Digest})
				if err := s.store.UpdateAgentTurn(ctx, turn); err != nil {
					return runtime.AgentResponse{}, err
				}
				return runtime.AgentResponse{State: "awaiting_approval", Status: "awaiting_approval", Summary: "operator approval required", Diagnostics: map[string]any{"action_id": action.ID, "tool": action.Tool, "digest": action.Digest}}, nil
			}
			action, err = s.executeAgentAction(ctx, run, action)
			if err != nil {
				return runtime.AgentResponse{}, err
			}
			turn.History = append(turn.History, map[string]any{"kind": "tool", "tool": action.Tool, "action_id": action.ID, "outcome": action.Result})
		}
		if err := s.store.UpdateAgentTurn(ctx, turn); err != nil {
			return runtime.AgentResponse{}, err
		}
	}
	turn.State = "failed"
	_ = s.store.UpdateAgentTurn(ctx, turn)
	return runtime.AgentResponse{}, fmt.Errorf("agent turn limit reached")
}

func (s *Service) validateAgentTool(role string, call agentToolCall) (config.ToolPolicy, error) {
	policy, ok := s.config.Manifest.ToolPolicy[call.Tool]
	if !ok || !policy.Enabled {
		return config.ToolPolicy{}, fmt.Errorf("tool %q is not enabled", call.Tool)
	}
	if !stringIn(policy.Roles, role) {
		return config.ToolPolicy{}, fmt.Errorf("role %q cannot invoke %q", role, call.Tool)
	}
	switch call.Tool {
	case "artifact_read", "file_write":
		if path, ok := call.Params["path"].(string); !ok || !safeAgentPath(path) {
			return config.ToolPolicy{}, fmt.Errorf("%s needs a safe relative path", call.Tool)
		}
	case "http_get", "http_write":
		raw, ok := call.Params["url"].(string)
		if !ok {
			return config.ToolPolicy{}, fmt.Errorf("http tool requires url")
		}
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Scheme != "https" || !stringIn(policy.AllowedHosts, parsed.Hostname()) {
			return config.ToolPolicy{}, fmt.Errorf("url must be an allowlisted https host")
		}
		if call.Tool == "http_write" {
			method, _ := call.Params["method"].(string)
			if method != "POST" && method != "PUT" && method != "PATCH" {
				return config.ToolPolicy{}, fmt.Errorf("http_write method must be POST, PUT, or PATCH")
			}
		}
	case "shell":
		command, ok := call.Params["command"].(string)
		if !ok || !stringIn(policy.AllowedCommands, command) {
			return config.ToolPolicy{}, fmt.Errorf("shell command is not allowlisted")
		}
	case "database_mutate":
		statement, ok := call.Params["statement"].(string)
		if !ok || !singleMutation(statement) {
			return config.ToolPolicy{}, fmt.Errorf("database_mutate requires one parameterized INSERT, UPDATE, or DELETE")
		}
	default:
		return config.ToolPolicy{}, fmt.Errorf("unsupported tool %q", call.Tool)
	}
	return policy, nil
}

func (s *Service) executeAgentAction(ctx context.Context, run domain.Run, action domain.AgentAction) (domain.AgentAction, error) {
	executionID := mustID()
	if err := s.store.CreateSandboxExecution(ctx, executionID, action.ID, run.ID, string(run.DeploymentTarget), action.Tool); err != nil {
		return action, err
	}
	result, err := s.executeTool(ctx, run, action)
	state := "completed"
	errorText := ""
	exit := 0
	if err != nil {
		state = "failed"
		errorText = err.Error()
		exit = 1
		result = map[string]any{"error": errorText}
	}
	_ = s.store.CompleteSandboxExecution(ctx, executionID, state, exit, stringValue(result, "output"))
	completed, completeErr := s.store.CompleteAgentAction(ctx, action.ID, state, result, errorText)
	if completeErr != nil {
		return action, completeErr
	}
	if err != nil {
		return completed, err
	}
	return completed, nil
}

func (s *Service) executeTool(ctx context.Context, run domain.Run, action domain.AgentAction) (map[string]any, error) {
	workspace, _, err := s.backendForRun(ctx, run)
	if err != nil {
		return nil, err
	}
	switch action.Tool {
	case "artifact_read":
		data, err := os.ReadFile(filepath.Join(workspace.RunPath(run.ID), action.Params["path"].(string)))
		if err != nil {
			return nil, err
		}
		if len(data) > 1<<20 {
			return nil, fmt.Errorf("artifact exceeds 1 MiB")
		}
		return map[string]any{"content": string(data)}, nil
	case "file_write":
		content, ok := action.Params["content"].(string)
		if !ok {
			return nil, fmt.Errorf("file_write content must be a string")
		}
		path := "agent-output/" + action.ID + "/" + action.Params["path"].(string)
		written, err := workspace.WriteArtifact(run.ID, path, []byte(content))
		if err != nil {
			return nil, err
		}
		return map[string]any{"path": written, "size": len(content)}, nil
	case "http_get":
		return agentHTTP(ctx, action.Params)
	case "http_write":
		return s.sandboxHTTP(ctx, run, action)
	case "shell":
		return s.sandboxShell(ctx, run, action)
	case "database_mutate":
		args := list(action.Params["args"])
		rows, err := s.store.ExecuteAppMutation(ctx, run.ID, action.Params["statement"].(string), args)
		if err != nil {
			return nil, err
		}
		return map[string]any{"rows_affected": rows, "schema": "app_" + strings.ReplaceAll(run.ID, "-", "")}, nil
	}
	return nil, fmt.Errorf("unsupported tool")
}

func (s *Service) sandboxShell(ctx context.Context, run domain.Run, action domain.AgentAction) (map[string]any, error) {
	args := []string{}
	for _, raw := range list(action.Params["args"]) {
		text, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("shell args must be strings")
		}
		args = append(args, text)
	}
	request := runtime.SandboxRequest{Command: append([]string{action.Params["command"].(string)}, args...)}
	result, err := s.runSandbox(ctx, run, request)
	return map[string]any{"output": result.Output, "exit_code": result.ExitCode, "duration_ms": result.DurationMS}, err
}
func (s *Service) sandboxHTTP(ctx context.Context, run domain.Run, action domain.AgentAction) (map[string]any, error) {
	method := action.Params["method"].(string)
	rawURL := action.Params["url"].(string)
	body, _ := action.Params["body"].(string)
	command := []string{"--fail-with-body", "--silent", "--show-error", "--request", method, "--data-raw", body, rawURL}
	request := runtime.SandboxRequest{Image: "curlimages/curl:8.12.1", Command: command, AllowedHosts: []string{mustHost(rawURL)}}
	result, err := s.runSandbox(ctx, run, request)
	return map[string]any{"output": result.Output, "exit_code": result.ExitCode, "duration_ms": result.DurationMS}, err
}
func (s *Service) runSandbox(ctx context.Context, run domain.Run, request runtime.SandboxRequest) (runtime.SandboxResult, error) {
	workspace, _, err := s.backendForRun(ctx, run)
	if err != nil {
		return runtime.SandboxResult{}, err
	}
	executor, ok := workspace.(runtime.SandboxBackend)
	if !ok {
		return runtime.SandboxResult{}, fmt.Errorf("sandbox unavailable for %s", run.DeploymentTarget)
	}
	return executor.RunSandbox(ctx, run.ID, request, s.config.Manifest.Runtime.Sandbox)
}
func agentHTTP(ctx context.Context, params map[string]any) (map[string]any, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, params["url"].(string), nil)
	if err != nil {
		return nil, err
	}
	response, err := (&http.Client{Timeout: 30 * time.Second}).Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	return map[string]any{"status": response.StatusCode, "body": string(raw)}, nil
}
func agentPrompt(history []map[string]any) string {
	encoded, _ := json.Marshal(history)
	return "You are a bounded product agent. Return strict JSON only: {\"final\":string,\"tool_calls\":[{\"tool\":string,\"params\":object}]}. Never invent tools. Consequential actions may require operator approval. Conversation: " + string(encoded)
}
func parseAgentPlan(value string) (agentPlan, error) {
	start := strings.Index(value, "{")
	end := strings.LastIndex(value, "}")
	if start < 0 || end < start {
		return agentPlan{}, fmt.Errorf("provider response must contain agent JSON")
	}
	var plan agentPlan
	if err := json.Unmarshal([]byte(value[start:end+1]), &plan); err != nil {
		return agentPlan{}, fmt.Errorf("decode agent JSON: %w", err)
	}
	if plan.Final == "" && len(plan.ToolCalls) == 0 {
		return agentPlan{}, fmt.Errorf("agent response is empty")
	}
	return plan, nil
}
func turnResponse(turn domain.AgentTurn) runtime.AgentResponse {
	artifacts := []map[string]any{}
	for _, event := range turn.History {
		if event["tool"] != "file_write" {
			continue
		}
		outcome, _ := event["outcome"].(map[string]any)
		path, _ := outcome["path"].(string)
		if path != "" {
			artifacts = append(artifacts, map[string]any{"path": path, "filename": filepath.Base(path)})
		}
	}
	diagnostics := map[string]any{}
	if len(artifacts) > 0 {
		diagnostics["artifacts"] = artifacts
	}
	return runtime.AgentResponse{Final: turn.Final, State: turn.State, Status: turn.State, Summary: turn.Final, Diagnostics: diagnostics}
}
func mustID() string {
	value, err := newID()
	if err == nil {
		return value
	}
	return fmt.Sprintf("generated-%d", time.Now().UnixNano())
}
func stringIn(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
func safeAgentPath(value string) bool {
	return value != "" && !strings.HasPrefix(value, "/") && !strings.Contains(value, "..")
}
func singleMutation(value string) bool {
	normalized := strings.TrimSpace(strings.ToUpper(value))
	return (strings.HasPrefix(normalized, "INSERT ") || strings.HasPrefix(normalized, "UPDATE ") || strings.HasPrefix(normalized, "DELETE ")) && !strings.Contains(strings.TrimSuffix(normalized, ";"), ";")
}
func list(value any) []any       { items, _ := value.([]any); return items }
func mustHost(raw string) string { parsed, _ := url.Parse(raw); return parsed.Hostname() }
func stringValue(value map[string]any, key string) string {
	text, _ := value[key].(string)
	return text
}
