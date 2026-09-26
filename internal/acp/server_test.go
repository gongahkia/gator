package acp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/workhistory"
	"github.com/gongahkia/gator/internal/workrun"
)

func TestServerPromptsThroughCanonicalWorkHistory(t *testing.T) {
	repository, state := fixtureRepository(t), t.TempDir()
	reader, writer := io.Pipe()
	output := &lockedBuffer{}
	requests := make(chan workrun.Request, 2)
	server, err := New(Config{
		Input: reader, Output: output, RepositoryPath: repository, DefaultProvider: "openai", AgentVersion: "test",
		NewWorkService: scriptedWorkFactory(state, requests, &scriptedModel{turns: []agent.Turn{{Text: "Inspection complete."}, {Text: "Second inspection complete."}}}),
	})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(context.Background()) }()

	sessionID := initializeAndCreate(t, writer, output, repository)
	prompt(t, writer, "prompt-one", sessionID, "Inspect the fixture.")
	_ = waitFor(t, output, func(message envelope) bool { return message.ID == "prompt-one" && message.Result != nil })
	first := <-requests
	if first.Mode != action.Inspect || first.ConversationID != "" || first.Contract.ExternalActions != action.Forbid {
		t.Fatalf("first ACP Work request = %#v", first)
	}
	prompt(t, writer, "prompt-two", sessionID, "Inspect it again.")
	_ = waitFor(t, output, func(message envelope) bool { return message.ID == "prompt-two" && message.Result != nil })
	second := <-requests
	if second.ConversationID == "" || second.ParentRevisionID == "" || second.Mode != action.Inspect {
		t.Fatalf("continued ACP Work request = %#v", second)
	}

	store, err := workhistory.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	records, err := store.List(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0].ID == "" || records[0].ConversationID == "" || records[0].Evidence.ArtifactManifestPath == "" {
		t.Fatalf("ACP Work history = %#v", records)
	}
	if _, err := os.Stat(filepath.Join(state, "gator", "journal")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ACP created legacy journal state: %v", err)
	}
	if !hasAgentText(output.String(), "Inspection complete.") {
		t.Fatalf("ACP did not stream Work event: %s", output.String())
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestServerRetainsFailedWork(t *testing.T) {
	repository, state := fixtureRepository(t), t.TempDir()
	reader, writer := io.Pipe()
	output := &lockedBuffer{}
	server, err := New(Config{
		Input: reader, Output: output, RepositoryPath: repository, DefaultProvider: "openai",
		NewWorkService: scriptedWorkFactory(state, nil, failingModel{}),
	})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(context.Background()) }()
	sessionID := initializeAndCreate(t, writer, output, repository)
	prompt(t, writer, "failure", sessionID, "Inspect and fail.")
	_ = waitFor(t, output, func(message envelope) bool { return message.ID == "failure" && message.Error != nil })
	store, err := workhistory.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	records, err := store.List(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Status != workhistory.Failed || records[0].Evidence.ArtifactManifestPath == "" {
		t.Fatalf("failed ACP Work history = %#v", records)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestExecuteModeBuildsCodeWorkRequest(t *testing.T) {
	request := acpWorkRequest("/source", "Implement the change.", action.Draft, [][]string{{"go", "test", "./..."}}, "work-1", "rev-1")
	if request.Mode != action.Draft || request.Contract.ExternalActions != action.Forbid || !request.RequireCode || len(request.Code.Verification) != 1 || request.ConversationID != "work-1" || request.ParentRevisionID != "rev-1" {
		t.Fatalf("ACP execute request = %#v", request)
	}
}

func scriptedWorkFactory(state string, requests chan<- workrun.Request, model agent.Model) func(string, string, *workrun.Request) (workrun.Service, error) {
	return func(_, _ string, request *workrun.Request) (workrun.Service, error) {
		if requests != nil {
			requests <- *request
		}
		return workrun.Service{Executor: workrun.Executor{Model: model, StateDir: state}}, nil
	}
}

func initializeAndCreate(t *testing.T, writer io.Writer, output *lockedBuffer, repository string) string {
	t.Helper()
	writeMessage(t, writer, map[string]any{"jsonrpc": "2.0", "id": "initialize", "method": "initialize", "params": map[string]any{"protocolVersion": 1}})
	_ = waitFor(t, output, func(message envelope) bool { return message.ID == "initialize" && message.Result != nil })
	writeMessage(t, writer, map[string]any{"jsonrpc": "2.0", "id": "new", "method": "session/new", "params": map[string]any{"cwd": repository, "mcpServers": []any{}}})
	created := waitFor(t, output, func(message envelope) bool { return message.ID == "new" && message.Result != nil })
	return created.Result.(map[string]any)["sessionId"].(string)
}

func prompt(t *testing.T, writer io.Writer, id, sessionID, text string) {
	t.Helper()
	writeMessage(t, writer, map[string]any{"jsonrpc": "2.0", "id": id, "method": "session/prompt", "params": map[string]any{"sessionId": sessionID, "prompt": []any{map[string]any{"type": "text", "text": text}}}})
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

func hasAgentText(output, text string) bool {
	for _, message := range decode(output) {
		if message.Method != "session/update" {
			continue
		}
		update, _ := message.Params["update"].(map[string]any)
		content, _ := update["content"].(map[string]any)
		if update["sessionUpdate"] == "agent_message_chunk" && content["text"] == text {
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

type failingModel struct{}

func (failingModel) Complete(context.Context, agent.TurnRequest) (agent.Turn, error) {
	return agent.Turn{}, errors.New("model failed")
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

func fixtureRepository(t *testing.T) string {
	t.Helper()
	repository := filepath.Join(t.TempDir(), "repository")
	if err := os.MkdirAll(repository, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, "README.md"), []byte("# fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return repository
}
