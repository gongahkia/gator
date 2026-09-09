package telemetry

import (
	"context"
	"encoding/json"
	"github.com/gongahkia/gator/internal/agent"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOTLPMetadataRedactionHierarchyAndFailure(t *testing.T) {
	trace := New("private-run", time.Now())
	trace.Record(agent.Event{Kind: agent.EventToolCalled, At: time.Now(), TaskID: "private-run/child", Text: "SECRET_PROMPT", ToolCall: &agent.ToolCall{Name: "read_file", Arguments: json.RawMessage(`{"path":"SECRET_DOCUMENT"}`)}})
	trace.Spans[0].End = time.Now()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		if strings.Contains(string(data), "SECRET") {
			t.Error("private content exported")
		}
		var body map[string]any
		if json.Unmarshal(data, &body) != nil || body["resourceSpans"] == nil {
			t.Error("invalid OTLP body")
		}
		w.WriteHeader(200)
	}))
	defer server.Close()
	if err := ExportOTLP(context.Background(), server.Client(), server.URL, trace); err != nil {
		t.Fatal(err)
	}
	if len(trace.Spans) != 3 || trace.Spans[2].Parent != trace.Spans[1].ID {
		t.Fatalf("hierarchy: %+v", trace.Spans)
	}
	failure := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
		_, _ = w.Write([]byte("SECRET_PROVIDER_ERROR"))
	}))
	defer failure.Close()
	err := ExportOTLP(context.Background(), failure.Client(), failure.URL, trace)
	if err == nil || strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("failure: %v", err)
	}
	for i := 0; i < 1000; i++ {
		trace.Record(agent.Event{Kind: agent.EventTurnStarted, At: time.Now()})
	}
	if trace.Dropped == 0 || len(trace.Spans) > 512 {
		t.Fatal("retention unbounded")
	}
}
