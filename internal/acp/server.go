// Package acp implements Gator's stdio Agent Client Protocol v1 surface.
//
// It deliberately shares Gator's native executor rather than adapting the
// older JSONL RPC protocol: ACP clients get a normal JSON-RPC 2.0 lifecycle,
// while Gator retains its isolated worktree, verifier, and approval policy.
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

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/journal"
	"github.com/gongahkia/gator/internal/model"
	gatorrun "github.com/gongahkia/gator/internal/run"
	"github.com/gongahkia/gator/internal/tools"
	"github.com/gongahkia/gator/internal/workspace"
)

// New constructs a strict, local ACP server.
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
	if config.NewExecutor == nil {
		return nil, errors.New("ACP executor factory is required")
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
		permissions: make(map[string]chan tools.CommandDecision),
	}, nil
}

// Serve processes ACP messages until stdio closes. A closed input cancels all
// active prompts before the server waits for their executor cleanup.
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
		// ACP follows JSON-RPC batch semantics, but a prompt can issue a
		// permission request before it completes. Supporting a batch containing
		// such a prompt would require holding unrelated responses indefinitely.
		// Local editor ACP clients issue one frame per message, so fail explicitly
		// instead of emitting a non-conformant partial batch response.
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
			"sessionCapabilities": map[string]any{"list": map[string]any{}, "resume": map[string]any{}, "close": map[string]any{}, "additionalDirectories": map[string]any{}},
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
	additional, err := s.validateAdditionalDirectories(params.AdditionalDirectories)
	if err != nil {
		return err
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
	session := &session{
		id:           id,
		cwd:          s.repository,
		additional:   append([]workspace.Root(nil), additional...),
		provider:     provider,
		model:        modelName,
		verification: cloneArgv(s.config.DefaultVerification),
		mode:         defaultMode(s.config.DefaultVerification),
		updatedAt:    time.Now().UTC(),
	}
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
	additional := append([]workspace.Root(nil), session.additional...)
	statePath := session.statePath
	s.mu.Unlock()

	executor, err := s.config.NewExecutor(provider, modelName, s.config.DefaultBaseURL)
	if err != nil {
		s.finishPrompt(response, ctx, err)
		return
	}
	request := gatorrun.Request{
		RepositoryPath:          s.repository,
		Task:                    task,
		Provider:                provider,
		Model:                   modelName,
		BaseURL:                 s.config.DefaultBaseURL,
		Verification:            verification,
		AdditionalReadOnlyRoots: additional,
		StateDir:                s.config.StateDir,
		ThreadID:                sessionID,
		Mode:                    mode,
		OnEvent: func(event agent.Event) {
			s.sendEvent(sessionID, messageID, event)
		},
		Approve: func(approvalContext context.Context, argv []string) (tools.CommandDecision, error) {
			return s.requestPermission(approvalContext, sessionID, argv)
		},
	}
	var outcome gatorrun.Outcome
	if statePath == "" {
		outcome, err = executor.Execute(ctx, request)
	} else {
		previous, loadErr := journal.LoadSession(statePath)
		if loadErr != nil {
			s.finishPrompt(response, ctx, fmt.Errorf("load retained ACP session: %w", loadErr))
			return
		}
		outcome, err = executor.Resume(ctx, previous, statePath, task, request)
	}
	if err == nil {
		s.mu.Lock()
		if session, found := s.sessions[sessionID]; found {
			session.statePath = outcome.StatePath
			session.messages = append([]agent.Message(nil), outcome.Result.Messages...)
			if session.title == "" {
				session.title = titleFor(task)
			}
		}
		s.mu.Unlock()
	}
	s.finishPrompt(response, ctx, err)
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
	var params struct {
		SessionID string `json:"sessionId"`
	}
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
		session.mode = gatorrun.PlanMode
	case "execute":
		if len(session.verification) == 0 {
			return errors.New("execute mode requires a verification policy; start gator acp with --verify or use a supported project test runner")
		}
		if session.statePath != "" {
			return errors.New("cannot change a retained ACP session from plan to execute; start a new session with a verification policy")
		}
		session.mode = gatorrun.ExecuteMode
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
	threads, err := journal.ListRecentThreads(s.config.StateDir, s.repository, 200)
	if err != nil {
		return fmt.Errorf("list retained ACP sessions: %w", err)
	}
	start := 0
	if strings.TrimSpace(params.Cursor) != "" {
		start, err = strconv.Atoi(params.Cursor)
		if err != nil || start < 0 || start > len(threads) {
			return errors.New("session/list cursor is invalid")
		}
	}
	end := min(start+100, len(threads))
	sessions := make([]map[string]any, 0, end-start)
	for _, thread := range threads[start:end] {
		sessions = append(sessions, map[string]any{
			"sessionId": thread.ID,
			"cwd":       s.repository,
			"title":     titleFor(thread.Task),
			"updatedAt": thread.UpdatedAt.UTC().Format(time.RFC3339Nano),
		})
	}
	result := map[string]any{"sessions": sessions}
	if end < len(threads) {
		result["nextCursor"] = strconv.Itoa(end)
	}
	s.sendResult(responseID(request.ID), result)
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
	if request.Method == "session/load" && !hasField(request.Params, "mcpServers") {
		return errors.New("session/load requires mcpServers (use [] when none are supplied)")
	}
	additional, err := s.validateAdditionalDirectories(params.AdditionalDirectories)
	if err != nil {
		return err
	}
	if len(params.MCPServers) != 0 {
		return errors.New("client-supplied MCP servers are not accepted; trust project .gator/mcp.json with gator mcp trust")
	}
	s.mu.Lock()
	if existing, found := s.sessions[params.SessionID]; found {
		existing.additional = append([]workspace.Root(nil), additional...)
		mode := s.modeState(existing)
		messages := append([]agent.Message(nil), existing.messages...)
		statePath := existing.statePath
		s.mu.Unlock()
		if request.Method == "session/load" && len(messages) == 0 && statePath != "" {
			previous, loadErr := journal.LoadSession(statePath)
			if loadErr != nil {
				return fmt.Errorf("load retained ACP session %q: %w", params.SessionID, loadErr)
			}
			messages = previous.Messages
		}
		if request.Method == "session/load" {
			s.replaySession(params.SessionID, messages)
			s.sendResult(responseID(request.ID), json.RawMessage("null"))
			return nil
		}
		s.sendResult(responseID(request.ID), map[string]any{"modes": mode})
		return nil
	}
	s.mu.Unlock()

	thread, err := journal.LoadThread(s.config.StateDir, s.repository, params.SessionID)
	if err != nil {
		return fmt.Errorf("load retained ACP session %q: %w", params.SessionID, err)
	}
	previous, err := journal.LoadSession(thread.HeadStatePath)
	if err != nil {
		return fmt.Errorf("load retained ACP session %q: %w", params.SessionID, err)
	}
	mode := gatorrun.ExecuteMode
	if previous.Mode == gatorrun.PlanMode.String() {
		mode = gatorrun.PlanMode
	}
	session := &session{
		id:           params.SessionID,
		cwd:          s.repository,
		additional:   append([]workspace.Root(nil), additional...),
		provider:     previous.Provider,
		model:        previous.Model,
		verification: cloneArgv(previous.Verification),
		mode:         mode,
		title:        titleFor(thread.Task),
		statePath:    thread.HeadStatePath,
		messages:     append([]agent.Message(nil), previous.Messages...),
		updatedAt:    time.Now().UTC(),
	}
	s.mu.Lock()
	s.sessions[session.id] = session
	s.mu.Unlock()
	if request.Method == "session/load" {
		s.replaySession(session.id, session.messages)
		s.sendResult(responseID(request.ID), json.RawMessage("null"))
		return nil
	}
	s.sendResult(responseID(request.ID), map[string]any{"modes": s.modeState(session)})
	return nil
}

// replaySession projects retained conversation entries into ACP session
// updates before session/load replies. It keeps tool results out of message
// text while preserving tool-call progress as structured updates.
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
			s.notify(sessionID, map[string]any{
				"sessionUpdate": update,
				"messageId":     s.nextID("replay"),
				"content":       map[string]any{"type": "text", "text": message.Content},
			})
		}
		if message.Role != agent.RoleAgent {
			continue
		}
		for _, call := range message.ToolCalls {
			if call.ID == "" || call.Name == "" {
				continue
			}
			s.notify(sessionID, map[string]any{
				"sessionUpdate": "tool_call",
				"toolCallId":    call.ID,
				"title":         call.Name,
				"kind":          acpToolKind(call.Name),
				"status":        "in_progress",
			})
		}
	}
	for _, message := range messages {
		if message.Role != agent.RoleTool || message.ToolCallID == "" {
			continue
		}
		s.notify(sessionID, map[string]any{
			"sessionUpdate": "tool_call_update",
			"toolCallId":    message.ToolCallID,
			"status":        "completed",
		})
	}
}

func (s *Server) closeSession(request inbound) error {
	var params struct {
		SessionID string `json:"sessionId"`
	}
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

func (s *Server) requestPermission(ctx context.Context, sessionID string, argv []string) (tools.CommandDecision, error) {
	id := s.nextID("permission")
	decision := make(chan tools.CommandDecision, 1)
	s.mu.Lock()
	s.permissions[id] = decision
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.permissions, id)
		s.mu.Unlock()
	}()
	s.send(outbound{
		JSONRPC: "2.0",
		ID:      json.RawMessage(strconv.Quote(id)),
		Method:  "session/request_permission",
		Params: map[string]any{
			"sessionId": sessionID,
			"toolCall": map[string]any{
				"toolCallId": id,
				"title":      "Run worktree command: " + strings.Join(argv, " "),
				"kind":       "execute",
				"status":     "pending",
				"rawInput":   map[string]any{"argv": append([]string(nil), argv...)},
			},
			"options": []map[string]any{
				{"optionId": "allow_once", "name": "Allow once", "kind": "allow_once"},
				{"optionId": "allow_always", "name": "Always allow this exact command", "kind": "allow_always"},
				{"optionId": "reject_once", "name": "Deny", "kind": "reject_once"},
			},
		},
	})
	select {
	case selected := <-decision:
		return selected, nil
	case <-ctx.Done():
		return tools.CommandDeny, ctx.Err()
	}
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
	decision, found := s.permissions[id]
	if found {
		delete(s.permissions, id)
	}
	s.mu.Unlock()
	if !found {
		return
	}
	selected := tools.CommandDeny
	if len(response.Error) == 0 {
		var result struct {
			Outcome struct {
				Outcome  string `json:"outcome"`
				OptionID string `json:"optionId"`
			} `json:"outcome"`
		}
		if json.Unmarshal(response.Result, &result) == nil && result.Outcome.Outcome == "selected" {
			switch result.Outcome.OptionID {
			case "allow_once":
				selected = tools.CommandAllowOnce
			case "allow_always":
				selected = tools.CommandAllowAlways
			}
		}
	}
	select {
	case decision <- selected:
	default:
	}
}

func (s *Server) sendEvent(sessionID, messageID string, event agent.Event) {
	switch event.Kind {
	case agent.EventTextDelta, agent.EventText:
		if strings.TrimSpace(event.Text) == "" {
			return
		}
		s.notify(sessionID, map[string]any{
			"sessionUpdate": "agent_message_chunk",
			"messageId":     messageID,
			"content":       map[string]any{"type": "text", "text": event.Text},
		})
	case agent.EventToolCalled:
		if event.ToolCall == nil {
			return
		}
		s.notify(sessionID, map[string]any{
			"sessionUpdate": "tool_call",
			"toolCallId":    event.ToolCall.ID,
			"title":         event.ToolCall.Name,
			"kind":          acpToolKind(event.ToolCall.Name),
			"status":        "in_progress",
		})
	case agent.EventToolFinished:
		if event.ToolCall == nil {
			return
		}
		status := "completed"
		if event.ToolError != "" {
			status = "failed"
		}
		s.notify(sessionID, map[string]any{
			"sessionUpdate": "tool_call_update",
			"toolCallId":    event.ToolCall.ID,
			"status":        status,
		})
	case agent.EventSubagent, agent.EventTerminal, agent.EventHook, agent.EventContextCompacted, agent.EventSteeringApplied, agent.EventWorktreeSetup:
		s.sendLifecycleEvent(sessionID, event)
	}
}

// sendLifecycleEvent gives ACP clients a structured progress record for
// Gator-owned work that does not originate from one model ToolCall. Encoding
// it as an ACP tool call keeps coordinator, terminal, hook, and compaction
// activity out of model-message chunks while using a standard session update.
func (s *Server) sendLifecycleEvent(sessionID string, event agent.Event) {
	if strings.TrimSpace(event.Text) == "" {
		return
	}
	kind, title := lifecycleEventTitle(event)
	callID := s.nextID(kind)
	s.notify(sessionID, map[string]any{
		"sessionUpdate": "tool_call",
		"toolCallId":    callID,
		"title":         title,
		"kind":          "other",
		"status":        "in_progress",
	})
	s.notify(sessionID, map[string]any{
		"sessionUpdate": "tool_call_update",
		"toolCallId":    callID,
		"status":        "completed",
	})
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
		prefix, kind = "Worktree setup", "worktree-setup"
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
	modes := []map[string]any{{"id": "plan", "name": "Plan", "description": "Read-only exploration in an isolated worktree."}}
	if len(session.verification) > 0 {
		modes = append(modes, map[string]any{"id": "execute", "name": "Execute", "description": "Create a tested patch in an isolated worktree."})
	}
	current := "plan"
	if session.mode == gatorrun.ExecuteMode {
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

func (s *Server) validateAdditionalDirectories(directories []string) ([]workspace.Root, error) {
	roots, err := workspace.OpenAdditionalRoots(directories, 32)
	if err != nil {
		return nil, fmt.Errorf("ACP additionalDirectories: %w", err)
	}
	return roots, nil
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

func (s *Server) nextID(prefix string) string {
	return fmt.Sprintf("gator-%s-%d", prefix, s.next.Add(1))
}

func (s *Server) sendResult(id json.RawMessage, result any) {
	if len(id) == 0 {
		return
	}
	s.send(outbound{JSONRPC: "2.0", ID: id, Result: result})
}

func (s *Server) sendError(id json.RawMessage, code int, message string) {
	if len(id) == 0 {
		return
	}
	s.send(outbound{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: message}})
}

func (s *Server) send(message outbound) {
	s.write.Lock()
	defer s.write.Unlock()
	_ = json.NewEncoder(s.config.Output).Encode(message)
}
