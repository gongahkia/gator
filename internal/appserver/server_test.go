package appserver

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/agent"
	internalrpc "github.com/gongahkia/gator/internal/rpc"
	gatorrun "github.com/gongahkia/gator/internal/run"
	protocol "github.com/gongahkia/gator/rpc"
)

var testToken = []byte("gator-app-server-test-token-0123456789")

func TestServerBridgesPlanRunAndReplaysSSE(t *testing.T) {
	bridge := testServer(t, &scriptedModel{turns: []agent.Turn{{Text: "Plan ready."}}})
	web := httptest.NewServer(bridge.Handler())
	defer web.Close()

	health, err := http.Get(web.URL + "/healthz")
	if err != nil {
		t.Fatalf("health request: %v", err)
	}
	defer health.Body.Close()
	if health.StatusCode != http.StatusOK {
		t.Fatalf("health status = %d", health.StatusCode)
	}

	request := protocol.Request{Version: protocol.Version, ID: "plan-1", Method: protocol.MethodRun, Params: protocol.Params{Mode: "plan", Task: "Inspect the fixture", Provider: "openai"}}
	response := postRPC(t, web.URL, request)
	defer response.Body.Close()
	if response.StatusCode != http.StatusAccepted || response.Header.Get("Location") != "/v1/events/plan-1" {
		t.Fatalf("run response status=%d location=%q", response.StatusCode, response.Header.Get("Location"))
	}
	var accepted struct {
		ID     string `json:"id"`
		Events string `json:"events"`
	}
	if err := json.NewDecoder(response.Body).Decode(&accepted); err != nil || accepted.ID != "plan-1" || accepted.Events != "/v1/events/plan-1" {
		t.Fatalf("accepted response = %#v, %v", accepted, err)
	}

	messages := collectRunMessages(t, web.URL+accepted.Events, "", 2)
	if !hasEvent(messages, "plan-1", "run_finished") || responseCount(messages, "plan-1") != 2 {
		t.Fatalf("stream messages = %#v", messages)
	}
	for _, message := range messages {
		if message.Type == "response" && strings.Contains(stringify(message.Result), string(testToken)) {
			t.Fatalf("token leaked in protocol response: %#v", message)
		}
	}
}

func TestServerRejectsUnauthorizedAndCrossOriginRequests(t *testing.T) {
	bridge := testServer(t, &scriptedModel{})
	web := httptest.NewServer(bridge.Handler())
	defer web.Close()

	request, err := http.NewRequest(http.MethodGet, web.URL+"/openapi.json", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized || response.Header.Get("WWW-Authenticate") == "" {
		t.Fatalf("unauthenticated response status=%d headers=%#v", response.StatusCode, response.Header)
	}

	request, err = http.NewRequest(http.MethodGet, web.URL+"/healthz", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Origin", "https://untrusted.example")
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-origin response status = %d", response.StatusCode)
	}

	request, err = http.NewRequest(http.MethodGet, web.URL+"/openapi.json", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+string(testToken))
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || !strings.Contains(response.Header.Get("Content-Type"), "application/json") {
		t.Fatalf("OpenAPI response status=%d headers=%#v", response.StatusCode, response.Header)
	}
}

func TestServerRejectsMalformedRequestAndUnknownStream(t *testing.T) {
	bridge := testServer(t, &scriptedModel{})
	web := httptest.NewServer(bridge.Handler())
	defer web.Close()

	request, err := http.NewRequest(http.MethodPost, web.URL+"/v1/rpc", strings.NewReader(`{"version":1,"id":"bad id","method":"status"}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+string(testToken))
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad request status = %d", response.StatusCode)
	}

	request, err = http.NewRequest(http.MethodGet, web.URL+"/v1/events/unknown-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+string(testToken))
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown stream status = %d", response.StatusCode)
	}
}

func TestServerRejectsDuplicateRequestIDs(t *testing.T) {
	bridge := testServer(t, &scriptedModel{})
	web := httptest.NewServer(bridge.Handler())
	defer web.Close()

	request := protocol.Request{Version: protocol.Version, ID: "status-1", Method: protocol.MethodStatus}
	first := postRPC(t, web.URL, request)
	first.Body.Close()
	if first.StatusCode != http.StatusAccepted {
		t.Fatalf("first request status = %d", first.StatusCode)
	}
	second := postRPC(t, web.URL, request)
	second.Body.Close()
	if second.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate request status = %d", second.StatusCode)
	}
}

func TestMessageHubReplaysInOrderAndDropsSlowSubscriber(t *testing.T) {
	hub := newMessageHub(2, 2, 1)
	if err := hub.reserve("run-1", true); err != nil {
		t.Fatalf("reserve stream: %v", err)
	}
	hub.publish(protocol.Message{ID: "run-1", Type: "event"})
	hub.publish(protocol.Message{ID: "run-1", Type: "response"})
	history, subscriber, found := hub.subscribe("run-1", 0)
	if !found || len(history) != 2 || history[0].Sequence != 1 || history[1].Sequence != 2 {
		t.Fatalf("history = %#v, found=%t", history, found)
	}
	subscriber.cancel()

	_, slow, found := hub.subscribe("run-1", 2)
	if !found {
		t.Fatal("subscribe slow client")
	}
	hub.publish(protocol.Message{ID: "run-1", Type: "event"})
	hub.publish(protocol.Message{ID: "run-1", Type: "event"})
	select {
	case <-slow.dropped:
	case <-time.After(time.Second):
		t.Fatal("slow subscriber was not dropped")
	}
	if err := hub.reserve("run-2", false); err != nil {
		t.Fatalf("reserve second stream: %v", err)
	}
	if err := hub.reserve("run-3", false); !errors.Is(err, errStreamCapacity) {
		t.Fatalf("stream capacity error = %v", err)
	}
}

func TestMessageHubReclaimsInactiveTerminalStreams(t *testing.T) {
	now := time.Date(2026, time.August, 23, 0, 0, 0, 0, time.UTC)
	hub := newMessageHubWithClock(1, 2, 1, time.Minute, func() time.Time { return now })
	if err := hub.reserve("complete-1", false); err != nil {
		t.Fatalf("reserve completed stream: %v", err)
	}
	hub.publish(protocol.Message{ID: "complete-1", Type: "response", Result: map[string]any{"ok": true}})
	now = now.Add(time.Minute)
	if err := hub.reserve("next-1", false); err != nil {
		t.Fatalf("reserve stream after expiry: %v", err)
	}
	if _, _, found := hub.subscribe("complete-1", 0); found {
		t.Fatal("expired stream remained available")
	}
}

func TestTerminalMessageKeepsAcceptedRunsOpen(t *testing.T) {
	accepted := protocol.Message{Type: "response", Result: map[string]any{"accepted": true}}
	if terminalMessage(accepted, true) {
		t.Fatal("accepted run was terminal")
	}
	if !terminalMessage(accepted, false) {
		t.Fatal("accepted command response was not terminal")
	}
	if !terminalMessage(protocol.Message{Type: "response", Result: map[string]any{"run_id": "run-1"}}, true) {
		t.Fatal("completed response was not terminal")
	}
	if !terminalMessage(protocol.Message{Type: "error"}, true) {
		t.Fatal("error was not terminal")
	}
}

func testServer(t *testing.T, model agent.Model) *Server {
	t.Helper()
	repository := initializedRepository(t)
	bridge, err := New(Config{
		RPC: internalrpc.Config{
			RepositoryPath: repository, StateDir: t.TempDir(), DefaultProvider: "openai", DefaultModel: "test-model",
			NewExecutor: func(_, _, _ string) (gatorrun.Executor, error) { return gatorrun.Executor{Model: model}, nil },
		},
		Token: testToken, Version: "test",
	})
	if err != nil {
		t.Fatalf("new app server: %v", err)
	}
	if err := bridge.Start(context.Background()); err != nil {
		t.Fatalf("start app server: %v", err)
	}
	t.Cleanup(func() {
		_ = bridge.Close()
		if err := bridge.Wait(); err != nil {
			t.Errorf("stop app server: %v", err)
		}
	})
	return bridge
}

func postRPC(t *testing.T, base string, message protocol.Request) *http.Response {
	t.Helper()
	payload, err := json.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, base+"/v1/rpc", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+string(testToken))
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func collectRunMessages(t *testing.T, address, after string, responses int) []protocol.Message {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+string(testToken))
	if after != "" {
		request.Header.Set("Last-Event-ID", after)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("SSE status = %d", response.StatusCode)
	}
	var messages []protocol.Message
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 1024), maxRequestBytes)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var message protocol.Message
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &message); err != nil {
			t.Fatalf("decode SSE data %q: %v", line, err)
		}
		messages = append(messages, message)
		if responseCount(messages, "plan-1") >= responses && hasEvent(messages, "plan-1", "run_finished") {
			return messages
		}
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("read SSE: %v", err)
	}
	t.Fatalf("incomplete SSE message stream: %#v", messages)
	return nil
}

func responseCount(messages []protocol.Message, id string) int {
	count := 0
	for _, message := range messages {
		if message.ID == id && message.Type == "response" {
			count++
		}
	}
	return count
}

func hasEvent(messages []protocol.Message, id, kind string) bool {
	for _, message := range messages {
		if message.ID == id && message.Type == "event" && message.Event != nil && message.Event.Kind == kind {
			return true
		}
	}
	return false
}

func stringify(value any) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
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
