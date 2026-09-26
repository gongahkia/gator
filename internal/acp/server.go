// Package acp implements Gator's stdio Agent Client Protocol v1 surface.
//
// ACP is a protocol edge: every prompt is normal Work. It translates ACP
// session and streaming conventions without owning an execution, history, or
// review lifecycle of its own.
package acp

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/artifact"
	"github.com/gongahkia/gator/internal/model"
	"github.com/gongahkia/gator/internal/workrun"
	"github.com/gongahkia/gator/internal/workspace"
)

// New constructs a strict, local ACP server. The supplied Work factory is the
// only execution boundary ACP may use.
func New(config Config) (*Server, error) {
	if config.Input == nil || config.Output == nil {
		return nil, errors.New("ACP input and output are required")
	}
	if strings.TrimSpace(config.RepositoryPath) == "" {
		return nil, errors.New("ACP repository path is required")
	}
	if strings.TrimSpace(config.DefaultProvider) == "" {
		return nil, errors.New("ACP default provider is required")
	}
	if config.NewWorkService == nil {
		return nil, errors.New("ACP Work service factory is required")
	}
	repository, err := workspace.Open(config.RepositoryPath)
	if err != nil {
		return nil, fmt.Errorf("open ACP repository: %w", err)
	}
	config.DefaultVerification = cloneArgv(config.DefaultVerification)
	if strings.TrimSpace(config.AgentVersion) == "" {
		config.AgentVersion = "dev"
	}
	return &Server{
		config:      config,
		repository:  repository.Path(),
		sessions:    make(map[string]*session),
		permissions: make(map[string]permission),
	}, nil
}

// Serve processes ACP messages until stdio closes. Closing input cancels all
// in-flight Work operations before returning.
func (s *Server) Serve(ctx context.Context) error {
	scanner := bufio.NewScanner(s.config.Input)
	scanner.Buffer(make([]byte, 4096), maxFrameBytes)
	for scanner.Scan() {
		s.handleFrame(ctx, append([]byte(nil), scanner.Bytes()...))
	}
	if err := scanner.Err(); err != nil {
		s.shutdown()
		s.wait.Wait()
		return fmt.Errorf("read ACP message: %w", err)
	}
	s.shutdown()
	s.wait.Wait()
	return nil
}

func (s *Server) shutdown() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, session := range s.sessions {
		if session.active != nil {
			session.active.cancel()
		}
	}
}

func (s *Server) handleFrame(ctx context.Context, frame []byte) {
	trimmed := strings.TrimSpace(string(frame))
	if trimmed == "" {
		s.sendError([]byte("null"), -32600, "request must be a JSON-RPC object")
		return
	}
	if strings.HasPrefix(trimmed, "[") {
		s.sendError([]byte("null"), -32600, "ACP batches are not supported; send one JSON-RPC message per line")
		return
	}
	var message inbound
	if err := json.Unmarshal(frame, &message); err != nil {
		s.sendError([]byte("null"), -32700, "request must be valid JSON")
		return
	}
	if message.JSONRPC != "2.0" {
		s.sendError(responseID(message.ID), -32600, "jsonrpc must be \"2.0\"")
		return
	}
	if strings.TrimSpace(message.Method) == "" {
		s.handleResponse(message)
		return
	}
	if err := s.handleRequest(ctx, message); err != nil {
		s.sendError(responseID(message.ID), -32602, err.Error())
	}
}

func (s *Server) handleRequest(ctx context.Context, request inbound) error {
	s.mu.Lock()
	initialized := s.initialized
	s.mu.Unlock()
	if request.Method != "initialize" && !initialized {
		return errors.New("initialize must be called before other ACP methods")
	}

	switch request.Method {
	case "initialize":
		return s.initialize(request)
	case "session/new":
		return s.newSession(request)
	case "session/prompt":
		return s.prompt(ctx, request)
	case "session/cancel":
		return s.cancel(request)
	case "session/set_mode":
		return s.setMode(request)
	case "session/list":
		return s.listSessions(request)
	case "session/load", "session/resume":
		return s.resumeSession(request)
	case "session/close":
		return s.closeSession(request)
	default:
		s.sendError(responseID(request.ID), -32601, "unsupported ACP method "+fmt.Sprintf("%q", request.Method))
		return nil
	}
}

func (s *Server) initialize(request inbound) error {
	var params struct {
		ProtocolVersion int `json:"protocolVersion"`
	}
	if err := decodeParams(request.Params, &params); err != nil {
		return fmt.Errorf("initialize parameters: %w", err)
	}
	if params.ProtocolVersion != protocolVersion {
		s.sendError(responseID(request.ID), -32000, fmt.Sprintf("unsupported ACP protocol version %d; Gator supports v%d", params.ProtocolVersion, protocolVersion))
		return nil
	}
	s.mu.Lock()
	alreadyInitialized := s.initialized
	s.initialized = true
	s.mu.Unlock()
	if alreadyInitialized {
		return errors.New("initialize may only be called once per ACP connection")
	}
	s.sendResult(responseID(request.ID), map[string]any{
		"protocolVersion": protocolVersion,
		"agentCapabilities": map[string]any{
			"loadSession":         true,
			"promptCapabilities":  map[string]any{"image": false, "audio": false, "embeddedContext": false},
			"mcpCapabilities":     map[string]any{"http": false, "sse": false},
			"sessionCapabilities": map[string]any{"list": map[string]any{}, "resume": map[string]any{}, "close": map[string]any{}},
			"auth":                map[string]any{},
		},
		"authMethods": []any{},
		"agentInfo":   map[string]any{"name": "gator", "version": s.config.AgentVersion},
	})
	return nil
}

func (s *Server) newSession(request inbound) error {
	var params struct {
		CWD                   string            `json:"cwd"`
		AdditionalDirectories []string          `json:"additionalDirectories"`
		MCPServers            []json.RawMessage `json:"mcpServers"`
	}
	if err := decodeParams(request.Params, &params); err != nil {
		return fmt.Errorf("session/new parameters: %w", err)
	}
	if !hasField(request.Params, "mcpServers") {
		return errors.New("session/new requires mcpServers (use [] when none are supplied)")
	}
	if err := s.validateCWD(params.CWD); err != nil {
		return err
	}
	if len(params.AdditionalDirectories) != 0 {
		return errors.New("ACP additionalDirectories are not supported by canonical Work")
	}
	if len(params.MCPServers) != 0 {
		return errors.New("client-supplied MCP servers are not accepted; trust project .gator/mcp.json with gator mcp trust")
	}
	provider, modelName, err := s.resolveModel()
	if err != nil {
		return err
	}
	id, err := s.newSessionID()
	if err != nil {
		return err
	}
	session := &session{id: id, cwd: s.repository, provider: provider, model: modelName, verification: cloneArgv(s.config.DefaultVerification), mode: defaultMode(s.config.DefaultVerification), updatedAt: time.Now().UTC()}
	s.mu.Lock()
	s.sessions[id] = session
	s.mu.Unlock()
	s.sendResult(responseID(request.ID), map[string]any{"sessionId": id, "modes": s.modeState(session)})
	return nil
}

func (s *Server) prompt(parent context.Context, request inbound) error {
	var params struct {
		SessionID string            `json:"sessionId"`
		Prompt    []json.RawMessage `json:"prompt"`
	}
	if err := decodeParams(request.Params, &params); err != nil {
		return fmt.Errorf("session/prompt parameters: %w", err)
	}
	task, err := promptText(params.Prompt)
	if err != nil {
		return err
	}
	s.mu.Lock()
	session, found := s.sessions[params.SessionID]
	if !found {
		s.mu.Unlock()
		return fmt.Errorf("unknown ACP session %q", params.SessionID)
	}
	if session.active != nil {
		s.mu.Unlock()
		return fmt.Errorf("ACP session %q already has an active prompt", params.SessionID)
	}
	if session.closing {
		s.mu.Unlock()
		return fmt.Errorf("ACP session %q is closing", params.SessionID)
	}
	ctx, cancel := context.WithCancel(parent)
	activity := &activePrompt{cancel: cancel, messageID: s.nextID("message"), done: make(chan struct{})}
	session.active = activity
	session.updatedAt = time.Now().UTC()
	s.wait.Add(1)
	s.mu.Unlock()

	id := append(json.RawMessage(nil), responseID(request.ID)...)
	go s.executePrompt(ctx, cancel, session.id, task, activity.messageID, id)
	return nil
}

func (s *Server) executePrompt(ctx context.Context, cancel context.CancelFunc, sessionID, task, messageID string, response json.RawMessage) {
	defer s.wait.Done()
	defer cancel()
	defer func() {
		s.mu.Lock()
		if session, found := s.sessions[sessionID]; found && session.active != nil && session.active.messageID == messageID {
			close(session.active.done)
			session.active = nil
			session.updatedAt = time.Now().UTC()
		}
		s.mu.Unlock()
	}()

	s.mu.Lock()
	session, found := s.sessions[sessionID]
	if !found {
		s.mu.Unlock()
		s.sendError(response, -32000, "ACP session was closed before the prompt could start")
		return
	}
	provider, modelName, mode := session.provider, session.model, session.mode
	verification := cloneArgv(session.verification)
	conversation, revision := session.conversation, session.revision
	s.mu.Unlock()

	request := acpWorkRequest(s.repository, task, mode, verification, conversation, revision)
	service, err := s.config.NewWorkService(provider, modelName, &request)
	if err != nil {
		s.finishPrompt(response, ctx, err)
		return
	}
	operation := service.Start(ctx, request)
	s.mu.Lock()
	if session, found := s.sessions[sessionID]; found && session.active != nil && session.active.messageID == messageID {
		session.active.operation = operation
	}
	s.mu.Unlock()
	completion := s.forwardWork(sessionID, messageID, operation)
	if completion.Outcome.ConversationID != "" || completion.Outcome.RevisionID != "" {
		s.mu.Lock()
		if session, found := s.sessions[sessionID]; found {
			if completion.Outcome.ConversationID != "" {
				session.conversation = completion.Outcome.ConversationID
			}
			if completion.Outcome.RevisionID != "" {
				session.revision = completion.Outcome.RevisionID
			}
			session.messages = append([]agent.Message(nil), completion.Outcome.Result.Messages...)
			if session.title == "" {
				session.title = titleFor(task)
			}
		}
		s.mu.Unlock()
	}
	s.finishPrompt(response, ctx, completion.Err)
}

func acpWorkRequest(source, task string, mode action.Mode, verification [][]string, conversation, revision string) workrun.Request {
	request := workrun.Request{SourcePath: source, Objective: task, ConversationID: conversation, ParentRevisionID: revision, MaxSteps: 16}
	if mode == action.Inspect {
		request.Mode = action.Inspect
		request.Contract = artifact.InspectionContract()
		return request
	}
	request.Mode = action.Draft
	request.Contract = artifact.DefaultContract("report.md")
	request.RequireCode = true
	request.Code = workrun.CodePolicy{MaxSteps: 16, Verification: cloneArgv(verification)}
	return request
}

func (s *Server) forwardWork(sessionID, messageID string, operation *workrun.Operation) workrun.Completion {
	events, interactions, completed := operation.Events, operation.Interactions, operation.Done
	var result workrun.Completion
	for events != nil || interactions != nil || completed != nil {
		select {
		case event, ok := <-events:
			if !ok {
				events = nil
				continue
			}
			s.sendEvent(sessionID, messageID, event)
		case interaction, ok := <-interactions:
			if !ok {
				interactions = nil
				continue
			}
			s.requestPermission(sessionID, operation, interaction)
		case value, ok := <-completed:
			if !ok {
				completed = nil
				continue
			}
			result = value
			completed = nil
		}
	}
	return result
}

func (s *Server) finishPrompt(response json.RawMessage, ctx context.Context, err error) {
	if ctx.Err() != nil {
		s.sendResult(response, map[string]any{"stopReason": "cancelled"})
		return
	}
	if err != nil {
		s.sendError(response, -32000, err.Error())
		return
	}
	s.sendResult(response, map[string]any{"stopReason": "end_turn"})
}

func (s *Server) cancel(request inbound) error {
	var params struct{ SessionID string `json:"sessionId"` }
	if err := decodeParams(request.Params, &params); err != nil {
		return fmt.Errorf("session/cancel parameters: %w", err)
	}
	s.mu.Lock()
	session, found := s.sessions[params.SessionID]
	if found && session.active != nil {
		session.active.cancel()
	}
	s.mu.Unlock()
	if !found {
		return fmt.Errorf("unknown ACP session %q", params.SessionID)
	}
	if len(request.ID) > 0 {
		s.sendResult(responseID(request.ID), map[string]any{})
	}
	return nil
}

func (s *Server) setMode(request inbound) error {
	var params struct {
		SessionID string `json:"sessionId"`
		ModeID    string `json:"modeId"`
	}
	if err := decodeParams(request.Params, &params); err != nil {
		return fmt.Errorf("session/set_mode parameters: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	session, found := s.sessions[params.SessionID]
	if !found {
		return fmt.Errorf("unknown ACP session %q", params.SessionID)
	}
	if session.active != nil {
		return errors.New("cannot change ACP session mode while a prompt is active")
	}
	switch params.ModeID {
	case "plan":
		session.mode = action.Inspect
	case "execute":
		if len(session.verification) == 0 {
			return errors.New("execute mode requires a verification policy; start gator agent acp with --verify or use a supported project test runner")
		}
		session.mode = action.Draft
	default:
		return fmt.Errorf("unknown ACP mode %q", params.ModeID)
	}
	session.updatedAt = time.Now().UTC()
	s.sendResult(responseID(request.ID), map[string]any{})
	s.notify(session.id, map[string]any{"sessionUpdate": "current_mode_update", "modeId": params.ModeID})
	return nil
}

func (s *Server) listSessions(request inbound) error {
	var params struct {
		CWD    string `json:"cwd"`
		Cursor string `json:"cursor"`
	}
	if len(request.Params) > 0 {
		if err := decodeParams(request.Params, &params); err != nil {
			return fmt.Errorf("session/list parameters: %w", err)
		}
	}
	if strings.TrimSpace(params.CWD) != "" {
		if err := s.validateCWD(params.CWD); err != nil {
			return err
		}
	}
	if params.Cursor != "" {
		return errors.New("ACP session/list only exposes sessions in this process; cursors are not supported")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sessions := make([]map[string]any, 0, len(s.sessions))
	for _, session := range s.sessions {
		sessions = append(sessions, map[string]any{"sessionId": session.id, "cwd": session.cwd, "title": session.title, "updatedAt": session.updatedAt.UTC().Format(time.RFC3339Nano)})
	}
	s.sendResult(responseID(request.ID), map[string]any{"sessions": sessions})
	return nil
}

func (s *Server) resumeSession(request inbound) error {
	var params struct {
		SessionID             string            `json:"sessionId"`
		CWD                   string            `json:"cwd"`
		AdditionalDirectories []string          `json:"additionalDirectories"`
		MCPServers            []json.RawMessage `json:"mcpServers"`
	}
	if err := decodeParams(request.Params, &params); err != nil {
		return fmt.Errorf("%s parameters: %w", request.Method, err)
	}
	if err := s.validateCWD(params.CWD); err != nil {
		return err
	}
	if len(params.AdditionalDirectories) != 0 {
		return errors.New("ACP additionalDirectories are not supported by canonical Work")
	}
	if len(params.MCPServers) != 0 {
		return errors.New("client-supplied MCP servers are not accepted; trust project .gator/mcp.json with gator mcp trust")
	}
	s.mu.Lock()
	session, found := s.sessions[params.SessionID]
	if found {
		messages := append([]agent.Message(nil), session.messages...)
		mode := s.modeState(session)
		s.mu.Unlock()
		if request.Method == "session/load" {
			s.replaySession(params.SessionID, messages)
			s.sendResult(responseID(request.ID), json.RawMessage("null"))
			return nil
		}
		s.sendResult(responseID(request.ID), map[string]any{"modes": mode})
		return nil
	}
	s.mu.Unlock()
	return fmt.Errorf("unknown ACP session %q; ACP sessions are process-local Work conversation handles", params.SessionID)
}

func (s *Server) replaySession(sessionID string, messages []agent.Message) {
	for _, message := range messages {
		var update string
		switch message.Role {
		case agent.RoleUser:
			update = "user_message_chunk"
		case agent.RoleAgent:
			update = "agent_message_chunk"
		default:
			continue
		}
		if message.Content != "" {
			s.notify(sessionID, map[string]any{"sessionUpdate": update, "messageId": s.nextID("replay"), "content": map[string]any{"type": "text", "text": message.Content}})
		}
		if message.Role != agent.RoleAgent {
			continue
		}
		for _, call := range message.ToolCalls {
			if call.ID == "" || call.Name == "" {
				continue
			}
			s.notify(sessionID, map[string]any{"sessionUpdate": "tool_call", "toolCallId": call.ID, "title": call.Name, "kind": acpToolKind(call.Name), "status": "in_progress"})
		}
	}
	for _, message := range messages {
		if message.Role == agent.RoleTool && message.ToolCallID != "" {
			s.notify(sessionID, map[string]any{"sessionUpdate": "tool_call_update", "toolCallId": message.ToolCallID, "status": "completed"})
		}
	}
}

func (s *Server) closeSession(request inbound) error {
	var params struct{ SessionID string `json:"sessionId"` }
	if err := decodeParams(request.Params, &params); err != nil {
		return fmt.Errorf("session/close parameters: %w", err)
	}
	s.mu.Lock()
	session, found := s.sessions[params.SessionID]
	var finished <-chan struct{}
	if found {
		session.closing = true
		if session.active != nil {
			session.active.cancel()
			finished = session.active.done
		}
	}
	s.mu.Unlock()
	if !found {
		return fmt.Errorf("unknown ACP session %q", params.SessionID)
	}
	if finished != nil {
		<-finished
	}
	s.mu.Lock()
	delete(s.sessions, params.SessionID)
	s.mu.Unlock()
	s.sendResult(responseID(request.ID), map[string]any{})
	return nil
}

func (s *Server) requestPermission(sessionID string, operation *workrun.Operation, interaction workrun.Interaction) {
	id := s.nextID("permission")
	s.mu.Lock()
	s.permissions[id] = permission{operation: operation, interactionID: interaction.ID}
	s.mu.Unlock()
	kind := "other"
	if interaction.Kind == "command" {
		kind = "execute"
	}
	s.send(outbound{JSONRPC: "2.0", ID: json.RawMessage(strconv.Quote(id)), Method: "session/request_permission", Params: map[string]any{
		"sessionId": sessionID,
		"toolCall": map[string]any{"toolCallId": id, "title": "Approve Work " + interaction.Kind, "kind": kind, "status": "pending", "rawInput": interaction.Preview},
		"options": []map[string]any{{"optionId": "allow_once", "name": "Allow once", "kind": "allow_once"}, {"optionId": "allow_always", "name": "Allow once for this Work", "kind": "allow_once"}, {"optionId": "reject_once", "name": "Deny", "kind": "reject_once"}},
	}})
}

func (s *Server) handleResponse(response inbound) {
	if len(response.ID) == 0 {
		return
	}
	var id string
	if json.Unmarshal(response.ID, &id) != nil || id == "" {
		return
	}
	s.mu.Lock()
	pending, found := s.permissions[id]
	if found {
		delete(s.permissions, id)
	}
	s.mu.Unlock()
	if !found {
		return
	}
	approved := false
	if len(response.Error) == 0 {
		var result struct {
			Outcome struct {
				Outcome  string `json:"outcome"`
				OptionID string `json:"optionId"`
			} `json:"outcome"`
		}
		if json.Unmarshal(response.Result, &result) == nil && result.Outcome.Outcome == "selected" {
			approved = result.Outcome.OptionID == "allow_once" || result.Outcome.OptionID == "allow_always"
		}
	}
	_ = pending.operation.Respond(pending.interactionID, approved)
}

func (s *Server) sendEvent(sessionID, messageID string, event agent.Event) {
	switch event.Kind {
	case agent.EventTextDelta, agent.EventText:
		if strings.TrimSpace(event.Text) == "" {
			return
		}
		s.notify(sessionID, map[string]any{"sessionUpdate": "agent_message_chunk", "messageId": messageID, "content": map[string]any{"type": "text", "text": event.Text}})
	case agent.EventToolCalled:
		if event.ToolCall != nil {
			s.notify(sessionID, map[string]any{"sessionUpdate": "tool_call", "toolCallId": event.ToolCall.ID, "title": event.ToolCall.Name, "kind": acpToolKind(event.ToolCall.Name), "status": "in_progress"})
		}
	case agent.EventToolFinished:
		if event.ToolCall != nil {
			status := "completed"
			if event.ToolError != "" {
				status = "failed"
			}
			s.notify(sessionID, map[string]any{"sessionUpdate": "tool_call_update", "toolCallId": event.ToolCall.ID, "status": status})
		}
	case agent.EventSubagent, agent.EventTerminal, agent.EventHook, agent.EventContextCompacted, agent.EventSteeringApplied, agent.EventWorktreeSetup:
		s.sendLifecycleEvent(sessionID, event)
	}
}

func (s *Server) sendLifecycleEvent(sessionID string, event agent.Event) {
	if strings.TrimSpace(event.Text) == "" {
		return
	}
	kind, title := lifecycleEventTitle(event)
	callID := s.nextID(kind)
	s.notify(sessionID, map[string]any{"sessionUpdate": "tool_call", "toolCallId": callID, "title": title, "kind": "other", "status": "in_progress"})
	s.notify(sessionID, map[string]any{"sessionUpdate": "tool_call_update", "toolCallId": callID, "status": "completed"})
}

func lifecycleEventTitle(event agent.Event) (string, string) {
	prefix, kind := "Gator", "lifecycle"
	switch event.Kind {
	case agent.EventSubagent:
		prefix, kind = "Subagent", "subagent"
	case agent.EventTerminal:
		prefix, kind = "Terminal", "terminal"
	case agent.EventHook:
		prefix, kind = "Hook", "hook"
	case agent.EventContextCompacted:
		prefix, kind = "Context", "context"
	case agent.EventSteeringApplied:
		prefix, kind = "Steering", "steering"
	case agent.EventWorktreeSetup:
		prefix, kind = "Work setup", "work-setup"
	}
	text := strings.Join(strings.Fields(event.Text), " ")
	if len([]rune(text)) > 768 {
		text = string([]rune(text)[:767]) + "…"
	}
	return kind, prefix + ": " + text
}

func (s *Server) notify(sessionID string, update map[string]any) {
	s.send(outbound{JSONRPC: "2.0", Method: "session/update", Params: map[string]any{"sessionId": sessionID, "update": update}})
}

func (s *Server) modeState(session *session) map[string]any {
	modes := []map[string]any{{"id": "plan", "name": "Plan", "description": "Read-only Work inspection."}}
	if len(session.verification) > 0 {
		modes = append(modes, map[string]any{"id": "execute", "name": "Execute", "description": "Draft a verified Code candidate for review."})
	}
	current := "plan"
	if session.mode == action.Draft {
		current = "execute"
	}
	return map[string]any{"currentModeId": current, "availableModes": modes}
}

func (s *Server) validateCWD(cwd string) error {
	if !filepath.IsAbs(cwd) {
		return errors.New("ACP cwd must be an absolute path")
	}
	root, err := workspace.Open(cwd)
	if err != nil {
		return fmt.Errorf("open ACP cwd: %w", err)
	}
	if root.Path() != s.repository {
		return fmt.Errorf("ACP cwd %q is outside the repository this Gator process was started for", cwd)
	}
	return nil
}

func (s *Server) resolveModel() (string, string, error) {
	if s.config.ResolveProvider != nil {
		return s.config.ResolveProvider(s.config.DefaultProvider, s.config.DefaultModel)
	}
	provider, err := model.ParseProvider(s.config.DefaultProvider)
	if err != nil {
		return "", "", err
	}
	return string(provider), model.EffectiveModel(provider, s.config.DefaultModel), nil
}

func (s *Server) newSessionID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate ACP session ID: %w", err)
	}
	return "acp-" + hex.EncodeToString(value[:]), nil
}

func (s *Server) nextID(prefix string) string { return fmt.Sprintf("gator-%s-%d", prefix, s.next.Add(1)) }

func (s *Server) sendResult(id json.RawMessage, result any) {
	if len(id) != 0 {
		s.send(outbound{JSONRPC: "2.0", ID: id, Result: result})
	}
}

func (s *Server) sendError(id json.RawMessage, code int, message string) {
	if len(id) != 0 {
		s.send(outbound{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: message}})
	}
}

func (s *Server) send(message outbound) {
	s.write.Lock()
	defer s.write.Unlock()
	_ = json.NewEncoder(s.config.Output).Encode(message)
}
