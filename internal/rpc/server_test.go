package rpc

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
	protocol "github.com/gongahkia/gator/rpc"
)

func TestServerReportsCapabilitiesAndCompletesPlanRun(t *testing.T) {
	repository := initializedRepository(t)
	input := strings.NewReader(strings.Join([]string{
		`{"version":1,"id":"capabilities-1","method":"capabilities"}`,
		`{"version":1,"id":"plan-1","method":"run","params":{"mode":"plan","task":"Inspect the fixture","provider":"openai"}}`,
	}, "\n") + "\n")
	var output bytes.Buffer
	server, err := New(Config{
		Input: input, Output: &output, RepositoryPath: repository, StateDir: t.TempDir(), DefaultProvider: "openai",
		NewExecutor: func(_, _, _ string) (gatorrun.Executor, error) {
			return gatorrun.Executor{Model: &scriptedModel{turns: []agent.Turn{{Text: "Plan ready."}}}}, nil
		},
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	if err := server.Serve(context.Background()); err != nil {
		t.Fatalf("serve: %v", err)
	}
	messages := decodeMessages(t, output.String())
	if !hasResponse(messages, "capabilities-1") || !hasResponse(messages, "plan-1") || !hasEvent(messages, "plan-1", "run_finished") {
		t.Fatalf("RPC messages = %#v", messages)
	}
}

func TestServerLoadsRepositoryAttachmentsForPlanRun(t *testing.T) {
	repository := initializedRepository(t)
	png := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00}
	if err := os.WriteFile(filepath.Join(repository, "screen.png"), png, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, "notes.md"), []byte("reference"), 0o644); err != nil {
		t.Fatal(err)
	}
	input := strings.NewReader(`{"version":1,"id":"plan-attachments","method":"run","params":{"mode":"plan","task":"Inspect the references","provider":"openai","images":["screen.png"],"attachments":["notes.md"]}}` + "\n")
	var output bytes.Buffer
	model := &scriptedModel{turns: []agent.Turn{{Text: "Plan ready."}}}
	server, err := New(Config{
		Input: input, Output: &output, RepositoryPath: repository, StateDir: t.TempDir(), DefaultProvider: "openai",
		NewExecutor: func(_, _, _ string) (gatorrun.Executor, error) {
			return gatorrun.Executor{Model: model}, nil
		},
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	if err := server.Serve(context.Background()); err != nil {
		t.Fatalf("serve: %v", err)
	}
	if len(model.requests) != 1 {
		t.Fatalf("model requests = %#v", model.requests)
	}
	message := model.requests[0].Messages[0]
	if len(message.Images) != 1 || message.Images[0].Name != "screen.png" || string(message.Images[0].Data) != string(png) {
		t.Fatalf("image attachment = %#v", message.Images)
	}
	if len(message.Attachments) != 1 || message.Attachments[0].Name != "notes.md" || string(message.Attachments[0].Data) != "reference" {
		t.Fatalf("document attachment = %#v", message.Attachments)
	}
}

func TestServerCancelsActiveRun(t *testing.T) {
	repository := initializedRepository(t)
	input := strings.NewReader(strings.Join([]string{
		`{"version":1,"id":"run-1","method":"run","params":{"mode":"plan","task":"Wait","provider":"openai"}}`,
		`{"version":1,"id":"cancel-1","method":"cancel","params":{"run_id":"run-1"}}`,
	}, "\n") + "\n")
	var output bytes.Buffer
	server, err := New(Config{
		Input: input, Output: &output, RepositoryPath: repository, StateDir: t.TempDir(), DefaultProvider: "openai",
		NewExecutor: func(_, _, _ string) (gatorrun.Executor, error) {
			return gatorrun.Executor{Model: waitingModel{}}, nil
		},
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	if err := server.Serve(context.Background()); err != nil {
		t.Fatalf("serve: %v", err)
	}
	messages := decodeMessages(t, output.String())
	if !hasResponse(messages, "cancel-1") || !hasError(messages, "run-1", "run_failed") {
		t.Fatalf("RPC messages = %#v", messages)
	}
}

func TestServerRejectsUnsafeRequestsWithoutStopping(t *testing.T) {
	repository := initializedRepository(t)
	input := strings.NewReader(strings.Join([]string{
		`{"version":1,"id":"bad-1","method":"run","params":{"task":"Change code","provider":"openai"}}`,
		`{"version":1,"id":"status-1","method":"status"}`,
	}, "\n") + "\n")
	var output bytes.Buffer
	server, err := New(Config{
		Input: input, Output: &output, RepositoryPath: repository, StateDir: t.TempDir(), DefaultProvider: "openai",
		NewExecutor: func(_, _, _ string) (gatorrun.Executor, error) {
			return gatorrun.Executor{}, errors.New("must not run")
		},
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	if err := server.Serve(context.Background()); err != nil {
		t.Fatalf("serve: %v", err)
	}
	messages := decodeMessages(t, output.String())
	if !hasError(messages, "bad-1", "invalid_request") || !hasResponse(messages, "status-1") {
		t.Fatalf("RPC messages = %#v", messages)
	}
}

func TestServerRejectsUnknownApprovalDecision(t *testing.T) {
	repository := initializedRepository(t)
	input := strings.NewReader(`{"version":1,"id":"approve-1","method":"approve","params":{"run_id":"run-1","decision":"maybe"}}` + "\n")
	var output bytes.Buffer
	server, err := New(Config{
		Input: input, Output: &output, RepositoryPath: repository, StateDir: t.TempDir(), DefaultProvider: "openai",
		NewExecutor: func(_, _, _ string) (gatorrun.Executor, error) {
			return gatorrun.Executor{}, errors.New("must not run")
		},
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	if err := server.Serve(context.Background()); err != nil {
		t.Fatalf("serve: %v", err)
	}
	messages := decodeMessages(t, output.String())
	if !hasError(messages, "approve-1", "invalid_request") {
		t.Fatalf("RPC messages = %#v", messages)
	}
}

func TestServerApprovesExploratoryCommand(t *testing.T) {
	repository := initializedRepository(t)
	reader, writer := io.Pipe()
	output := &lockedBuffer{}
	server, err := New(Config{
		Input: reader, Output: output, RepositoryPath: repository, StateDir: t.TempDir(), DefaultProvider: "openai",
		NewExecutor: func(_, _, _ string) (gatorrun.Executor, error) {
			return gatorrun.Executor{Model: &scriptedModel{turns: []agent.Turn{
				{ToolCalls: []agent.ToolCall{{ID: "cmd", Name: "run_command", Arguments: json.RawMessage(`{"argv":["echo","exploratory"]}`)}}},
				{Text: "Stopped after the exploratory command."},
			}}}, nil
		},
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		done <- server.Serve(context.Background())
	}()
	if _, err := io.WriteString(writer, `{"version":1,"id":"run-1","method":"run","params":{"task":"Explore","provider":"openai","verify":[["true"]]}}`+"\n"); err != nil {
		t.Fatalf("write run: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for command approval request")
		}
		if hasEventWithArgv(decodeLoose(output.String()), "run-1", "command_approval_requested", []string{"echo", "exploratory"}) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, err := io.WriteString(writer, `{"version":1,"id":"approve-1","method":"approve","params":{"run_id":"run-1","decision":"allow_once"}}`+"\n"); err != nil {
		t.Fatalf("write approve: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("serve: %v", err)
	}
	messages := decodeLoose(output.String())
	if !hasResponse(messages, "approve-1") || !hasEvent(messages, "run-1", "command_approval_resolved") {
		t.Fatalf("RPC messages = %#v", messages)
	}
}

type scriptedModel struct {
	mu       sync.Mutex
	turns    []agent.Turn
	next     int
	requests []agent.TurnRequest
}

func (m *scriptedModel) Complete(_ context.Context, request agent.TurnRequest) (agent.Turn, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests = append(m.requests, request)
	if m.next >= len(m.turns) {
		return agent.Turn{}, errors.New("unexpected model call")
	}
	turn := m.turns[m.next]
	m.next++
	return turn, nil
}

type waitingModel struct{}

func (waitingModel) Complete(ctx context.Context, _ agent.TurnRequest) (agent.Turn, error) {
	<-ctx.Done()
	return agent.Turn{}, ctx.Err()
}

func decodeMessages(t *testing.T, output string) []protocol.Message {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	messages := make([]protocol.Message, 0, len(lines))
	for _, line := range lines {
		var message protocol.Message
		if err := json.Unmarshal([]byte(line), &message); err != nil {
			t.Fatalf("decode RPC message %q: %v", line, err)
		}
		messages = append(messages, message)
	}
	return messages
}

func hasResponse(messages []protocol.Message, id string) bool {
	for _, message := range messages {
		if message.ID == id && message.Type == "response" {
			return true
		}
	}
	return false
}

func hasEvent(messages []protocol.Message, id, kind string) bool {
	for _, message := range messages {
		if message.ID == id && message.Type == "event" && message.Event != nil && message.Event.Kind == kind {
			return true
		}
	}
	return false
}

func hasError(messages []protocol.Message, id, code string) bool {
	for _, message := range messages {
		if message.ID == id && message.Type == "error" && message.Error != nil && message.Error.Code == code {
			return true
		}
	}
	return false
}

func hasEventWithArgv(messages []protocol.Message, id, kind string, argv []string) bool {
	for _, message := range messages {
		if message.ID != id || message.Type != "event" || message.Event == nil || message.Event.Kind != kind {
			continue
		}
		if len(message.Event.Argv) != len(argv) {
			continue
		}
		match := true
		for index := range argv {
			if message.Event.Argv[index] != argv[index] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func decodeLoose(output string) []protocol.Message {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	messages := make([]protocol.Message, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var message protocol.Message
		if err := json.Unmarshal([]byte(line), &message); err != nil {
			continue
		}
		messages = append(messages, message)
	}
	return messages
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
	for _, arguments := range [][]string{{"init", "--quiet"}, {"add", "README.md"}, {"-c", "user.name=Gator Test", "-c", "user.email=gator@example.invalid", "commit", "--quiet", "-m", "fixture"}} {
		if len(arguments) == 2 && arguments[0] == "add" {
			if err := os.WriteFile(filepath.Join(repository, "README.md"), []byte("# fixture\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		command := exec.Command("git", arguments...)
		command.Dir = repository
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v: %s", strings.Join(arguments, " "), err, output)
		}
	}
	return repository
}
