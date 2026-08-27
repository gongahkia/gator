package acp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/agent"
	gatorrun "github.com/gongahkia/gator/internal/run"
	"github.com/gongahkia/gator/internal/sandbox"
)

func TestServerInitializesRunsPlanListsAndResumesSession(t *testing.T) {
	repository := initializedRepository(t)
	stateDir := t.TempDir()
	reader, writer := io.Pipe()
	output := &lockedBuffer{}
	server, err := New(Config{
		Input: reader, Output: output, RepositoryPath: repository, StateDir: stateDir, DefaultProvider: "openai", AgentVersion: "test",
		NewExecutor: func(_, _, _ string) (gatorrun.Executor, error) {
			return gatorrun.Executor{Model: &scriptedModel{turns: []agent.Turn{{Text: "Plan ready."}}}}, nil
		},
	})
	if err != nil {
		t.Fatalf("new ACP server: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(context.Background()) }()

	writeMessage(t, writer, map[string]any{
		"jsonrpc": "2.0", "id": "initialize", "method": "initialize",
		"params": map[string]any{"protocolVersion": 1, "clientCapabilities": map[string]any{}},
	})
	initialized := waitFor(t, output, func(message envelope) bool { return message.ID == "initialize" && message.Result != nil })
	capabilities := initialized.Result.(map[string]any)["agentCapabilities"].(map[string]any)
	if initialized.Result.(map[string]any)["protocolVersion"] != float64(1) || capabilities["loadSession"] != true {
		t.Fatalf("initialize response = %#v", initialized)
	}

	writeMessage(t, writer, map[string]any{
		"jsonrpc": "2.0", "id": "new", "method": "session/new",
		"params": map[string]any{"cwd": repository, "mcpServers": []any{}},
	})
	created := waitFor(t, output, func(message envelope) bool { return message.ID == "new" && message.Result != nil })
	sessionID, _ := created.Result.(map[string]any)["sessionId"].(string)
	if !strings.HasPrefix(sessionID, "acp-") {
		t.Fatalf("session/new response = %#v", created)
	}
	modes := created.Result.(map[string]any)["modes"].(map[string]any)
	if modes["currentModeId"] != "plan" {
		t.Fatalf("default ACP mode = %#v, want plan", modes)
	}

	writeMessage(t, writer, map[string]any{
		"jsonrpc": "2.0", "id": "prompt", "method": "session/prompt",
		"params": map[string]any{"sessionId": sessionID, "prompt": []any{map[string]any{"type": "text", "text": "Inspect the fixture."}}},
	})
	update := waitFor(t, output, func(message envelope) bool {
		return message.Method == "session/update" && message.Params["sessionId"] == sessionID && message.Params["update"].(map[string]any)["sessionUpdate"] == "agent_message_chunk"
	})
	content := update.Params["update"].(map[string]any)["content"].(map[string]any)
	if content["text"] != "Plan ready." {
		t.Fatalf("agent update = %#v", update)
	}
	prompt := waitFor(t, output, func(message envelope) bool { return message.ID == "prompt" && message.Result != nil })
	if prompt.Result.(map[string]any)["stopReason"] != "end_turn" {
		t.Fatalf("prompt response = %#v", prompt)
	}

	writeMessage(t, writer, map[string]any{"jsonrpc": "2.0", "id": "list", "method": "session/list", "params": map[string]any{"cwd": repository}})
	listed := waitFor(t, output, func(message envelope) bool { return message.ID == "list" && message.Result != nil })
	if !containsSession(listed.Result.(map[string]any)["sessions"].([]any), sessionID) {
		t.Fatalf("session/list response = %#v", listed)
	}

	writeMessage(t, writer, map[string]any{"jsonrpc": "2.0", "id": "close", "method": "session/close", "params": map[string]any{"sessionId": sessionID}})
	_ = waitFor(t, output, func(message envelope) bool { return message.ID == "close" && message.Result != nil })
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatalf("serve ACP: %v", err)
	}

	reconnectReader, reconnectWriter := io.Pipe()
	reconnectOutput := &lockedBuffer{}
	reconnected, err := New(Config{
		Input: reconnectReader, Output: reconnectOutput, RepositoryPath: repository, StateDir: stateDir, DefaultProvider: "openai", AgentVersion: "test",
		NewExecutor: func(_, _, _ string) (gatorrun.Executor, error) {
			return gatorrun.Executor{Model: &scriptedModel{turns: []agent.Turn{{Text: "Resumed."}}}}, nil
		},
	})
	if err != nil {
		t.Fatalf("new reconnected ACP server: %v", err)
	}
	reconnectDone := make(chan error, 1)
	go func() { reconnectDone <- reconnected.Serve(context.Background()) }()
	writeMessage(t, reconnectWriter, map[string]any{
		"jsonrpc": "2.0", "id": "reinitialize", "method": "initialize",
		"params": map[string]any{"protocolVersion": 1, "clientCapabilities": map[string]any{}},
	})
	_ = waitFor(t, reconnectOutput, func(message envelope) bool { return message.ID == "reinitialize" && message.Result != nil })
	writeMessage(t, reconnectWriter, map[string]any{"jsonrpc": "2.0", "id": "resume", "method": "session/resume", "params": map[string]any{"sessionId": sessionID, "cwd": repository}})
	resumed := waitFor(t, reconnectOutput, func(message envelope) bool { return message.ID == "resume" && message.Result != nil })
	if resumed.Result.(map[string]any)["modes"].(map[string]any)["currentModeId"] != "plan" {
		t.Fatalf("session/resume after reconnect = %#v", resumed)
	}
	if err := reconnectWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-reconnectDone; err != nil {
		t.Fatalf("serve reconnected ACP: %v", err)
	}
}

func TestServerRequestsPermissionAndUsesClientSelection(t *testing.T) {
	repository := initializedRepository(t)
	reader, writer := io.Pipe()
	output := &lockedBuffer{}
	server, err := New(Config{
		Input: reader, Output: output, RepositoryPath: repository, StateDir: t.TempDir(), DefaultProvider: "openai", DefaultVerification: [][]string{{"true"}},
		NewExecutor: func(_, _, _ string) (gatorrun.Executor, error) {
			return gatorrun.Executor{Model: &scriptedModel{turns: []agent.Turn{
				{ToolCalls: []agent.ToolCall{{ID: "command", Name: "run_command", Arguments: json.RawMessage(`{"argv":["echo","ACP approval"]}`)}}},
				{ToolCalls: []agent.ToolCall{{ID: "status", Name: "git_status", Arguments: json.RawMessage(`{}`)}}},
				{ToolCalls: []agent.ToolCall{{ID: "diff", Name: "git_diff", Arguments: json.RawMessage(`{}`)}}},
				{ToolCalls: []agent.ToolCall{{ID: "verify", Name: "run_command", Arguments: json.RawMessage(`{"argv":["true"]}`)}}},
				{Text: "The command completed."},
			}}, Sandbox: sandbox.Policy{Mode: sandbox.Off}}, nil
		},
	})
	if err != nil {
		t.Fatalf("new ACP server: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(context.Background()) }()

	initializeAndCreate(t, writer, output, repository)
	created := waitFor(t, output, func(message envelope) bool { return message.ID == "new" && message.Result != nil })
	sessionID := created.Result.(map[string]any)["sessionId"].(string)
	writeMessage(t, writer, map[string]any{
		"jsonrpc": "2.0", "id": "prompt", "method": "session/prompt",
		"params": map[string]any{"sessionId": sessionID, "prompt": []any{map[string]any{"type": "text", "text": "Run the approved command."}}},
	})
	permission := waitFor(t, output, func(message envelope) bool { return message.Method == "session/request_permission" })
	call := permission.Params["toolCall"].(map[string]any)
	if call["kind"] != "execute" || call["rawInput"].(map[string]any)["argv"].([]any)[0] != "echo" {
		t.Fatalf("permission request = %#v", permission)
	}
	writeMessage(t, writer, map[string]any{
		"jsonrpc": "2.0", "id": permission.ID,
		"result": map[string]any{"outcome": map[string]any{"outcome": "selected", "optionId": "allow_once"}},
	})
	prompt := waitFor(t, output, func(message envelope) bool { return message.ID == "prompt" && message.Result != nil })
	if prompt.Result.(map[string]any)["stopReason"] != "end_turn" {
		t.Fatalf("prompt response = %#v", prompt)
	}
	if !hasToolUpdate(output.String(), "command", "completed") {
		t.Fatalf("missing completed tool update:\n%s", output.String())
	}

	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatalf("serve ACP: %v", err)
	}
}

func TestAdditionalWorkspaceDirectoriesAreBoundedToRepository(t *testing.T) {
	repository := initializedRepository(t)
	nested := filepath.Join(repository, "packages", "api")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	server, err := New(Config{
		Input: strings.NewReader(""), Output: &lockedBuffer{}, RepositoryPath: repository, StateDir: t.TempDir(), DefaultProvider: "openai",
		NewExecutor: func(_, _, _ string) (gatorrun.Executor, error) { return gatorrun.Executor{}, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	roots, err := server.validateAdditionalDirectories([]string{nested, nested})
	if err != nil {
		t.Fatalf("validate additional workspace: %v", err)
	}
	canonicalNested, err := filepath.EvalSymlinks(nested)
	if err != nil {
		t.Fatal(err)
	}
	if len(roots) != 1 || roots[0].Path() != canonicalNested {
		t.Fatalf("additional workspace roots = %#v", roots)
	}
	outside := t.TempDir()
	outsideRoots, err := server.validateAdditionalDirectories([]string{outside})
	canonicalOutside, resolveErr := filepath.EvalSymlinks(outside)
	if resolveErr != nil {
		t.Fatal(resolveErr)
	}
	if err != nil || len(outsideRoots) != 1 || outsideRoots[0].Path() != canonicalOutside {
		t.Fatalf("external workspace root = %#v, %v", outsideRoots, err)
	}
	params, err := json.Marshal(map[string]any{"cwd": repository, "additionalDirectories": []string{outside}, "mcpServers": []any{}})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.newSession(inbound{ID: json.RawMessage(`"new"`), Params: params}); err != nil {
		t.Fatalf("new session with external root: %v", err)
	}
	server.mu.Lock()
	var session *session
	for _, candidate := range server.sessions {
		session = candidate
	}
	server.mu.Unlock()
	if session == nil || len(session.additional) != 1 || session.additional[0].Path() != canonicalOutside {
		t.Fatalf("new-session external roots = %#v", session)
	}
	resumeParams, err := json.Marshal(map[string]any{"sessionId": session.id, "cwd": repository, "additionalDirectories": []any{}})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.resumeSession(inbound{ID: json.RawMessage(`"resume"`), Method: "session/resume", Params: resumeParams}); err != nil {
		t.Fatalf("resume session with empty roots: %v", err)
	}
	server.mu.Lock()
	additionalCount := len(session.additional)
	server.mu.Unlock()
	if additionalCount != 0 {
		t.Fatalf("resume restored omitted roots: %#v", session.additional)
	}
}

func TestServerReportsSubagentAndTerminalLifecycleAsStructuredTools(t *testing.T) {
	output := &lockedBuffer{}
	server := &Server{config: Config{Output: output}}
	server.sendEvent("session-1", "message-1", agent.Event{Kind: agent.EventSubagent, Text: "starting 2 isolated writers"})
	server.sendEvent("session-1", "message-1", agent.Event{Kind: agent.EventTerminal, Text: "term-001 exited with code 0"})
	messages := decode(output.String())
	if !hasToolCall(messages, "gator-subagent-1", "Subagent: starting 2 isolated writers") || !hasToolUpdate(output.String(), "gator-subagent-1", "completed") {
		t.Fatalf("subagent lifecycle updates = %s", output.String())
	}
	if !hasToolCall(messages, "gator-terminal-2", "Terminal: term-001 exited with code 0") || !hasToolUpdate(output.String(), "gator-terminal-2", "completed") {
		t.Fatalf("terminal lifecycle updates = %s", output.String())
	}
}

func TestServerRejectsForeignWorkspaceAndClientMCP(t *testing.T) {
	repository := initializedRepository(t)
	input := strings.NewReader(strings.Join([]string{
		`{"jsonrpc":"2.0","id":"initialize","method":"initialize","params":{"protocolVersion":1}}`,
		`{"jsonrpc":"2.0","id":"foreign","method":"session/new","params":{"cwd":"/tmp","mcpServers":[]}}`,
		`{"jsonrpc":"2.0","id":"mcp","method":"session/new","params":{"cwd":"` + repository + `","mcpServers":[{"name":"untrusted"}]}}`,
	}, "\n") + "\n")
	var output bytes.Buffer
	server, err := New(Config{
		Input: input, Output: &output, RepositoryPath: repository, StateDir: t.TempDir(), DefaultProvider: "openai",
		NewExecutor: func(_, _, _ string) (gatorrun.Executor, error) {
			return gatorrun.Executor{}, errors.New("must not run")
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Serve(context.Background()); err != nil {
		t.Fatalf("serve ACP: %v", err)
	}
	messages := decode(output.String())
	if !hasError(messages, "foreign") || !hasError(messages, "mcp") {
		t.Fatalf("ACP errors = %#v", messages)
	}
}

func TestCloseSessionCancelsAnActivePromptBeforeReleasingIt(t *testing.T) {
	repository := initializedRepository(t)
	reader, writer := io.Pipe()
	output := &lockedBuffer{}
	model := &waitingModel{started: make(chan struct{})}
	server, err := New(Config{
		Input: reader, Output: output, RepositoryPath: repository, StateDir: t.TempDir(), DefaultProvider: "openai",
		NewExecutor: func(_, _, _ string) (gatorrun.Executor, error) {
			return gatorrun.Executor{Model: model}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(context.Background()) }()

	initializeAndCreate(t, writer, output, repository)
	created := waitFor(t, output, func(message envelope) bool { return message.ID == "new" && message.Result != nil })
	sessionID := created.Result.(map[string]any)["sessionId"].(string)
	writeMessage(t, writer, map[string]any{
		"jsonrpc": "2.0", "id": "prompt", "method": "session/prompt",
		"params": map[string]any{"sessionId": sessionID, "prompt": []any{map[string]any{"type": "text", "text": "Wait."}}},
	})
	select {
	case <-model.started:
	case <-time.After(3 * time.Second):
		t.Fatal("prompt did not reach the model")
	}
	writeMessage(t, writer, map[string]any{"jsonrpc": "2.0", "id": "close", "method": "session/close", "params": map[string]any{"sessionId": sessionID}})
	closed := waitFor(t, output, func(message envelope) bool { return message.ID == "close" && message.Result != nil })
	if closed.Error != nil {
		t.Fatalf("session/close response = %#v", closed)
	}
	cancelled := waitFor(t, output, func(message envelope) bool { return message.ID == "prompt" && message.Result != nil })
	if cancelled.Result.(map[string]any)["stopReason"] != "cancelled" {
		t.Fatalf("cancelled prompt response = %#v", cancelled)
	}

	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatalf("serve ACP: %v", err)
	}
}

func initializeAndCreate(t *testing.T, writer io.Writer, output *lockedBuffer, repository string) {
	t.Helper()
	writeMessage(t, writer, map[string]any{"jsonrpc": "2.0", "id": "initialize", "method": "initialize", "params": map[string]any{"protocolVersion": 1}})
	_ = waitFor(t, output, func(message envelope) bool { return message.ID == "initialize" && message.Result != nil })
	writeMessage(t, writer, map[string]any{"jsonrpc": "2.0", "id": "new", "method": "session/new", "params": map[string]any{"cwd": repository, "mcpServers": []any{}}})
}

func writeMessage(t *testing.T, writer io.Writer, message any) {
	t.Helper()
	contents, err := json.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(writer, string(contents)+"\n"); err != nil {
		t.Fatal(err)
	}
}

type envelope struct {
	ID     string
	Method string
	Params map[string]any
	Result any
	Error  *rpcError
}

func decode(value string) []envelope {
	decoder := json.NewDecoder(strings.NewReader(value))
	var result []envelope
	for {
		var raw struct {
			ID     any            `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
			Result any            `json:"result"`
			Error  *rpcError      `json:"error"`
		}
		if err := decoder.Decode(&raw); err != nil {
			return result
		}
		id, _ := raw.ID.(string)
		result = append(result, envelope{ID: id, Method: raw.Method, Params: raw.Params, Result: raw.Result, Error: raw.Error})
	}
}

func waitFor(t *testing.T, output *lockedBuffer, match func(envelope) bool) envelope {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		for _, message := range decode(output.String()) {
			if match(message) {
				return message
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for ACP message; output:\n%s", output.String())
	return envelope{}
}

func containsSession(sessions []any, id string) bool {
	for _, raw := range sessions {
		session, _ := raw.(map[string]any)
		if session["sessionId"] == id {
			return true
		}
	}
	return false
}

func hasError(messages []envelope, id string) bool {
	for _, message := range messages {
		if message.ID == id && message.Error != nil {
			return true
		}
	}
	return false
}

func hasToolUpdate(output, callID, status string) bool {
	for _, message := range decode(output) {
		if message.Method != "session/update" {
			continue
		}
		update, _ := message.Params["update"].(map[string]any)
		if update["sessionUpdate"] == "tool_call_update" && update["toolCallId"] == callID && update["status"] == status {
			return true
		}
	}
	return false
}

func hasToolCall(messages []envelope, callID, title string) bool {
	for _, message := range messages {
		if message.Method != "session/update" {
			continue
		}
		update, _ := message.Params["update"].(map[string]any)
		if update["sessionUpdate"] == "tool_call" && update["toolCallId"] == callID && update["title"] == title && update["kind"] == "other" {
			return true
		}
	}
	return false
}

type scriptedModel struct {
	mu    sync.Mutex
	turns []agent.Turn
	next  int
}

func (m *scriptedModel) Complete(_ context.Context, _ agent.TurnRequest) (agent.Turn, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.next >= len(m.turns) {
		return agent.Turn{}, errors.New("unexpected model call")
	}
	turn := m.turns[m.next]
	m.next++
	return turn, nil
}

type waitingModel struct {
	started chan struct{}
	once    sync.Once
}

func (m *waitingModel) Complete(ctx context.Context, _ agent.TurnRequest) (agent.Turn, error) {
	m.once.Do(func() { close(m.started) })
	<-ctx.Done()
	return agent.Turn{}, ctx.Err()
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(value []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(value)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func initializedRepository(t *testing.T) string {
	t.Helper()
	repository := filepath.Join(t.TempDir(), "repository")
	if err := os.MkdirAll(repository, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, "README.md"), []byte("# fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, arguments := range [][]string{{"init", "--quiet"}, {"add", "README.md"}, {"-c", "user.name=Gator Test", "-c", "user.email=gator@example.invalid", "commit", "--quiet", "-m", "fixture"}} {
		command := exec.Command("git", arguments...)
		command.Dir = repository
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v: %s", strings.Join(arguments, " "), err, output)
		}
	}
	return repository
}
