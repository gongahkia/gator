// Package telemetry exports bounded metadata without making vendor schemas durable state.
package telemetry

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/gongahkia/gator/internal/agent"
)

type Span struct {
	ID     string    `json:"id"`
	Parent string    `json:"parent,omitempty"`
	Name   string    `json:"name"`
	Start  time.Time `json:"start"`
	End    time.Time `json:"end"`
	Task   string    `json:"task,omitempty"`
	Step   int       `json:"step,omitempty"`
}
type Trace struct {
	Version     int    `json:"version"`
	ID          string `json:"id"`
	Spans       []Span `json:"spans"`
	Dropped     int    `json:"dropped"`
	ExportError string `json:"export_error,omitempty"`
	mu          sync.Mutex
}

func ID(value string, n int) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])[:n]
}
func New(run string, at time.Time) *Trace {
	id := ID(run, 32)
	return &Trace{Version: 1, ID: id, Spans: []Span{{ID: ID(run+"/work", 16), Name: "work", Start: at}}}
}
func (t *Trace) Record(event agent.Event) {
	switch event.Kind {
	case agent.EventTextDelta, agent.EventText:
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.Spans) >= 512 {
		t.Dropped++
		return
	}
	parent := t.Spans[0].ID
	if event.TaskID != "" {
		parent = ID(event.TaskID, 16)
		found := false
		for _, span := range t.Spans {
			found = found || span.ID == parent
		}
		if !found {
			t.Spans = append(t.Spans, Span{ID: parent, Parent: t.Spans[0].ID, Name: "specialist", Task: event.TaskID, Start: event.At, End: event.At})
		}
	}
	name := string(event.Kind)
	if event.ToolCall != nil {
		name = "tool/" + event.ToolCall.Name
	}
	t.Spans = append(t.Spans, Span{ID: ID(fmt.Sprintf("%s/%d", t.ID, len(t.Spans)), 16), Parent: parent, Name: name, Start: event.At, End: event.At, Task: event.TaskID, Step: event.Step})
}
func (t *Trace) Finish(directory, endpoint string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Spans[0].End = time.Now().UTC()
	if endpoint != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		err := ExportOTLP(ctx, http.DefaultClient, endpoint, t)
		cancel()
		if err != nil {
			t.ExportError = err.Error()
		}
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(directory, "trace.json"), data, 0600)
}
func ExportOTLP(ctx context.Context, client *http.Client, endpoint string, trace *Trace) error {
	spans := make([]map[string]any, 0, len(trace.Spans))
	for _, span := range trace.Spans {
		spans = append(spans, map[string]any{"traceId": trace.ID, "spanId": span.ID, "parentSpanId": span.Parent, "name": span.Name, "kind": 1, "startTimeUnixNano": strconv.FormatInt(span.Start.UnixNano(), 10), "endTimeUnixNano": strconv.FormatInt(span.End.UnixNano(), 10)})
	}
	payload := map[string]any{"resourceSpans": []any{map[string]any{"resource": map[string]any{"attributes": []any{map[string]any{"key": "service.name", "value": map[string]string{"stringValue": "gator"}}}}, "scopeSpans": []any{map[string]any{"scope": map[string]string{"name": "gator.work", "version": "1"}, "spans": spans}}}}}
	return Post(ctx, client, endpoint, "", payload, nil)
}
func Post(ctx context.Context, client *http.Client, endpoint, key string, value, result any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(data) > 8*1024*1024 {
		return errors.New("export payload exceeds limit")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return errors.New("invalid export endpoint")
	}
	request.Header.Set("Content-Type", "application/json")
	if key != "" {
		request.Header.Set("x-api-key", key)
	}
	bounded := *client
	bounded.Timeout = 10 * time.Second
	bounded.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := bounded.Do(request)
	if err != nil {
		return errors.New("export transport failed")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("export HTTP status %d", response.StatusCode)
	}
	if result != nil {
		return json.NewDecoder(io.LimitReader(response.Body, 1024*1024)).Decode(result)
	}
	_, err = io.Copy(io.Discard, io.LimitReader(response.Body, 1024*1024))
	return err
}
