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
	"io"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/journal"
	"github.com/gongahkia/gator/internal/model"
	gatorrun "github.com/gongahkia/gator/internal/run"
	"github.com/gongahkia/gator/internal/tools"
	"github.com/gongahkia/gator/internal/workspace"
)

const (
	protocolVersion = 1
	maxFrameBytes   = 1024 * 1024
	maxPromptBytes  = 512 * 1024
)

// Config supplies the local Gator instance exposed to an ACP client.
// RepositoryPath is intentionally fixed at process start: accepting an
// arbitrary client cwd would let an editor widen the launched agent's scope.
type Config struct {
	Input               io.Reader
	Output              io.Writer
	RepositoryPath      string
	StateDir            string
	DefaultProvider     string
	DefaultModel        string
	DefaultBaseURL      string
	DefaultVerification [][]string
	AgentVersion        string
	ResolveProvider     func(provider, model string) (string, string, error)
	NewExecutor         func(provider, model, baseURL string) (gatorrun.Executor, error)
}

// Server accepts one JSON-RPC message per stdio line. A prompt executes in a
// separate goroutine so cancel requests and responses to permission requests
// remain responsive while a model is running.
type Server struct {
	config     Config
	repository string

	write sync.Mutex
	mu    sync.Mutex

	initialized bool
	sessions    map[string]*session
	permissions map[string]chan tools.CommandDecision
	next        atomic.Uint64
	wait        sync.WaitGroup
}

type session struct {
	id           string
	cwd          string
	provider     string
	model        string
	verification [][]string
	mode         gatorrun.Mode
	title        string
	statePath    string
	updatedAt    time.Time
	active       *activePrompt
}

type activePrompt struct {
	cancel    context.CancelFunc
	messageID string
}

type inbound struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	Result  json.RawMessage `json:"result"`
	Error   json.RawMessage `json:"error"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type outbound struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  any             `json:"params,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

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
// active prompts; their terminal responses are intentionally not written to a
// departed client.
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
		CWD        string            `json:"cwd"`
		MCPServers []json.RawMessage `json:"mcpServers"`
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
	ctx, cancel := context.WithCancel(parent)
	activity := &activePrompt{cancel: cancel, messageID: s.nextID("message")}
	session.active = activity
	session.updatedAt = time.Now().UTC()
	s.wait.Add(1)
	s.mu.Unlock()

	id := append(json.RawMessage(nil), responseID(request.ID)...)
	go s.executePrompt(ctx, session.id, task, activity.messageID, id)
	return nil
}

func (s *Server) executePrompt(ctx context.Context, sessionID, task, messageID string, response json.RawMessage) {
	defer s.wait.Done()
	defer func() {
		s.mu.Lock()
		if session, found := s.sessions[sessionID]; found && session.active != nil && session.active.messageID == messageID {
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
	statePath := session.statePath
	s.mu.Unlock()

	executor, err := s.config.NewExecutor(provider, modelName, s.config.DefaultBaseURL)
	if err != nil {
		s.finishPrompt(response, ctx, err)
		return
	}
	request := gatorrun.Request{
		RepositoryPath: s.repository,
		Task:           task,
		Provider:       provider,
		Model:          modelName,
		BaseURL:        s.config.DefaultBaseURL,
		Verification:   verification,
		StateDir:       s.config.StateDir,
		ThreadID:       sessionID,
		Mode:           mode,
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
		CWD string `json:"cwd"`
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
	sessions := make([]map[string]any, 0, len(threads))
	for _, thread := range threads {
		sessions = append(sessions, map[string]any{
			"sessionId": thread.ID,
			"cwd":       s.repository,
			"title":     titleFor(thread.Task),
			"updatedAt": thread.UpdatedAt.UTC().Format(time.RFC3339Nano),
		})
	}
	s.sendResult(responseID(request.ID), map[string]any{"sessions": sessions})
	return nil
}

func (s *Server) resumeSession(request inbound) error {
	var params struct {
		SessionID  string            `json:"sessionId"`
		CWD        string            `json:"cwd"`
		MCPServers []json.RawMessage `json:"mcpServers"`
	}
	if err := decodeParams(request.Params, &params); err != nil {
		return fmt.Errorf("%s parameters: %w", request.Method, err)
	}
	if err := s.validateCWD(params.CWD); err != nil {
		return err
	}
	if len(params.MCPServers) != 0 {
		return errors.New("client-supplied MCP servers are not accepted; trust project .gator/mcp.json with gator mcp trust")
	}
	s.mu.Lock()
	if existing, found := s.sessions[params.SessionID]; found {
		mode := s.modeState(existing)
		s.mu.Unlock()
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
		provider:     previous.Provider,
		model:        previous.Model,
		verification: cloneArgv(previous.Verification),
		mode:         mode,
		title:        titleFor(thread.Task),
		statePath:    thread.HeadStatePath,
		updatedAt:    time.Now().UTC(),
	}
	s.mu.Lock()
	s.sessions[session.id] = session
	s.mu.Unlock()
	s.sendResult(responseID(request.ID), map[string]any{"modes": s.modeState(session)})
	return nil
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
	if found {
		if session.active != nil {
			session.active.cancel()
		}
		delete(s.sessions, params.SessionID)
	}
	s.mu.Unlock()
	if !found {
		return fmt.Errorf("unknown ACP session %q", params.SessionID)
	}
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
		ID:      json.RawMessage(strconvQuote(id)),
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
	}
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

func decodeParams(raw json.RawMessage, destination any) error {
	if len(raw) == 0 || string(raw) == "null" {
		return errors.New("params object is required")
	}
	if err := json.Unmarshal(raw, destination); err != nil {
		return err
	}
	return nil
}

func hasField(raw json.RawMessage, field string) bool {
	var object map[string]json.RawMessage
	return json.Unmarshal(raw, &object) == nil && object[field] != nil
}

func responseID(id json.RawMessage) json.RawMessage {
	if len(id) == 0 {
		return nil
	}
	return append(json.RawMessage(nil), id...)
}

func promptText(blocks []json.RawMessage) (string, error) {
	if len(blocks) == 0 {
		return "", errors.New("session/prompt requires at least one content block")
	}
	var task strings.Builder
	for index, raw := range blocks {
		var header struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &header); err != nil {
			return "", fmt.Errorf("prompt content block %d is invalid: %w", index+1, err)
		}
		var content string
		switch header.Type {
		case "text":
			var block struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal(raw, &block); err != nil {
				return "", err
			}
			content = block.Text
		case "resource_link":
			var block struct {
				Name string `json:"name"`
				URI  string `json:"uri"`
			}
			if err := json.Unmarshal(raw, &block); err != nil {
				return "", err
			}
			if strings.TrimSpace(block.Name) == "" || strings.TrimSpace(block.URI) == "" {
				return "", fmt.Errorf("prompt resource link %d requires name and uri", index+1)
			}
			content = "Developer-provided resource link: " + block.Name + " (" + block.URI + ")"
		default:
			return "", fmt.Errorf("prompt content block %d has unsupported type %q", index+1, header.Type)
		}
		if task.Len()+len(content) > maxPromptBytes {
			return "", fmt.Errorf("prompt exceeds the %d KiB ACP limit", maxPromptBytes/1024)
		}
		if task.Len() > 0 {
			task.WriteString("\n\n")
		}
		task.WriteString(content)
	}
	if strings.TrimSpace(task.String()) == "" {
		return "", errors.New("session/prompt text must not be empty")
	}
	return task.String(), nil
}

func defaultMode(verification [][]string) gatorrun.Mode {
	if len(verification) == 0 {
		return gatorrun.PlanMode
	}
	return gatorrun.ExecuteMode
}

func titleFor(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if len([]rune(value)) <= 96 {
		return value
	}
	return string([]rune(value)[:95]) + "…"
}

func acpToolKind(name string) string {
	switch name {
	case "read_file", "list_files":
		return "read"
	case "search_files":
		return "search"
	case "write_file", "apply_patch":
		return "edit"
	case "run_command":
		return "execute"
	default:
		return "other"
	}
}

func cloneArgv(source [][]string) [][]string {
	if len(source) == 0 {
		return nil
	}
	result := make([][]string, len(source))
	for index, argv := range source {
		result[index] = append([]string(nil), argv...)
	}
	return result
}

func strconvQuote(value string) []byte {
	encoded, _ := json.Marshal(value)
	return encoded
}
