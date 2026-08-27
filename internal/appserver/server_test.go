package appserver

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/agent"
	internalrpc "github.com/gongahkia/gator/internal/rpc"
	gatorrun "github.com/gongahkia/gator/internal/run"
	"github.com/gongahkia/gator/internal/sandbox"
	"github.com/gongahkia/gator/internal/terminal"
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
	capabilities := submitAndCollect(t, web.URL, protocol.Request{Version: protocol.Version, ID: "capabilities-1", Method: protocol.MethodCapabilities}, hasTerminalResponse)
	if len(capabilities) != 1 || capabilities[0].Type != "response" || strings.Contains(stringify(capabilities[0].Result), "terminal_") {
		t.Fatalf("default server capabilities = %#v", capabilities)
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

func TestServerReturnsTooManyRequestsWhenStreamCapacityIsFull(t *testing.T) {
	bridge := testServer(t, &scriptedModel{})
	bridge.hub.maxStreams = 1
	web := httptest.NewServer(bridge.Handler())
	defer web.Close()

	first := postRPC(t, web.URL, protocol.Request{Version: protocol.Version, ID: "capacity-1", Method: protocol.MethodStatus})
	first.Body.Close()
	if first.StatusCode != http.StatusAccepted {
		t.Fatalf("first request status = %d", first.StatusCode)
	}
	second := postRPC(t, web.URL, protocol.Request{Version: protocol.Version, ID: "capacity-2", Method: protocol.MethodStatus})
	second.Body.Close()
	if second.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("overload status = %d", second.StatusCode)
	}
}

func TestServerRejectsLostReplayCursorAndAllowsRecoverableReconnect(t *testing.T) {
	bridge := testServer(t, &scriptedModel{})
	bridge.hub.maxHistory = 2
	if err := bridge.hub.reserve("replay-1", false); err != nil {
		t.Fatalf("reserve replay stream: %v", err)
	}
	for index := 0; index < 3; index++ {
		bridge.hub.publish(protocol.Message{ID: "replay-1", Type: "event"})
	}
	web := httptest.NewServer(bridge.Handler())
	defer web.Close()

	stale, err := http.NewRequest(http.MethodGet, web.URL+"/v1/events/replay-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	stale.Header.Set("Authorization", "Bearer "+string(testToken))
	stale.Header.Set("Last-Event-ID", "0")
	response, err := http.DefaultClient.Do(stale)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("lost replay status = %d", response.StatusCode)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	recovered, err := http.NewRequestWithContext(ctx, http.MethodGet, web.URL+"/v1/events/replay-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	recovered.Header.Set("Authorization", "Bearer "+string(testToken))
	recovered.Header.Set("Last-Event-ID", "1")
	response, err = http.DefaultClient.Do(recovered)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("recover replay status = %d", response.StatusCode)
	}
	reader := bufio.NewScanner(response.Body)
	reader.Buffer(make([]byte, 1024), maxRequestBytes)
	var sequences []int
	for reader.Scan() {
		line := reader.Text()
		if !strings.HasPrefix(line, "id: ") {
			continue
		}
		sequence, err := strconv.Atoi(strings.TrimPrefix(line, "id: "))
		if err != nil {
			t.Fatal(err)
		}
		sequences = append(sequences, sequence)
		if len(sequences) == 2 {
			break
		}
	}
	cancel()
	if got := fmt.Sprint(sequences); got != "[2 3]" {
		t.Fatalf("recovered event ids = %s", got)
	}
}

func TestServerCloseReleasesWaitingSSESubscriber(t *testing.T) {
	bridge := testServer(t, &scriptedModel{})
	if err := bridge.hub.reserve("waiting-1", true); err != nil {
		t.Fatalf("reserve waiting stream: %v", err)
	}
	web := httptest.NewServer(bridge.Handler())
	defer web.Close()
	request, err := http.NewRequest(http.MethodGet, web.URL+"/v1/events/waiting-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+string(testToken))
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() {
		_, err := io.ReadAll(response.Body)
		finished <- err
	}()
	if err := bridge.Close(); err != nil {
		t.Fatalf("close bridge: %v", err)
	}
	select {
	case err := <-finished:
		if err != nil {
			t.Fatalf("read closed SSE: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("SSE subscriber remained blocked after server close")
	}
}

func TestServerAuthenticatedShutdownDelegatesToListenerOwner(t *testing.T) {
	bridge := testServer(t, &scriptedModel{})
	called := make(chan struct{})
	bridge.requestShutdown = func() { close(called) }
	web := httptest.NewServer(bridge.Handler())
	defer web.Close()
	request, err := http.NewRequest(http.MethodPost, web.URL+"/v1/shutdown", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+string(testToken))
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("shutdown status = %d", response.StatusCode)
	}
	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("listener owner was not asked to shut down")
	}
}

func TestServerWaitContextHonorsShutdownDeadline(t *testing.T) {
	bridge, err := New(Config{
		RPC: internalrpc.Config{
			RepositoryPath: t.TempDir(), DefaultProvider: "openai", DefaultModel: "test-model",
			NewExecutor: func(_, _, _ string) (gatorrun.Executor, error) { return gatorrun.Executor{}, nil },
		},
		Token: testToken,
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := bridge.WaitContext(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("wait deadline error = %v", err)
	}
}

func TestServerControlsDetachedTerminalInItsAuthenticatedSession(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the PTY dependency reports unsupported on Windows")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh is unavailable")
	}
	registry := terminal.NewRegistry()
	defer registry.Close()
	bridge := testServerWithRegistry(t, &backgroundTerminalModel{argv: []string{sh, "-lc", "printf 'ready\\n'; IFS= read line; printf 'received:%s\\n' \"$line\"; sleep 30"}}, registry)
	web := httptest.NewServer(bridge.Handler())
	defer web.Close()
	capabilities := submitAndCollect(t, web.URL, protocol.Request{Version: protocol.Version, ID: "background-capabilities-001", Method: protocol.MethodCapabilities}, hasTerminalResponse)
	for _, method := range []string{protocol.MethodTerminalList, protocol.MethodTerminalRead, protocol.MethodTerminalWrite, protocol.MethodTerminalResize, protocol.MethodTerminalStop, protocol.MethodTerminalRestart} {
		if len(capabilities) != 1 || capabilities[0].Type != "response" || !strings.Contains(stringify(capabilities[0].Result), method) {
			t.Fatalf("background terminal method %q was not advertised: %#v", method, capabilities)
		}
	}

	run := protocol.Request{Version: protocol.Version, ID: "background-run-001", Method: protocol.MethodRun, Params: protocol.Params{
		Mode: "execute", Task: "Start the development server", Provider: "openai", Verify: [][]string{{"true"}}, MaxSteps: 7,
	}}
	messages := submitRunAndApproveTerminalOperations(t, web.URL, run)
	if !hasEvent(messages, run.ID, "terminal") {
		t.Fatalf("run did not report detached terminal event: %#v", messages)
	}

	list := submitAndCollect(t, web.URL, protocol.Request{Version: protocol.Version, ID: "terminal-list-001", Method: protocol.MethodTerminalList}, hasTerminalResponse)
	if len(list) != 1 || list[0].Type != "response" || !strings.Contains(stringify(list[0].Result), `"background":true`) || !strings.Contains(stringify(list[0].Result), "term-") {
		t.Fatalf("terminal list response = %#v", list)
	}
	terminalID := firstTerminalID(t, list[0].Result)
	resize := protocol.Request{Version: protocol.Version, ID: "terminal-resize-001", Method: protocol.MethodTerminalResize, Params: protocol.Params{TerminalID: terminalID, Rows: 30, Columns: 100}}
	if messages := submitAndCollect(t, web.URL, resize, hasTerminalResponse); len(messages) != 1 || messages[0].Type != "response" || !strings.Contains(stringify(messages[0].Result), `"rows":30`) || !strings.Contains(stringify(messages[0].Result), `"columns":100`) {
		t.Fatalf("terminal resize response = %#v", messages)
	}
	write := protocol.Request{Version: protocol.Version, ID: "terminal-write-001", Method: protocol.MethodTerminalWrite, Params: protocol.Params{TerminalID: terminalID, Input: "controller\r"}}
	if messages := submitAndCollect(t, web.URL, write, hasTerminalResponse); len(messages) != 1 || messages[0].Type != "response" {
		t.Fatalf("terminal write response = %#v", messages)
	}
	for attempt := 1; attempt <= 20; attempt++ {
		read := protocol.Request{Version: protocol.Version, ID: fmt.Sprintf("terminal-read-%02d", attempt), Method: protocol.MethodTerminalRead, Params: protocol.Params{TerminalID: terminalID}}
		messages = submitAndCollect(t, web.URL, read, hasTerminalResponse)
		if len(messages) == 1 && strings.Contains(stringify(messages[0].Result), "received:controller") {
			break
		}
		if attempt == 20 {
			t.Fatalf("terminal read did not observe developer input: %#v", messages)
		}
		time.Sleep(25 * time.Millisecond)
	}
	stop := protocol.Request{Version: protocol.Version, ID: "terminal-stop-001", Method: protocol.MethodTerminalStop, Params: protocol.Params{TerminalID: terminalID}}
	if messages := submitAndCollect(t, web.URL, stop, hasTerminalResponse); len(messages) != 1 || messages[0].Type != "response" {
		t.Fatalf("terminal stop response = %#v", messages)
	}
	for attempt := 1; attempt <= 20; attempt++ {
		list = submitAndCollect(t, web.URL, protocol.Request{Version: protocol.Version, ID: fmt.Sprintf("terminal-list-after-stop-%02d", attempt), Method: protocol.MethodTerminalList}, hasTerminalResponse)
		var payload struct {
			Tasks []terminal.Task `json:"tasks"`
		}
		encoded, _ := json.Marshal(list[0].Result)
		_ = json.Unmarshal(encoded, &payload)
		if len(payload.Tasks) == 1 && payload.Tasks[0].Status == "exited" {
			break
		}
		if attempt == 20 {
			t.Fatalf("terminal did not exit after stop: %#v", list)
		}
		time.Sleep(25 * time.Millisecond)
	}
	restart := protocol.Request{Version: protocol.Version, ID: "terminal-restart-001", Method: protocol.MethodTerminalRestart, Params: protocol.Params{TerminalID: terminalID}}
	messages = submitAndCollect(t, web.URL, restart, hasTerminalResponse)
	if len(messages) != 1 || messages[0].Type != "response" {
		t.Fatalf("terminal restart response = %#v", messages)
	}
	var restarted terminal.Task
	encoded, _ := json.Marshal(messages[0].Result)
	if err := json.Unmarshal(encoded, &restarted); err != nil || restarted.ID == terminalID || restarted.Status != "running" {
		t.Fatalf("restarted terminal=%#v err=%v", restarted, err)
	}
	if messages := submitAndCollect(t, web.URL, protocol.Request{Version: protocol.Version, ID: "terminal-stop-restarted-001", Method: protocol.MethodTerminalStop, Params: protocol.Params{TerminalID: restarted.ID}}, hasTerminalResponse); len(messages) != 1 || messages[0].Type != "response" {
		t.Fatalf("restarted terminal stop response = %#v", messages)
	}
}

func TestMessageHubReplaysInOrderAndDropsSlowSubscriber(t *testing.T) {
	hub := newMessageHub(2, 2, 1)
	if err := hub.reserve("run-1", true); err != nil {
		t.Fatalf("reserve stream: %v", err)
	}
	hub.publish(protocol.Message{ID: "run-1", Type: "event"})
	hub.publish(protocol.Message{ID: "run-1", Type: "response"})
	history, subscriber, err := hub.subscribe("run-1", 0, false)
	if err != nil || len(history) != 2 || history[0].Sequence != 1 || history[1].Sequence != 2 {
		t.Fatalf("history = %#v, err=%v", history, err)
	}
	subscriber.cancel()

	_, slow, err := hub.subscribe("run-1", 2, true)
	if err != nil {
		t.Fatalf("subscribe slow client: %v", err)
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
	if _, _, err := hub.subscribe("complete-1", 0, false); !errors.Is(err, errStreamNotFound) {
		t.Fatal("expired stream remained available")
	}
}

func TestMessageHubRejectsUnrecoverableReplayCursors(t *testing.T) {
	hub := newMessageHub(1, 2, 2)
	if err := hub.reserve("run-1", false); err != nil {
		t.Fatalf("reserve stream: %v", err)
	}
	for index := 0; index < 3; index++ {
		hub.publish(protocol.Message{ID: "run-1", Type: "event"})
	}
	if _, _, err := hub.subscribe("run-1", 0, true); !errors.Is(err, errReplayUnavailable) {
		t.Fatalf("stale replay cursor error = %v", err)
	}
	history, subscription, err := hub.subscribe("run-1", 1, true)
	if err != nil || len(history) != 2 || history[0].Sequence != 2 || history[1].Sequence != 3 {
		t.Fatalf("recoverable replay = %#v, %v", history, err)
	}
	subscription.cancel()
	if _, _, err := hub.subscribe("run-1", 4, true); !errors.Is(err, errEventCursorAhead) {
		t.Fatalf("ahead replay cursor error = %v", err)
	}
}

func FuzzEventCursor(f *testing.F) {
	for _, value := range []string{"", "0", "1", "18446744073709551615", "-1", "+1", "not-a-number", " 42 "} {
		f.Add(value)
	}
	f.Fuzz(func(t *testing.T, value string) {
		cursor, present, err := eventCursor(value)
		if err == nil && strings.TrimSpace(value) != "" && !present {
			t.Fatalf("non-empty valid cursor was not marked present: %q", value)
		}
		if err == nil && !present && cursor != 0 {
			t.Fatalf("empty cursor = %d", cursor)
		}
	})
}

func FuzzServerHTTPInput(f *testing.F) {
	repository := f.TempDir()
	bridge, err := New(Config{
		RPC: internalrpc.Config{
			RepositoryPath: repository, DefaultProvider: "openai", DefaultModel: "test-model",
			NewExecutor: func(_, _, _ string) (gatorrun.Executor, error) { return gatorrun.Executor{}, nil },
		},
		Token: testToken,
	})
	if err != nil {
		f.Fatalf("new fuzz server: %v", err)
	}
	for _, seed := range []struct{ method, path, body, cursor string }{
		{http.MethodPost, "/v1/rpc", `{"version":1,"id":"status-1","method":"status"}`, ""},
		{http.MethodPost, "/v1/rpc", `not-json`, ""},
		{http.MethodGet, "/v1/events/status-1", "", "0"},
		{http.MethodGet, "/openapi.json", "", ""},
	} {
		f.Add(seed.method, seed.path, seed.body, seed.cursor)
	}
	f.Fuzz(func(t *testing.T, method, path, body, cursor string) {
		request := &http.Request{
			Method: method,
			URL:    &url.URL{Path: path},
			Header: http.Header{
				"Authorization": []string{"Bearer " + string(testToken)},
				"Content-Type":  []string{"application/json"},
			},
			Body: io.NopCloser(strings.NewReader(body)),
		}
		if cursor != "" {
			request.Header.Set("Last-Event-ID", cursor)
		}
		response := httptest.NewRecorder()
		bridge.Handler().ServeHTTP(response, request)
		if response.Code < 100 || response.Code > 599 {
			t.Fatalf("invalid response status %d", response.Code)
		}
	})
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
	return testServerWithRegistry(t, model, nil)
}

func testServerWithRegistry(t *testing.T, model agent.Model, registry *terminal.Registry) *Server {
	t.Helper()
	repository := initializedRepository(t)
	bridge, err := New(Config{
		RPC: internalrpc.Config{
			RepositoryPath: repository, StateDir: t.TempDir(), DefaultProvider: "openai", DefaultModel: "test-model",
			NewExecutor: func(_, _, _ string) (gatorrun.Executor, error) {
				return gatorrun.Executor{Model: model, Sandbox: sandbox.Policy{Mode: sandbox.Off}}, nil
			},
			TerminalRegistry: registry,
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

func submitAndCollect(t *testing.T, base string, request protocol.Request, complete func([]protocol.Message) bool) []protocol.Message {
	t.Helper()
	response := postRPC(t, base, request)
	defer response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("submit %s status = %d", request.ID, response.StatusCode)
	}
	var accepted struct {
		Events string `json:"events"`
	}
	if err := json.NewDecoder(response.Body).Decode(&accepted); err != nil || accepted.Events == "" {
		t.Fatalf("submit %s response=%#v err=%v", request.ID, accepted, err)
	}
	return collectMessages(t, base+accepted.Events, request.ID, complete)
}

func submitRunAndApproveTerminalOperations(t *testing.T, base string, run protocol.Request) []protocol.Message {
	t.Helper()
	response := postRPC(t, base, run)
	defer response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("submit %s status = %d", run.ID, response.StatusCode)
	}
	var accepted struct {
		Events string `json:"events"`
	}
	if err := json.NewDecoder(response.Body).Decode(&accepted); err != nil || accepted.Events == "" {
		t.Fatalf("submit %s response=%#v err=%v", run.ID, accepted, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, base+accepted.Events, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+string(testToken))
	stream, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Body.Close()
	if stream.StatusCode != http.StatusOK {
		t.Fatalf("run SSE status = %d", stream.StatusCode)
	}
	approvals := 0
	var messages []protocol.Message
	scanner := bufio.NewScanner(stream.Body)
	scanner.Buffer(make([]byte, 1024), maxRequestBytes)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var message protocol.Message
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &message); err != nil {
			t.Fatalf("decode run SSE data %q: %v", line, err)
		}
		if message.ID != run.ID {
			continue
		}
		messages = append(messages, message)
		if message.Type == "error" && message.Error != nil {
			t.Fatalf("run error %s: %s", message.Error.Code, message.Error.Message)
		}
		if message.Type == "event" && message.Event != nil && message.Event.Kind == "command_approval_requested" && (message.Event.Tool == "terminal_start" || message.Event.Tool == "terminal_detach") {
			approvals++
			approval := protocol.Request{Version: protocol.Version, ID: fmt.Sprintf("terminal-approval-%d", approvals), Method: protocol.MethodApprove, Params: protocol.Params{RunID: run.ID, Decision: "allow_once"}}
			approvalResponse := postRPC(t, base, approval)
			approvalResponse.Body.Close()
			if approvalResponse.StatusCode != http.StatusAccepted {
				t.Fatalf("approve %s status = %d", approval.ID, approvalResponse.StatusCode)
			}
		}
		if responseCount(messages, run.ID) >= 2 && hasEvent(messages, run.ID, "run_finished") {
			if approvals != 2 {
				t.Fatalf("terminal approvals = %d, want 2; messages=%#v", approvals, messages)
			}
			return messages
		}
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("read run SSE: %v", err)
	}
	t.Fatalf("incomplete run SSE message stream: %#v", messages)
	return nil
}

func collectMessages(t *testing.T, address, id string, complete func([]protocol.Message) bool) []protocol.Message {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+string(testToken))
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
		if message.ID == id {
			messages = append(messages, message)
		}
		if complete(messages) {
			return messages
		}
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("read SSE: %v", err)
	}
	t.Fatalf("incomplete SSE message stream for %s: %#v", id, messages)
	return nil
}

func hasTerminalResponse(messages []protocol.Message) bool {
	return len(messages) == 1 && (messages[0].Type == "response" || messages[0].Type == "error")
}

func firstTerminalID(t *testing.T, result any) string {
	t.Helper()
	payload, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Tasks []terminal.Task `json:"tasks"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil || len(decoded.Tasks) != 1 || decoded.Tasks[0].ID == "" {
		t.Fatalf("decode terminal list result=%s err=%v", payload, err)
	}
	return decoded.Tasks[0].ID
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

type backgroundTerminalModel struct {
	mu    sync.Mutex
	argv  []string
	calls int
}

func (m *backgroundTerminalModel) Complete(_ context.Context, request agent.TurnRequest) (agent.Turn, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch m.calls {
	case 0:
		m.calls++
		arguments, _ := json.Marshal(map[string]any{"argv": m.argv})
		return agent.Turn{ToolCalls: []agent.ToolCall{{ID: "start", Name: "terminal_start", Arguments: arguments}}}, nil
	case 1:
		var task terminal.Task
		for _, message := range request.Messages {
			if message.Role != agent.RoleTool || message.ToolName != "terminal_start" {
				continue
			}
			var result struct {
				Result terminal.Task `json:"result"`
			}
			if err := json.Unmarshal([]byte(message.Content), &result); err != nil {
				return agent.Turn{}, err
			}
			task = result.Result
		}
		if task.ID == "" {
			return agent.Turn{}, errors.New("terminal_start result was absent from model context")
		}
		m.calls++
		arguments, _ := json.Marshal(map[string]string{"id": task.ID})
		return agent.Turn{ToolCalls: []agent.ToolCall{{ID: "detach", Name: "terminal_detach", Arguments: arguments}}}, nil
	case 2:
		m.calls++
		return agent.Turn{ToolCalls: []agent.ToolCall{{ID: "status", Name: "git_status", Arguments: json.RawMessage(`{}`)}}}, nil
	case 3:
		m.calls++
		return agent.Turn{ToolCalls: []agent.ToolCall{{ID: "diff", Name: "git_diff", Arguments: json.RawMessage(`{}`)}}}, nil
	case 4:
		m.calls++
		return agent.Turn{ToolCalls: []agent.ToolCall{{ID: "verify", Name: "run_command", Arguments: json.RawMessage(`{"argv":["true"]}`)}}}, nil
	case 5:
		m.calls++
		return agent.Turn{Text: "The detached server is ready."}, nil
	default:
		return agent.Turn{}, errors.New("unexpected model call")
	}
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
