package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/model/openai"
)

// Test-only cost boundary for the real Electron/Go UI. The ordinary production
// model adapter remains unchanged. It sends native Responses requests through
// this loopback proxy to the existing durable, shared evaluation budget guard.
// No key, request body, or raw provider diagnostic is recorded in browser traces.
func TestLiveDesktopUI(t *testing.T) {
	path := os.Getenv("GATOR_LIVE_DESKTOP_REPORT")
	if path == "" {
		t.Skip("explicit opt-in live Electron evaluation")
	}
	key := os.Getenv("OPENAI_API_KEY")
	if key == "" {
		t.Fatal("OpenAI key unavailable; configure it locally, never in a test artifact")
	}
	path, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	budgetDirectory, err := filepath.Abs("../../scripts/project-followup-eval/results")
	if err != nil || filepath.Dir(path) != budgetDirectory {
		t.Fatal("live UI reports must reuse this goal's cumulative results/budget.json directory")
	}
	report, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal("report must be a new path in the cumulative evaluation results directory")
	}
	defer report.Close()
	var random [24]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatal(err)
	}
	localToken := hex.EncodeToString(random[:])
	suite := os.Getenv("GATOR_LIVE_DESKTOP_SUITE")
	if suite != "" && suite != "coding" {
		t.Fatal("unknown live UI suite")
	}
	maxCalls := int32(8)
	timeout := 6 * time.Minute
	script := "test:e2e:live"
	if suite == "coding" {
		maxCalls = 32
		timeout = 10 * time.Minute
		script = "test:e2e:live:coding"
	}
	var calls atomic.Int32
	guard := projectEvalTransport{directory: filepath.Dir(path)}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/v1/responses" || r.Header.Get("Authorization") != "Bearer "+localToken {
			http.Error(w, "live UI fixture request denied", http.StatusForbidden)
			return
		}
		if calls.Add(1) > maxCalls {
			http.Error(w, "live UI request bound reached; do not replay", http.StatusForbidden)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 65*time.Second)
		defer cancel()
		upstream, err := http.NewRequestWithContext(ctx, "POST", "https://us.api.openai.com/v1/responses", io.NopCloser(io.LimitReader(r.Body, 128*1024+1)))
		if err != nil {
			http.Error(w, "live UI request unavailable", http.StatusForbidden)
			return
		}
		upstream.Header.Set("Authorization", "Bearer "+key)
		upstream.Header.Set("Content-Type", "application/json")
		response, err := guard.RoundTrip(upstream)
		if err != nil {
			http.Error(w, "live UI budget or upstream boundary rejected the request; inspect retained accounting", http.StatusForbidden)
			return
		}
		defer response.Body.Close()
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			http.Error(w, "live UI upstream request failed; raw diagnostics withheld", http.StatusForbidden)
			return
		}
		body, err := io.ReadAll(io.LimitReader(response.Body, 2*1024*1024+1))
		if err != nil || len(body) > 2*1024*1024 || !json.Valid(body) {
			http.Error(w, "live UI response unavailable; inspect accounting", http.StatusForbidden)
			return
		}
		// The budget guard buffers the actual response to settle observed usage.
		// Preserve the native Go streaming protocol, without claiming this tests
		// real provider token timing. No generated answer is substituted here.
		w.Header().Set("Content-Type", "text/event-stream")
		writeProjectUIResponse(w, body)
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	command := exec.CommandContext(ctx, "npm", "run", script, "--prefix", "../../desktop")
	// cwd is cmd/gator under go test. Keep real credentials solely in this Go
	// process; Electron receives only a short-lived local proxy token.
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if name == "OPENAI_API_KEY" || strings.HasSuffix(name, "_API_KEY") || strings.HasPrefix(name, "GATOR_LIVE_") {
			continue
		}
		command.Env = append(command.Env, entry)
	}
	command.Env = append(command.Env, "GATOR_LIVE_UI_SUITE="+suite, "GATOR_LIVE_UI_ENDPOINT="+server.URL+"/v1/responses", "GATOR_LIVE_UI_TOKEN="+localToken, "GATOR_LIVE_UI_BROWSER_REPORT="+path+".browser.json", "GATOR_LIVE_UI_ARTIFACT_DIR="+path+".artifacts")
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	start := time.Now()
	err = command.Run()
	var spend projectEvalSpend
	data, _ := os.ReadFile(filepath.Join(filepath.Dir(path), "budget.json"))
	_ = json.Unmarshal(data, &spend)
	_ = json.NewEncoder(report).Encode(struct {
		Calls         int32
		Milliseconds  int64
		Passed        bool
		Spend         projectEvalSpend
		TokenTiming   string
		ManualQuality string
	}{calls.Load(), time.Since(start).Milliseconds(), err == nil && calls.Load() >= 2, spend, "buffered live Responses; provider token timing not assessed", "unreviewed by a human"})
	if err != nil {
		t.Fatal("live Electron journey failed; browser report and cumulative accounting retained")
	}
	if calls.Load() < 2 {
		t.Fatal("journey did not exercise real model requests")
	}
}

func writeProjectUIResponse(w io.Writer, body json.RawMessage) {
	// Marshal, rather than interpolating a pretty-printed API body: SSE data
	// must remain one line or its embedded newlines truncate the JSON event.
	event, _ := json.Marshal(struct {
		Type     string          `json:"type"`
		Response json.RawMessage `json:"response"`
	}{"response.completed", body})
	fmt.Fprintf(w, "data: %s\n\n", event)
}

func TestProjectUIProxyPreservesPrettyPrintedNativeResponses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		writeProjectUIResponse(w, json.RawMessage("{\n\"output\": [{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"Bounded fixture λ answer\"}]}],\n\"usage\":{\"input_tokens\":2,\"output_tokens\":3}\n}"))
	}))
	defer server.Close()
	m := openai.Responses{APIKey: "local-fixture", BaseURL: server.URL, Client: server.Client()}
	turn, err := m.CompleteStream(context.Background(), agent.TurnRequest{}, func(string) {})
	if err != nil || turn.Text != "Bounded fixture λ answer" {
		t.Fatalf("native streaming protocol was changed: %v %q", err, turn.Text)
	}
}
